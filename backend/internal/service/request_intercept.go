package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

type RequestInterceptProtocol string

const (
	RequestInterceptProtocolOpenAIChat      RequestInterceptProtocol = "openai_chat"
	RequestInterceptProtocolAnthropic       RequestInterceptProtocol = "anthropic_messages"
	RequestInterceptProtocolOpenAIResponses RequestInterceptProtocol = "openai_responses"
)

type RequestInterceptResult struct {
	Content string
	Reason  string
}

type RequestInterceptRule struct {
	MatchContent    string `json:"match_content"`
	ResponseContent string `json:"response_content"`
}

var arithmeticQARe = regexp.MustCompile(`(?is)Q:\s*([-+]?\d+(?:\.\d+)?)\s*([+\-*/xX×÷])\s*([-+]?\d+(?:\.\d+)?)\s*=\s*\??\s*(?:\r?\n|\s)*A:\s*$`)

func (s *SettingService) EvaluateRequestIntercept(ctx context.Context, protocol RequestInterceptProtocol, body []byte) (*RequestInterceptResult, error) {
	if s == nil {
		return nil, nil
	}
	settings, err := s.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.RequestInterceptEnabled {
		return nil, nil
	}

	text := ExtractRequestInterceptText(protocol, body)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	if answer, ok := EvaluateArithmeticFewShot(text); ok {
		return &RequestInterceptResult{Content: answer, Reason: "arithmetic"}, nil
	}

	if response, ok := requestInterceptExactRuleMatched(settings.RequestInterceptRules, text); ok {
		return &RequestInterceptResult{Content: response, Reason: "exact"}, nil
	}
	return nil, nil
}

func NormalizeRequestInterceptRules(rules []RequestInterceptRule) []RequestInterceptRule {
	normalized := make([]RequestInterceptRule, 0, len(rules))
	for _, rule := range rules {
		match := strings.TrimSpace(rule.MatchContent)
		if match == "" {
			continue
		}
		normalized = append(normalized, RequestInterceptRule{
			MatchContent:    match,
			ResponseContent: rule.ResponseContent,
		})
	}
	return normalized
}

func ParseRequestInterceptRules(raw string) []RequestInterceptRule {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var rules []RequestInterceptRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil
	}
	return NormalizeRequestInterceptRules(rules)
}

func ExtractRequestInterceptText(protocol RequestInterceptProtocol, body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	var parts []string
	addString := func(value gjson.Result) {
		if value.Type == gjson.String {
			if text := strings.TrimSpace(value.String()); text != "" {
				parts = append(parts, text)
			}
		}
	}
	addContent := func(value gjson.Result) {
		switch value.Type {
		case gjson.String:
			addString(value)
		case gjson.JSON:
			if value.IsArray() {
				value.ForEach(func(_, block gjson.Result) bool {
					addString(block.Get("text"))
					addString(block.Get("input_text"))
					addString(block.Get("output_text"))
					return true
				})
			}
		}
	}

	switch protocol {
	case RequestInterceptProtocolOpenAIChat:
		gjson.GetBytes(body, "messages").ForEach(func(_, message gjson.Result) bool {
			addContent(message.Get("content"))
			return true
		})
	case RequestInterceptProtocolAnthropic:
		addContent(gjson.GetBytes(body, "system"))
		gjson.GetBytes(body, "messages").ForEach(func(_, message gjson.Result) bool {
			addContent(message.Get("content"))
			return true
		})
	case RequestInterceptProtocolOpenAIResponses:
		addString(gjson.GetBytes(body, "instructions"))
		input := gjson.GetBytes(body, "input")
		if input.Type == gjson.String {
			addString(input)
		} else if input.IsArray() {
			input.ForEach(func(_, item gjson.Result) bool {
				addContent(item.Get("content"))
				addString(item.Get("text"))
				return true
			})
		}
	default:
		gjson.ParseBytes(body).ForEach(func(_, value gjson.Result) bool {
			addContent(value)
			return true
		})
	}
	return strings.Join(parts, "\n")
}

func EvaluateArithmeticFewShot(text string) (string, bool) {
	matches := arithmeticQARe.FindAllStringSubmatch(strings.TrimSpace(text), -1)
	if len(matches) == 0 {
		return "", false
	}
	match := matches[len(matches)-1]
	left, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return "", false
	}
	right, err := strconv.ParseFloat(match[3], 64)
	if err != nil {
		return "", false
	}

	var result float64
	switch match[2] {
	case "+":
		result = left + right
	case "-":
		result = left - right
	case "*", "x", "X", "×":
		result = left * right
	case "/", "÷":
		if right == 0 {
			return "", false
		}
		result = left / right
	default:
		return "", false
	}
	return formatArithmeticResult(result), true
}

func formatArithmeticResult(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.10f", value), "0"), ".")
}

func requestInterceptExactRuleMatched(rules []RequestInterceptRule, text string) (string, bool) {
	normalizedText := strings.TrimSpace(text)
	for _, rule := range NormalizeRequestInterceptRules(rules) {
		if normalizedText == rule.MatchContent {
			return rule.ResponseContent, true
		}
	}
	return "", false
}
