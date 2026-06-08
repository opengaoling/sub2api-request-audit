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

var (
	arithmeticQARe      = regexp.MustCompile(`(?is)Q:\s*([-+]?\d+(?:\.\d+)?)\s*([+\-*/xX×÷])\s*([-+]?\d+(?:\.\d+)?)\s*=\s*\??\s*(?:\r?\n|\s)*A:\s*$`)
	pythonPrintOutputRe = regexp.MustCompile(`(?is)print\s*\(\s*((?:"(?:\\.|[^"\\])*")|(?:'(?:\\.|[^'\\])*'))\s*\+\s*str\s*\(\s*\(?\s*([-+]?\d+(?:\.\d+)?)\s*([+\-*/])\s*([-+]?\d+(?:\.\d+)?)\s*\)?\s*\)\s*\)`)
)

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

	candidates := ExtractRequestInterceptTextCandidates(protocol, body)
	candidates = appendRequestInterceptCombinedCandidate(candidates)
	if len(candidates) == 0 {
		return nil, nil
	}

	for _, text := range candidates {
		if answer, ok := EvaluateArithmeticFewShot(text); ok {
			return &RequestInterceptResult{Content: answer, Reason: "arithmetic"}, nil
		}

		if answer, ok := EvaluatePythonPrintOutput(text); ok {
			return &RequestInterceptResult{Content: answer, Reason: "python_print_output"}, nil
		}

		if response, ok := requestInterceptExactRuleMatched(settings.RequestInterceptRules, text); ok {
			return &RequestInterceptResult{Content: response, Reason: "exact"}, nil
		}
	}
	return nil, nil
}

func appendRequestInterceptCombinedCandidate(candidates []string) []string {
	normalized := make([]string, 0, len(candidates)+1)
	for _, candidate := range candidates {
		if text := strings.TrimSpace(candidate); text != "" {
			normalized = append(normalized, text)
		}
	}
	if len(normalized) <= 1 {
		return normalized
	}
	combined := strings.Join(normalized, "\n")
	for _, candidate := range normalized {
		if candidate == combined {
			return normalized
		}
	}
	return append(normalized, combined)
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
	return strings.Join(ExtractRequestInterceptTextCandidates(protocol, body), "\n")
}

func ExtractRequestInterceptTextCandidates(protocol RequestInterceptProtocol, body []byte) []string {
	if !gjson.ValidBytes(body) {
		return nil
	}
	candidates := make([]string, 0)
	contentParts := func(value gjson.Result) []string {
		var parts []string
		addString := func(value gjson.Result) {
			if value.Type == gjson.String {
				if text := strings.TrimSpace(value.String()); text != "" {
					parts = append(parts, text)
				}
			}
		}
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
		return parts
	}
	addCandidate := func(parts []string) {
		if len(parts) == 0 {
			return
		}
		if text := strings.TrimSpace(strings.Join(parts, "\n")); text != "" {
			candidates = append(candidates, text)
		}
	}
	addStringCandidate := func(value gjson.Result) {
		if value.Type == gjson.String {
			if text := strings.TrimSpace(value.String()); text != "" {
				candidates = append(candidates, text)
			}
		}
	}

	switch protocol {
	case RequestInterceptProtocolOpenAIChat:
		gjson.GetBytes(body, "messages").ForEach(func(_, message gjson.Result) bool {
			role := strings.ToLower(strings.TrimSpace(message.Get("role").String()))
			if role != "" && role != "user" {
				return true
			}
			addCandidate(contentParts(message.Get("content")))
			return true
		})
	case RequestInterceptProtocolAnthropic:
		gjson.GetBytes(body, "messages").ForEach(func(_, message gjson.Result) bool {
			role := strings.ToLower(strings.TrimSpace(message.Get("role").String()))
			if role != "" && role != "user" {
				return true
			}
			addCandidate(contentParts(message.Get("content")))
			return true
		})
	case RequestInterceptProtocolOpenAIResponses:
		input := gjson.GetBytes(body, "input")
		if input.Type == gjson.String {
			addStringCandidate(input)
		} else if input.IsArray() {
			input.ForEach(func(_, item gjson.Result) bool {
				addCandidate(contentParts(item.Get("content")))
				addStringCandidate(item.Get("text"))
				return true
			})
		}
	default:
		gjson.ParseBytes(body).ForEach(func(_, value gjson.Result) bool {
			addCandidate(contentParts(value))
			return true
		})
	}
	return candidates
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

func EvaluatePythonPrintOutput(text string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if !strings.Contains(normalized, "what is the output of this python code") ||
		!strings.Contains(normalized, "reply with only the output") {
		return "", false
	}
	match := pythonPrintOutputRe.FindStringSubmatch(text)
	if len(match) != 5 {
		return "", false
	}
	prefix, err := strconv.Unquote(match[1])
	if err != nil {
		return "", false
	}
	value, ok := evaluateSimpleArithmetic(match[2], match[3], match[4])
	if !ok {
		return "", false
	}
	return prefix + value, true
}

func evaluateSimpleArithmetic(leftRaw, operator, rightRaw string) (string, bool) {
	left, err := strconv.ParseFloat(leftRaw, 64)
	if err != nil {
		return "", false
	}
	right, err := strconv.ParseFloat(rightRaw, 64)
	if err != nil {
		return "", false
	}

	var result float64
	switch operator {
	case "+":
		result = left + right
	case "-":
		result = left - right
	case "*":
		result = left * right
	case "/":
		if right == 0 {
			return "", false
		}
		result = left / right
	default:
		return "", false
	}
	return formatArithmeticResult(result), true
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
