import { describe, expect, it } from 'vitest'
import { availableLocales, i18n, loadLocaleMessages } from '..'
import contentV04Messages from '../locales/contentV04'
import type { LocaleCode, LocaleMessages } from '../registry'

function flatten(value: unknown, prefix = ''): Record<string, string> {
  if (typeof value === 'string') return { [prefix]: value }
  return Object.entries(value as Record<string, unknown>).reduce<Record<string, string>>(
    (all, [key, child]) => Object.assign(all, flatten(child, prefix ? `${prefix}.${key}` : key)),
    {}
  )
}

const locales = Object.keys(contentV04Messages) as LocaleCode[]
const reference = flatten(contentV04Messages.en)

describe('content v0.4 locale pack', () => {
  it('provides the same non-empty keys in all five languages', () => {
    for (const locale of locales) {
      const messages = flatten(contentV04Messages[locale])
      expect(Object.keys(messages).sort(), locale).toEqual(Object.keys(reference).sort())
      for (const [key, text] of Object.entries(messages)) {
        expect(text.trim(), `${locale}:${key}`).not.toBe('')
        expect(text, `${locale}:${key}`).not.toMatch(/ZXQ(?:PH|SPLIT)/)
      }
    }
  })

  it('covers AI setup guidance, ten analytics dimensions, and localized placements', () => {
    for (const locale of locales) {
      const pack = contentV04Messages[locale].contentV04 as LocaleMessages
      const ai = pack.ai as Record<string, string>
      const analytics = pack.analytics as Record<string, unknown>
      expect(ai.configureFirst).toBeTruthy()
      expect(ai.noDefault).toBeTruthy()
      expect(analytics).toHaveProperty('trend')
      expect(analytics).toHaveProperty('articleTop')
      expect(analytics).toHaveProperty('newsTop')
      expect(analytics).toHaveProperty('placementCtr')
      expect(analytics).toHaveProperty('category')
      expect(analytics).toHaveProperty('language')
      expect(analytics).toHaveProperty('source')
      expect(analytics).toHaveProperty('referrer')
      expect(analytics).toHaveProperty('duration')
      expect(analytics).toHaveProperty('publishing')
      expect(analytics.locations).toBeTruthy()
    }
  })

  it('keeps the locale switcher list sourced from the locale registry', () => {
    expect(availableLocales.map(({ code }) => code)).toEqual(locales)
    expect(Object.keys(contentV04Messages)).toEqual(availableLocales.map(({ code }) => code))
  })

  it('merges the domain pack into the lazily loaded locale without replacing existing messages', async () => {
    await loadLocaleMessages('en')
    const messages = i18n.global.getLocaleMessage('en') as LocaleMessages
    const content = messages.contentV04 as LocaleMessages
    expect((content.ai as Record<string, string>).configureFirst).toBe('Please configure a model first')
    expect(((content.analytics as LocaleMessages).locations as Record<string, string>).home).toBe('Home')
    expect(i18n.global.te('home.login', 'en')).toBe(true)
  })
})
