package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestRunContentModerationSkipsBeforeInputBuildWhenRiskControlDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(`{"model":"claude","messages":[]}`)))

	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{
		service.SettingKeyRiskControlEnabled: "false",
	}}
	svc := service.NewContentModerationService(
		settingRepo,
		&contentModerationHandlerTestRepo{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	var logs bytes.Buffer
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&logs),
		zapcore.InfoLevel,
	))

	decision := runContentModeration(
		c,
		logger,
		svc,
		nil,
		middleware.AuthSubject{UserID: 1},
		service.ContentModerationProtocolAnthropicMessages,
		"claude",
		[]byte(`{"model":"claude","messages":[]}`),
	)

	require.Nil(t, decision)
	require.NotContains(t, logs.String(), "content_moderation.gateway_check_start")
	require.NotContains(t, logs.String(), "content_moderation.skip_feature_disabled")
	require.NotContains(t, logs.String(), "content_moderation.gateway_check_done")
}
