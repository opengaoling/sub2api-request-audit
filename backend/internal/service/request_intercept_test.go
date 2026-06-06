package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateArithmeticFewShot_UserExample(t *testing.T) {
	text := "Calculate and respond with ONLY the number, nothing else.\n\nQ: 3 + 5 = ?\nA: 8\n\nQ: 12 - 7 = ?\nA: 5\n\nQ: 40 + 39 = ?\nA:"

	got, ok := EvaluateArithmeticFewShot(text)

	require.True(t, ok)
	require.Equal(t, "79", got)
}

func TestRequestInterceptExactRuleMatched(t *testing.T) {
	response, ok := requestInterceptExactRuleMatched([]RequestInterceptRule{{MatchContent: "hi", ResponseContent: "hello"}}, "hi")
	require.True(t, ok)
	require.Equal(t, "hello", response)

	_, ok = requestInterceptExactRuleMatched([]RequestInterceptRule{{MatchContent: "hi", ResponseContent: "hello"}}, "hi,how are you")
	require.False(t, ok)
}

func TestExtractRequestInterceptTextOpenAIChat(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"x"}}]}]}`)

	got := ExtractRequestInterceptText(RequestInterceptProtocolOpenAIChat, body)

	require.Equal(t, "hello", got)
}
