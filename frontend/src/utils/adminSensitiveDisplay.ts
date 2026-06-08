const OPS_ROLES = new Set(['ops', 'operation', 'operator', 'operations'])
const SUPPORT_ROLES = new Set(['customer_service', 'customer-service', 'customerservice', 'support'])
const BUSINESS_ROLES = new Set(['business', 'business_operator', 'business-operator', '运营', 'yunying'])

export function normalizeAdminRole(role: string | null | undefined): string {
  return String(role || '').trim().toLowerCase()
}

export function isPrivilegedAdminRole(role: string | null | undefined): boolean {
  const normalized = normalizeAdminRole(role)
  return normalized === '' || normalized === 'admin' || OPS_ROLES.has(normalized)
}

export function isSupportAdminRole(role: string | null | undefined): boolean {
  return SUPPORT_ROLES.has(normalizeAdminRole(role))
}

export function isBusinessAdminRole(role: string | null | undefined): boolean {
  return BUSINESS_ROLES.has(normalizeAdminRole(role))
}

export function maskEmailAddress(email: string | null | undefined): string {
  const value = String(email || '').trim()
  if (!value) return ''
  const atIndex = value.indexOf('@')
  if (atIndex <= 0) return value.length <= 1 ? '*' : `${value[0]}***`
  return `${value[0]}***${value.slice(atIndex)}`
}

export function maskUpstreamAccountName(name: string | null | undefined): string {
  const value = String(name || '').trim()
  if (!value) return ''
  if (value.length <= 2) return `${value[0] || '*'}*`
  if (value.length <= 4) return `${value.slice(0, 1)}**${value.slice(-1)}`
  return `${value.slice(0, 2)}***${value.slice(-2)}`
}

export function formatUserEmailForRole(email: string | null | undefined, role: string | null | undefined): string {
  const value = String(email || '').trim()
  if (!value) return ''
  return isSupportAdminRole(role) ? maskEmailAddress(value) : value
}

export function formatUpstreamAccountForRole(name: string | null | undefined, role: string | null | undefined): string {
  const value = String(name || '').trim()
  if (!value) return ''
  if (isPrivilegedAdminRole(role)) return value
  return maskUpstreamAccountName(value)
}

export function formatApiKeyOptionLabel(name: string | null | undefined, id: number | null | undefined): string {
  const normalizedId = typeof id === 'number' && Number.isFinite(id) ? id : 0
  const value = String(name || '').trim()
  if (value && normalizedId > 0) return `${value} · #${normalizedId}`
  if (value) return value
  if (normalizedId > 0) return `#${normalizedId}`
  return ''
}

export function redactSensitivePreview(value: string | null | undefined, maxLength = 500): string {
  const text = String(value || '').trim()
  if (!text) return ''
  const truncated = text.length > maxLength ? text.slice(0, maxLength) : text
  return truncated
    .replace(/\b([A-Za-z0-9._%+-])[A-Za-z0-9._%+-]*@([A-Za-z0-9.-]+\.[A-Za-z]{2,})\b/g, '$1***@$2')
    .replace(/((?:authorization|proxy-authorization)\s*[:=]\s*)(bearer\s+)?[^\s",'`]+/gi, '$1$2***')
    .replace(/((?:api[_-]?key|token|session[_-]?key|cookie|secret|password)\s*[:=]\s*)[^\s",'`]+/gi, '$1***')
}
