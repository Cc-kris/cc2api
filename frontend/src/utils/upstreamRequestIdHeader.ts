// V03-R11：账号 extra.upstream_request_id_header 的前端读写与校验，与后端
// ValidateUpstreamRequestIDHeaderExtra 规则一致（合法 HTTP 头字段名，≤ 64 字节）。
// 留空表示未配置，后端默认读 x-request-id。

export const UPSTREAM_REQUEST_ID_HEADER_EXTRA_KEY = 'upstream_request_id_header'
export const DEFAULT_UPSTREAM_REQUEST_ID_HEADER = 'x-request-id'
export const MAX_UPSTREAM_REQUEST_ID_HEADER_LENGTH = 64

// RFC 7230 token 字符。
const HEADER_FIELD_NAME_RE = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/

export function readUpstreamRequestIdHeader(extra: unknown): string {
  if (!extra || typeof extra !== 'object') return ''
  const value = (extra as Record<string, unknown>)[UPSTREAM_REQUEST_ID_HEADER_EXTRA_KEY]
  return typeof value === 'string' ? value.trim() : ''
}

/** 返回 true 表示头名合法（空值视为未配置，也合法）。 */
export function isValidUpstreamRequestIdHeader(raw: string): boolean {
  const name = (raw || '').trim()
  if (!name) return true
  return new TextEncoder().encode(name).length <= MAX_UPSTREAM_REQUEST_ID_HEADER_LENGTH && HEADER_FIELD_NAME_RE.test(name)
}

/**
 * 把头名写回 extra，返回新对象。空值删除键。
 * 若 extra 为空且头名也为空，返回原值，避免给不需要 extra 的请求凭空加字段。
 */
export function applyUpstreamRequestIdHeader<T extends Record<string, unknown> | undefined>(
  extra: T,
  raw: string
): T | Record<string, unknown> {
  const name = (raw || '').trim()
  if (!name) {
    if (!extra || !(UPSTREAM_REQUEST_ID_HEADER_EXTRA_KEY in extra)) return extra
    const next = { ...extra }
    delete next[UPSTREAM_REQUEST_ID_HEADER_EXTRA_KEY]
    return next
  }
  return { ...(extra || {}), [UPSTREAM_REQUEST_ID_HEADER_EXTRA_KEY]: name }
}
