package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

const openAIResponsesItemTypeErrorFragment = "cannot determine type of 'item'"

func isOpenAIResponsesItemTypeCompatibilityError(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if msg == "" {
		msg = strings.ToLower(strings.TrimSpace(string(body)))
	}
	return strings.Contains(msg, openAIResponsesItemTypeErrorFragment)
}

func shouldFallbackOpenAIResponsesItemTypeError(account *Account, statusCode int, body []byte) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
	}
	if mode, ok := account.Extra[openai_compat.ExtraKeyResponsesMode].(string); ok &&
		openai_compat.NormalizeResponsesSupportMode(mode) == openai_compat.ResponsesSupportModeForceResponses {
		return false
	}
	return isOpenAIResponsesItemTypeCompatibilityError(statusCode, body)
}

func (s *OpenAIGatewayService) markOpenAIAPIKeyResponsesUnsupported(ctx context.Context, account *Account, reason string) {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[openai_compat.ExtraKeyResponsesSupported] = false

	logger.LegacyPrintf("service.openai_gateway",
		"[OpenAI] Marked APIKey account Responses unsupported: account_id=%d account=%s reason=%s",
		account.ID,
		account.Name,
		reason,
	)

	if s == nil || s.accountRepo == nil || account.ID <= 0 {
		return
	}
	updateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if err := s.accountRepo.UpdateExtra(updateCtx, account.ID, map[string]any{
		openai_compat.ExtraKeyResponsesSupported: false,
	}); err != nil {
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI] Failed to persist Responses unsupported marker: account_id=%d err=%v",
			account.ID,
			err,
		)
	}
}
