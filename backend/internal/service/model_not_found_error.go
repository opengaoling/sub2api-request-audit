package service

import (
	"net/http"
	"regexp"
	"strings"
)

var upstreamModelNotFoundKeywords = []string{"model not found", "unknown model", "not found"}

func isUpstreamModelNotFoundError(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
	}
	normalized := normalizeModelNotFoundBody(body)
	if normalized == "" || !strings.Contains(normalized, "model") {
		return false
	}
	return containsModelNotFoundKeyword(normalized)
}

func isModelNotFoundError(statusCode int, body []byte) bool {
	return isUpstreamModelNotFoundError(statusCode, body) || statusCode == http.StatusNotFound
}

// openAICodexPlanGatedModelPhrase matches deterministic Codex 400 responses
// when a ChatGPT OAuth account's plan cannot serve the requested model, e.g.
// {"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."}
const openAICodexPlanGatedModelPhrase = "model is not supported when using codex"

var openAICodexPlanGatedModelPattern = regexp.MustCompile(`(?i)(?:the\s+)?['"]([^'"]+)['"]\s+model\s+is\s+not\s+supported\s+when\s+using\s+codex`)

func isOpenAICodexPlanGatedModelError(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	normalized := normalizeModelNotFoundBody(body)
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, openAICodexPlanGatedModelPhrase)
}

func extractOpenAICodexPlanGatedModel(statusCode int, body []byte) (string, bool) {
	if !isOpenAICodexPlanGatedModelError(statusCode, body) {
		return "", false
	}
	matches := openAICodexPlanGatedModelPattern.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return "", false
	}
	model := strings.TrimSpace(matches[1])
	if model == "" {
		return "", false
	}
	return model, true
}

func containsModelNotFoundKeyword(normalizedBody string) bool {
	if normalizedBody == "" {
		return false
	}
	for _, keyword := range upstreamModelNotFoundKeywords {
		if strings.Contains(normalizedBody, keyword) {
			return true
		}
	}
	return false
}

func normalizeModelNotFoundBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	normalized := strings.ToLower(string(body))
	normalized = strings.NewReplacer("_", " ", "-", " ", "\n", " ", "\r", " ", "\t", " ").Replace(normalized)
	return strings.Join(strings.Fields(normalized), " ")
}
