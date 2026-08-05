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

export function getLocaleDefinition(locale: LocaleCode): LocaleDefinition {
  return localeDefinitionMap.get(locale) ?? localeDefinitionMap.get(DEFAULT_LOCALE)!
}

export const availableLocales = localeDefinitions.map(({ code, name }) => ({ code, name }))
