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

func TestEvaluatePythonPrintOutput_UserExample(t *testing.T) {
	text := "What is the output of this Python code?\n\nprint(\"RP_ANSWER=\" + str(81 + 50))\n\nReply with ONLY the output."

	got, ok := EvaluatePythonPrintOutput(text)

	require.True(t, ok)
	require.Equal(t, "RP_ANSWER=131", got)
}

func TestEvaluatePythonPrintOutput_RequiresMonitorPrompt(t *testing.T) {
	_, ok := EvaluatePythonPrintOutput(`print("RP_ANSWER=" + str(81 + 50))`)

	require.False(t, ok)
}

func TestEvaluatePythonPrintOutput_DivideByZeroIgnored(t *testing.T) {
	text := "What is the output of this Python code?\n\nprint(\"RP_ANSWER=\" + str(81 / 0))\n\nReply with ONLY the output."

	_, ok := EvaluatePythonPrintOutput(text)

	require.False(t, ok)
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

func TestExtractRequestInterceptTextAnthropicIgnoresSystem(t *testing.T) {
	body := []byte(`{
		"messages": [
			{
				"content": [
					{"cache_control":{"type":"ephemeral"},"text":"hi","type":"text"}
				],
				"role": "user"
			}
		],
		"model": "claude-sonnet-4-6",
		"stream": true,
		"system": [
			{"cache_control":{"type":"ephemeral"},"text":"You are Claude Code, Anthropic's official CLI for Claude.","type":"text"}
		]
	}`)

	got := ExtractRequestInterceptText(RequestInterceptProtocolAnthropic, body)

	require.Equal(t, "hi", got)
}

func TestExtractRequestInterceptTextJoinsUserMessageBlocks(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"text","text":"how are you"}]}]}`)

	got := ExtractRequestInterceptText(RequestInterceptProtocolOpenAIChat, body)

	require.Equal(t, "hi\nhow are you", got)
}

func TestExtractRequestInterceptTextAnthropicStringContent(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"What is the output of this Python code?\n\nprint(\"RP_ANSWER=\" + str(81 + 50))\n\nReply with ONLY the output."}],"model":"claude-haiku-4-5-20251001","stream":false}`)

	got := ExtractRequestInterceptText(RequestInterceptProtocolAnthropic, body)

	require.Contains(t, got, `print("RP_ANSWER=" + str(81 + 50))`)
}
