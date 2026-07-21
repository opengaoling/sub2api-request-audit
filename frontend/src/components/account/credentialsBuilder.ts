export function applyInterceptWarmup(
  credentials: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials.intercept_warmup_requests = true
  } else if (mode === 'edit') {
    delete credentials.intercept_warmup_requests
  }
}

// ========== 请求头覆写（仅 anthropic/openai 平台的 api_key 账号） ==========

export const HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY = 'header_override_enabled'
export const HEADER_OVERRIDES_CREDENTIAL_KEY = 'header_overrides'

export interface HeaderOverrideRow {
  name: string
  value: string
}

/** 请求头覆写支持的平台（与后端 IsHeaderOverrideEligible 保持一致） */
export function isHeaderOverridePlatform(platform: string): boolean {
  const normalized = platform.trim().toLowerCase()
  return normalized === 'anthropic' || normalized === 'openai'
}

/** 禁止覆写的请求头（与后端 headerOverrideBlockedNames 保持一致） */
const HEADER_OVERRIDE_BLOCKED_NAMES = new Set([
  'host',
  'content-length',
  'content-type',
  'transfer-encoding',
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'proxy-connection',
  'te',
  'trailer',
  'upgrade',
  'authorization',
  'x-api-key',
  'x-goog-api-key',
  'cookie',
  'accept-encoding',
  'sec-websocket-key',
  'sec-websocket-version',
  'sec-websocket-extensions',
  'sec-websocket-protocol',
  'sec-websocket-accept',
  'session_id',
  'conversation_id',
  'x-codex-turn-state',
  'x-codex-turn-metadata',
  'chatgpt-account-id',
  'x-claude-code-session-id',
  'x-client-request-id'
])

/** RFC 7230 token：合法的 HTTP header 名称字符集 */
const HEADER_NAME_PATTERN = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/

function isValidHeaderOverrideName(name: string): boolean {
  return HEADER_NAME_PATTERN.test(name)
}

/** 模板：Claude Code CLI API Key 请求使用的标准客户端请求头 */
const ANTHROPIC_HEADER_OVERRIDE_TEMPLATE: HeaderOverrideRow[] = [
  { name: 'user-agent', value: 'claude-cli/2.1.161 (external, cli)' },
  { name: 'x-app', value: 'cli' },
  {
    name: 'anthropic-beta',
    value:
      'claude-code-20250219,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14'
  },
  { name: 'anthropic-version', value: '2023-06-01' },
  { name: 'anthropic-dangerous-direct-browser-access', value: 'true' },
  { name: 'x-stainless-lang', value: 'js' },
  { name: 'x-stainless-package-version', value: '0.94.0' },
  { name: 'x-stainless-os', value: 'Linux' },
  { name: 'x-stainless-arch', value: 'arm64' },
  { name: 'x-stainless-runtime', value: 'node' },
  { name: 'x-stainless-runtime-version', value: 'v24.3.0' },
  { name: 'x-stainless-retry-count', value: '0' },
  { name: 'x-stainless-timeout', value: '600' }
]

/** 模板：Codex CLI API Key 请求使用的标准客户端请求头 */
const OPENAI_HEADER_OVERRIDE_TEMPLATE: HeaderOverrideRow[] = [
  {
    name: 'user-agent',
    value: 'codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color'
  },
  { name: 'originator', value: 'codex_cli_rs' },
  { name: 'openai-beta', value: 'responses=experimental' },
  { name: 'version', value: '0.144.1' },
  { name: 'accept', value: 'text/event-stream' },
  { name: 'accept-language', value: 'en-US,en;q=0.9' }
]

export function getHeaderOverrideTemplate(platform: string): HeaderOverrideRow[] {
  const normalized = platform.trim().toLowerCase()
  const template =
    normalized === 'openai' ? OPENAI_HEADER_OVERRIDE_TEMPLATE : ANTHROPIC_HEADER_OVERRIDE_TEMPLATE
  return template.map((row) => ({ ...row }))
}

/** 与后端 maxHeaderOverride* 常量保持一致 */
const HEADER_OVERRIDE_MAX_ENTRIES = 64
const HEADER_OVERRIDE_MAX_NAME_LENGTH = 200
const HEADER_OVERRIDE_MAX_VALUE_LENGTH = 8192

/** header value 不允许包含控制字符（与后端 httpguts.ValidHeaderFieldValue 对齐） */
// eslint-disable-next-line no-control-regex
const HEADER_VALUE_INVALID_PATTERN = /[\x00-\x08\x0a-\x1f\x7f]/

/** 长度限制按 UTF-8 字节计（与后端 Go len() 对齐，避免多字节值前端放行后端 400） */
const HEADER_TEXT_ENCODER = new TextEncoder()
function utf8ByteLength(value: string): number {
  return HEADER_TEXT_ENCODER.encode(value).length
}

/**
 * 校验请求头覆写行，返回首个错误的 i18n key（无错误返回 null）。
 * 名称为空但值非空 → invalidName；名称非法 → invalidName；
 * 禁止覆写 → blockedName；大小写不敏感重名 → duplicateName；
 * 值含控制字符或超长 → invalidValue；条目过多 → tooManyEntries。
 */
export function validateHeaderOverrideRows(
  rows: HeaderOverrideRow[]
): 'invalidName' | 'blockedName' | 'duplicateName' | 'invalidValue' | 'tooManyEntries' | null {
  const seen = new Set<string>()
  for (const row of rows) {
    const name = row.name.trim()
    const value = row.value.trim()
    if (!name) {
      if (value) return 'invalidName'
      continue
    }
    if (!isValidHeaderOverrideName(name) || name.length > HEADER_OVERRIDE_MAX_NAME_LENGTH) {
      return 'invalidName'
    }
    const lower = name.toLowerCase()
    if (HEADER_OVERRIDE_BLOCKED_NAMES.has(lower)) return 'blockedName'
    if (seen.has(lower)) return 'duplicateName'
    if (
      HEADER_VALUE_INVALID_PATTERN.test(value) ||
      utf8ByteLength(value) > HEADER_OVERRIDE_MAX_VALUE_LENGTH
    ) {
      return 'invalidValue'
    }
    seen.add(lower)
  }
  if (seen.size > HEADER_OVERRIDE_MAX_ENTRIES) return 'tooManyEntries'
  return null
}

/** 行数组 → credentials 存储对象（名称小写化，丢弃空行） */
export function buildHeaderOverridesObject(rows: HeaderOverrideRow[]): Record<string, string> {
  const result: Record<string, string> = {}
  for (const row of rows) {
    const name = row.name.trim().toLowerCase()
    if (!name) continue
    result[name] = row.value.trim()
  }
  return result
}

/** credentials 存储对象 → 行数组（按名称排序保证稳定展示） */
export function splitHeaderOverridesObject(record: unknown): HeaderOverrideRow[] {
  if (!record || typeof record !== 'object' || Array.isArray(record)) return []
  return Object.entries(record as Record<string, unknown>)
    .filter(([, value]) => typeof value === 'string')
    .map(([name, value]) => ({ name, value: value as string }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

/**
 * 将请求头覆写写入 credentials。
 * create 模式：关闭时不写入任何字段；edit 模式：关闭时删除字段（全量替换语义）。
 */
export function applyHeaderOverride(
  credentials: Record<string, unknown>,
  enabled: boolean,
  rows: HeaderOverrideRow[],
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY] = true
    credentials[HEADER_OVERRIDES_CREDENTIAL_KEY] = buildHeaderOverridesObject(rows)
  } else if (mode === 'edit') {
    delete credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]
    delete credentials[HEADER_OVERRIDES_CREDENTIAL_KEY]
  }
}
