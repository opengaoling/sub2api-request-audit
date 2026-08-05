package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIQuotaRecoveryRepo struct {
	AccountRepository
	clearRateLimitIDs []int64
}

func (r *openAIQuotaRecoveryRepo) ClearRateLimit(_ context.Context, accountID int64) error {
	r.clearRateLimitIDs = append(r.clearRateLimitIDs, accountID)
	return nil
}

func TestBuildOpenAIQuotaUsageExtraUpdatesMapsFiveHourAndSevenDayWindows(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC).Unix()
	updates := buildOpenAIQuotaUsageExtraUpdates(&OpenAIQuotaUsage{
		FetchedAt: fetchedAt,
		RateLimit: &OpenAIRateLimit{
			PrimaryWindow: &OpenAIRateLimitWindow{
				UsedPercent:        72.5,
				LimitWindowSeconds: 604800,
				ResetAfterSeconds:  500000,
			},
			SecondaryWindow: &OpenAIRateLimitWindow{
				UsedPercent:        18.25,
				LimitWindowSeconds: 18000,
				ResetAt:            fetchedAt + 1200,
			},
		},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: 2},
	})

	if got := updates["codex_7d_used_percent"]; got != 72.5 {
		t.Fatalf("codex_7d_used_percent = %v, want 72.5", got)
	}
	if got := updates["codex_5h_used_percent"]; got != 18.25 {
		t.Fatalf("codex_5h_used_percent = %v, want 18.25", got)
	}
	if got := updates["codex_5h_reset_after_seconds"]; got != 1200 {
		t.Fatalf("codex_5h_reset_after_seconds = %v, want 1200", got)
	}
	if got := updates["codex_reset_credit_available_count"]; got != 2 {
		t.Fatalf("codex_reset_credit_available_count = %v, want 2", got)
	}
}

func TestIsOpenAIQuotaSnapshotStale(t *testing.T) {
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		"codex_usage_updated_at": now.Add(-openaiQuotaRefreshInterval - time.Second).Format(time.RFC3339),
	}}
	if !isOpenAIQuotaSnapshotStale(account, now) {
		t.Fatal("expected stale OpenAI quota snapshot")
	}
	account.Extra["codex_usage_updated_at"] = now.Add(-time.Minute).Format(time.RFC3339)
	if isOpenAIQuotaSnapshotStale(account, now) {
		t.Fatal("expected fresh OpenAI quota snapshot")
	}
}

func TestOpenAIQuotaUsageRecoveredRequiresHealthyQuotaWindows(t *testing.T) {
	window := func(used float64) *OpenAIRateLimitWindow {
		return &OpenAIRateLimitWindow{UsedPercent: used}
	}

	tests := []struct {
		name      string
		usage     *OpenAIQuotaUsage
		recovered bool
	}{
		{
			name: "both windows below 100 percent",
			usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
				Allowed: true, PrimaryWindow: window(72), SecondaryWindow: window(18),
			}},
			recovered: true,
		},
		{
			name: "one window remains exhausted",
			usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
				Allowed: true, PrimaryWindow: window(100), SecondaryWindow: window(18),
			}},
		},
		{
			name: "upstream still rejects quota",
			usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
				Allowed: false, LimitReached: true, PrimaryWindow: window(72),
			}},
		},
		{
			name: "quota windows missing",
			usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{Allowed: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.recovered, openAIQuotaUsageRecovered(tt.usage))
		})
	}
}

func TestOpenAIQuotaService_ClearsRecoveredRateLimit(t *testing.T) {
	repo := &openAIQuotaRecoveryRepo{}
	svc := &OpenAIQuotaService{accountRepo: repo}
	resetAt := time.Now().Add(time.Hour)
	account := &Account{
		ID:              42,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		RateLimitResetAt: &resetAt,
	}
	usage := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
		Allowed: true,
		PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 72},
		SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 18},
	}}

	require.NoError(t, svc.reconcileRecoveredRateLimit(context.Background(), account, usage))
	require.Equal(t, []int64{42}, repo.clearRateLimitIDs)
}

func TestOpenAIQuotaService_KeepsUnrecoveredRateLimit(t *testing.T) {
	repo := &openAIQuotaRecoveryRepo{}
	svc := &OpenAIQuotaService{accountRepo: repo}
	resetAt := time.Now().Add(time.Hour)
	account := &Account{
		ID:              43,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		RateLimitResetAt: &resetAt,
	}
	usage := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
		Allowed: true,
		PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 100},
		SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 18},
	}}

	require.NoError(t, svc.reconcileRecoveredRateLimit(context.Background(), account, usage))
	require.Empty(t, repo.clearRateLimitIDs)
}

func TestOpenAIQuotaRefreshIntervalIsOneHour(t *testing.T) {
	if openaiQuotaRefreshInterval != time.Hour {
		t.Fatalf("openaiQuotaRefreshInterval = %v, want %v", openaiQuotaRefreshInterval, time.Hour)
	}
}
