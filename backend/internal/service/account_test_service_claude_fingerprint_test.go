//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountTestService_ClaudeOAuthUsesAccountFingerprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	// 模拟上游返回成功 SSE 数据
	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`event: message_start
data: {"type":"message_start","message":{"id":"msg_1"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

event: message_stop
data: {"type":"message_stop"}

`))

	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}

	repository := &clientFingerprintRepositoryStub{
		fingerprints: map[string]CapturedFingerprint{
			"anthropic:test-fp-1": {
				ID:        "test-fp-1",
				Platform:  string(PlatformAnthropic),
				UserAgent: "claude-cli/2.1.290 (external, cli)",
				Headers: map[string]string{
					"user-agent":         "claude-cli/2.1.290 (external, cli)",
					"x-stainless-os":     "Darwin",
					"x-stainless-arch":   "arm64",
					"x-stainless-lang":   "js",
					"x-stainless-runtime": "node",
				},
			},
		},
	}
	identitySvc := NewIdentityService(&identityCacheStub{})
	identitySvc.SetClientFingerprintRepository(repository)

	svc := &AccountTestService{
		httpUpstream:    upstream,
		identityService: identitySvc,
	}

	account := &Account{
		ID:          1001,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
		Extra: map[string]any{
			AnthropicFingerprintExtraKey: "test-fp-1",
		},
	}

	err := svc.testClaudeAccountConnection(ctx, account, "claude-3-5-sonnet-20241022")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)

	req := upstream.requests[0]
	// 验证请求头是否应用了账号设置的对应指纹
	require.Equal(t, "claude-cli/2.1.290 (external, cli)", req.Header.Get("User-Agent"))
	require.Equal(t, "Darwin", req.Header.Get("X-Stainless-Os"))
	require.Equal(t, "arm64", req.Header.Get("X-Stainless-Arch"))
	require.Equal(t, "node", req.Header.Get("X-Stainless-Runtime"))
	require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
}

func TestAccountTestService_OpenAIOAuthUsesAccountFingerprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))

	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}

	repository := &clientFingerprintRepositoryStub{
		fingerprints: map[string]CapturedFingerprint{
			"openai:test-fp-2": {
				ID:        "test-fp-2",
				Platform:  string(PlatformOpenAI),
				UserAgent: "codex_cli_rs/0.144.1",
				Headers: map[string]string{
					"user-agent": "codex_cli_rs/0.144.1",
					"originator": "custom_codex_originator",
				},
			},
		},
	}
	identitySvc := NewIdentityService(&identityCacheStub{})
	identitySvc.SetClientFingerprintRepository(repository)

	svc := &AccountTestService{
		httpUpstream:    upstream,
		identityService: identitySvc,
	}

	account := &Account{
		ID:          1002,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
		Extra: map[string]any{
			OpenAIFingerprintExtraKey: "test-fp-2",
		},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)

	req := upstream.requests[0]
	require.Equal(t, "codex_cli_rs/0.144.1", req.Header.Get("User-Agent"))
	require.Equal(t, "custom_codex_originator", req.Header.Get("Originator"))
}
