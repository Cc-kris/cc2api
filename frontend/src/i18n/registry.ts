export type LocaleCode = 'en' | 'zh' | 'ru' | 'fr' | 'de'

export type LocaleMessages = { [key: string]: string | LocaleMessages }

export interface LocaleDefinition {
  code: LocaleCode
  name: string
  intlLocale: string
  loader: () => Promise<{ default: LocaleMessages }>
}

export const DEFAULT_LOCALE: LocaleCode = 'en'

export const localeDefinitions: readonly LocaleDefinition[] = [
  { code: 'en', name: 'English', intlLocale: 'en-US', loader: () => import('./locales/en') },
  { code: 'zh', name: '中文', intlLocale: 'zh-CN', loader: () => import('./locales/zh') },
  { code: 'ru', name: 'Русский', intlLocale: 'ru-RU', loader: () => import('./locales/ru') },
  { code: 'fr', name: 'Français', intlLocale: 'fr-FR', loader: () => import('./locales/fr') },
  { code: 'de', name: 'Deutsch', intlLocale: 'de-DE', loader: () => import('./locales/de') }
] as const

const localeDefinitionMap = new Map(localeDefinitions.map((definition) => [definition.code, definition]))

export function isLocaleCode(value: string): value is LocaleCode {
  return localeDefinitionMap.has(value as LocaleCode)
}

// URL 前缀、cookie、localStorage 与浏览器语言可能带地区后缀（zh-CN、en-US…），
// 统一归一化到短码，保证四处记忆与五语清单始终同源（R12-02/R12-03）。
const localeAliases: Record<string, LocaleCode> = {
  'zh-cn': 'zh',
  'zh-hans': 'zh',
  'zh-sg': 'zh',
  'zh-tw': 'zh',
  'zh-hk': 'zh',
  'zh-hant': 'zh',
  'en-us': 'en',
  'en-gb': 'en',
  'en-au': 'en',
  'en-ca': 'en',
  'ru-ru': 'ru',
  'fr-fr': 'fr',
  'fr-ca': 'fr',
  'de-de': 'de',
  'de-at': 'de',
  'de-ch': 'de'
}

export function normalizeLocaleCode(value: string | null | undefined): LocaleCode | null {
  const normalized = String(value ?? '').trim().replace(/_/g, '-').toLowerCase()
  if (!normalized) {
    return null
  }
  if (isLocaleCode(normalized)) {
    return normalized
  }
  return localeAliases[normalized] ?? localeAliases[normalized.split('-')[0]] ?? null
}

export function getLocaleDefinition(locale: LocaleCode): LocaleDefinition {
  return localeDefinitionMap.get(locale) ?? localeDefinitionMap.get(DEFAULT_LOCALE)!
}

export const availableLocales = localeDefinitions.map(({ code, name }) => ({ code, name }))
