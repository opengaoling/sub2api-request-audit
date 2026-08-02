package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPartialStreamUsageResultPreservesObservedUsage(t *testing.T) {
	start := time.Now().Add(-time.Second)
	resp := &http.Response{Header: http.Header{"X-Request-Id": []string{"req-partial"}}}
	firstTokenMs := 80
	streamResult := &streamingResult{
		usage:            &ClaudeUsage{InputTokens: 12, OutputTokens: 4},
		firstTokenMs:     &firstTokenMs,
		clientDisconnect: true,
	}

	result := partialStreamUsageResult(resp, streamResult, "claude-3-7-sonnet", "claude-3-7-sonnet", start, errors.New("stream interrupted"))

	require.NotNil(t, result)
	require.Equal(t, "req-partial", result.RequestID)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.True(t, result.Stream)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 80, *result.FirstTokenMs)
}

func TestPartialStreamUsageResultSkipsFailoverErrors(t *testing.T) {
	result := partialStreamUsageResult(
		&http.Response{},
		&streamingResult{usage: &ClaudeUsage{InputTokens: 1}},
		"model",
		"model",
		time.Now(),
		&UpstreamFailoverError{StatusCode: http.StatusBadGateway},
	)

	require.Nil(t, result)
}
