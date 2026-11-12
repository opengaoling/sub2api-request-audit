package service

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculateOAuth7dSchedulingScore(t *testing.T) {
	t.Run("user scenario: 60 percent with 5d1h vs 88 percent with 1d22h", func(t *testing.T) {
		remA := int64((5*24 + 1) * 3600) // 121h
		scoreA := calculateOAuth7dSchedulingScore(60.0, remA)

		remB := int64((1*24 + 22) * 3600) // 46h
		scoreB := calculateOAuth7dSchedulingScore(88.0, remB)

		// 60 + (121/168)*100 = 60 + 72.0238 = 132.0238
		// 88 + (46/168)*100 = 88 + 27.3810 = 115.3810
		require.InDelta(t, 132.0238, scoreA, 1e-3)
		require.InDelta(t, 115.3810, scoreB, 1e-3)
		require.Less(t, scoreB, scoreA, "88% with 1d22h remaining should have lower score (higher priority) than 60% with 5d1h remaining")
	})

	t.Run("same usage nearer reset wins", func(t *testing.T) {
		scoreNear := calculateOAuth7dSchedulingScore(50.0, 2*3600)
		scoreFar := calculateOAuth7dSchedulingScore(50.0, 48*3600)
		require.Less(t, scoreNear, scoreFar)
	})

	t.Run("same reset lower usage wins", func(t *testing.T) {
		scoreLow := calculateOAuth7dSchedulingScore(20.0, 24*3600)
		scoreHigh := calculateOAuth7dSchedulingScore(70.0, 24*3600)
		require.Less(t, scoreLow, scoreHigh)
	})

	t.Run("boundary handling", func(t *testing.T) {
		// NaN and Inf defense
		require.Equal(t, 100.0, calculateOAuth7dSchedulingScore(math.NaN(), oauth7dWindowSeconds))
		require.Equal(t, 100.0, calculateOAuth7dSchedulingScore(math.Inf(1), oauth7dWindowSeconds))
		require.Equal(t, 100.0, calculateOAuth7dSchedulingScore(-10.0, oauth7dWindowSeconds))

		// Expired / zero remaining
		require.Equal(t, 15.0, calculateOAuth7dSchedulingScore(15.0, 0))
		require.Equal(t, 15.0, calculateOAuth7dSchedulingScore(15.0, -100))

		// Beyond 7d or unknown (math.MaxInt64)
		require.Equal(t, 115.0, calculateOAuth7dSchedulingScore(15.0, math.MaxInt64))
		require.Equal(t, 115.0, calculateOAuth7dSchedulingScore(15.0, oauth7dWindowSeconds+3600))
	})
}

func TestCompareOAuth7dSchedulingUsage(t *testing.T) {
	t.Run("user scenario: 88 percent with 1d22h preferred over 60 percent with 5d1h", func(t *testing.T) {
		remA := int64((5*24 + 1) * 3600)  // 5d1h
		remB := int64((1*24 + 22) * 3600) // 1d22h
		usedA := 60.0
		usedB := 88.0

		// B compared to A
		preferB, ok := compareOAuth7dSchedulingUsage(remB, remA, usedB, usedA)
		require.True(t, ok)
		require.True(t, preferB, "账号B（88%离重置剩1天22小时）应优于账号A（60%离重置剩5天1小时），以尽快消耗即将重置的额度避免浪费")

		// A compared to B
		preferA, ok := compareOAuth7dSchedulingUsage(remA, remB, usedA, usedB)
		require.True(t, ok)
		require.False(t, preferA)
	})

	t.Run("exhausted account never preferred over non-exhausted", func(t *testing.T) {
		remExhausted := int64(1 * 3600) // 1 hour left
		remAvailable := int64(5 * 24 * 3600)
		usedExhausted := 100.0
		usedAvailable := 60.0

		preferAvailable, ok := compareOAuth7dSchedulingUsage(remAvailable, remExhausted, usedAvailable, usedExhausted)
		require.True(t, ok)
		require.True(t, preferAvailable, "未耗尽的账号必须优先于已达到100%耗尽的账号")
	})

	t.Run("both exhausted prefer nearer reset", func(t *testing.T) {
		preferNear, ok := compareOAuth7dSchedulingUsage(1800, 3600, 100.0, 100.0)
		require.True(t, ok)
		require.True(t, preferNear, "均耗尽时，距离重置时间更近（先刷新）的优先")
	})

	t.Run("tie breaking", func(t *testing.T) {
		// Exactly identical
		_, ok := compareOAuth7dSchedulingUsage(3600, 3600, 50.0, 50.0)
		require.False(t, ok)
	})
}

func TestCompareOpenAIOAuthCodex7dSchedulingAccounts_UserScenario(t *testing.T) {
	now := time.Now()
	accA := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Priority: 1,
		Extra: map[string]any{
			"codex_7d_reset_at":     now.Add(time.Duration(5*24+1) * time.Hour).UTC().Format(time.RFC3339),
			"codex_7d_used_percent": 60.0,
		},
	}
	accB := &Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Priority: 1,
		Extra: map[string]any{
			"codex_7d_reset_at":     now.Add(time.Duration(1*24+22) * time.Hour).UTC().Format(time.RFC3339),
			"codex_7d_used_percent": 88.0,
		},
	}

	preferB, ok := compareOpenAIOAuthCodex7dSchedulingAccounts(accB, accA, now)
	require.True(t, ok)
	require.True(t, preferB, "OpenAI OAuth 调度在综合评分下应优先选择已用88%但仅剩1天22小时即将重置的账号")

	preferA, ok := compareOpenAIOAuthCodex7dSchedulingAccounts(accA, accB, now)
	require.True(t, ok)
	require.False(t, preferA)
}

func TestCompareAnthropicOAuth7dSchedulingAccounts_UserScenario(t *testing.T) {
	now := time.Now()
	accA := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Priority: 1,
		Extra: map[string]any{
			"passive_usage_7d_reset":       now.Add(time.Duration(5*24+1) * time.Hour).Unix(),
			"passive_usage_7d_utilization": 0.60,
		},
	}
	accB := &Account{
		ID:       2,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Priority: 1,
		Extra: map[string]any{
			"passive_usage_7d_reset":       now.Add(time.Duration(1*24+22) * time.Hour).Unix(),
			"passive_usage_7d_utilization": 0.88,
		},
	}

	preferB, ok := compareAnthropicOAuth7dSchedulingAccounts(accB, accA, now)
	require.True(t, ok)
	require.True(t, preferB, "Anthropic OAuth 调度在综合评分下应优先选择已用88%但仅剩1天22小时即将重置的账号")

	preferA, ok := compareAnthropicOAuth7dSchedulingAccounts(accA, accB, now)
	require.True(t, ok)
	require.False(t, preferA)
}
