import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  LOCALE_COOKIE_NAME,
  LOCALE_STORAGE_KEY,
  applyResolvedLocale,
  buildLocaleSwitchTarget,
  detectLocaleByIP,
  detectLocaleFromPath,
  initLocaleSync,
  persistLocale,
  resolveLocale
} from '../localeDetection'
import { availableLocales, normalizeLocaleCode } from '../registry'

describe('locale detection', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('accepts a supported locale returned by the server', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ code: 0, data: { locale: 'ru', country_code: 'RU' } })
      })
    )

    await expect(detectLocaleByIP()).resolves.toBe('ru')
  })

  it('falls back to English for an unsupported server locale', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ code: 0, data: { locale: 'es', country_code: 'ES' } })
      })
    )

    await expect(detectLocaleByIP()).resolves.toBe('en')
  })

  it('falls back to English when detection fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network unavailable')))

    await expect(detectLocaleByIP()).resolves.toBe('en')
  })

  it('falls back to English when detection times out', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => new Promise((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
      }))
    )

    const localePromise = detectLocaleByIP()
    await vi.advanceTimersByTimeAsync(2500)
    await expect(localePromise).resolves.toBe('en')
  })

  it('registers exactly the five enabled languages', () => {
    expect(availableLocales.map((locale) => locale.code)).toEqual(['en', 'zh', 'ru', 'fr', 'de'])
  })
})

// ==================== R12 语言检测 / cookie 双写 / 跨标签同步 / 切换器 ====================
describe('R12-01 无语言前缀由服务端 301 收口', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('首页与列表路径不含语言段', () => {
    expect(detectLocaleFromPath('/')).toBeNull()
    expect(detectLocaleFromPath('')).toBeNull()
    expect(detectLocaleFromPath('/news')).toBeNull()
    expect(detectLocaleFromPath('/articles')).toBeNull()
  })

  it('带前缀的首页/列表/详情都能识别出语言', () => {
    expect(detectLocaleFromPath('/en')).toBe('en')
    expect(detectLocaleFromPath('/zh/')).toBe('zh')
    expect(detectLocaleFromPath('/ru/news')).toBe('ru')
    expect(detectLocaleFromPath('/fr/articles')).toBe('fr')
    expect(detectLocaleFromPath('/news/de/some-slug')).toBe('de')
  })

  it('未知语言段不被当成语言', () => {
    expect(detectLocaleFromPath('/pricing')).toBeNull()
    expect(detectLocaleFromPath('/ja/news')).toBeNull()
  })
})

describe('R12-02 五语同源与检测优先级 URL > cookie > localStorage > 浏览器', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    document.cookie = `${LOCALE_COOKIE_NAME}=; path=/; max-age=0`
    window.localStorage.removeItem(LOCALE_STORAGE_KEY)
  })

  it('语言别名归一化到五个短码', () => {
    expect(normalizeLocaleCode('zh-CN')).toBe('zh')
    expect(normalizeLocaleCode('zh-TW')).toBe('zh')
    expect(normalizeLocaleCode('en-US')).toBe('en')
    expect(normalizeLocaleCode('ru-RU')).toBe('ru')
    expect(normalizeLocaleCode('fr-FR')).toBe('fr')
    expect(normalizeLocaleCode('de-DE')).toBe('de')
    expect(normalizeLocaleCode('ja-JP')).toBeNull()
    expect(normalizeLocaleCode(undefined)).toBeNull()
  })

  it('URL 优先级最高，压过 cookie 与 localStorage', () => {
    persistLocale('de')
    const resolution = resolveLocale('/fr/news')
    expect(resolution).toEqual({ locale: 'fr', source: 'url', storageCorrected: false })
  })

  it('无 URL 前缀时以 cookie 为准，并标记需要修正 localStorage', () => {
    document.cookie = `${LOCALE_COOKIE_NAME}=ru; path=/`
    window.localStorage.setItem(LOCALE_STORAGE_KEY, 'en')
    const resolution = resolveLocale('/news')
    expect(resolution).toEqual({ locale: 'ru', source: 'cookie', storageCorrected: true })
  })

  it('只有 localStorage 时使用 storage 来源', () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, 'fr')
    expect(resolveLocale('/')).toEqual({ locale: 'fr', source: 'storage', storageCorrected: false })
  })

  it('都没有时回落到浏览器语言', () => {
    vi.stubGlobal('navigator', { languages: ['de-AT', 'en-US'], language: 'de-AT' })
    expect(resolveLocale('/')).toEqual({ locale: 'de', source: 'browser', storageCorrected: false })
  })

  it('浏览器语言也不支持时回落到默认语言', () => {
    vi.stubGlobal('navigator', { languages: ['ja-JP'], language: 'ja-JP' })
    expect(resolveLocale('/')).toEqual({ locale: 'en', source: 'default', storageCorrected: false })
  })
})

describe('R12-03 cookie + localStorage 双写与跨标签同步', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    document.cookie = `${LOCALE_COOKIE_NAME}=; path=/; max-age=0`
    window.localStorage.removeItem(LOCALE_STORAGE_KEY)
  })

  it('persistLocale 双写 cookie 与 localStorage', () => {
    persistLocale('de')
    expect(document.cookie).toContain(`${LOCALE_COOKIE_NAME}=de`)
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe('de')
  })

  it('URL 来源落地时回写 cookie，cookie 来源不重复写', () => {
    applyResolvedLocale({ locale: 'ru', source: 'url', storageCorrected: false })
    expect(document.cookie).toContain(`${LOCALE_COOKIE_NAME}=ru`)
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe('ru')

    document.cookie = `${LOCALE_COOKIE_NAME}=; path=/; max-age=0`
    applyResolvedLocale({ locale: 'fr', source: 'cookie', storageCorrected: false })
    expect(document.cookie).not.toContain(`${LOCALE_COOKIE_NAME}=fr`)
  })

  it('storage 事件让其它标签跟随 cookie 权威值', () => {
    const seen: string[] = []
    const dispose = initLocaleSync((locale, source) => seen.push(`${locale}:${source}`))
    expect(seen).toEqual([]) // 首次无记忆，浏览器兜底不回调

    // 另一个标签切换语言：先写 cookie 再写 localStorage 并触发 storage 事件
    persistLocale('de')
    window.dispatchEvent(new StorageEvent('storage', { key: LOCALE_STORAGE_KEY, newValue: 'de' }))
    expect(seen).toEqual(['de:cookie'])

    dispose()
    persistLocale('fr')
    window.dispatchEvent(new StorageEvent('storage', { key: LOCALE_STORAGE_KEY, newValue: 'fr' }))
    expect(seen).toEqual(['de:cookie'])
  })

  it('无 cookie 时忽略不相关 key 的 storage 事件', () => {
    const seen: string[] = []
    const dispose = initLocaleSync((locale) => seen.push(locale))
    window.dispatchEvent(new StorageEvent('storage', { key: 'other_key', newValue: 'de' }))
    expect(seen).toEqual([])
    dispose()
  })
})

describe('R12-04 切换器保留当前页面与查询串', () => {
  it('首页与列表只替换语言段', () => {
    expect(buildLocaleSwitchTarget('/', '', 'ru')).toBe('/ru/')
    expect(buildLocaleSwitchTarget('/zh', '', 'en')).toBe('/en/')
    expect(buildLocaleSwitchTarget('/zh/', '', 'de')).toBe('/de/')
    expect(buildLocaleSwitchTarget('/zh/news', '?page=2', 'de')).toBe('/de/news?page=2')
    expect(buildLocaleSwitchTarget('/articles', '', 'fr')).toBe('/fr/articles')
  })

  it('详情页替换语言段并保留 slug 与查询串', () => {
    expect(buildLocaleSwitchTarget('/news/zh/some-slug', '', 'fr')).toBe('/news/fr/some-slug')
    expect(buildLocaleSwitchTarget('/articles/de/a-b?tab=x', '?tab=x', 'en')).toBe('/articles/en/a-b?tab=x')
  })

  it('非内容页不改变 URL', () => {
    expect(buildLocaleSwitchTarget('/pricing', '', 'de')).toBeNull()
    expect(buildLocaleSwitchTarget('/login', '?next=/x', 'ru')).toBeNull()
  })
})
