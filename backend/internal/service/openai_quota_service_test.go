package service

import (
	"testing"
	"time"
)

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

func TestOpenAIQuotaRefreshIntervalIsOneHour(t *testing.T) {
	if openaiQuotaRefreshInterval != time.Hour {
		t.Fatalf("openaiQuotaRefreshInterval = %v, want %v", openaiQuotaRefreshInterval, time.Hour)
	}
}
