package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	chatGPTUsageURL            = "https://chatgpt.com/backend-api/wham/usage"
	chatGPTRateLimitResetURL   = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume"
	openaiQuotaUpstreamTimeout = 20 * time.Second
	openaiQuotaRefreshInterval = 10 * time.Minute
	openaiQuotaCodexOriginator = "Codex Desktop"
	openaiQuotaLanguageTag     = "zh-CN"
	openaiQuotaSecFetchSite    = "none"
	openaiQuotaSecFetchMode    = "no-cors"
	openaiQuotaSecFetchDest    = "empty"
)

type OpenAIRateLimitWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
	ResetAt            int64   `json:"reset_at"`
}

type OpenAIRateLimit struct {
	Allowed         bool                   `json:"allowed"`
	LimitReached    bool                   `json:"limit_reached"`
	PrimaryWindow   *OpenAIRateLimitWindow `json:"primary_window,omitempty"`
	SecondaryWindow *OpenAIRateLimitWindow `json:"secondary_window,omitempty"`
}

type OpenAIAdditionalRateLimit struct {
	LimitName      string           `json:"limit_name"`
	MeteredFeature string           `json:"metered_feature"`
	RateLimit      *OpenAIRateLimit `json:"rate_limit,omitempty"`
}

type OpenAIRateLimitResetCredits struct {
	AvailableCount int `json:"available_count"`
}

type OpenAIQuotaUsage struct {
	UserID                string                       `json:"user_id,omitempty"`
	AccountID             string                       `json:"account_id,omitempty"`
	Email                 string                       `json:"email,omitempty"`
	PlanType              string                       `json:"plan_type,omitempty"`
	RateLimit             *OpenAIRateLimit             `json:"rate_limit,omitempty"`
	AdditionalRateLimits  []OpenAIAdditionalRateLimit  `json:"additional_rate_limits,omitempty"`
	RateLimitResetCredits *OpenAIRateLimitResetCredits `json:"rate_limit_reset_credits,omitempty"`
	FetchedAt             int64                        `json:"fetched_at"`
}

type OpenAIQuotaResetCredit struct {
	ID              string `json:"id,omitempty"`
	ResetType       string `json:"reset_type,omitempty"`
	Status          string `json:"status,omitempty"`
	GrantedAt       string `json:"granted_at,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	RedeemStartedAt string `json:"redeem_started_at,omitempty"`
	RedeemedAt      string `json:"redeemed_at,omitempty"`
}

type OpenAIQuotaResetResult struct {
	Code         string                  `json:"code"`
	Credit       *OpenAIQuotaResetCredit `json:"credit,omitempty"`
	WindowsReset int                     `json:"windows_reset"`
}

type OpenAIQuotaService struct {
	accountRepo          AccountRepository
	proxyRepo            ProxyRepository
	tokenProvider        *OpenAITokenProvider
	privacyClientFactory PrivacyClientFactory
}

func NewOpenAIQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *OpenAITokenProvider,
	privacyClientFactory PrivacyClientFactory,
) *OpenAIQuotaService {
	return &OpenAIQuotaService{
		accountRepo:          accountRepo,
		proxyRepo:            proxyRepo,
		tokenProvider:        tokenProvider,
		privacyClientFactory: privacyClientFactory,
	}
}

func (s *OpenAIQuotaService) QueryUsage(ctx context.Context, accountID int64) (*OpenAIQuotaUsage, error) {
	accessToken, chatGPTAccountID, proxyURL, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}

	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_CLIENT_ERROR", "failed to build upstream client: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()

	var payload OpenAIQuotaUsage
	resp, err := client.R().
		SetContext(callCtx).
		SetHeaders(buildCodexCommonHeaders(accessToken, chatGPTAccountID)).
		SetSuccessResult(&payload).
		Get(chatGPTUsageURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		status := resp.StatusCode
		slog.Warn("openai_quota_query_failed", "account_id", accountID, "status", status)
		return nil, infraerrors.Newf(mapOpenAIQuotaUpstreamStatus(status), "OPENAI_QUOTA_UPSTREAM_ERROR", "upstream returned status %d", status)
	}

	payload.FetchedAt = time.Now().Unix()
	s.persistUsageSnapshot(ctx, accountID, &payload)
	return &payload, nil
}

func (s *OpenAIQuotaService) RefreshStaleUsage(ctx context.Context, account *Account) error {
	if s == nil || account == nil || !account.IsOpenAIOAuth() {
		return nil
	}
	if !isOpenAIQuotaSnapshotStale(account, time.Now()) {
		return nil
	}
	_, err := s.QueryUsage(ctx, account.ID)
	return err
}

func isOpenAIQuotaSnapshotStale(account *Account, now time.Time) bool {
	if account == nil || !account.IsOpenAIOAuth() {
		return false
	}
	if account.Extra == nil {
		return true
	}
	raw, ok := account.Extra["codex_usage_updated_at"]
	if !ok {
		return true
	}
	updatedAt, err := parseTime(fmt.Sprint(raw))
	if err != nil {
		return true
	}
	return now.Sub(updatedAt) >= openaiQuotaRefreshInterval
}

func (s *OpenAIQuotaService) persistUsageSnapshot(ctx context.Context, accountID int64, usage *OpenAIQuotaUsage) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || usage == nil {
		return
	}
	updates := buildOpenAIQuotaUsageExtraUpdates(usage)
	if len(updates) == 0 {
		return
	}
	if err := s.accountRepo.UpdateExtra(ctx, accountID, updates); err != nil {
		slog.Warn("openai_quota_snapshot_persist_failed", "account_id", accountID, "error", err)
	}
}

func buildOpenAIQuotaUsageExtraUpdates(usage *OpenAIQuotaUsage) map[string]any {
	if usage == nil {
		return nil
	}
	baseTime := time.Now().UTC()
	if usage.FetchedAt > 0 {
		baseTime = time.Unix(usage.FetchedAt, 0).UTC()
	}
	updates := make(map[string]any)
	if usage.RateLimit != nil {
		snapshot := &OpenAICodexUsageSnapshot{UpdatedAt: baseTime.Format(time.RFC3339)}
		setQuotaWindow := func(window *OpenAIRateLimitWindow, used **float64, reset **int, minutes **int) {
			if window == nil {
				return
			}
			usedPercent := window.UsedPercent
			resetAfterSeconds := window.ResetAfterSeconds
			if resetAfterSeconds <= 0 && window.ResetAt > 0 && usage.FetchedAt > 0 {
				resetAfterSeconds = window.ResetAt - usage.FetchedAt
				if resetAfterSeconds < 0 {
					resetAfterSeconds = 0
				}
			}
			windowMinutes := window.LimitWindowSeconds / 60
			resetAfterSecondsValue := int(resetAfterSeconds)
			*used = &usedPercent
			*reset = &resetAfterSecondsValue
			if windowMinutes > 0 {
				minutesValue := int(windowMinutes)
				*minutes = &minutesValue
			}
		}
		setQuotaWindow(usage.RateLimit.PrimaryWindow, &snapshot.PrimaryUsedPercent, &snapshot.PrimaryResetAfterSeconds, &snapshot.PrimaryWindowMinutes)
		setQuotaWindow(usage.RateLimit.SecondaryWindow, &snapshot.SecondaryUsedPercent, &snapshot.SecondaryResetAfterSeconds, &snapshot.SecondaryWindowMinutes)
		for key, value := range buildCodexUsageExtraUpdates(snapshot, baseTime) {
			updates[key] = value
		}
	}
	if usage.RateLimitResetCredits != nil {
		updates["codex_reset_credit_available_count"] = usage.RateLimitResetCredits.AvailableCount
		updates["codex_reset_credit_fetched_at"] = baseTime.Format(time.RFC3339)
	}
	return updates
}

func (s *OpenAIQuotaService) ResetCredit(ctx context.Context, accountID int64) (*OpenAIQuotaResetResult, error) {
	accessToken, chatGPTAccountID, proxyURL, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}

	redeemRequestID, err := generateOpenAIQuotaRedeemRequestID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_QUOTA_REDEEM_ID_FAILED", "failed to generate redeem id: %v", err)
	}

	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_CLIENT_ERROR", "failed to build upstream client: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()

	headers := buildCodexCommonHeaders(accessToken, chatGPTAccountID)
	headers["content-type"] = "application/json"
	var payload OpenAIQuotaResetResult
	resp, err := client.R().
		SetContext(callCtx).
		SetHeaders(headers).
		SetBody(map[string]string{"redeem_request_id": redeemRequestID}).
		SetSuccessResult(&payload).
		Post(chatGPTRateLimitResetURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_RESET_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		status := resp.StatusCode
		slog.Warn("openai_quota_reset_failed", "account_id", accountID, "status", status)
		return nil, infraerrors.Newf(mapOpenAIQuotaUpstreamStatus(status), "OPENAI_QUOTA_RESET_UPSTREAM_ERROR", "upstream returned status %d", status)
	}

	return &payload, nil
}

func (s *OpenAIQuotaService) prepareUpstreamCall(ctx context.Context, accountID int64) (string, string, string, error) {
	if s == nil || s.accountRepo == nil || s.tokenProvider == nil || s.privacyClientFactory == nil {
		return "", "", "", infraerrors.New(http.StatusInternalServerError, "OPENAI_QUOTA_NOT_CONFIGURED", "openai quota service is not configured")
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return "", "", "", infraerrors.New(http.StatusNotFound, "OPENAI_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformOpenAI {
		return "", "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_QUOTA_INVALID_PLATFORM", "account is not an OpenAI account")
	}
	if account.Type != AccountTypeOAuth {
		return "", "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_QUOTA_INVALID_TYPE", "account is not an OAuth account")
	}

	chatGPTAccountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if chatGPTAccountID == "" {
		chatGPTAccountID = strings.TrimSpace(account.GetCredential("organization_id"))
	}
	if chatGPTAccountID == "" {
		return "", "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_QUOTA_MISSING_ACCOUNT_ID", "chatgpt_account_id is missing; please re-authorize this account")
	}

	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil || strings.TrimSpace(accessToken) == "" {
		return "", "", "", infraerrors.New(http.StatusBadGateway, "OPENAI_QUOTA_TOKEN_UNAVAILABLE", "failed to acquire access token")
	}

	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		} else if s.proxyRepo != nil {
			if proxy, proxyErr := s.proxyRepo.GetByID(ctx, *account.ProxyID); proxyErr == nil && proxy != nil {
				proxyURL = proxy.URL()
			}
		}
	}
	return accessToken, chatGPTAccountID, proxyURL, nil
}

func buildCodexCommonHeaders(accessToken, chatGPTAccountID string) map[string]string {
	return map[string]string{
		"authorization":      "Bearer " + accessToken,
		"chatgpt-account-id": chatGPTAccountID,
		"oai-language":       openaiQuotaLanguageTag,
		"originator":         openaiQuotaCodexOriginator,
		"accept":             "application/json",
		"sec-fetch-site":     openaiQuotaSecFetchSite,
		"sec-fetch-mode":     openaiQuotaSecFetchMode,
		"sec-fetch-dest":     openaiQuotaSecFetchDest,
		"priority":           "u=4, i",
	}
}

func generateOpenAIQuotaRedeemRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]), nil
}

func mapOpenAIQuotaUpstreamStatus(status int) int {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return status
	case http.StatusTooManyRequests:
		return status
	default:
		return http.StatusBadGateway
	}
}
