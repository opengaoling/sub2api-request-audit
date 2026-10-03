package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	PricingSyncSourceModelsDev = "models.dev"
	PricingSyncSourceOfficial  = "official"
	PricingSyncSourceCustom    = "custom"

	DefaultModelsDevURL = "https://models.dev/api.json"
)

// PricingSyncSource 同步数据源信息
type PricingSyncSource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Format      string `json:"format"` // "models.dev", "litellm", "auto"
	Description string `json:"description"`
}

// PricingSyncDiffItem 差异项
type PricingSyncDiffItem struct {
	Model                  string  `json:"model"`
	Platform               string  `json:"platform"`
	Status                 string  `json:"status"` // "added", "updated", "unchanged"
	CurrentInputPrice      float64 `json:"current_input_price"`
	UpstreamInputPrice     float64 `json:"upstream_input_price"`
	CurrentOutputPrice     float64 `json:"current_output_price"`
	UpstreamOutputPrice    float64 `json:"upstream_output_price"`
	CurrentCacheReadPrice  float64 `json:"current_cache_read_price"`
	UpstreamCacheReadPrice float64 `json:"upstream_cache_read_price"`
}

// PricingSyncPreviewResult 预览结果
type PricingSyncPreviewResult struct {
	Source         string                `json:"source"`
	SourceURL      string                `json:"source_url"`
	TotalUpstream  int                   `json:"total_upstream"`
	AddedCount     int                   `json:"added_count"`
	UpdatedCount   int                   `json:"updated_count"`
	UnchangedCount int                   `json:"unchanged_count"`
	Items          []PricingSyncDiffItem `json:"items"`
}

// PricingSyncApplyResult 同步应用结果
type PricingSyncApplyResult struct {
	SyncedCount int    `json:"synced_count"`
	Message     string `json:"message"`
}

// ModelsDevProvider models.dev 数据结构
type ModelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	Models map[string]ModelsDevModel `json:"models"`
}

type ModelsDevModel struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Cost   ModelsDevCost   `json:"cost"`
	Limit  ModelsDevLimit  `json:"limit"`
}

type ModelsDevCost struct {
	Input     *float64 `json:"input"`
	Output    *float64 `json:"output"`
	CacheRead *float64 `json:"cache_read"`
}

type ModelsDevLimit struct {
	Context *int `json:"context"`
	Input   *int `json:"input"`
	Output  *int `json:"output"`
}

// GetSyncSources 获取可用定价同步源
func (s *PricingService) GetSyncSources() []PricingSyncSource {
	officialURL := s.cfg.Pricing.RemoteURL
	if strings.TrimSpace(officialURL) == "" {
		officialURL = "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json"
	}

	return []PricingSyncSource{
		{
			ID:          PricingSyncSourceModelsDev,
			Name:        "models.dev 价格预设",
			URL:         DefaultModelsDevURL,
			Format:      "models.dev",
			Description: "基于 models.dev 官方标准接口，涵盖 OpenAI、Anthropic、Google Gemini、DeepSeek 等主流大模型最新价格",
		},
		{
			ID:          PricingSyncSourceOfficial,
			Name:        "官方镜像源 (LiteLLM 完整格式)",
			URL:         officialURL,
			Format:      "litellm",
			Description: "Sub2API / LiteLLM 官方镜像源，包含完整的模型能力、区间配置和上下文参数",
		},
		{
			ID:          PricingSyncSourceCustom,
			Name:        "自定义 URL",
			URL:         "",
			Format:      "auto",
			Description: "支持自定义的上游定价 JSON 地址（自动兼容 models.dev 与 LiteLLM 格式）",
		},
	}
}

// resolveSyncURL 根据入参解析同步目标 URL
func (s *PricingService) resolveSyncURL(sourceID, customURL string) (string, error) {
	switch sourceID {
	case PricingSyncSourceModelsDev:
		return DefaultModelsDevURL, nil
	case PricingSyncSourceOfficial:
		officialURL := s.cfg.Pricing.RemoteURL
		if strings.TrimSpace(officialURL) == "" {
			officialURL = "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json"
		}
		return officialURL, nil
	case PricingSyncSourceCustom:
		trimmed := strings.TrimSpace(customURL)
		if trimmed == "" {
			return "", fmt.Errorf("自定义 URL 不能为空")
		}
		return trimmed, nil
	default:
		if strings.HasPrefix(sourceID, "http://") || strings.HasPrefix(sourceID, "https://") {
			return sourceID, nil
		}
		if trimmed := strings.TrimSpace(customURL); trimmed != "" {
			return trimmed, nil
		}
		return DefaultModelsDevURL, nil
	}
}

// fetchAndParseUpstreamPricing 获取并解析上游定价数据
func (s *PricingService) fetchAndParseUpstreamPricing(ctx context.Context, targetURL string) (map[string]*LiteLLMModelPricing, error) {
	if s.remoteClient == nil {
		return nil, fmt.Errorf("remote pricing client not configured")
	}

	validURL, err := s.validatePricingURL(targetURL)
	if err != nil {
		return nil, fmt.Errorf("validate URL: %w", err)
	}

	body, err := s.remoteClient.FetchPricingJSON(ctx, validURL)
	if err != nil {
		return nil, fmt.Errorf("fetch remote pricing: %w", err)
	}

	// 智能识别格式：判断是否为 models.dev 格式
	if isModelsDevFormat(body) {
		return parseModelsDevPricing(body)
	}

	// 否则作为 LiteLLM 格式解析
	return s.parsePricingData(body)
}

func isModelsDevFormat(body []byte) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	for _, key := range []string{"openai", "anthropic", "google", "deepseek"} {
		if val, ok := raw[key]; ok {
			var probe struct {
				Models map[string]json.RawMessage `json:"models"`
			}
			if err := json.Unmarshal(val, &probe); err == nil && len(probe.Models) > 0 {
				return true
			}
		}
	}
	return false
}

func parseModelsDevPricing(body []byte) (map[string]*LiteLLMModelPricing, error) {
	var providers map[string]ModelsDevProvider
	if err := json.Unmarshal(body, &providers); err != nil {
		return nil, fmt.Errorf("parse models.dev json: %w", err)
	}

	result := make(map[string]*LiteLLMModelPricing)
	for providerID, prov := range providers {
		mappedProvider := mapModelsDevProvider(providerID)
		for modelID, m := range prov.Models {
			cost := m.Cost
			if cost.Input == nil && cost.Output == nil {
				continue
			}

			pricing := &LiteLLMModelPricing{
				LiteLLMProvider:       mappedProvider,
				Mode:                  "chat",
				SupportsPromptCaching: cost.CacheRead != nil && *cost.CacheRead > 0,
			}

			if cost.Input != nil && *cost.Input >= 0 {
				pricing.InputCostPerToken = *cost.Input / 1e6
				// OpenAI / Anthropic 默认缓存写入单价通常为输入价格的 1.25 倍
				if mappedProvider == "openai" || mappedProvider == "anthropic" {
					pricing.CacheCreationInputTokenCost = pricing.InputCostPerToken * 1.25
				}
			}
			if cost.Output != nil && *cost.Output >= 0 {
				pricing.OutputCostPerToken = *cost.Output / 1e6
			}
			if cost.CacheRead != nil && *cost.CacheRead >= 0 {
				pricing.CacheReadInputTokenCost = *cost.CacheRead / 1e6
			}

			result[modelID] = pricing
		}
	}

	return result, nil
}

func mapModelsDevProvider(providerID string) string {
	lower := strings.ToLower(strings.TrimSpace(providerID))
	switch lower {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "google", "gemini":
		return "google"
	case "deepseek":
		return "deepseek"
	default:
		return lower
	}
}

func isPriceDiff(a, b float64) bool {
	return math.Abs(a-b) > 1e-9
}

// PreviewSyncPricing 预览同步差异
func (s *PricingService) PreviewSyncPricing(ctx context.Context, sourceID, customURL, platform string) (*PricingSyncPreviewResult, error) {
	targetURL, err := s.resolveSyncURL(sourceID, customURL)
	if err != nil {
		return nil, err
	}

	upstreamPricing, err := s.fetchAndParseUpstreamPricing(ctx, targetURL)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	currentData := make(map[string]*LiteLLMModelPricing, len(s.pricingData))
	for k, v := range s.pricingData {
		currentData[k] = v
	}
	s.mu.RUnlock()

	normPlatform := strings.ToLower(strings.TrimSpace(platform))
	filterProvider := platformToLiteLLMProvider[normPlatform]
	if filterProvider == "" && normPlatform != "" && normPlatform != "all" {
		filterProvider = normPlatform
	}

	var items []PricingSyncDiffItem
	added := 0
	updated := 0
	unchanged := 0

	// 收集上游匹配模型并排序
	var modelNames []string
	for name, up := range upstreamPricing {
		if filterProvider != "" && up.LiteLLMProvider != "" && !strings.EqualFold(up.LiteLLMProvider, filterProvider) {
			continue
		}
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	for _, name := range modelNames {
		up := upstreamPricing[name]
		cur, exists := currentData[name]

		item := PricingSyncDiffItem{
			Model:                  name,
			Platform:               up.LiteLLMProvider,
			UpstreamInputPrice:     up.InputCostPerToken,
			UpstreamOutputPrice:    up.OutputCostPerToken,
			UpstreamCacheReadPrice: up.CacheReadInputTokenCost,
		}

		if !exists || cur == nil {
			item.Status = "added"
			added++
		} else {
			item.CurrentInputPrice = cur.InputCostPerToken
			item.CurrentOutputPrice = cur.OutputCostPerToken
			item.CurrentCacheReadPrice = cur.CacheReadInputTokenCost

			if isPriceDiff(cur.InputCostPerToken, up.InputCostPerToken) ||
				isPriceDiff(cur.OutputCostPerToken, up.OutputCostPerToken) ||
				isPriceDiff(cur.CacheReadInputTokenCost, up.CacheReadInputTokenCost) {
				item.Status = "updated"
				updated++
			} else {
				item.Status = "unchanged"
				unchanged++
			}
		}
		items = append(items, item)
	}

	return &PricingSyncPreviewResult{
		Source:         sourceID,
		SourceURL:      targetURL,
		TotalUpstream:  len(upstreamPricing),
		AddedCount:     added,
		UpdatedCount:   updated,
		UnchangedCount: unchanged,
		Items:          items,
	}, nil
}

// ApplySyncPricing 执行同步并生效
func (s *PricingService) ApplySyncPricing(ctx context.Context, sourceID, customURL, platform string, selectedModels []string) (*PricingSyncApplyResult, error) {
	targetURL, err := s.resolveSyncURL(sourceID, customURL)
	if err != nil {
		return nil, err
	}

	upstreamPricing, err := s.fetchAndParseUpstreamPricing(ctx, targetURL)
	if err != nil {
		return nil, err
	}

	normPlatform := strings.ToLower(strings.TrimSpace(platform))
	filterProvider := platformToLiteLLMProvider[normPlatform]
	if filterProvider == "" && normPlatform != "" && normPlatform != "all" {
		filterProvider = normPlatform
	}

	selectedMap := make(map[string]bool, len(selectedModels))
	for _, m := range selectedModels {
		selectedMap[strings.TrimSpace(m)] = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	syncedCount := 0
	for name, up := range upstreamPricing {
		if filterProvider != "" && up.LiteLLMProvider != "" && !strings.EqualFold(up.LiteLLMProvider, filterProvider) {
			continue
		}
		if len(selectedMap) > 0 && !selectedMap[name] {
			continue
		}

		// 合并入系统价格库
		if existing, ok := s.pricingData[name]; ok && existing != nil {
			// 保留已有能力标记，仅更新价格字段
			existing.InputCostPerToken = up.InputCostPerToken
			existing.OutputCostPerToken = up.OutputCostPerToken
			if up.CacheReadInputTokenCost > 0 {
				existing.CacheReadInputTokenCost = up.CacheReadInputTokenCost
				existing.SupportsPromptCaching = true
			}
			if up.CacheCreationInputTokenCost > 0 {
				existing.CacheCreationInputTokenCost = up.CacheCreationInputTokenCost
			}
			if up.LiteLLMProvider != "" {
				existing.LiteLLMProvider = up.LiteLLMProvider
			}
		} else {
			s.pricingData[name] = up
		}
		syncedCount++
	}

	s.lastUpdated = time.Now()

	// 持久化到本地文件
	if marshaled, err := json.MarshalIndent(s.pricingData, "", "  "); err == nil {
		pricingFile := s.getPricingFilePath()
		if err := os.WriteFile(pricingFile, marshaled, 0644); err != nil {
			logger.With(zap.String("component", "service.pricing")).
				Warn("Failed to save synced pricing to local file", zap.Error(err))
		}
	}

	logger.With(zap.String("component", "service.pricing")).
		Info(fmt.Sprintf("[Pricing] Successfully synced %d models from %s", syncedCount, targetURL))

	return &PricingSyncApplyResult{
		SyncedCount: syncedCount,
		Message:     fmt.Sprintf("已成功同步 %d 个模型的价格数据", syncedCount),
	}, nil
}
