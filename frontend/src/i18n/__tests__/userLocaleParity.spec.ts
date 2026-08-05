import { describe, expect, it } from 'vitest'
import en from '../locales/en'
import de from '../locales/de'
import fr from '../locales/fr'
import ru from '../locales/ru'

const USER_SCOPES = [
  'common',
  'nav',
  'auth',
  'dashboard',
  'groups',
  'keys',
  'videoGeneration',
  'usage',
  'monitorCommon',
  'channelStatus',
  'availableChannels',
  'modelSquare',
  'affiliate',
  'redeem',
  'profile',
  'empty',
  'table',
  'pagination',
  'errors',
  'dates',
  'subscriptionProgress',
  'version',
  'purchase',
  'customPage',
  'announcements',
  'userSubscriptions',
  'payment'
] as const

function flattenMessages(value: unknown, prefix = ''): Record<string, string> {
  if (typeof value === 'string') {
    return { [prefix]: value }
  }

  const result: Record<string, string> = {}
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
    Object.assign(result, flattenMessages(child, prefix ? `${prefix}.${key}` : key))
  }
  return result
}

function userMessages(locale: Record<string, unknown>): Record<string, string> {
  const selected = Object.fromEntries(USER_SCOPES.map((scope) => [scope, locale[scope]]))
  return flattenMessages(selected)
}

function placeholders(value: string): string[] {
  return [...value.matchAll(/\{[^{}]+\}/g)].map((match) => match[0]).sort()
}

describe('user locale resources', () => {
  const source = userMessages(en)

  for (const [localeCode, locale] of Object.entries({ de, fr, ru })) {
    it(`${localeCode} has the same user-facing keys and placeholders as English`, () => {
      const translated = userMessages(locale)
      expect(Object.keys(translated).sort()).toEqual(Object.keys(source).sort())

      for (const key of Object.keys(source)) {
        expect(translated[key], `${localeCode}:${key}`).toBeTruthy()
        expect(placeholders(translated[key]), `${localeCode}:${key}`).toEqual(placeholders(source[key]))
        expect(translated[key], `${localeCode}:${key}`).not.toMatch(/ZXQ(?:PH|SPLIT)/)
      }
    })
  }
})
