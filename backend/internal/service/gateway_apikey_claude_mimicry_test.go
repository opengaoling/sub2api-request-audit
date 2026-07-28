//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAPIKeyClaudeCodeMimicryPassesProjectClientValidator(t *testing.T) {
	deviceID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sessionID := generateSessionUUID("apikey-claude-code-mimicry")
	metadataUserID := FormatMetadataUserID(deviceID, "", sessionID, claude.CLICurrentVersion)
	body := []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"hello"}]}`)

	normalized, _ := normalizeClaudeOAuthRequestBody(body, "claude-opus-4-6", claudeOAuthNormalizeOptions{
		injectMetadata: true,
		metadataUserID: metadataUserID,
	})
	userAgent := claude.DefaultHeaders["User-Agent"]
	actualMetadata := gjson.GetBytes(normalized, "metadata.user_id").String()

	require.NotEmpty(t, actualMetadata)
	require.True(t, isClaudeCodeClient(userAgent, actualMetadata))
}

func TestAPIKeyClaudeCodeMimicryBetaUsesFullClientSet(t *testing.T) {
	service := &GatewayService{}
	beta, shouldSet := service.computeFinalAnthropicBeta(
		"apikey", true, "claude-opus-4-6", nil, []byte(`{}`), nil,
	)

	require.True(t, shouldSet)
	require.Contains(t, beta, claude.BetaClaudeCode)
	require.Contains(t, beta, claude.BetaOAuth)
}

func TestClaudeCodeMimicryUsesOverriddenUserAgentVersion(t *testing.T) {
	account := headerOverrideTestAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]any{
		credKeyHeaderOverrideEnabled: true,
		credKeyHeaderOverrides: map[string]any{
			"user-agent": "claude-cli/2.1.161 (external, cli)",
		},
	})

	userAgent := claudeCodeMimicryUserAgent(account)
	metadataUserID := FormatMetadataUserID(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"",
		generateSessionUUID("ua-override"),
		ExtractCLIVersion(userAgent),
	)

	require.Equal(t, "claude-cli/2.1.161 (external, cli)", userAgent)
	require.True(t, isClaudeCodeClient(userAgent, metadataUserID))
}
