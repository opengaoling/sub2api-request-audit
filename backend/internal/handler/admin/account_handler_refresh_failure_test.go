package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountHandler_MarkRefreshFailureIfAccountUnavailable_Status401SetsError(t *testing.T) {
	adminSvc := newStubAdminService()
	handler := &AccountHandler{adminService: adminSvc}
	account := &service.Account{
		ID:       42,
		Platform: service.PlatformGemini,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
	}

	handler.markRefreshFailureIfAccountUnavailable(
		context.Background(),
		account,
		errors.New(`token refresh failed: status 401, body: {"error":"invalid_token"}`),
	)

	require.Equal(t, 1, adminSvc.setAccountErrorCalls)
	require.Equal(t, int64(42), adminSvc.setAccountErrorID)
	require.Contains(t, adminSvc.setAccountErrorMsg, "status 401")
}

func TestAccountHandler_MarkRefreshFailureIfAccountUnavailable_OpenAIValidAccessTokenSkipsError(t *testing.T) {
	adminSvc := newStubAdminService()
	handler := &AccountHandler{adminService: adminSvc}
	account := &service.Account{
		ID:       43,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"access_token":  "still-valid-at",
			"refresh_token": "revoked-rt",
			"expires_at":    time.Now().Add(30 * time.Minute).Format(time.RFC3339),
		},
	}

	handler.markRefreshFailureIfAccountUnavailable(
		context.Background(),
		account,
		errors.New("invalid_grant: refresh token revoked"),
	)

	require.Zero(t, adminSvc.setAccountErrorCalls)
}

func TestAccountHandler_MarkRefreshFailureIfAccountUnavailable_GeminiValidAccessTokenSkipsError(t *testing.T) {
	adminSvc := newStubAdminService()
	handler := &AccountHandler{adminService: adminSvc}
	account := &service.Account{
		ID:       45,
		Platform: service.PlatformGemini,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"access_token":  "still-valid-at",
			"refresh_token": "revoked-rt",
			"expires_at":    time.Now().Add(30 * time.Minute).Unix(),
		},
	}

	handler.markRefreshFailureIfAccountUnavailable(
		context.Background(),
		account,
		errors.New(`token refresh failed: status 401, body: {"error":"invalid_token"}`),
	)

	require.Zero(t, adminSvc.setAccountErrorCalls)
}

func TestAccountHandler_MarkRefreshFailureIfAccountUnavailable_OpenAIExpiredAccessTokenSetsError(t *testing.T) {
	adminSvc := newStubAdminService()
	handler := &AccountHandler{adminService: adminSvc}
	account := &service.Account{
		ID:       44,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"access_token":  "expired-at",
			"refresh_token": "revoked-rt",
			"expires_at":    time.Now().Add(-time.Minute).Format(time.RFC3339),
		},
	}

	handler.markRefreshFailureIfAccountUnavailable(
		context.Background(),
		account,
		errors.New("invalid_grant: refresh token revoked"),
	)

	require.Equal(t, 1, adminSvc.setAccountErrorCalls)
	require.Equal(t, int64(44), adminSvc.setAccountErrorID)
	require.Contains(t, adminSvc.setAccountErrorMsg, "invalid_grant")
}
