//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestClientFingerprintRepository_PersistsDistinctPlatformFingerprints(t *testing.T) {
	ctx := context.Background()
	repository := NewClientFingerprintRepository(integrationDB)
	openAI := service.CapturedFingerprint{
		ID: "integration-openai-fingerprint", Platform: string(service.PlatformOpenAI),
		Headers: map[string]string{"user-agent": "codex-test", "originator": "codex_cli_rs"}, UserAgent: "codex-test",
	}
	anthropic := service.CapturedFingerprint{
		ID: "integration-anthropic-fingerprint", Platform: string(service.PlatformAnthropic),
		Headers: map[string]string{"user-agent": "claude-test", "x-stainless-lang": "js"}, UserAgent: "claude-test",
	}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM client_request_fingerprints WHERE fingerprint_hash IN ($1, $2)", openAI.ID, anthropic.ID)
	})

	require.NoError(t, repository.Upsert(ctx, openAI))
	require.NoError(t, repository.Upsert(ctx, openAI))
	require.NoError(t, repository.Upsert(ctx, anthropic))

	openAIRecord, err := repository.Get(ctx, string(service.PlatformOpenAI), openAI.ID)
	require.NoError(t, err)
	require.NotNil(t, openAIRecord)
	require.Equal(t, int64(2), openAIRecord.CaptureCount)
	require.Equal(t, "codex_cli_rs", openAIRecord.Headers["originator"])
	require.NotContains(t, openAIRecord.Headers, "x-stainless-lang")

	anthropicRecord, err := repository.Get(ctx, string(service.PlatformAnthropic), anthropic.ID)
	require.NoError(t, err)
	require.NotNil(t, anthropicRecord)
	require.Equal(t, int64(1), anthropicRecord.CaptureCount)
	require.Equal(t, "js", anthropicRecord.Headers["x-stainless-lang"])
	require.NotContains(t, anthropicRecord.Headers, "originator")
}
