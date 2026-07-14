//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type modelNotFoundRateLimitCall struct {
	accountID int64
	scope     string
	resetAt   time.Time
	reason    string
}

type modelMappingRemoveCall struct {
	accountID      int64
	requestedModel string
	upstreamModel  string
}

type modelNotFoundAccountRepoStub struct {
	mockAccountRepoForGemini
	tempCalls           int
	modelRateLimitCalls []modelNotFoundRateLimitCall
	modelRateLimitErr   error
	removeMappingCalls  []modelMappingRemoveCall
	removeMappingResult bool
	removeMappingErr    error
}

func (r *modelNotFoundAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
	return nil
}

func (r *modelNotFoundAccountRepoStub) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	call := modelNotFoundRateLimitCall{
		accountID: id,
		scope:     scope,
		resetAt:   resetAt,
	}
	if len(reason) > 0 {
		call.reason = reason[0]
	}
	r.modelRateLimitCalls = append(r.modelRateLimitCalls, call)
	return r.modelRateLimitErr
}

func (r *modelNotFoundAccountRepoStub) RemoveModelMapping(ctx context.Context, id int64, requestedModel, upstreamModel string) (bool, error) {
	r.removeMappingCalls = append(r.removeMappingCalls, modelMappingRemoveCall{
		accountID:      id,
		requestedModel: requestedModel,
		upstreamModel:  upstreamModel,
	})
	return r.removeMappingResult, r.removeMappingErr
}

func TestRateLimitService_HandleUpstreamError_OpenAIModelNotFoundDoesNotAffectScheduling(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{},
		[]byte(`{"error":{"code":"model_not_found","message":"model not found"}}`),
		"gpt-5.4",
	)

	require.False(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
}

func TestRateLimitService_HandleUpstreamModelNotFound_OpenAIModelNotFoundIgnored(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamModelNotFound(
		context.Background(),
		account,
		"gpt-5.4",
		http.StatusNotFound,
		[]byte(`{"error":{"code":"model_not_found","message":"model not found"}}`),
	)

	require.False(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
}

func TestRateLimitService_HandleUpstreamError_Bare404KeepsTempUnschedulablePath(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{},
		[]byte(`{"error":{"message":"endpoint not found"}}`),
		"gpt-5.4",
	)

	require.True(t, handled)
	require.Equal(t, 1, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
}

func TestRateLimitService_HandleUpstreamError_CodexPlanGatedModelRemovesModelMapping(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{removeMappingResult: true}
	svc := &RateLimitService{accountRepo: repo}
	account := openAICodexPlanGatedOAuthAccount()
	account.Credentials["model_mapping"] = map[string]any{
		"gpt-5.6-sol": "gpt-5.6-sol",
		"gpt-5.4":     "gpt-5.4",
	}

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadRequest,
		http.Header{},
		[]byte(`{"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."}`),
		"gpt-5.6-sol",
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Len(t, repo.removeMappingCalls, 1)
	call := repo.removeMappingCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.Equal(t, "gpt-5.6-sol", call.requestedModel)
	require.Equal(t, "gpt-5.6-sol", call.upstreamModel)
	require.Equal(t, map[string]any{"gpt-5.4": "gpt-5.4"}, account.Credentials["model_mapping"])
}

func TestRateLimitService_HandleUpstreamError_CodexPlanGatedModelRemovesRequestedMappingForMappedUpstreamModel(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{removeMappingResult: true}
	svc := &RateLimitService{accountRepo: repo}
	account := openAICodexPlanGatedOAuthAccount()
	account.Credentials["model_mapping"] = map[string]any{
		"gpt-5.6-sol": "gpt-5.6-sol-upstream",
		"gpt-5.4":     "gpt-5.4",
	}

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadRequest,
		http.Header{},
		[]byte(`{"detail":"The 'gpt-5.6-sol-upstream' model is not supported when using Codex with a ChatGPT account."}`),
		"gpt-5.6-sol",
	)

	require.True(t, handled)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Len(t, repo.removeMappingCalls, 1)
	require.Equal(t, "gpt-5.6-sol", repo.removeMappingCalls[0].requestedModel)
	require.Equal(t, "gpt-5.6-sol-upstream", repo.removeMappingCalls[0].upstreamModel)
	require.Equal(t, map[string]any{"gpt-5.4": "gpt-5.4"}, account.Credentials["model_mapping"])
}

func TestRateLimitService_HandleUpstreamError_Generic400DoesNotRemoveModelMapping(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{removeMappingResult: true}
	svc := &RateLimitService{accountRepo: repo}
	account := openAICodexPlanGatedOAuthAccount()
	account.Credentials["model_mapping"] = map[string]any{
		"gpt-5.3-codex": "gpt-5.3-codex",
	}

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadRequest,
		http.Header{},
		[]byte(`{"error":{"message":"model not found: gpt-5.3-codex"}}`),
		"gpt-5.3-codex",
	)

	require.False(t, handled)
	require.Empty(t, repo.removeMappingCalls)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Equal(t, map[string]any{"gpt-5.3-codex": "gpt-5.3-codex"}, account.Credentials["model_mapping"])
}

func TestRateLimitService_HandleUpstreamError_CodexPlanGatedModelIgnoresAPIKeyAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := openAICodexPlanGatedOAuthAccount()
	account.Type = AccountTypeAPIKey

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadRequest,
		http.Header{},
		[]byte(`{"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."}`),
		"gpt-5.6-sol",
	)

	require.False(t, handled)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Empty(t, repo.removeMappingCalls)
}

func openAIModelNotFoundTempAccount() *Account {
	return &Account{
		ID:          101,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusNotFound),
					"keywords":         []any{"not found"},
					"duration_minutes": float64(10),
				},
			},
		},
	}
}

func openAICodexPlanGatedOAuthAccount() *Account {
	return &Account{
		ID:          202,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{},
	}
}
