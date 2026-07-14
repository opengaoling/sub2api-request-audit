package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestInterceptRunsBeforeContentModeration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		matchContent    = "bad prompt"
		responseContent = "local intercept response"
	)

	tests := []struct {
		name     string
		path     string
		platform string
		body     string
		run      func(*gin.Context, *service.SettingService, *service.ContentModerationService)
	}{
		{
			name:     "anthropic_messages",
			path:     "/v1/messages",
			platform: service.PlatformAnthropic,
			body:     `{"model":"claude-sonnet-4-5","max_tokens":128,"messages":[{"role":"user","content":"bad prompt"}]}`,
			run: func(c *gin.Context, settingSvc *service.SettingService, moderationSvc *service.ContentModerationService) {
				(&GatewayHandler{
					gatewayService:           &service.GatewayService{},
					settingService:           settingSvc,
					contentModerationService: moderationSvc,
				}).Messages(c)
			},
		},
		{
			name:     "gateway_chat_completions",
			path:     "/v1/chat/completions",
			platform: service.PlatformAnthropic,
			body:     `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"bad prompt"}]}`,
			run: func(c *gin.Context, settingSvc *service.SettingService, moderationSvc *service.ContentModerationService) {
				(&GatewayHandler{
					gatewayService:           &service.GatewayService{},
					settingService:           settingSvc,
					contentModerationService: moderationSvc,
				}).ChatCompletions(c)
			},
		},
		{
			name:     "gateway_responses",
			path:     "/v1/responses",
			platform: service.PlatformAnthropic,
			body:     `{"model":"claude-sonnet-4-5","input":[{"role":"user","content":[{"type":"input_text","text":"bad prompt"}]}]}`,
			run: func(c *gin.Context, settingSvc *service.SettingService, moderationSvc *service.ContentModerationService) {
				(&GatewayHandler{
					gatewayService:           &service.GatewayService{},
					settingService:           settingSvc,
					contentModerationService: moderationSvc,
				}).Responses(c)
			},
		},
		{
			name:     "openai_messages",
			path:     "/openai/v1/messages",
			platform: service.PlatformOpenAI,
			body:     `{"model":"gpt-5.5","max_tokens":128,"messages":[{"role":"user","content":"bad prompt"}]}`,
			run: func(c *gin.Context, settingSvc *service.SettingService, moderationSvc *service.ContentModerationService) {
				openAIRequestInterceptPriorityHandler(settingSvc, moderationSvc).Messages(c)
			},
		},
		{
			name:     "openai_chat_completions",
			path:     "/openai/v1/chat/completions",
			platform: service.PlatformOpenAI,
			body:     `{"model":"gpt-5.5","messages":[{"role":"user","content":"bad prompt"}]}`,
			run: func(c *gin.Context, settingSvc *service.SettingService, moderationSvc *service.ContentModerationService) {
				openAIRequestInterceptPriorityHandler(settingSvc, moderationSvc).ChatCompletions(c)
			},
		},
		{
			name:     "openai_responses",
			path:     "/openai/v1/responses",
			platform: service.PlatformOpenAI,
			body:     `{"model":"gpt-5.5","input":[{"role":"user","content":[{"type":"input_text","text":"bad prompt"}]}]}`,
			run: func(c *gin.Context, settingSvc *service.SettingService, moderationSvc *service.ContentModerationService) {
				openAIRequestInterceptPriorityHandler(settingSvc, moderationSvc).Responses(c)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupID := int64(42)
			settingSvc, moderationSvc, moderationRepo, moderationCalls := requestInterceptPriorityServices(
				t,
				groupID,
				matchContent,
				responseContent,
			)
			c, recorder := newRequestInterceptPriorityContext(t, tt.path, tt.body, tt.platform, groupID)

			tt.run(c, settingSvc, moderationSvc)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "exact", recorder.Header().Get("X-Sub2API-Request-Intercepted"))
			require.Contains(t, recorder.Body.String(), responseContent)
			require.NotContains(t, recorder.Body.String(), "moderation blocked")
			require.Zero(t, moderationCalls.Load(), "content moderation API must not be called when request intercept matches")
			require.Empty(t, moderationRepo.logSnapshot(), "content moderation log must not be written when request intercept matches")
		})
	}
}

func requestInterceptPriorityServices(t *testing.T, groupID int64, matchContent string, responseContent string) (*service.SettingService, *service.ContentModerationService, *contentModerationHandlerTestRepo, *atomic.Int32) {
	t.Helper()

	var moderationCalls atomic.Int32
	moderationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		moderationCalls.Add(1)
		require.Equal(t, "/v1/moderations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"category_scores":{"violence":0.99}}]}`))
	}))
	t.Cleanup(moderationServer.Close)

	cfg := &service.ContentModerationConfig{
		Enabled:      true,
		Mode:         service.ContentModerationModePreBlock,
		BaseURL:      moderationServer.URL,
		Model:        "omni-moderation-latest",
		APIKeys:      []string{"sk-test"},
		SampleRate:   100,
		AllGroups:    true,
		BlockMessage: "moderation blocked",
	}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{}}
	settingSvc := service.NewSettingService(settingRepo, &config.Config{})
	require.NoError(t, settingSvc.UpdateSettings(context.Background(), &service.SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
		RequestInterceptRules: []service.RequestInterceptRule{
			{MatchContent: matchContent, ResponseContent: responseContent},
		},
	}))
	require.NoError(t, settingRepo.Set(context.Background(), service.SettingKeyRiskControlEnabled, "true"))
	require.NoError(t, settingRepo.Set(context.Background(), service.SettingKeyContentModerationConfig, string(rawCfg)))

	moderationRepo := &contentModerationHandlerTestRepo{}
	moderationSvc := service.NewContentModerationService(
		settingRepo,
		moderationRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return settingSvc, moderationSvc, moderationRepo, &moderationCalls
}

func newRequestInterceptPriorityContext(t *testing.T, path string, body string, platform string, groupID int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	user := &service.User{ID: 1001, Status: service.StatusActive, Balance: 100}
	apiKey := &service.APIKey{
		ID:      2001,
		UserID:  user.ID,
		Name:    "request-intercept-priority-test",
		GroupID: &groupID,
		Status:  service.StatusAPIKeyActive,
		User:    user,
		Group: &service.Group{
			ID:                    groupID,
			Name:                  "priority-test",
			Platform:              platform,
			Status:                service.StatusActive,
			AllowMessagesDispatch: true,
		},
	}
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: user.ID, Concurrency: 1})
	return c, recorder
}

func openAIRequestInterceptPriorityHandler(settingSvc *service.SettingService, moderationSvc *service.ContentModerationService) *OpenAIGatewayHandler {
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
	}
	return &OpenAIGatewayHandler{
		gatewayService:           &service.OpenAIGatewayService{},
		billingCacheService:      &service.BillingCacheService{},
		apiKeyService:            &service.APIKeyService{},
		settingService:           settingSvc,
		contentModerationService: moderationSvc,
		concurrencyHelper:        NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
}
