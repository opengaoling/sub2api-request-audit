//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestCheckErrorPolicy — 6 table-driven cases for the pure logic function
// ---------------------------------------------------------------------------

func TestCheckErrorPolicy(t *testing.T) {
	tests := []struct {
		name       string
		account    *Account
		statusCode int
		body       []byte
		expected   ErrorPolicyResult
	}{
		{
			name: "no_policy_oauth_returns_none",
			account: &Account{
				ID:       1,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
				// no custom error codes, no temp rules
			},
			statusCode: 500,
			body:       []byte(`"error"`),
			expected:   ErrorPolicyNone,
		},
		{
			name: "custom_error_codes_hit_returns_matched",
			account: &Account{
				ID:       2,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429), float64(500)},
				},
			},
			statusCode: 500,
			body:       []byte(`"error"`),
			expected:   ErrorPolicyMatched,
		},
		{
			name: "custom_error_codes_miss_returns_skipped",
			account: &Account{
				ID:       3,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429), float64(500)},
				},
			},
			statusCode: 503,
			body:       []byte(`"error"`),
			expected:   ErrorPolicySkipped,
		},
		{
			name: "temp_unschedulable_hit_returns_temp_unscheduled",
			account: &Account{
				ID:       4,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(10),
							"description":      "overloaded rule",
						},
					},
				},
			},
			statusCode: 503,
			body:       []byte(`overloaded service`),
			expected:   ErrorPolicyTempUnscheduled,
		},
		{
			name: "temp_unschedulable_401_first_hit_returns_temp_unscheduled",
			account: &Account{
				ID:       14,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(401),
							"keywords":         []any{"unauthorized"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicyTempUnscheduled,
		},
		{
			// Antigravity 401 不走升级逻辑（由 applyErrorPolicy 的 temp_unschedulable_rules 自行控制），
			// second hit 仍然返回 TempUnscheduled。
			name: "temp_unschedulable_401_second_hit_antigravity_stays_temp",
			account: &Account{
				ID:                      15,
				Type:                    AccountTypeOAuth,
				Platform:                PlatformAntigravity,
				TempUnschedulableReason: `{"status_code":401,"until_unix":1735689600}`,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(401),
							"keywords":         []any{"unauthorized"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicyTempUnscheduled,
		},
		{
			name: "temp_unschedulable_body_miss_returns_none",
			account: &Account{
				ID:       5,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(10),
							"description":      "overloaded rule",
						},
					},
				},
			},
			statusCode: 503,
			body:       []byte(`random msg`),
			expected:   ErrorPolicyNone,
		},
		{
			name: "custom_error_codes_override_temp_unschedulable",
			account: &Account{
				ID:       6,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(503)},
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(10),
							"description":      "overloaded rule",
						},
					},
				},
			},
			statusCode: 503,
			body:       []byte(`overloaded`),
			expected:   ErrorPolicyMatched, // custom codes take precedence
		},
		{
			name: "pool_mode_custom_error_codes_hit_returns_matched",
			account: &Account{
				ID:       7,
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":                  true,
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(401), float64(403)},
				},
			},
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicyMatched,
		},
		{
			name: "pool_mode_without_custom_error_codes_returns_skipped",
			account: &Account{
				ID:       8,
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode": true,
				},
			},
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicySkipped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &errorPolicyRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

			result := svc.CheckErrorPolicy(context.Background(), tt.account, tt.statusCode, tt.body)
			require.Equal(t, tt.expected, result, "unexpected ErrorPolicyResult")
		})
	}
}

func TestHandleUpstreamError_PoolModeCustomErrorCodesOverride(t *testing.T) {
	t.Run("pool_mode_without_custom_error_codes_still_skips", func(t *testing.T) {
		repo := &errorPolicyRepoStub{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := &Account{
			ID:       30,
			Type:     AccountTypeAPIKey,
			Platform: PlatformOpenAI,
			Credentials: map[string]any{
				"pool_mode": true,
			},
		}

		shouldDisable := svc.HandleUpstreamError(context.Background(), account, 401, http.Header{}, []byte("unauthorized"))

		require.False(t, shouldDisable)
		require.Equal(t, 0, repo.setErrCalls)
		require.Equal(t, 0, repo.tempCalls)
	})

	t.Run("pool_mode_with_custom_error_codes_uses_local_error_policy", func(t *testing.T) {
		repo := &errorPolicyRepoStub{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := &Account{
			ID:       31,
			Type:     AccountTypeAPIKey,
			Platform: PlatformOpenAI,
			Credentials: map[string]any{
				"pool_mode":                  true,
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(401)},
			},
		}

		shouldDisable := svc.HandleUpstreamError(context.Background(), account, 401, http.Header{}, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrCalls)
		require.Equal(t, 0, repo.tempCalls)
	})
}

func TestCheckErrorPolicy_GlobalTempUnschedulableRule(t *testing.T) {
	repo := &errorPolicyRepoStub{}
	settingRepo := &errorPolicySettingRepoStub{values: map[string]string{
		SettingKeyGlobalTempUnschedulableEnabled: "true",
		SettingKeyGlobalTempUnschedulableRules: `[{
			"error_code": 429,
			"keywords": ["usage limit has been reached"],
			"duration_minutes": 60,
			"description": "plus usage limit"
		}]`,
	}}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(NewSettingService(settingRepo, &config.Config{}))

	result := svc.CheckErrorPolicy(
		context.Background(),
		&Account{ID: 99, Type: AccountTypeOAuth, Platform: PlatformOpenAI},
		http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"The Usage Limit Has Been Reached","type":"usage_limit_reached"}}`),
	)

	require.Equal(t, ErrorPolicyTempUnscheduled, result)
	require.Equal(t, 1, repo.tempCalls)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.lastTempMsg), &state))
	require.Equal(t, http.StatusTooManyRequests, state.StatusCode)
	require.Equal(t, "usage limit has been reached", state.MatchedKeyword)
}

func TestCheckErrorPolicy_GlobalTempUnschedulableSkipsOAuthExpiredAuthenticationToken(t *testing.T) {
	repo := &errorPolicyRepoStub{}
	settingRepo := &errorPolicySettingRepoStub{values: map[string]string{
		SettingKeyGlobalTempUnschedulableEnabled: "true",
		SettingKeyGlobalTempUnschedulableRules: `[{
			"match_type": "keyword",
			"keywords": [
				"Your credit balance is too low",
				"This organization has been disabled.",
				"You exceeded your current quota",
				"Permission denied",
				"The security token included in the request is invalid",
				"Operation not allowed",
				"Your account is not authorized",
				"You have reached your specified",
				"openrouter.ai"
			],
			"duration_minutes": 60,
			"description": "user configured upstream account errors"
		}]`,
	}}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(NewSettingService(settingRepo, &config.Config{}))

	body := []byte(`{
		"error": {
			"message": "OAuth 401: Provided authentication token is expired. Please try signing in again. upstream=openrouter.ai"
		}
	}`)

	result := svc.CheckErrorPolicy(
		context.Background(),
		&Account{ID: 100, Type: AccountTypeOAuth, Platform: PlatformOpenAI},
		http.StatusUnauthorized,
		body,
	)

	require.Equal(t, ErrorPolicyNone, result)
	require.Equal(t, 0, repo.tempCalls)
}

func TestCheckErrorPolicy_GlobalTempUnschedulableMatchTypes(t *testing.T) {
	body := []byte(`{
		"error": {
			"code": 402,
			"message": "This request requires more credits, or fewer max_tokens."
		}
	}`)

	tests := []struct {
		name          string
		matchType     string
		statusCode    int
		body          []byte
		wantResult    ErrorPolicyResult
		wantTempCalls int
		wantKeyword   string
	}{
		{
			name:          "402_with_credits_triggers_temp_unschedulable",
			matchType:     TempUnschedulableMatchTypeCombined,
			statusCode:    402,
			body:          body,
			wantResult:    ErrorPolicyTempUnscheduled,
			wantTempCalls: 1,
			wantKeyword:   "credits",
		},
		{
			name:          "combined_402_without_keyword_does_not_trigger",
			matchType:     TempUnschedulableMatchTypeCombined,
			statusCode:    402,
			body:          []byte(`{"error":{"code":402,"message":"payment required"}}`),
			wantResult:    ErrorPolicyNone,
			wantTempCalls: 0,
		},
		{
			name:          "combined_keyword_without_status_match_does_not_trigger",
			matchType:     TempUnschedulableMatchTypeCombined,
			statusCode:    429,
			body:          body,
			wantResult:    ErrorPolicyNone,
			wantTempCalls: 0,
		},
		{
			name:          "status_code_only_triggers_without_keyword",
			matchType:     TempUnschedulableMatchTypeStatusCode,
			statusCode:    402,
			body:          []byte(`{"error":{"message":"payment required"}}`),
			wantResult:    ErrorPolicyTempUnscheduled,
			wantTempCalls: 1,
			wantKeyword:   "",
		},
		{
			name:          "keyword_only_triggers_without_status_match",
			matchType:     TempUnschedulableMatchTypeKeyword,
			statusCode:    429,
			body:          body,
			wantResult:    ErrorPolicyTempUnscheduled,
			wantTempCalls: 1,
			wantKeyword:   "credits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &errorPolicyRepoStub{}
			settingRepo := &errorPolicySettingRepoStub{values: map[string]string{
				SettingKeyGlobalTempUnschedulableEnabled: "true",
				SettingKeyGlobalTempUnschedulableRules: `[{
					"match_type": "` + tt.matchType + `",
					"error_code": 402,
					"keywords": ["credits"],
					"duration_minutes": 30
				}]`,
			}}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc.SetSettingService(NewSettingService(settingRepo, &config.Config{}))

			result := svc.CheckErrorPolicy(
				context.Background(),
				&Account{ID: 40, Type: AccountTypeOAuth, Platform: PlatformAntigravity},
				tt.statusCode,
				tt.body,
			)

			require.Equal(t, tt.wantResult, result)
			require.Equal(t, tt.wantTempCalls, repo.tempCalls)
			require.Equal(t, 0, repo.setErrCalls)
			if tt.wantTempCalls > 0 {
				require.Contains(t, repo.lastTempMsg, `"status_code":`+strconv.Itoa(tt.statusCode))
				require.Contains(t, repo.lastTempMsg, `"match_type":"`+tt.matchType+`"`)
				require.Contains(t, repo.lastTempMsg, `"matched_keyword":"`+tt.wantKeyword+`"`)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestApplyErrorPolicy — 4 table-driven cases for the wrapper method
// ---------------------------------------------------------------------------

func TestApplyErrorPolicy(t *testing.T) {
	tests := []struct {
		name              string
		account           *Account
		statusCode        int
		body              []byte
		expectedHandled   bool
		expectedStatus    int  // expected outStatus
		expectedSwitchErr bool // expect *AntigravityAccountSwitchError
		handleErrorCalls  int
	}{
		{
			name: "none_not_handled",
			account: &Account{
				ID:       10,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
			},
			statusCode:       500,
			body:             []byte(`"error"`),
			expectedHandled:  false,
			expectedStatus:   500, // passthrough
			handleErrorCalls: 0,
		},
		{
			name: "skipped_handled_no_handleError",
			account: &Account{
				ID:       11,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)},
				},
			},
			statusCode:       500, // not in custom codes
			body:             []byte(`"error"`),
			expectedHandled:  true,
			expectedStatus:   http.StatusInternalServerError, // skipped → 500
			handleErrorCalls: 0,
		},
		{
			name: "matched_handled_calls_handleError",
			account: &Account{
				ID:       12,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(500)},
				},
			},
			statusCode:       500,
			body:             []byte(`"error"`),
			expectedHandled:  true,
			expectedStatus:   500, // matched → original status
			handleErrorCalls: 1,
		},
		{
			name: "temp_unscheduled_returns_switch_error",
			account: &Account{
				ID:       13,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			statusCode:        503,
			body:              []byte(`overloaded`),
			expectedHandled:   true,
			expectedStatus:    503, // temp_unscheduled → original status
			expectedSwitchErr: true,
			handleErrorCalls:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &errorPolicyRepoStub{}
			rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc := &AntigravityGatewayService{
				rateLimitService: rlSvc,
			}

			var handleErrorCount int
			p := antigravityRetryLoopParams{
				ctx:     context.Background(),
				prefix:  "[test]",
				account: tt.account,
				handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
					handleErrorCount++
					return nil
				},
				isStickySession: true,
			}

			handled, outStatus, retErr := svc.applyErrorPolicy(p, tt.statusCode, http.Header{}, tt.body)

			require.Equal(t, tt.expectedHandled, handled, "handled mismatch")
			require.Equal(t, tt.expectedStatus, outStatus, "outStatus mismatch")
			require.Equal(t, tt.handleErrorCalls, handleErrorCount, "handleError call count mismatch")

			if tt.expectedSwitchErr {
				var switchErr *AntigravityAccountSwitchError
				require.ErrorAs(t, retErr, &switchErr)
				require.Equal(t, tt.account.ID, switchErr.OriginalAccountID)
			} else {
				require.NoError(t, retErr)
			}
		})
	}
}

func TestApplyErrorPolicy_GeminiRateLimitBypassesCustomSkip(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AntigravityGatewayService{
		rateLimitService: rlSvc,
		accountRepo:      repo,
		cache:            cache,
	}

	account := &Account{
		ID:       31,
		Type:     AccountTypeAPIKey,
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(500)},
		},
	}
	body := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"}
			]
		}
	}`)
	p := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accountRepo: repo,
		groupID:     42,
		sessionHash: "gemini:sticky",
		handleError: func(context.Context, string, *Account, int, http.Header, []byte, string, int64, string, bool) *handleModelRateLimitResult {
			t.Fatal("model rate limit should be handled before custom error fallback")
			return nil
		},
	}

	handled, outStatus, retErr := svc.applyErrorPolicy(p, http.StatusTooManyRequests, http.Header{}, body)

	require.True(t, handled)
	require.Equal(t, http.StatusTooManyRequests, outStatus)
	require.NoError(t, retErr)
	require.Len(t, repo.modelRateLimitCalls, 2)
	require.Equal(t, "gemini-3-flash", repo.modelRateLimitCalls[0].modelKey)
	require.Equal(t, antigravityGeminiModelRateLimitKey, repo.modelRateLimitCalls[1].modelKey)
	require.Len(t, cache.deleteCalls, 1)
	require.Equal(t, int64(42), cache.deleteCalls[0].groupID)
	require.Equal(t, "gemini:sticky", cache.deleteCalls[0].sessionHash)
}

// ---------------------------------------------------------------------------
// errorPolicyRepoStub — minimal AccountRepository stub for error policy tests
// ---------------------------------------------------------------------------

type errorPolicyRepoStub struct {
	mockAccountRepoForGemini
	tempCalls    int
	setErrCalls  int
	lastErrorMsg string
	lastTempMsg  string
}

func (r *errorPolicyRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
	r.lastTempMsg = reason
	return nil
}

func (r *errorPolicyRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrCalls++
	r.lastErrorMsg = errorMsg
	return nil
}

type errorPolicySettingRepoStub struct {
	values map[string]string
}

func (r *errorPolicySettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *errorPolicySettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *errorPolicySettingRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *errorPolicySettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *errorPolicySettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *errorPolicySettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *errorPolicySettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}
