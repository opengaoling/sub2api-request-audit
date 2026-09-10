//go:build unit

package service

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type anthropicFableRepo struct {
	mockAccountRepoForPlatform
	modelRateLimitCalls int
	modelScope          string
	modelResetAt        time.Time
	modelReason         string
	rateLimitCalls      int
	tempUnschedCalls    int
}

func (r *anthropicFableRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, resetAt time.Time, reason ...string) error {
	r.modelRateLimitCalls++
	r.modelScope = scope
	r.modelResetAt = resetAt
	if len(reason) > 0 {
		r.modelReason = reason[0]
	}
	return nil
}

func (r *anthropicFableRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls++
	return nil
}

func (r *anthropicFableRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

var fableCreditsRequiredBody = []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Usage credits are required for this model.","details":{"error_code":"credits_required","can_user_purchase_credits":true,"has_chargeable_saved_payment_method":true,"disabled_reason":"org_level_disabled_until","exhausted_included_allowance":false,"model":"claude-fable-5","model_display_name":"Fable"}},"request_id":"req_011CeuRLj5VYNEP7yxZBHdjz"}`)

func TestPersistAnthropicFableCreditsRequired_MarksModelOnly(t *testing.T) {
	repo := &anthropicFableRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 7, Type: AccountTypeOAuth, Platform: PlatformAnthropic}

	now := time.Now()
	resetAt := now.Add(6 * time.Hour)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(resetAt.Unix(), 10))

	handled := svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, fableCreditsRequiredBody, "claude-fable-5")

	require.False(t, handled)
	require.Equal(t, 1, repo.modelRateLimitCalls, "Fable credits_required should set the model-scoped rate limit")
	require.Equal(t, anthropicFableRateLimitKey, repo.modelScope)
	require.Equal(t, anthropicFableCreditsRequiredReason, repo.modelReason)
	require.Equal(t, 0, repo.rateLimitCalls, "account-level rate limit must not be set for a model-only failure")
	require.Equal(t, 0, repo.tempUnschedCalls)
}

func TestPersistAnthropicFableCreditsRequired_NonFableModelFallsThrough(t *testing.T) {
	repo := &anthropicFableRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 8, Type: AccountTypeOAuth, Platform: PlatformAnthropic}

	body := `{"type":"error","error":{"type":"rate_limit_error","details":{"error_code":"credits_required","model":"claude-sonnet-4-5"}}}`
	headers := http.Header{}

	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, []byte(body), "claude-sonnet-4-5")

	require.Equal(t, 0, repo.modelRateLimitCalls, "non-Fable model credits_required should not set the Fable scope")
	require.Equal(t, 0, repo.rateLimitCalls, "no window headers present, account must stay schedulable")
	require.Equal(t, 0, repo.tempUnschedCalls)
}

func TestPersistAnthropicFableCreditsRequired_FallsBackToRequestModel(t *testing.T) {
	repo := &anthropicFableRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 9, Type: AccountTypeOAuth, Platform: PlatformAnthropic}

	body := `{"type":"error","error":{"type":"rate_limit_error","details":{"error_code":"credits_required"}}}`
	headers := http.Header{}

	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, []byte(body), "claude-fable-5")

	require.Equal(t, 1, repo.modelRateLimitCalls, "missing details.model should fall back to the requested model")
	require.Equal(t, anthropicFableRateLimitKey, repo.modelScope)
	require.Equal(t, 0, repo.rateLimitCalls)
}

func TestPersistAnthropicFableWindowLimit_MarksModelOnly(t *testing.T) {
	repo := &anthropicFableRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 10, Type: AccountTypeOAuth, Platform: PlatformAnthropic}

	now := time.Now()
	resetAt := now.Add(2 * 24 * time.Hour)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-7d_oi-utilization", "1.0")
	headers.Set("anthropic-ratelimit-unified-7d_oi-reset", strconv.FormatInt(resetAt.Unix(), 10))

	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, fableCreditsRequiredBody, "claude-fable-5")

	require.Equal(t, anthropicFableRateLimitKey, repo.modelScope)
	require.Equal(t, anthropicFableWindowReason, repo.modelReason)
	require.Equal(t, 0, repo.rateLimitCalls, "7d_oi exhaustion limits the Fable family only, not the whole account")
	require.Equal(t, 0, repo.tempUnschedCalls)
}

func TestHandleUpstreamError_FableCreditsRequired_SkipsTempUnschedRule(t *testing.T) {
	repo := &anthropicFableRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{
		ID:       11,
		Type:     AccountTypeOAuth,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusTooManyRequests),
					"keywords":         []any{"credits_required"},
					"duration_minutes": float64(10),
				},
			},
		},
	}
	headers := http.Header{}

	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, fableCreditsRequiredBody, "claude-fable-5")

	require.Equal(t, 0, repo.tempUnschedCalls, "Fable 429 must not be widened by a broad keyword temp-unsched rule")
	require.Equal(t, 0, repo.rateLimitCalls)
	require.GreaterOrEqual(t, repo.modelRateLimitCalls, 1)
}

func TestIsAnthropicFableModel(t *testing.T) {
	require.True(t, isAnthropicFableModel("claude-fable-5"))
	require.True(t, isAnthropicFableModel("claude-fable-5[1m]"))
	require.True(t, isAnthropicFableModel("Claude-FABLE-5"))
	require.False(t, isAnthropicFableModel("claude-sonnet-4-5"))
	require.False(t, isAnthropicFableModel(""))
}

func TestAnthropicFableModelKeyIncludesFamilyKey(t *testing.T) {
	a := &Account{Platform: PlatformAnthropic, Credentials: map[string]any{}}
	keys := a.modelRateLimitKeysForRequest(context.Background(), "claude-fable-5[1m]")
	require.Equal(t, []string{"claude-fable-5[1m]", anthropicFableRateLimitKey}, keys)
}

func TestAnthropicFableModelRateLimitBlocksFableVariantsOnly(t *testing.T) {
	resetAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	a := &Account{
		Platform: PlatformAnthropic,
		Credentials: map[string]any{},
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				anthropicFableRateLimitKey: map[string]any{
					"rate_limited_at":     time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
					"rate_limit_reset_at": resetAt,
					"reason":              anthropicFableCreditsRequiredReason,
				},
			},
		},
	}
	require.True(t, a.isModelRateLimitedWithContext(context.Background(), "claude-fable-5"), "Fable requests must be blocked")
	require.True(t, a.isModelRateLimitedWithContext(context.Background(), "claude-fable-5[1m]"), "Fable variants must be blocked via the family key")
	require.False(t, a.isModelRateLimitedWithContext(context.Background(), "claude-sonnet-4-5"), "other models must stay schedulable")
}
