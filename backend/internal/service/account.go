// Package service provides business logic and domain services for the application.
package service

import (
	"encoding/json"
	"errors"
	"hash/fnv"
	"log/slog"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type Account struct {
	ID                      int64
	Name                    string
	Notes                   *string
	Platform                string
	Type                    string
	Credentials             map[string]any
	Extra                   map[string]any
	ProxyID                 *int64
	ProxyFallbackOriginID   *int64
	ProxyFallbackOriginName *string // 仅展示用
	Concurrency             int
	Priority                int
	// RateMultiplier 账号计费倍率（>=0，允许 0 表示该账号计费为 0）。
	// 使用指针用于兼容旧版本调度缓存（Redis）中缺字段的情况：nil 表示按 1.0 处理。
	RateMultiplier     *float64
	LoadFactor         *int // 调度负载因子；nil 表示使用 Concurrency
	Status             string
	ErrorMessage       string
	LastUsedAt         *time.Time
	ExpiresAt          *time.Time
	AutoPauseOnExpired bool
	CreatedAt          time.Time
	UpdatedAt          time.Time

	Schedulable bool

	RateLimitedAt    *time.Time
	RateLimitResetAt *time.Time
	OverloadUntil    *time.Time

	TempUnschedulableUntil  *time.Time
	TempUnschedulableReason string

	SessionWindowStart  *time.Time
	SessionWindowEnd    *time.Time
	SessionWindowStatus string

	Proxy         *Proxy
	AccountGroups []AccountGroup
	GroupIDs      []int64
	Groups        []*Group

	// model_mapping 热路径缓存（非持久化字段）
	modelMappingCache               map[string]string
	modelMappingCacheReady          bool
	modelMappingCacheCredentialsPtr uintptr
	modelMappingCacheRawPtr         uintptr
	modelMappingCacheRawLen         int
	modelMappingCacheRawSig         uint64
}

type OpenAIEndpointCapability string

const (
	OpenAIEndpointCapabilityChatCompletions OpenAIEndpointCapability = "chat_completions"
	OpenAIEndpointCapabilityEmbeddings      OpenAIEndpointCapability = "embeddings"
)

const openAIEndpointCapabilitiesCredentialKey = "openai_capabilities"

const (
	TempUnschedulableMatchTypeStatusCode = "status_code"
	TempUnschedulableMatchTypeKeyword    = "keyword"
	TempUnschedulableMatchTypeCombined   = "combined"
)

type TempUnschedulableRule struct {
	MatchType       string   `json:"match_type,omitempty"`
	ErrorCode       int      `json:"error_code"`
	Keywords        []string `json:"keywords"`
	DurationMinutes int      `json:"duration_minutes"`
	Description     string   `json:"description"`
}

func (a *Account) IsActive() bool {
	return a.Status == StatusActive
}

// BillingRateMultiplier 返回账号计费倍率。
// - nil 表示未配置/旧缓存缺字段，按 1.0 处理
// - 允许 0，表示该账号计费为 0
// - 负数属于非法数据，出于安全考虑按 1.0 处理
func (a *Account) BillingRateMultiplier() float64 {
	if a == nil || a.RateMultiplier == nil {
		return 1.0
	}
	if *a.RateMultiplier < 0 {
		return 1.0
	}
	return *a.RateMultiplier
}

func (a *Account) EffectiveLoadFactor() int {
	if a == nil {
		return 1
	}
	if a.LoadFactor != nil && *a.LoadFactor > 0 {
		return *a.LoadFactor
	}
	if a.Concurrency > 0 {
		return a.Concurrency
	}
	return 1
}

func (a *Account) IsSchedulable() bool {
	if !a.IsActive() || !a.Schedulable {
		return false
	}
	now := time.Now()
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return false
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return false
	}
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return false
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return false
	}
	if a.IsAPIKeyOrBedrock() && a.IsQuotaExceeded() {
		return false
	}
	return true
}

func (a *Account) IsRateLimited() bool {
	if a.RateLimitResetAt == nil {
		return false
	}
	return time.Now().Before(*a.RateLimitResetAt)
}

func (a *Account) IsOverloaded() bool {
	if a.OverloadUntil == nil {
		return false
	}
	return time.Now().Before(*a.OverloadUntil)
}

func (a *Account) IsOAuth() bool {
	return a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken
}

// IsPrivacySet 检查账号的 privacy 是否已成功设置。
// OpenAI: privacy_mode == "training_off"
// Antigravity: privacy_mode == "privacy_set"
// 其他平台: 无 privacy 概念，始终返回 true
func (a *Account) IsPrivacySet() bool {
	switch a.Platform {
	case PlatformOpenAI:
		return a.getExtraString("privacy_mode") == PrivacyModeTrainingOff
	case PlatformAntigravity:
		return a.getExtraString("privacy_mode") == AntigravityPrivacySet
	default:
		return true
	}
}

func (a *Account) IsGemini() bool {
	return a.Platform == PlatformGemini
}

func (a *Account) GeminiOAuthType() string {
	if a.Platform != PlatformGemini || a.Type != AccountTypeOAuth {
		return ""
	}
	oauthType := strings.TrimSpace(a.GetCredential("oauth_type"))
	if oauthType == "" && strings.TrimSpace(a.GetCredential("project_id")) != "" {
		return "code_assist"
	}
	return oauthType
}

func (a *Account) GeminiTierID() string {
	tierID := strings.TrimSpace(a.GetCredential("tier_id"))
	return tierID
}

func (a *Account) IsGeminiCodeAssist() bool {
	if a.Platform != PlatformGemini || a.Type != AccountTypeOAuth {
		return false
	}
	oauthType := a.GeminiOAuthType()
	if oauthType == "" {
		return strings.TrimSpace(a.GetCredential("project_id")) != ""
	}
	return oauthType == "code_assist"
}

func (a *Account) CanGetUsage() bool {
	return a.Type == AccountTypeOAuth
}

func (a *Account) GetCredential(key string) string {
	if a.Credentials == nil {
		return ""
	}
	v, ok := a.Credentials[key]
	if !ok || v == nil {
		return ""
	}

	// 支持多种类型（兼容历史数据中 expires_at 等字段可能是数字或字符串）
	switch val := v.(type) {
	case string:
		return val
	case json.Number:
		// GORM datatypes.JSONMap 使用 UseNumber() 解析，数字类型为 json.Number
		return val.String()
	case float64:
		// JSON 解析后数字默认为 float64
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	default:
		return ""
	}
}

// GetCredentialAsTime 解析凭证中的时间戳字段，支持多种格式
// 兼容以下格式：
//   - RFC3339 字符串: "2025-01-01T00:00:00Z"
//   - Unix 时间戳字符串: "1735689600"
//   - Unix 时间戳数字: 1735689600 (float64/int64/json.Number)
func (a *Account) GetCredentialAsTime(key string) *time.Time {
	s := a.GetCredential(key)
	if s == "" {
		return nil
	}
	// 尝试 RFC3339 格式
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	// 尝试 Unix 时间戳（纯数字字符串）
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		t := time.Unix(ts, 0)
		return &t
	}
	return nil
}

// GetCredentialAsInt64 解析凭证中的 int64 字段
// 用于读取 _token_version 等内部字段
func (a *Account) GetCredentialAsInt64(key string) int64 {
	if a == nil || a.Credentials == nil {
		return 0
	}
	val, ok := a.Credentials[key]
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return i
		}
	}
	return 0
}

func (a *Account) IsTempUnschedulableEnabled() bool {
	if a.Credentials == nil {
		return false
	}
	raw, ok := a.Credentials["temp_unschedulable_enabled"]
	if !ok || raw == nil {
		return false
	}
	enabled, ok := raw.(bool)
	return ok && enabled
}

func (a *Account) GetTempUnschedulableRules() []TempUnschedulableRule {
	if a.Credentials == nil {
		return nil
	}
	raw, ok := a.Credentials["temp_unschedulable_rules"]
	if !ok || raw == nil {
		return nil
	}

	arr, ok := raw.([]any)
	if !ok {
		return nil
	}

	rules := make([]TempUnschedulableRule, 0, len(arr))
	for _, item := range arr {
		entry, ok := item.(map[string]any)
		if !ok || entry == nil {
			continue
		}

		rule := TempUnschedulableRule{
			ErrorCode:       parseTempUnschedInt(entry["error_code"]),
			Keywords:        parseTempUnschedStrings(entry["keywords"]),
			DurationMinutes: parseTempUnschedInt(entry["duration_minutes"]),
			Description:     parseTempUnschedString(entry["description"]),
		}

		if rule.ErrorCode <= 0 || rule.DurationMinutes <= 0 || len(rule.Keywords) == 0 {
			continue
		}

		rules = append(rules, rule)
	}

	return NormalizeTempUnschedulableRules(rules)
}

func ParseTempUnschedulableRules(raw string) []TempUnschedulableRule {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var rules []TempUnschedulableRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil
	}
	return NormalizeTempUnschedulableRules(rules)
}

func NormalizeTempUnschedulableRules(rules []TempUnschedulableRule) []TempUnschedulableRule {
	normalized := make([]TempUnschedulableRule, 0, len(rules))
	for _, rule := range rules {
		keywords := normalizeTempUnschedKeywords(rule.Keywords)
		if rule.ErrorCode <= 0 || rule.DurationMinutes <= 0 || len(keywords) == 0 {
			continue
		}
		normalized = append(normalized, TempUnschedulableRule{
			ErrorCode:       rule.ErrorCode,
			Keywords:        keywords,
			DurationMinutes: rule.DurationMinutes,
			Description:     strings.TrimSpace(rule.Description),
		})
	}
	return normalized
}

func ParseGlobalTempUnschedulableRules(raw string) []TempUnschedulableRule {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var rules []TempUnschedulableRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil
	}
	return NormalizeGlobalTempUnschedulableRules(rules)
}

func NormalizeGlobalTempUnschedulableRules(rules []TempUnschedulableRule) []TempUnschedulableRule {
	normalized := make([]TempUnschedulableRule, 0, len(rules))
	for _, rule := range rules {
		matchType := normalizeTempUnschedMatchType(rule.MatchType)
		keywords := normalizeTempUnschedKeywords(rule.Keywords)
		if rule.DurationMinutes <= 0 || !isValidGlobalTempUnschedRuleCondition(matchType, rule.ErrorCode, keywords) {
			continue
		}
		normalized = append(normalized, TempUnschedulableRule{
			MatchType:       matchType,
			ErrorCode:       globalTempUnschedErrorCodeForMatchType(matchType, rule.ErrorCode),
			Keywords:        globalTempUnschedKeywordsForMatchType(matchType, keywords),
			DurationMinutes: rule.DurationMinutes,
			Description:     strings.TrimSpace(rule.Description),
		})
	}
	return normalized
}

func normalizeTempUnschedMatchType(matchType string) string {
	switch strings.TrimSpace(matchType) {
	case TempUnschedulableMatchTypeStatusCode:
		return TempUnschedulableMatchTypeStatusCode
	case TempUnschedulableMatchTypeKeyword:
		return TempUnschedulableMatchTypeKeyword
	case TempUnschedulableMatchTypeCombined:
		return TempUnschedulableMatchTypeCombined
	default:
		// 旧配置没有 match_type，按原来的“状态码 + 关键词组合”语义兼容。
		return TempUnschedulableMatchTypeCombined
	}
}

func isValidGlobalTempUnschedRuleCondition(matchType string, errorCode int, keywords []string) bool {
	switch matchType {
	case TempUnschedulableMatchTypeStatusCode:
		return errorCode > 0
	case TempUnschedulableMatchTypeKeyword:
		return len(keywords) > 0
	case TempUnschedulableMatchTypeCombined:
		return errorCode > 0 && len(keywords) > 0
	default:
		return false
	}
}

func globalTempUnschedErrorCodeForMatchType(matchType string, errorCode int) int {
	if matchType == TempUnschedulableMatchTypeKeyword {
		return 0
	}
	return errorCode
}

func globalTempUnschedKeywordsForMatchType(matchType string, keywords []string) []string {
	if matchType == TempUnschedulableMatchTypeStatusCode {
		return nil
	}
	return keywords
}

func normalizeTempUnschedKeywords(keywords []string) []string {
	out := make([]string, 0, len(keywords))
	for _, item := range keywords {
		s := strings.TrimSpace(item)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseTempUnschedString(value any) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func parseTempUnschedStrings(value any) []string {
	if value == nil {
		return nil
	}

	var raw []string
	switch v := value.(type) {
	case []string:
		raw = v
	case []any:
		raw = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	default:
		return nil
	}

	return normalizeTempUnschedKeywords(raw)
}

func normalizeAccountNotes(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseTempUnschedInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
		}
	}
	return 0
}

const (
	// OpenAICompactModeAuto follows compact-probe results when deciding compact eligibility.
	OpenAICompactModeAuto = "auto"
	// OpenAICompactModeForceOn always treats the account as compact-supported.
	OpenAICompactModeForceOn = "force_on"
	// OpenAICompactModeForceOff always treats the account as compact-unsupported.
	OpenAICompactModeForceOff = "force_off"
)

func normalizeOpenAICompactMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case OpenAICompactModeForceOn:
		return OpenAICompactModeForceOn
	case OpenAICompactModeForceOff:
		return OpenAICompactModeForceOff
	default:
		return OpenAICompactModeAuto
	}
}

func stringMappingFromRaw(raw any) map[string]string {
	switch mapping := raw.(type) {
	case map[string]any:
		if len(mapping) == 0 {
			return nil
		}
		result := make(map[string]string, len(mapping))
		for key, value := range mapping {
			if str, ok := value.(string); ok {
				result[key] = str
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	case map[string]string:
		if len(mapping) == 0 {
			return nil
		}
		result := make(map[string]string, len(mapping))
		for key, value := range mapping {
			result[key] = value
		}
		return result
	default:
		return nil
	}
}

func (a *Account) GetModelMapping() map[string]string {
	credentialsPtr := mapPtr(a.Credentials)
	rawMapping, _ := a.Credentials["model_mapping"].(map[string]any)
	rawPtr := mapPtr(rawMapping)
	rawLen := len(rawMapping)
	rawSig := uint64(0)
	rawSigReady := false

	if a.modelMappingCacheReady &&
		a.modelMappingCacheCredentialsPtr == credentialsPtr &&
		a.modelMappingCacheRawPtr == rawPtr &&
		a.modelMappingCacheRawLen == rawLen {
		rawSig = modelMappingSignature(rawMapping)
		rawSigReady = true
		if a.modelMappingCacheRawSig == rawSig {
			return a.modelMappingCache
		}
	}

	mapping := a.resolveModelMapping(rawMapping)
	if !rawSigReady {
		rawSig = modelMappingSignature(rawMapping)
	}

	a.modelMappingCache = mapping
	a.modelMappingCacheReady = true
	a.modelMappingCacheCredentialsPtr = credentialsPtr
	a.modelMappingCacheRawPtr = rawPtr
	a.modelMappingCacheRawLen = rawLen
	a.modelMappingCacheRawSig = rawSig
	return mapping
}

func (a *Account) resolveModelMapping(rawMapping map[string]any) map[string]string {
	if a.Credentials == nil {
		// Antigravity 平台使用默认映射
		if a.Platform == domain.PlatformAntigravity {
			return domain.DefaultAntigravityModelMapping
		}
		// Bedrock 默认映射由 forwardBedrock 统一处理（需配合 region prefix 调整）
		return nil
	}
	if len(rawMapping) == 0 {
		// Antigravity 平台使用默认映射
		if a.Platform == domain.PlatformAntigravity {
			return domain.DefaultAntigravityModelMapping
		}
		return nil
	}

	result := make(map[string]string)
	for k, v := range rawMapping {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	if len(result) > 0 {
		if a.Platform == domain.PlatformAntigravity {
			ensureAntigravityDefaultPassthroughs(result, []string{
				"gemini-3-flash",
				"gemini-3.1-pro-high",
				"gemini-3.1-pro-low",
			})
		}
		return result
	}

	// Antigravity 平台使用默认映射
	if a.Platform == domain.PlatformAntigravity {
		return domain.DefaultAntigravityModelMapping
	}
	return nil
}

func mapPtr(m map[string]any) uintptr {
	if m == nil {
		return 0
	}
	return reflect.ValueOf(m).Pointer()
}

func modelMappingSignature(rawMapping map[string]any) uint64 {
	if len(rawMapping) == 0 {
		return 0
	}
	keys := make([]string, 0, len(rawMapping))
	for k := range rawMapping {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := fnv.New64a()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		if v, ok := rawMapping[k].(string); ok {
			_, _ = h.Write([]byte(v))
		} else {
			_, _ = h.Write([]byte{1})
		}
		_, _ = h.Write([]byte{0xff})
	}
	return h.Sum64()
}

func ensureAntigravityDefaultPassthrough(mapping map[string]string, model string) {
	if mapping == nil || model == "" {
		return
	}
	if _, exists := mapping[model]; exists {
		return
	}
	for pattern := range mapping {
		if matchWildcard(pattern, model) {
			return
		}
	}
	mapping[model] = model
}

func ensureAntigravityDefaultPassthroughs(mapping map[string]string, models []string) {
	for _, model := range models {
		ensureAntigravityDefaultPassthrough(mapping, model)
	}
}

func normalizeRequestedModelForLookup(platform, requestedModel string) string {
	trimmed := strings.TrimSpace(requestedModel)
	if trimmed == "" {
		return ""
	}
	if platform != PlatformGemini && platform != PlatformAntigravity {
		return trimmed
	}
	if trimmed == "gemini-3.1-pro-preview-customtools" {
		return "gemini-3.1-pro-preview"
	}
	return trimmed
}

func mappingSupportsRequestedModel(mapping map[string]string, requestedModel string) bool {
	if requestedModel == "" {
		return false
	}
	if _, exists := mapping[requestedModel]; exists {
		return true
	}
	for pattern := range mapping {
		if matchWildcard(pattern, requestedModel) {
			return true
		}
	}
	return false
}

func resolveRequestedModelInMapping(mapping map[string]string, requestedModel string) (mappedModel string, matched bool) {
	if requestedModel == "" {
		return "", false
	}
	if mappedModel, exists := mapping[requestedModel]; exists {
		return mappedModel, true
	}
	return matchWildcardMappingResult(mapping, requestedModel)
}

// IsModelSupported 检查模型是否在 model_mapping 中（支持通配符）
// 如果未配置 mapping，返回 true（允许所有模型）
func (a *Account) IsModelSupported(requestedModel string) bool {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return true // 无映射 = 允许所有
	}
	if mappingSupportsRequestedModel(mapping, requestedModel) {
		return true
	}
	normalized := normalizeRequestedModelForLookup(a.Platform, requestedModel)
	return normalized != requestedModel && mappingSupportsRequestedModel(mapping, normalized)
}

// GetMappedModel 获取映射后的模型名（支持通配符，最长优先匹配）
// 如果未配置 mapping，返回原始模型名
func (a *Account) GetMappedModel(requestedModel string) string {
	mappedModel, _ := a.ResolveMappedModel(requestedModel)
	return mappedModel
}

// ResolveMappedModel 获取映射后的模型名，并返回是否命中了账号级映射。
// matched=true 表示命中了精确映射或通配符映射，即使映射结果与原模型名相同。
func (a *Account) ResolveMappedModel(requestedModel string) (mappedModel string, matched bool) {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return requestedModel, false
	}
	if mappedModel, matched := resolveRequestedModelInMapping(mapping, requestedModel); matched {
		return mappedModel, true
	}
	normalized := normalizeRequestedModelForLookup(a.Platform, requestedModel)
	if normalized != requestedModel {
		if mappedModel, matched := resolveRequestedModelInMapping(mapping, normalized); matched {
			return mappedModel, true
		}
	}
	return requestedModel, false
}

// GetOpenAICompactMode returns the compact routing mode for an OpenAI account.
// Missing or invalid values fall back to "auto".
func (a *Account) GetOpenAICompactMode() string {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return OpenAICompactModeAuto
	}
	mode, _ := a.Extra["openai_compact_mode"].(string)
	return normalizeOpenAICompactMode(mode)
}

// OpenAICompactSupportKnown reports whether compact capability is known for this
// account and, when known, whether it is supported.
func (a *Account) OpenAICompactSupportKnown() (supported bool, known bool) {
	if a == nil || !a.IsOpenAI() {
		return false, false
	}

	switch a.GetOpenAICompactMode() {
	case OpenAICompactModeForceOn:
		return true, true
	case OpenAICompactModeForceOff:
		return false, true
	}

	if a.Extra == nil {
		return false, false
	}
	supported, ok := a.Extra["openai_compact_supported"].(bool)
	if !ok {
		return false, false
	}
	return supported, true
}

// AllowsOpenAICompact reports whether the account may be considered for compact
// requests. Unknown capability remains allowed to avoid breaking older accounts
// before an explicit probe has been run.
func (a *Account) AllowsOpenAICompact() bool {
	if a == nil || !a.IsOpenAI() {
		return false
	}
	supported, known := a.OpenAICompactSupportKnown()
	if !known {
		return true
	}
	return supported
}

// GetCompactModelMapping returns compact-only model remapping configuration.
// This mapping is intended for /responses/compact only and does not affect
// normal /responses traffic.
func (a *Account) GetCompactModelMapping() map[string]string {
	if a == nil || a.Credentials == nil {
		return nil
	}
	return stringMappingFromRaw(a.Credentials["compact_model_mapping"])
}

// ResolveCompactMappedModel resolves compact-only model remapping and reports
// whether a compact-specific mapping rule matched.
func (a *Account) ResolveCompactMappedModel(requestedModel string) (mappedModel string, matched bool) {
	mapping := a.GetCompactModelMapping()
	if len(mapping) == 0 {
		return requestedModel, false
	}
	if mappedModel, matched := resolveRequestedModelInMapping(mapping, requestedModel); matched {
		return mappedModel, true
	}
	return requestedModel, false
}

func (a *Account) GetBaseURL() string {
	if a.Type != AccountTypeAPIKey {
		return ""
	}
	baseURL := a.GetCredential("base_url")
	if baseURL == "" {
		return "https://api.anthropic.com"
	}
	if a.Platform == PlatformAntigravity {
		return strings.TrimRight(baseURL, "/") + "/antigravity"
	}
	return baseURL
}

// GetGeminiBaseURL 返回 Gemini 兼容端点的 base URL。
// Antigravity 平台的 APIKey 账号自动拼接 /antigravity。
func (a *Account) GetGeminiBaseURL(defaultBaseURL string) string {
	baseURL := strings.TrimSpace(a.GetCredential("base_url"))
	if baseURL == "" {
		return defaultBaseURL
	}
	if a.Platform == PlatformAntigravity && a.Type == AccountTypeAPIKey {
		return strings.TrimRight(baseURL, "/") + "/antigravity"
	}
	return baseURL
}

func (a *Account) GetExtraString(key string) string {
	if a.Extra == nil {
		return ""
	}
	if v, ok := a.Extra[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (a *Account) GetClaudeUserID() string {
	if v := strings.TrimSpace(a.GetExtraString("claude_user_id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(a.GetExtraString("anthropic_user_id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(a.GetCredential("claude_user_id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(a.GetCredential("anthropic_user_id")); v != "" {
		return v
	}
	return ""
}

// matchAntigravityWildcard 通配符匹配（仅支持末尾 *）
// 用于 model_mapping 的通配符匹配
func matchAntigravityWildcard(pattern, str string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(str, prefix)
	}
	return pattern == str
}

// matchWildcard 通用通配符匹配（仅支持末尾 *）
// 复用 Antigravity 的通配符逻辑，供其他平台使用
func matchWildcard(pattern, str string) bool {
	return matchAntigravityWildcard(pattern, str)
}

func matchWildcardMappingResult(mapping map[string]string, requestedModel string) (string, bool) {
	// 收集所有匹配的 pattern，按长度降序排序（最长优先）
	type patternMatch struct {
		pattern string
		target  string
	}
	var matches []patternMatch

	for pattern, target := range mapping {
		if matchWildcard(pattern, requestedModel) {
			matches = append(matches, patternMatch{pattern, target})
		}
	}

	if len(matches) == 0 {
		return requestedModel, false // 无匹配，返回原始模型名
	}

	// 按 pattern 长度降序排序
	sort.Slice(matches, func(i, j int) bool {
		if len(matches[i].pattern) != len(matches[j].pattern) {
			return len(matches[i].pattern) > len(matches[j].pattern)
		}
		return matches[i].pattern < matches[j].pattern
	})

	return matches[0].target, true
}

func (a *Account) IsCustomErrorCodesEnabled() bool {
	if a.Type != AccountTypeAPIKey || a.Credentials == nil {
		return false
	}
	if v, ok := a.Credentials["custom_error_codes_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

// IsPoolMode 检查 API Key 账号是否启用池模式。
// 池模式下，上游错误不标记本地账号状态，而是在同一账号上重试。
func (a *Account) IsPoolMode() bool {
	if !a.IsAPIKeyOrBedrock() || a.Credentials == nil {
		return false
	}
	if v, ok := a.Credentials["pool_mode"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

const (
	defaultPoolModeRetryCount = 3
	maxPoolModeRetryCount     = 10
)

// GetPoolModeRetryCount 返回池模式同账号重试次数。
// 未配置或配置非法时回退为默认值 3；小于 0 按 0 处理；过大则截断到 10。
func (a *Account) GetPoolModeRetryCount() int {
	if a == nil || !a.IsPoolMode() || a.Credentials == nil {
		return defaultPoolModeRetryCount
	}
	raw, ok := a.Credentials["pool_mode_retry_count"]
	if !ok || raw == nil {
		return defaultPoolModeRetryCount
	}
	count := parsePoolModeRetryCount(raw)
	if count < 0 {
		return 0
	}
	if count > maxPoolModeRetryCount {
		return maxPoolModeRetryCount
	}
	return count
}

func parsePoolModeRetryCount(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
		}
	}
	return defaultPoolModeRetryCount
}

// defaultPoolModeRetryableStatusCodes 池模式下默认触发同账号重试的状态码。
// 未在 Account.Credentials 中显式配置 pool_mode_retry_status_codes 时使用。
var defaultPoolModeRetryableStatusCodes = []int{401, 403, 429}

// isPoolModeRetryableStatus 池模式下应触发同账号重试的状态码（默认列表）。
func isPoolModeRetryableStatus(statusCode int) bool {
	for _, c := range defaultPoolModeRetryableStatusCodes {
		if c == statusCode {
			return true
		}
	}
	return false
}

// GetPoolModeRetryStatusCodes 返回账号自定义的池模式同账号重试状态码列表。
//
// 返回值语义：
//   - nil：未配置 → 调用方应回退到默认值 [401, 403, 429]
//   - 长度为 0 的切片：管理员显式置空 → 关闭按状态码触发的同账号重试
//   - 非空切片：去重、过滤为合法 HTTP 状态码（100-599）后的覆盖列表
func (a *Account) GetPoolModeRetryStatusCodes() []int {
	if a == nil || a.Credentials == nil {
		return nil
	}
	raw, ok := a.Credentials["pool_mode_retry_status_codes"]
	if !ok || raw == nil {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	seen := make(map[int]struct{}, len(arr))
	codes := make([]int, 0, len(arr))
	for _, v := range arr {
		var code int
		switch n := v.(type) {
		case float64:
			code = int(n)
		case int:
			code = n
		case int64:
			code = int(n)
		case json.Number:
			i, err := n.Int64()
			if err != nil {
				continue
			}
			code = int(i)
		case string:
			i, err := strconv.Atoi(strings.TrimSpace(n))
			if err != nil {
				continue
			}
			code = i
		default:
			continue
		}
		if code < 100 || code > 599 {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}

// IsPoolModeRetryableStatus 在账号上下文中判断给定状态码是否应触发同账号重试。
// 若账号未配置 pool_mode_retry_status_codes，则回退到默认列表。
func (a *Account) IsPoolModeRetryableStatus(statusCode int) bool {
	codes := a.GetPoolModeRetryStatusCodes()
	if codes == nil {
		return isPoolModeRetryableStatus(statusCode)
	}
	for _, c := range codes {
		if c == statusCode {
			return true
		}
	}
	return false
}

func (a *Account) GetCustomErrorCodes() []int {
	if a.Credentials == nil {
		return nil
	}
	raw, ok := a.Credentials["custom_error_codes"]
	if !ok || raw == nil {
		return nil
	}
	if arr, ok := raw.([]any); ok {
		result := make([]int, 0, len(arr))
		for _, v := range arr {
			if f, ok := v.(float64); ok {
				result = append(result, int(f))
			}
		}
		return result
	}
	return nil
}

func (a *Account) ShouldHandleErrorCode(statusCode int) bool {
	if !a.IsCustomErrorCodesEnabled() {
		return true
	}
	codes := a.GetCustomErrorCodes()
	if len(codes) == 0 {
		return true
	}
	for _, code := range codes {
		if code == statusCode {
			return true
		}
	}
	return false
}

func (a *Account) IsInterceptWarmupEnabled() bool {
	if a.Credentials == nil {
		return false
	}
	if v, ok := a.Credentials["intercept_warmup_requests"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

func (a *Account) IsBedrock() bool {
	return a.Platform == PlatformAnthropic && a.Type == AccountTypeBedrock
}

func (a *Account) IsBedrockAPIKey() bool {
	return a.IsBedrock() && a.GetCredential("auth_mode") == "apikey"
}

// IsAPIKeyOrBedrock 返回账号类型是否支持配额和池模式等特性
func (a *Account) IsAPIKeyOrBedrock() bool {
	return a.Type == AccountTypeAPIKey || a.Type == AccountTypeBedrock
}

func (a *Account) IsOpenAI() bool {
	return a.Platform == PlatformOpenAI
}

func (a *Account) IsAnthropic() bool {
	return a.Platform == PlatformAnthropic
}

func (a *Account) IsOpenAIOAuth() bool {
	return a.IsOpenAI() && a.Type == AccountTypeOAuth
}

func (a *Account) IsOpenAIApiKey() bool {
	return a.IsOpenAI() && a.Type == AccountTypeAPIKey
}

func (a *Account) GetOpenAIBaseURL() string {
	if !a.IsOpenAI() {
		return ""
	}
	if a.Type == AccountTypeAPIKey {
		baseURL := a.GetCredential("base_url")
		if baseURL != "" {
			return baseURL
		}
	}
	return "https://api.openai.com"
}

func (a *Account) GetOpenAIAccessToken() string {
	if !a.IsOpenAI() {
		return ""
	}
	return a.GetCredential("access_token")
}

func (a *Account) GetOpenAIRefreshToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("refresh_token")
}

func (a *Account) GetOpenAIIDToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("id_token")
}

func (a *Account) GetOpenAIApiKey() string {
	if !a.IsOpenAIApiKey() {
		return ""
	}
	return a.GetCredential("api_key")
}

func (a *Account) GetOpenAIUserAgent() string {
	if !a.IsOpenAI() {
		return ""
	}
	return a.GetCredential("user_agent")
}

func (a *Account) GetChatGPTAccountID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("chatgpt_account_id")
}

func (a *Account) GetOpenAIDeviceID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return strings.TrimSpace(a.GetExtraString("openai_device_id"))
}

func (a *Account) GetOpenAISessionID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return strings.TrimSpace(a.GetExtraString("openai_session_id"))
}

func (a *Account) SupportsOpenAIEndpointCapability(capability OpenAIEndpointCapability) bool {
	if a == nil {
		return false
	}
	if capability == "" {
		return true
	}
	if !a.IsOpenAI() {
		return false
	}
	switch capability {
	case OpenAIEndpointCapabilityChatCompletions:
	case OpenAIEndpointCapabilityEmbeddings:
		if a.Type != AccountTypeAPIKey {
			return false
		}
	default:
		return false
	}

	configured, found := a.openAIEndpointCapabilitySet()
	if !found {
		return true
	}
	return configured[string(capability)]
}

func (a *Account) openAIEndpointCapabilitySet() (map[string]bool, bool) {
	if a == nil || a.Credentials == nil {
		return nil, false
	}
	raw, found := a.Credentials[openAIEndpointCapabilitiesCredentialKey]
	if !found || raw == nil {
		return nil, false
	}

	result := make(map[string]bool)
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		result[value] = true
	}

	switch capabilities := raw.(type) {
	case []any:
		for _, item := range capabilities {
			if value, ok := item.(string); ok {
				add(value)
			}
		}
	case []string:
		for _, value := range capabilities {
			add(value)
		}
	case map[string]any:
		for key, value := range capabilities {
			enabled, ok := value.(bool)
			if ok && enabled {
				add(key)
			}
		}
	case map[string]bool:
		for key, enabled := range capabilities {
			if enabled {
				add(key)
			}
		}
	}

	return result, true
}

func (a *Account) SupportsOpenAIImageCapability(capability OpenAIImagesCapability) bool {
	if !a.IsOpenAI() {
		return false
	}
	switch capability {
	case OpenAIImagesCapabilityBasic, OpenAIImagesCapabilityNative:
		return a.Type == AccountTypeOAuth || a.Type == AccountTypeAPIKey
	default:
		return true
	}
}

func (a *Account) GetChatGPTUserID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("chatgpt_user_id")
}

func (a *Account) GetOpenAIOrganizationID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("organization_id")
}

func (a *Account) GetOpenAITokenExpiresAt() *time.Time {
	if !a.IsOpenAIOAuth() {
		return nil
	}
	return a.GetCredentialAsTime("expires_at")
}

func (a *Account) IsOpenAITokenExpired() bool {
	expiresAt := a.GetOpenAITokenExpiresAt()
	if expiresAt == nil {
		return false
	}
	return time.Now().Add(60 * time.Second).After(*expiresAt)
}

// IsMixedSchedulingEnabled 检查 antigravity 账户是否启用混合调度
// 启用后可参与 anthropic/gemini 分组的账户调度
func (a *Account) IsMixedSchedulingEnabled() bool {
	if a.Platform != PlatformAntigravity {
		return false
	}
	if a.Extra == nil {
		return false
	}
	if v, ok := a.Extra["mixed_scheduling"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

// IsOveragesEnabled 检查 Antigravity 账号是否启用 AI Credits 超量请求。
func (a *Account) IsOveragesEnabled() bool {
	if a.Platform != PlatformAntigravity {
		return false
	}
	if a.Extra == nil {
		return false
	}
	if v, ok := a.Extra["allow_overages"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

// IsOpenAIPassthroughEnabled 返回 OpenAI 账号是否启用"自动透传（仅替换认证）"。
//
// 新字段：accounts.extra.openai_passthrough。
// 兼容字段：accounts.extra.openai_oauth_passthrough（历史 OAuth 开关）。
// 字段缺失或类型不正确时，按 false（关闭）处理。
func (a *Account) IsOpenAIPassthroughEnabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
	}
	if enabled, ok := a.Extra["openai_passthrough"].(bool); ok {
		return enabled
	}
	if enabled, ok := a.Extra["openai_oauth_passthrough"].(bool); ok {
		return enabled
	}
	return false
}

// IsOpenAIResponsesWebSocketV2Enabled 返回 OpenAI 账号是否开启 Responses WebSocket v2。
//
// 分类型新字段：
// - OAuth 账号：accounts.extra.openai_oauth_responses_websockets_v2_enabled
// - API Key 账号：accounts.extra.openai_apikey_responses_websockets_v2_enabled
//
// 兼容字段：
// - accounts.extra.responses_websockets_v2_enabled
// - accounts.extra.openai_ws_enabled（历史开关）
//
// 优先级：
// 1. 按账号类型读取分类型字段
// 2. 分类型字段缺失时，回退兼容字段
func (a *Account) IsOpenAIResponsesWebSocketV2Enabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
	}
	if a.IsOpenAIOAuth() {
		if enabled, ok := a.Extra["openai_oauth_responses_websockets_v2_enabled"].(bool); ok {
			return enabled
		}
	}
	if a.IsOpenAIApiKey() {
		if enabled, ok := a.Extra["openai_apikey_responses_websockets_v2_enabled"].(bool); ok {
			return enabled
		}
	}
	if enabled, ok := a.Extra["responses_websockets_v2_enabled"].(bool); ok {
		return enabled
	}
	if enabled, ok := a.Extra["openai_ws_enabled"].(bool); ok {
		return enabled
	}
	return false
}

const (
	OpenAIWSIngressModeOff         = "off"
	OpenAIWSIngressModeShared      = "shared"
	OpenAIWSIngressModeDedicated   = "dedicated"
	OpenAIWSIngressModeCtxPool     = "ctx_pool"
	OpenAIWSIngressModePassthrough = "passthrough"
)

func normalizeOpenAIWSIngressMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case OpenAIWSIngressModeOff:
		return OpenAIWSIngressModeOff
	case OpenAIWSIngressModeCtxPool:
		return OpenAIWSIngressModeCtxPool
	case OpenAIWSIngressModePassthrough:
		return OpenAIWSIngressModePassthrough
	case OpenAIWSIngressModeShared:
		return OpenAIWSIngressModeShared
	case OpenAIWSIngressModeDedicated:
		return OpenAIWSIngressModeDedicated
	default:
		return ""
	}
}

func normalizeOpenAIWSIngressDefaultMode(mode string) string {
	if normalized := normalizeOpenAIWSIngressMode(mode); normalized != "" {
		if normalized == OpenAIWSIngressModeShared || normalized == OpenAIWSIngressModeDedicated {
			return OpenAIWSIngressModeCtxPool
		}
		return normalized
	}
	return OpenAIWSIngressModeCtxPool
}

// ResolveOpenAIResponsesWebSocketV2Mode 返回账号在 WSv2 ingress 下的有效模式（off/ctx_pool/passthrough）。
//
// 优先级：
// 1. 分类型 mode 新字段（string）
// 2. 分类型 enabled 旧字段（bool）
// 3. 兼容 enabled 旧字段（bool）
// 4. defaultMode（非法时回退 ctx_pool）
func (a *Account) ResolveOpenAIResponsesWebSocketV2Mode(defaultMode string) string {
	resolvedDefault := normalizeOpenAIWSIngressDefaultMode(defaultMode)
	if a == nil || !a.IsOpenAI() {
		return OpenAIWSIngressModeOff
	}
	if a.Extra == nil {
		return resolvedDefault
	}

	resolveModeString := func(key string) (string, bool) {
		raw, ok := a.Extra[key]
		if !ok {
			return "", false
		}
		mode, ok := raw.(string)
		if !ok {
			return "", false
		}
		normalized := normalizeOpenAIWSIngressMode(mode)
		if normalized == "" {
			return "", false
		}
		return normalized, true
	}
	resolveBoolMode := func(key string) (string, bool) {
		raw, ok := a.Extra[key]
		if !ok {
			return "", false
		}
		enabled, ok := raw.(bool)
		if !ok {
			return "", false
		}
		if enabled {
			return OpenAIWSIngressModeCtxPool, true
		}
		return OpenAIWSIngressModeOff, true
	}

	if a.IsOpenAIOAuth() {
		if mode, ok := resolveModeString("openai_oauth_responses_websockets_v2_mode"); ok {
			return mode
		}
		if mode, ok := resolveBoolMode("openai_oauth_responses_websockets_v2_enabled"); ok {
			return mode
		}
	}
	if a.IsOpenAIApiKey() {
		if mode, ok := resolveModeString("openai_apikey_responses_websockets_v2_mode"); ok {
			return mode
		}
		if mode, ok := resolveBoolMode("openai_apikey_responses_websockets_v2_enabled"); ok {
			return mode
		}
	}
	if mode, ok := resolveBoolMode("responses_websockets_v2_enabled"); ok {
		return mode
	}
	if mode, ok := resolveBoolMode("openai_ws_enabled"); ok {
		return mode
	}
	// 兼容旧值：shared/dedicated 语义都归并到 ctx_pool。
	if resolvedDefault == OpenAIWSIngressModeShared || resolvedDefault == OpenAIWSIngressModeDedicated {
		return OpenAIWSIngressModeCtxPool
	}
	return resolvedDefault
}

// IsOpenAIWSForceHTTPEnabled 返回账号级"强制 HTTP"开关。
// 字段：accounts.extra.openai_ws_force_http。
func (a *Account) IsOpenAIWSForceHTTPEnabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra["openai_ws_force_http"].(bool)
	return ok && enabled
}

// IsOpenAIWSAllowStoreRecoveryEnabled 返回账号级 store 恢复开关。
// 字段：accounts.extra.openai_ws_allow_store_recovery。
func (a *Account) IsOpenAIWSAllowStoreRecoveryEnabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra["openai_ws_allow_store_recovery"].(bool)
	return ok && enabled
}

// IsOpenAIOAuthPassthroughEnabled 兼容旧接口，等价于 OAuth 账号的 IsOpenAIPassthroughEnabled。
func (a *Account) IsOpenAIOAuthPassthroughEnabled() bool {
	return a != nil && a.IsOpenAIOAuth() && a.IsOpenAIPassthroughEnabled()
}

// IsAnthropicAPIKeyPassthroughEnabled 返回 Anthropic API Key 账号是否启用"自动透传（仅替换认证）"。
// 字段：accounts.extra.anthropic_passthrough。
// 字段缺失或类型不正确时，按 false（关闭）处理。
func (a *Account) IsAnthropicAPIKeyPassthroughEnabled() bool {
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeAPIKey || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra["anthropic_passthrough"].(bool)
	return ok && enabled
}

// WebSearch 模拟三态常量
const (
	WebSearchModeDefault  = "default"  // 跟随渠道配置
	WebSearchModeEnabled  = "enabled"  // 强制开启
	WebSearchModeDisabled = "disabled" // 强制关闭
)

// GetWebSearchEmulationMode 返回账号的 WebSearch 模拟模式。
// 三态：default（跟随渠道）/ enabled（强制开启）/ disabled（强制关闭）。
// 兼容旧 bool 值：true→enabled, false→default（并记录 debug 日志）。
func (a *Account) GetWebSearchEmulationMode() string {
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeAPIKey || a.Extra == nil {
		return WebSearchModeDefault
	}
	raw := a.Extra[featureKeyWebSearchEmulation]
	// Tolerant: legacy bool values (pre-migration or stale writes)
	if b, ok := raw.(bool); ok {
		slog.Debug("legacy bool web_search_emulation value", "account_id", a.ID, "value", b)
		if b {
			return WebSearchModeEnabled
		}
		return WebSearchModeDefault
	}
	mode, ok := raw.(string)
	if !ok {
		return WebSearchModeDefault
	}
	switch mode {
	case WebSearchModeEnabled, WebSearchModeDisabled:
		return mode
	default:
		return WebSearchModeDefault
	}
}

// IsCodexCLIOnlyEnabled 返回 OpenAI OAuth 账号是否启用"仅允许 Codex 官方客户端"。
// 字段：accounts.extra.codex_cli_only。
// 字段缺失或类型不正确时，按 false（关闭）处理。
func (a *Account) IsCodexCLIOnlyEnabled() bool {
	if a == nil || !a.IsOpenAIOAuth() || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra["codex_cli_only"].(bool)
	return ok && enabled
}

// GetCodexCLIOnlyAllowedClients 返回 codex_cli_only 之上额外放行的命名客户端预设 ID 列表。
// 仅 OpenAI OAuth 账号生效；缺失或类型不符时返回空。预设 ID 的具体匹配规则由
// openai 包的 registry 固化，配置只能引用预设键、不能自定义规则。
func (a *Account) GetCodexCLIOnlyAllowedClients() []string {
	if a == nil || !a.IsOpenAIOAuth() || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra["codex_cli_only_allowed_clients"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		result := make([]string, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				result = append(result, s)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// WindowCostSchedulability 窗口费用调度状态
type WindowCostSchedulability int

const (
	// WindowCostSchedulable 可正常调度
	WindowCostSchedulable WindowCostSchedulability = iota
	// WindowCostStickyOnly 仅允许粘性会话
	WindowCostStickyOnly
	// WindowCostNotSchedulable 完全不可调度
	WindowCostNotSchedulable
)

// IsAnthropicOAuthOrSetupToken 判断是否为 Anthropic OAuth 或 SetupToken 类型账号
// 仅这两类账号支持 5h 窗口额度控制和会话数量控制
func (a *Account) IsAnthropicOAuthOrSetupToken() bool {
	return a.Platform == PlatformAnthropic && (a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken)
}

// IsTLSFingerprintEnabled 检查是否启用 TLS 指纹伪装
// 仅适用于 Anthropic OAuth/SetupToken 类型账号
// 启用后将模拟 Claude Code (Node.js) 客户端的 TLS 握手特征
func (a *Account) IsTLSFingerprintEnabled() bool {
	// 仅支持 Anthropic OAuth/SetupToken 账号
	if !a.IsAnthropicOAuthOrSetupToken() {
		return false
	}
	if a.Extra == nil {
		return false
	}
	if v, ok := a.Extra["enable_tls_fingerprint"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

// GetTLSFingerprintProfileID 获取账号绑定的 TLS 指纹模板 ID
// 返回 0 表示未绑定（使用内置默认 profile）
func (a *Account) GetTLSFingerprintProfileID() int64 {
	if a.Extra == nil {
		return 0
	}
	v, ok := a.Extra["tls_fingerprint_profile_id"]
	if !ok {
		return 0
	}
	switch id := v.(type) {
	case float64:
		return int64(id)
	case int64:
		return id
	case int:
		return int64(id)
	case json.Number:
		if i, err := id.Int64(); err == nil {
			return i
		}
	}
	return 0
}

// GetUserMsgQueueMode 获取用户消息队列模式
// "serialize" = 串行队列, "throttle" = 软性限速, "" = 未设置（使用全局配置）
func (a *Account) GetUserMsgQueueMode() string {
	if a.Extra == nil {
		return ""
	}
	// 优先读取新字段 user_msg_queue_mode（白名单校验，非法值视为未设置）
	if mode, ok := a.Extra["user_msg_queue_mode"].(string); ok && mode != "" {
		if mode == config.UMQModeSerialize || mode == config.UMQModeThrottle {
			return mode
		}
		return "" // 非法值 fallback 到全局配置
	}
	// 向后兼容: user_msg_queue_enabled: true → "serialize"
	if enabled, ok := a.Extra["user_msg_queue_enabled"].(bool); ok && enabled {
		return config.UMQModeSerialize
	}
	return ""
}

// IsSessionIDMaskingEnabled 检查是否启用会话ID伪装
// 仅适用于 Anthropic OAuth/SetupToken 类型账号
// 启用后将在一段时间内（15分钟）固定 metadata.user_id 中的 session ID，
// 使上游认为请求来自同一个会话
func (a *Account) IsSessionIDMaskingEnabled() bool {
	if !a.IsAnthropicOAuthOrSetupToken() {
		return false
	}
	if a.Extra == nil {
		return false
	}
	if v, ok := a.Extra["session_id_masking_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

// IsCustomBaseURLEnabled 检查是否启用自定义 base URL 中继转发
// 仅适用于 Anthropic OAuth/SetupToken 类型账号
func (a *Account) IsCustomBaseURLEnabled() bool {
	if !a.IsAnthropicOAuthOrSetupToken() {
		return false
	}
	if a.Extra == nil {
		return false
	}
	if v, ok := a.Extra["custom_base_url_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

// GetCustomBaseURL 返回自定义中继服务的 base URL
func (a *Account) GetCustomBaseURL() string {
	return a.GetExtraString("custom_base_url")
}

// IsCacheTTLOverrideEnabled 检查是否启用缓存 TTL 强制替换
// 仅适用于 Anthropic OAuth/SetupToken 类型账号
// 启用后将所有 cache creation tokens 归入指定的 TTL 类型（5m 或 1h）
func (a *Account) IsCacheTTLOverrideEnabled() bool {
	if !a.IsAnthropicOAuthOrSetupToken() {
		return false
	}
	if a.Extra == nil {
		return false
	}
	if v, ok := a.Extra["cache_ttl_override_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
		}
	}
	return false
}

// GetCacheTTLOverrideTarget 获取缓存 TTL 强制替换的目标类型
// 返回 "5m" 或 "1h"，默认 "5m"
func (a *Account) GetCacheTTLOverrideTarget() string {
	if a.Extra == nil {
		return "5m"
	}
	if v, ok := a.Extra["cache_ttl_override_target"]; ok {
		if target, ok := v.(string); ok && (target == "5m" || target == "1h") {
			return target
		}
	}
	return "5m"
}

// GetQuotaLimit 获取 API Key 账号的配额限制（美元）
// 返回 0 表示未启用
func (a *Account) GetQuotaLimit() float64 {
	return a.getExtraFloat64("quota_limit")
}

// GetQuotaUsed 获取 API Key 账号的已用配额（美元）
func (a *Account) GetQuotaUsed() float64 {
	return a.getExtraFloat64("quota_used")
}

// GetQuotaDailyLimit 获取日额度限制（美元），0 表示未启用
func (a *Account) GetQuotaDailyLimit() float64 {
	return a.getExtraFloat64("quota_daily_limit")
}

// GetQuotaDailyUsed 获取当日已用额度（美元）
func (a *Account) GetQuotaDailyUsed() float64 {
	return a.getExtraFloat64("quota_daily_used")
}

// GetQuotaWeeklyLimit 获取周额度限制（美元），0 表示未启用
func (a *Account) GetQuotaWeeklyLimit() float64 {
	return a.getExtraFloat64("quota_weekly_limit")
}

// GetQuotaWeeklyUsed 获取本周已用额度（美元）
func (a *Account) GetQuotaWeeklyUsed() float64 {
	return a.getExtraFloat64("quota_weekly_used")
}

// getExtraFloat64 从 Extra 中读取指定 key 的 float64 值
func (a *Account) getExtraFloat64(key string) float64 {
	if a.Extra == nil {
		return 0
	}
	if v, ok := a.Extra[key]; ok {
		return parseExtraFloat64(v)
	}
	return 0
}

// getExtraTime 从 Extra 中读取 RFC3339 时间戳
func (a *Account) getExtraTime(key string) time.Time {
	if a.Extra == nil {
		return time.Time{}
	}
	if v, ok := a.Extra[key]; ok {
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return t
			}
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// getExtraBool 从 Extra 中读取指定 key 的 bool 值
func (a *Account) getExtraBool(key string) bool {
	if a.Extra == nil {
		return false
	}
	if v, ok := a.Extra[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// getExtraString 从 Extra 中读取指定 key 的字符串值
func (a *Account) getExtraString(key string) string {
	if a.Extra == nil {
		return ""
	}
	if v, ok := a.Extra[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getExtraStringDefault 从 Extra 中读取指定 key 的字符串值，不存在时返回 defaultVal
func (a *Account) getExtraStringDefault(key, defaultVal string) string {
	if v := a.getExtraString(key); v != "" {
		return v
	}
	return defaultVal
}

// getExtraInt 从 Extra 中读取指定 key 的 int 值
func (a *Account) getExtraInt(key string) int {
	if a.Extra == nil {
		return 0
	}
	if v, ok := a.Extra[key]; ok {
		return int(parseExtraFloat64(v))
	}
	return 0
}

// GetQuotaDailyResetMode 获取日额度重置模式："rolling"（默认）或 "fixed"
func (a *Account) GetQuotaDailyResetMode() string {
	if m := a.getExtraString("quota_daily_reset_mode"); m == "fixed" {
		return "fixed"
	}
	return "rolling"
}

// GetQuotaDailyResetHour 获取固定重置的小时（0-23），默认 0
func (a *Account) GetQuotaDailyResetHour() int {
	return a.getExtraInt("quota_daily_reset_hour")
}

// GetQuotaWeeklyResetMode 获取周额度重置模式："rolling"（默认）或 "fixed"
func (a *Account) GetQuotaWeeklyResetMode() string {
	if m := a.getExtraString("quota_weekly_reset_mode"); m == "fixed" {
		return "fixed"
	}
	return "rolling"
}

// GetQuotaWeeklyResetDay 获取固定重置的星期几（0=周日, 1=周一, ..., 6=周六），默认 1（周一）
func (a *Account) GetQuotaWeeklyResetDay() int {
	if a.Extra == nil {
		return 1
	}
	if _, ok := a.Extra["quota_weekly_reset_day"]; !ok {
		return 1
	}
	return a.getExtraInt("quota_weekly_reset_day")
}

// GetQuotaWeeklyResetHour 获取周配额固定重置的小时（0-23），默认 0
func (a *Account) GetQuotaWeeklyResetHour() int {
	return a.getExtraInt("quota_weekly_reset_hour")
}

// GetQuotaResetTimezone 获取固定重置的时区名（IANA），默认 "UTC"
func (a *Account) GetQuotaResetTimezone() string {
	if tz := a.getExtraString("quota_reset_timezone"); tz != "" {
		return tz
	}
	return "UTC"
}

// --- Quota Notification Getters ---

// QuotaNotifyConfig returns the notify configuration for a given quota dimension.
// dim must be one of quotaDimDaily, quotaDimWeekly, quotaDimTotal.
func (a *Account) QuotaNotifyConfig(dim string) (enabled bool, threshold float64, thresholdType string) {
	enabled = a.getExtraBool("quota_notify_" + dim + "_enabled")
	threshold = a.getExtraFloat64("quota_notify_" + dim + "_threshold")
	thresholdType = a.getExtraStringDefault("quota_notify_"+dim+"_threshold_type", thresholdTypeFixed)
	return
}

func (a *Account) GetQuotaNotifyDailyEnabled() bool {
	e, _, _ := a.QuotaNotifyConfig(quotaDimDaily)
	return e
}

func (a *Account) GetQuotaNotifyDailyThreshold() float64 {
	_, t, _ := a.QuotaNotifyConfig(quotaDimDaily)
	return t
}

func (a *Account) GetQuotaNotifyDailyThresholdType() string {
	_, _, tt := a.QuotaNotifyConfig(quotaDimDaily)
	return tt
}

func (a *Account) GetQuotaNotifyWeeklyEnabled() bool {
	e, _, _ := a.QuotaNotifyConfig(quotaDimWeekly)
	return e
}

func (a *Account) GetQuotaNotifyWeeklyThreshold() float64 {
	_, t, _ := a.QuotaNotifyConfig(quotaDimWeekly)
	return t
}

func (a *Account) GetQuotaNotifyWeeklyThresholdType() string {
	_, _, tt := a.QuotaNotifyConfig(quotaDimWeekly)
	return tt
}

func (a *Account) GetQuotaNotifyTotalEnabled() bool {
	e, _, _ := a.QuotaNotifyConfig(quotaDimTotal)
	return e
}

func (a *Account) GetQuotaNotifyTotalThreshold() float64 {
	_, t, _ := a.QuotaNotifyConfig(quotaDimTotal)
	return t
}

func (a *Account) GetQuotaNotifyTotalThresholdType() string {
	_, _, tt := a.QuotaNotifyConfig(quotaDimTotal)
	return tt
}

// nextFixedDailyReset 计算在 after 之后的下一个每日固定重置时间点
func nextFixedDailyReset(hour int, tz *time.Location, after time.Time) time.Time {
	t := after.In(tz)
	today := time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, tz)
	if !after.Before(today) {
		return today.AddDate(0, 0, 1)
	}
	return today
}

// lastFixedDailyReset 计算 now 之前最近一次的每日固定重置时间点
func lastFixedDailyReset(hour int, tz *time.Location, now time.Time) time.Time {
	t := now.In(tz)
	today := time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, tz)
	if now.Before(today) {
		return today.AddDate(0, 0, -1)
	}
	return today
}

// nextFixedWeeklyReset 计算在 after 之后的下一个每周固定重置时间点
// day: 0=Sunday, 1=Monday, ..., 6=Saturday
func nextFixedWeeklyReset(day, hour int, tz *time.Location, after time.Time) time.Time {
	t := after.In(tz)
	todayReset := time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, tz)
	currentDay := int(todayReset.Weekday())

	daysForward := (day - currentDay + 7) % 7
	if daysForward == 0 && !after.Before(todayReset) {
		daysForward = 7
	}
	return todayReset.AddDate(0, 0, daysForward)
}

// lastFixedWeeklyReset 计算 now 之前最近一次的每周固定重置时间点
func lastFixedWeeklyReset(day, hour int, tz *time.Location, now time.Time) time.Time {
	t := now.In(tz)
	todayReset := time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, tz)
	currentDay := int(todayReset.Weekday())

	daysBack := (currentDay - day + 7) % 7
	if daysBack == 0 && now.Before(todayReset) {
		daysBack = 7
	}
	return todayReset.AddDate(0, 0, -daysBack)
}

// isFixedDailyPeriodExpired 检查日配额是否在固定时间模式下已过期
func (a *Account) isFixedDailyPeriodExpired(periodStart time.Time) bool {
	if periodStart.IsZero() {
		return true
	}
	tz, err := time.LoadLocation(a.GetQuotaResetTimezone())
	if err != nil {
		tz = time.UTC
	}
	lastReset := lastFixedDailyReset(a.GetQuotaDailyResetHour(), tz, time.Now())
	return periodStart.Before(lastReset)
}

// isFixedWeeklyPeriodExpired 检查周配额是否在固定时间模式下已过期
func (a *Account) isFixedWeeklyPeriodExpired(periodStart time.Time) bool {
	if periodStart.IsZero() {
		return true
	}
	tz, err := time.LoadLocation(a.GetQuotaResetTimezone())
	if err != nil {
		tz = time.UTC
	}
	lastReset := lastFixedWeeklyReset(a.GetQuotaWeeklyResetDay(), a.GetQuotaWeeklyResetHour(), tz, time.Now())
	return periodStart.Before(lastReset)
}

// ComputeQuotaResetAt 根据当前配置计算并填充 extra 中的 quota_daily_reset_at / quota_weekly_reset_at
// 在保存账号配置时调用
func ComputeQuotaResetAt(extra map[string]any) {
	now := time.Now()
	tzName, _ := extra["quota_reset_timezone"].(string)
	if tzName == "" {
		tzName = "UTC"
	}
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		tz = time.UTC
	}

	// 日配额固定重置时间
	if mode, _ := extra["quota_daily_reset_mode"].(string); mode == "fixed" {
		hour := int(parseExtraFloat64(extra["quota_daily_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		resetAt := nextFixedDailyReset(hour, tz, now)
		extra["quota_daily_reset_at"] = resetAt.UTC().Format(time.RFC3339)
	} else {
		delete(extra, "quota_daily_reset_at")
	}

	// 周配额固定重置时间
	if mode, _ := extra["quota_weekly_reset_mode"].(string); mode == "fixed" {
		day := 1 // 默认周一
		if d, ok := extra["quota_weekly_reset_day"]; ok {
			day = int(parseExtraFloat64(d))
		}
		if day < 0 || day > 6 {
			day = 1
		}
		hour := int(parseExtraFloat64(extra["quota_weekly_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		resetAt := nextFixedWeeklyReset(day, hour, tz, now)
		extra["quota_weekly_reset_at"] = resetAt.UTC().Format(time.RFC3339)
	} else {
		delete(extra, "quota_weekly_reset_at")
	}
}

// NormalizeFixedQuotaWindows aligns preserved quota usage with the active fixed reset window.
//
// Editing an existing account can switch a daily/weekly quota from rolling to fixed reset
// while preserving quota_*_used and quota_*_start. If the preserved start belongs to the
// old rolling window, response mapping treats the usage as expired and the dashboard shows
// 0 until the next reset. Normalize those stale starts before persisting the edited account.
func NormalizeFixedQuotaWindows(extra map[string]any) {
	if extra == nil {
		return
	}
	now := time.Now()
	tzName, _ := extra["quota_reset_timezone"].(string)
	if tzName == "" {
		tzName = "UTC"
	}
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		tz = time.UTC
	}

	if mode, _ := extra["quota_daily_reset_mode"].(string); mode == "fixed" && parseExtraFloat64(extra["quota_daily_limit"]) > 0 {
		hour := int(parseExtraFloat64(extra["quota_daily_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		lastReset := lastFixedDailyReset(hour, tz, now)
		start := parseExtraTime(extra["quota_daily_start"])
		if start.IsZero() || start.Before(lastReset) {
			extra["quota_daily_used"] = 0.0
			extra["quota_daily_start"] = lastReset.UTC().Format(time.RFC3339)
		}
	}

	if mode, _ := extra["quota_weekly_reset_mode"].(string); mode == "fixed" && parseExtraFloat64(extra["quota_weekly_limit"]) > 0 {
		day := 1
		if rawDay, ok := extra["quota_weekly_reset_day"]; ok {
			day = int(parseExtraFloat64(rawDay))
		}
		if day < 0 || day > 6 {
			day = 1
		}
		hour := int(parseExtraFloat64(extra["quota_weekly_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		lastReset := lastFixedWeeklyReset(day, hour, tz, now)
		start := parseExtraTime(extra["quota_weekly_start"])
		if start.IsZero() || start.Before(lastReset) {
			extra["quota_weekly_used"] = 0.0
			extra["quota_weekly_start"] = lastReset.UTC().Format(time.RFC3339)
		}
	}
}

// ValidateQuotaResetConfig 校验配额固定重置时间配置的合法性
func ValidateQuotaResetConfig(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	// 校验时区
	if tz, ok := extra["quota_reset_timezone"].(string); ok && tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return errors.New("invalid quota_reset_timezone: must be a valid IANA timezone name")
		}
	}
	// 日配额重置模式
	if mode, ok := extra["quota_daily_reset_mode"].(string); ok {
		if mode != "rolling" && mode != "fixed" {
			return errors.New("quota_daily_reset_mode must be 'rolling' or 'fixed'")
		}
	}
	// 日配额重置小时
	if v, ok := extra["quota_daily_reset_hour"]; ok {
		hour := int(parseExtraFloat64(v))
		if hour < 0 || hour > 23 {
			return errors.New("quota_daily_reset_hour must be between 0 and 23")
		}
	}
	// 周配额重置模式
	if mode, ok := extra["quota_weekly_reset_mode"].(string); ok {
		if mode != "rolling" && mode != "fixed" {
			return errors.New("quota_weekly_reset_mode must be 'rolling' or 'fixed'")
		}
	}
	// 周配额重置星期几
	if v, ok := extra["quota_weekly_reset_day"]; ok {
		day := int(parseExtraFloat64(v))
		if day < 0 || day > 6 {
			return errors.New("quota_weekly_reset_day must be between 0 (Sunday) and 6 (Saturday)")
		}
	}
	// 周配额重置小时
	if v, ok := extra["quota_weekly_reset_hour"]; ok {
		hour := int(parseExtraFloat64(v))
		if hour < 0 || hour > 23 {
			return errors.New("quota_weekly_reset_hour must be between 0 and 23")
		}
	}
	return nil
}

// HasAnyQuotaLimit 检查是否配置了任一维度的配额限制
func (a *Account) HasAnyQuotaLimit() bool {
	return a.GetQuotaLimit() > 0 || a.GetQuotaDailyLimit() > 0 || a.GetQuotaWeeklyLimit() > 0
}

// isPeriodExpired 检查指定周期（自 periodStart 起经过 dur）是否已过期
func isPeriodExpired(periodStart time.Time, dur time.Duration) bool {
	if periodStart.IsZero() {
		return true // 从未使用过，视为过期（下次 increment 会初始化）
	}
	return time.Since(periodStart) >= dur
}

// IsDailyQuotaPeriodExpired 检查日配额周期是否已过期（用于显示层判断是否需要将 used 归零）
func (a *Account) IsDailyQuotaPeriodExpired() bool {
	start := a.getExtraTime("quota_daily_start")
	if a.GetQuotaDailyResetMode() == "fixed" {
		return a.isFixedDailyPeriodExpired(start)
	}
	return isPeriodExpired(start, 24*time.Hour)
}

// IsWeeklyQuotaPeriodExpired 检查周配额周期是否已过期（用于显示层判断是否需要将 used 归零）
func (a *Account) IsWeeklyQuotaPeriodExpired() bool {
	start := a.getExtraTime("quota_weekly_start")
	if a.GetQuotaWeeklyResetMode() == "fixed" {
		return a.isFixedWeeklyPeriodExpired(start)
	}
	return isPeriodExpired(start, 7*24*time.Hour)
}

// IsQuotaExceeded 检查 API Key 账号配额是否已超限（任一维度超限即返回 true）
func (a *Account) IsQuotaExceeded() bool {
	// 总额度
	if limit := a.GetQuotaLimit(); limit > 0 && a.GetQuotaUsed() >= limit {
		return true
	}
	// 日额度（周期过期视为未超限，下次 increment 会重置）
	if limit := a.GetQuotaDailyLimit(); limit > 0 {
		start := a.getExtraTime("quota_daily_start")
		var expired bool
		if a.GetQuotaDailyResetMode() == "fixed" {
			expired = a.isFixedDailyPeriodExpired(start)
		} else {
			expired = isPeriodExpired(start, 24*time.Hour)
		}
		if !expired && a.GetQuotaDailyUsed() >= limit {
			return true
		}
	}
	// 周额度
	if limit := a.GetQuotaWeeklyLimit(); limit > 0 {
		start := a.getExtraTime("quota_weekly_start")
		var expired bool
		if a.GetQuotaWeeklyResetMode() == "fixed" {
			expired = a.isFixedWeeklyPeriodExpired(start)
		} else {
			expired = isPeriodExpired(start, 7*24*time.Hour)
		}
		if !expired && a.GetQuotaWeeklyUsed() >= limit {
			return true
		}
	}
	return false
}

// GetWindowCostLimit 获取 5h 窗口费用阈值（美元）
// 返回 0 表示未启用
func (a *Account) GetWindowCostLimit() float64 {
	if a.Extra == nil {
		return 0
	}
	if v, ok := a.Extra["window_cost_limit"]; ok {
		return parseExtraFloat64(v)
	}
	return 0
}

// GetWindowCostStickyReserve 获取粘性会话预留额度（美元）
// 默认值为 10
func (a *Account) GetWindowCostStickyReserve() float64 {
	if a.Extra == nil {
		return 10.0
	}
	if v, ok := a.Extra["window_cost_sticky_reserve"]; ok {
		val := parseExtraFloat64(v)
		if val > 0 {
			return val
		}
	}
	return 10.0
}

// GetMaxSessions 获取最大并发会话数
// 返回 0 表示未启用
func (a *Account) GetMaxSessions() int {
	if a.Extra == nil {
		return 0
	}
	if v, ok := a.Extra["max_sessions"]; ok {
		return parseExtraInt(v)
	}
	return 0
}

// GetSessionIdleTimeoutMinutes 获取会话空闲超时分钟数
// 默认值为 5 分钟
func (a *Account) GetSessionIdleTimeoutMinutes() int {
	if a.Extra == nil {
		return 5
	}
	if v, ok := a.Extra["session_idle_timeout_minutes"]; ok {
		val := parseExtraInt(v)
		if val > 0 {
			return val
		}
	}
	return 5
}

// GetBaseRPM 获取基础 RPM 限制
// 返回 0 表示未启用（负数视为无效配置，按 0 处理）
func (a *Account) GetBaseRPM() int {
	if a.Extra == nil {
		return 0
	}
	if v, ok := a.Extra["base_rpm"]; ok {
		val := parseExtraInt(v)
		if val > 0 {
			return val
		}
	}
	return 0
}

// GetRPMStrategy 获取 RPM 策略
// "tiered" = 三区模型（默认）, "sticky_exempt" = 粘性豁免
func (a *Account) GetRPMStrategy() string {
	if a.Extra == nil {
		return "tiered"
	}
	if v, ok := a.Extra["rpm_strategy"]; ok {
		if s, ok := v.(string); ok && s == "sticky_exempt" {
			return "sticky_exempt"
		}
	}
	return "tiered"
}

// GetRPMStickyBuffer 获取 RPM 粘性缓冲数量
// Cache-driven: buffer = concurrency + maxSessions（覆盖幽灵窗口 + 稳态会话需求）
// floor = baseRPM / 5（向后兼容 maxSessions=0 且 concurrency=0 场景）
func (a *Account) GetRPMStickyBuffer() int {
	if a.Extra == nil {
		return 0
	}

	// 手动 override 最高优先级
	if v, ok := a.Extra["rpm_sticky_buffer"]; ok {
		val := parseExtraInt(v)
		if val > 0 {
			return val
		}
	}

	base := a.GetBaseRPM()
	if base <= 0 {
		return 0
	}

	// Cache-driven buffer = concurrency + maxSessions
	conc := a.Concurrency
	if conc < 0 {
		conc = 0
	}
	sess := a.GetMaxSessions()
	if sess < 0 {
		sess = 0
	}

	buffer := conc + sess

	// floor: 向后兼容
	floor := base / 5
	if floor < 1 {
		floor = 1
	}
	if buffer < floor {
		buffer = floor
	}

	return buffer
}

// CheckRPMSchedulability 根据当前 RPM 计数检查调度状态
// 复用 WindowCostSchedulability 三态：Schedulable / StickyOnly / NotSchedulable
func (a *Account) CheckRPMSchedulability(currentRPM int) WindowCostSchedulability {
	baseRPM := a.GetBaseRPM()
	if baseRPM <= 0 {
		return WindowCostSchedulable
	}

	if currentRPM < baseRPM {
		return WindowCostSchedulable
	}

	strategy := a.GetRPMStrategy()
	if strategy == "sticky_exempt" {
		return WindowCostStickyOnly // 粘性豁免无红区
	}

	// tiered: 黄区 + 红区
	buffer := a.GetRPMStickyBuffer()
	if currentRPM < baseRPM+buffer {
		return WindowCostStickyOnly
	}
	return WindowCostNotSchedulable
}

// CheckWindowCostSchedulability 根据当前窗口费用检查调度状态
// - 费用 < 阈值: WindowCostSchedulable（可正常调度）
// - 费用 >= 阈值 且 < 阈值+预留: WindowCostStickyOnly（仅粘性会话）
// - 费用 >= 阈值+预留: WindowCostNotSchedulable（不可调度）
func (a *Account) CheckWindowCostSchedulability(currentWindowCost float64) WindowCostSchedulability {
	limit := a.GetWindowCostLimit()
	if limit <= 0 {
		return WindowCostSchedulable
	}

	if currentWindowCost < limit {
		return WindowCostSchedulable
	}

	stickyReserve := a.GetWindowCostStickyReserve()
	if currentWindowCost < limit+stickyReserve {
		return WindowCostStickyOnly
	}

	return WindowCostNotSchedulable
}

// GetCurrentWindowStartTime 获取当前有效的窗口开始时间
// 逻辑：
// 1. 如果窗口未过期（SessionWindowEnd 存在且在当前时间之后），使用记录的 SessionWindowStart
// 2. 否则（窗口过期或未设置），使用新的预测窗口开始时间（从当前整点开始）
func (a *Account) GetCurrentWindowStartTime() time.Time {
	now := time.Now()

	// 窗口未过期，使用记录的窗口开始时间
	if a.SessionWindowStart != nil && a.SessionWindowEnd != nil && now.Before(*a.SessionWindowEnd) {
		return *a.SessionWindowStart
	}

	// 窗口已过期或未设置，预测新的窗口开始时间（从当前整点开始）
	// 与 ratelimit_service.go 中 UpdateSessionWindow 的预测逻辑保持一致
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
}

// parseExtraFloat64 从 extra 字段解析 float64 值
func parseExtraFloat64(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return 0
}

func parseExtraTime(value any) time.Time {
	if s, ok := value.(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseExtraInt 从 extra 字段解析 int 值
// ParseExtraInt 从 extra 字段的 any 值解析为 int。
// 支持 int, int64, float64, json.Number, string 类型，无法解析时返回 0。
func ParseExtraInt(value any) int {
	return parseExtraInt(value)
}

func parseExtraInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
		}
	}
	return 0
}
