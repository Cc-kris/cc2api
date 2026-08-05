import { afterEach, describe, expect, it, vi } from 'vitest'
import { detectLocaleByIP } from '../localeDetection'
import { availableLocales } from '../registry'

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
