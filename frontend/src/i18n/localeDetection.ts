import {
  DEFAULT_LOCALE,
  isLocaleCode,
  normalizeLocaleCode,
  type LocaleCode
} from './registry'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'
const DETECTION_TIMEOUT_MS = 2500

// 语言记忆：cookie 供 SSR 首屏读取（path=/ 且非 HttpOnly），localStorage 负责跨标签同步。
// 见技术方案 R12：优先级 URL > cookie > localStorage > 浏览器。
export const LOCALE_COOKIE_NAME = 'galley_locale'
export const LOCALE_STORAGE_KEY = 'sub2api_locale'
const LOCALE_COOKIE_MAX_AGE = 31536000

export type LocaleSource = 'url' | 'cookie' | 'storage' | 'browser' | 'default'

export interface LocaleResolution {
  locale: LocaleCode
  source: LocaleSource
  /** cookie 与 localStorage 不一致时以 cookie 为准，并回写修正 localStorage */
  storageCorrected: boolean
}

interface LocaleDetectionResponse {
  code?: number
  data?: {
    locale?: string
  }
}

export async function detectLocaleByIP(): Promise<LocaleCode> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), DETECTION_TIMEOUT_MS)
  try {
    const response = await fetch(`${API_BASE_URL}/settings/locale`, {
      credentials: 'include',
      headers: { Accept: 'application/json' },
      signal: controller.signal
    })
    if (!response.ok) {
      return DEFAULT_LOCALE
    }

    const payload = (await response.json()) as LocaleDetectionResponse
    return normalizeLocaleCode(payload.data?.locale) ?? DEFAULT_LOCALE
  } catch {
    return DEFAULT_LOCALE
  } finally {
    window.clearTimeout(timeout)
  }
}

function readCookie(name: string): string | null {
  if (typeof document === 'undefined' || !document.cookie) {
    return null
  }
  const prefix = `${name}=`
  for (const part of document.cookie.split(';')) {
    const entry = part.trim()
    if (entry.startsWith(prefix)) {
      return decodeURIComponent(entry.slice(prefix.length))
    }
  }
  return null
}

export function readCookieLocale(): LocaleCode | null {
  return normalizeLocaleCode(readCookie(LOCALE_COOKIE_NAME))
}

export function readStoredLocale(): LocaleCode | null {
  try {
    return normalizeLocaleCode(window.localStorage.getItem(LOCALE_STORAGE_KEY))
  } catch {
    return null
  }
}

function pathSegments(pathname: string): string[] {
  return String(pathname || '').split('?')[0].split('/').filter(Boolean)
}

/**
 * 读取 URL 里的语言：列表/首页为首段（`/ru/news`），详情为第二段（`/news/ru/slug`）。
 * 无语言（`/`、`/news`）返回 null，由后端无条件 301 到 `/zh/...` 收口（R12-01）。
 */
export function detectLocaleFromPath(pathname: string): LocaleCode | null {
  const segments = pathSegments(pathname)
  const prefix = normalizeLocaleCode(segments[0])
  if (prefix) {
    return prefix
  }
  if ((segments[0] === 'news' || segments[0] === 'articles') && segments.length > 2) {
    return normalizeLocaleCode(segments[1])
  }
  return null
}

export function detectBrowserLocale(): LocaleCode | null {
  if (typeof navigator === 'undefined') {
    return null
  }
  const candidates = navigator.languages?.length ? navigator.languages : [navigator.language]
  for (const candidate of candidates) {
    const locale = normalizeLocaleCode(candidate)
    if (locale) {
      return locale
    }
  }
  return null
}

export function resolveLocale(
  pathname: string = typeof window !== 'undefined' ? window.location.pathname : ''
): LocaleResolution {
  const fromUrl = detectLocaleFromPath(pathname)
  if (fromUrl) {
    return { locale: fromUrl, source: 'url', storageCorrected: false }
  }

  const cookieLocale = readCookieLocale()
  const storedLocale = readStoredLocale()
  if (cookieLocale) {
    return {
      locale: cookieLocale,
      source: 'cookie',
      storageCorrected: Boolean(storedLocale) && storedLocale !== cookieLocale
    }
  }
  if (storedLocale) {
    return { locale: storedLocale, source: 'storage', storageCorrected: false }
  }

  const browserLocale = detectBrowserLocale()
  return browserLocale
    ? { locale: browserLocale, source: 'browser', storageCorrected: false }
    : { locale: DEFAULT_LOCALE, source: 'default', storageCorrected: false }
}

/** cookie + localStorage 双写；URL 前缀与手动切换都走这里，保证 SSR 能读到同一语言。 */
export function persistLocale(locale: LocaleCode): void {
  if (isLocaleCode(locale) && typeof document !== 'undefined') {
    document.cookie = `${LOCALE_COOKIE_NAME}=${locale}; path=/; max-age=${LOCALE_COOKIE_MAX_AGE}; SameSite=Lax`
  }
  try {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  } catch {
    // 隐私模式下 localStorage 不可写，cookie 仍是主记忆
  }
}

/** 落地一次解析结果：URL 优先与 cookie 修正都需要立即双写。 */
export function applyResolvedLocale(resolution: LocaleResolution): LocaleCode {
  if (resolution.source === 'url' || resolution.storageCorrected) {
    persistLocale(resolution.locale)
  }
  return resolution.locale
}

/**
 * 计算切换语言后的目标地址：保留当前页面（首页/列表/详情）与查询串，只替换语言段（R12-04）。
 * 非内容页返回 null，表示只切换界面语言、不改 URL。
 */
export function buildLocaleSwitchTarget(
  pathname: string,
  search: string,
  locale: LocaleCode
): string | null {
  const raw = String(pathname || '/').split('?')[0]
  const suffix = search ? (search.startsWith('?') ? search : `?${search}`) : ''
  const segments = pathSegments(raw).map((segment) => decodeURIComponent(segment))

  // 已带前缀（`/de/...`）：仅替换首段语言；首页保持规范写法 `/<lang>/`
  if (segments.length > 0 && isLocaleCode(segments[0])) {
    if (segments.length === 1) {
      return `/${locale}/${suffix}`
    }
    segments[0] = locale
    return `/${segments.join('/')}${suffix}`
  }
  if (segments.length === 0) {
    return `/${locale}/${suffix}`
  }

  const [kind] = segments
  if (kind !== 'news' && kind !== 'articles') {
    return null
  }
  // 列表 /news、/articles → /<lang>/news；详情 /news/<locale>/<slug> → /news/<lang>/<slug>
  if (segments.length > 2 && isLocaleCode(segments[1])) {
    return `/${kind}/${locale}/${segments.slice(2).join('/')}${suffix}`
  }
  return `/${locale}/${kind}${suffix}`
}

/**
 * 跨标签语言同步：storage 事件只给出另一个标签写入的 localStorage，
 * 但 cookie 才是权威值（可能同时被其它标签更新），因此每次都以解析结果为准。
 */
export function initLocaleSync(
  onLocale: (locale: LocaleCode, source: LocaleSource) => void
): () => void {
  if (typeof window === 'undefined') {
    return () => {}
  }

  const apply = (resolution: LocaleResolution) => {
    const locale = applyResolvedLocale(resolution)
    // 浏览器语言只是兜底，交给既有 initI18n（IP 识别）处理，避免覆盖服务端判定
    if (resolution.source !== 'browser' && resolution.source !== 'default') {
      onLocale(locale, resolution.source)
    }
  }

  apply(resolveLocale())

  const handleStorage = (event: StorageEvent) => {
    if (event.key !== LOCALE_STORAGE_KEY) {
      return
    }
    apply(resolveLocale(window.location.pathname))
  }

  window.addEventListener('storage', handleStorage)
  return () => window.removeEventListener('storage', handleStorage)
}
