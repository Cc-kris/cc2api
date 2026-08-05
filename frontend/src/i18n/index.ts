import { createI18n } from 'vue-i18n'
import { detectLocaleByIP } from './localeDetection'
import {
  DEFAULT_LOCALE,
  availableLocales,
  getLocaleDefinition,
  isLocaleCode,
  type LocaleCode,
  type LocaleMessages
} from './registry'

const LOCALE_KEY = 'sub2api_locale'

function getSavedLocale(): LocaleCode | null {
  const saved = localStorage.getItem(LOCALE_KEY)
  if (saved && isLocaleCode(saved)) {
    return saved
  }
  return null
}

export const i18n = createI18n({
  legacy: false,
  locale: getSavedLocale() ?? DEFAULT_LOCALE,
  fallbackLocale: DEFAULT_LOCALE,
  messages: {},
  // 禁用 HTML 消息警告 - 引导步骤使用富文本内容（driver.js 支持 HTML）
  // 这些内容是内部定义的，不存在 XSS 风险
  warnHtmlMessage: false
})

const loadedLocales = new Set<LocaleCode>()

export async function loadLocaleMessages(locale: LocaleCode): Promise<void> {
  if (loadedLocales.has(locale)) {
    return
  }

  const module = await getLocaleDefinition(locale).loader()
  i18n.global.setLocaleMessage(locale, module.default as LocaleMessages)
  loadedLocales.add(locale)
}

export async function initI18n(): Promise<void> {
  const savedLocale = getSavedLocale()
  const current = savedLocale ?? (await detectLocaleByIP())
  await loadLocaleMessages(current)
  i18n.global.locale.value = current
  localStorage.setItem(LOCALE_KEY, current)
  document.documentElement.setAttribute('lang', current)
}

export async function setLocale(locale: string): Promise<void> {
  if (!isLocaleCode(locale)) {
    return
  }

  await loadLocaleMessages(locale)
  i18n.global.locale.value = locale
  localStorage.setItem(LOCALE_KEY, locale)
  document.documentElement.setAttribute('lang', locale)

  // 同步更新浏览器页签标题，使其跟随语言切换
  const { resolveDocumentTitle } = await import('@/router/title')
  const { default: router } = await import('@/router')
  const { useAppStore } = await import('@/stores/app')
  const route = router.currentRoute.value
  const appStore = useAppStore()
  document.title = resolveDocumentTitle(route.meta.title, appStore.siteName, route.meta.titleKey as string)

  const { useAuthStore } = await import('@/stores/auth')
  const authStore = useAuthStore()
  if (authStore.user) {
    const { useAnnouncementStore } = await import('@/stores/announcements')
    void useAnnouncementStore().fetchAnnouncements(true)
  }
}

export function getLocale(): LocaleCode {
  const current = i18n.global.locale.value
  return isLocaleCode(current) ? current : DEFAULT_LOCALE
}

export { availableLocales, DEFAULT_LOCALE, isLocaleCode }
export type { LocaleCode }

export default i18n
