import { DEFAULT_LOCALE, isLocaleCode, type LocaleCode } from './registry'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'
const DETECTION_TIMEOUT_MS = 2500

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
    const locale = payload.data?.locale
    return locale && isLocaleCode(locale) ? locale : DEFAULT_LOCALE
  } catch {
    return DEFAULT_LOCALE
  } finally {
    window.clearTimeout(timeout)
  }
}
