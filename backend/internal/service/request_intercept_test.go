package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type requestInterceptSettingRepoStub struct {
	values map[string]string
}

func (r *requestInterceptSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value, UpdatedAt: time.Now()}, nil
}

func (r *requestInterceptSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *requestInterceptSettingRepoStub) Set(ctx context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *requestInterceptSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *requestInterceptSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *requestInterceptSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *requestInterceptSettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestEvaluateArithmeticFewShot_UserExample(t *testing.T) {
	text := "Calculate and respond with ONLY the number, nothing else.\n\nQ: 3 + 5 = ?\nA: 8\n\nQ: 12 - 7 = ?\nA: 5\n\nQ: 40 + 39 = ?\nA:"

	got, ok := EvaluateArithmeticFewShot(text)

	require.True(t, ok)
	require.Equal(t, "79", got)
}

func TestEvaluateSingleArithmeticPrompt(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "subtract",
			text: "39 - 5 = ?\nReply with only the digit.",
			want: "34",
		},
		{
			name: "multiply",
			text: "7 * 6 = ?\nReply with only the number.",
			want: "42",
		},
		{
			name: "divide",
			text: "9 / 2 = ?\nReply with only the answer.",
			want: "4.5",
		},
	}

	for _, tt := range tests {
		got, ok := EvaluateSingleArithmeticPrompt(tt.text)
		require.True(t, ok, tt.name)
		require.Equal(t, tt.want, got, tt.name)
	}
}

func TestEvaluateSingleArithmeticPromptRequiresOnlyAnswerInstruction(t *testing.T) {
	_, ok := EvaluateSingleArithmeticPrompt("39 - 5 = ?")

	require.False(t, ok)
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

func TestAppendRequestInterceptCombinedCandidateKeepsIndividualMessages(t *testing.T) {
	got := appendRequestInterceptCombinedCandidate([]string{"context", "hi"})

	require.Equal(t, []string{"context", "hi", "context\nhi"}, got)

	response, ok := requestInterceptExactRuleMatched([]RequestInterceptRule{{MatchContent: "hi", ResponseContent: "hello"}}, got[1])
	require.True(t, ok)
	require.Equal(t, "hello", response)
}

func TestRequestInterceptExactRuleMatchedJSONInstruction(t *testing.T) {
	text := `{"family":"","model":""}`

	response, ok := requestInterceptExactRuleMatched([]RequestInterceptRule{
		{MatchContent: text, ResponseContent: `{"family":"gpt","model":"gpt-5-codex"}`},
	}, text)

	require.True(t, ok)
	require.Equal(t, `{"family":"gpt","model":"gpt-5-codex"}`, response)
}

func TestEvaluateRequestInterceptBuiltInMathRule(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	groupID := int64(123)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
	}))
	body := []byte(`{"messages":[{"role":"user","content":"39 - 5 = ?\nReply with only the digit."}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIChat, &groupID, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "builtin_arithmetic", result.Reason)
	require.Equal(t, "34", result.Content)
}

func TestEvaluateRequestInterceptUsesSavedRulesImmediately(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	groupID := int64(123)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: `{"family":"","model":""}`, ResponseContent: `{"family":"gpt","model":"gpt-5-codex"}`},
		},
	}))
	body := []byte(`{"messages":[{"role":"user","content":"context"},{"role":"user","content":"{\"family\":\"\",\"model\":\"\"}"}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIChat, &groupID, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "exact", result.Reason)
	require.Equal(t, `{"family":"gpt","model":"gpt-5-codex"}`, result.Content)
}

func TestEvaluateRequestInterceptIgnoresHistoricalOpenAIChatMatches(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	groupID := int64(123)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: "hi", ResponseContent: "Hello! How can I help you today?"},
		},
	}))
	body := []byte(`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"Hello! How can I help you today?"},{"role":"user","content":"Tell me a deployment plan."}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIChat, &groupID, body)

	require.NoError(t, err)
	require.Nil(t, result)
}

func TestEvaluateRequestInterceptIgnoresHistoricalAnthropicMatches(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	groupID := int64(123)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: "你好", ResponseContent: "Hello! How can I help you today?"},
		},
	}))
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"你好"}]},{"role":"assistant","content":[{"type":"text","text":"Hello! How can I help you today?"}]},{"role":"user","content":[{"type":"text","text":"检查这个错误日志"}]}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolAnthropic, &groupID, body)

	require.NoError(t, err)
	require.Nil(t, result)
}

func TestEvaluateRequestInterceptIgnoresHistoricalOpenAIResponsesMatches(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	groupID := int64(123)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: "hi", ResponseContent: "Hello! How can I help you today?"},
		},
	}))
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]},{"role":"assistant","content":[{"type":"output_text","text":"Hello! How can I help you today?"}]},{"role":"user","content":[{"type":"input_text","text":"Summarize this request."}]}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIResponses, &groupID, body)

	require.NoError(t, err)
	require.Nil(t, result)
}

func TestEvaluateRequestInterceptOpenAIResponsesUsesSavedRules(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	groupID := int64(123)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: "local-only", ResponseContent: "intercepted"},
		},
	}))
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":[{"type":"input_text","text":"local-only"}]}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIResponses, &groupID, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "exact", result.Reason)
	require.Equal(t, "intercepted", result.Content)
}

func TestEvaluateRequestInterceptOnlyAppliesToConfiguredGroup(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	targetGroupID := int64(123)
	otherGroupID := int64(456)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{targetGroupID},
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: "hi", ResponseContent: "hello"},
		},
	}))
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIChat, &otherGroupID, body)
	require.NoError(t, err)
	require.Nil(t, result)

	result, err = svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIChat, nil, body)
	require.NoError(t, err)
	require.Nil(t, result)

	result, err = svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIChat, &targetGroupID, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "hello", result.Content)
}

func TestEvaluateRequestInterceptAppliesToAnyConfiguredGroup(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	firstGroupID := int64(123)
	secondGroupID := int64(456)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{firstGroupID, secondGroupID},
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: "hi", ResponseContent: "hello"},
		},
	}))
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)

	result, err := svc.EvaluateRequestIntercept(ctx, RequestInterceptProtocolOpenAIChat, &secondGroupID, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "hello", result.Content)
}

func TestGetAllSettingsMigratesLegacyRequestInterceptGroupIDToScope(t *testing.T) {
	ctx := context.Background()
	repo := &requestInterceptSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{
		RequestInterceptEnabled: true,
		RequestInterceptGroupID: 123,
		RequestInterceptRules: []RequestInterceptRule{
			{MatchContent: "hi", ResponseContent: "hello"},
		},
	}))
	require.NoError(t, repo.Set(ctx, SettingKeyRequestInterceptGroupID, "123"))
	require.NoError(t, repo.Set(ctx, SettingKeyRequestInterceptGroupScope, "[]"))

	settings, err := svc.GetAllSettings(ctx)

	require.NoError(t, err)
	require.Equal(t, int64(123), settings.RequestInterceptGroupID)
	require.Equal(t, []int64{123}, settings.RequestInterceptGroupScope)
}

func TestExtractRequestInterceptTextOpenAIChat(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"x"}}]}]}`)

	got := ExtractRequestInterceptText(RequestInterceptProtocolOpenAIChat, body)

	require.Equal(t, "hello", got)
}

func TestExtractRequestInterceptTextOpenAIChatUsesLatestUserMessage(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"latest"}]}`)

	got := ExtractRequestInterceptText(RequestInterceptProtocolOpenAIChat, body)

	require.Equal(t, "latest", got)
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
