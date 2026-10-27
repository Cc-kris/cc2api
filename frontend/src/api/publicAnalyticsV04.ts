/**
 * Anonymous BI ingestion contract (backend: POST /public/analytics/events).
 * The request is fire-and-forget: callers must never await it on the render
 * path, and a failure is reported as `false` instead of throwing.
 */
export interface PublicAnalyticsEvent {
  eventId: string
  eventName:
    | 'page_view'
    | 'page_heartbeat'
    | 'page_leave'
    | 'content_impression'
    | 'content_click'
    | 'content_read'
  contentType?: 'news' | 'article' | 'page'
  contentId?: number
  locale?: string
  placement?: string
  pagePath: string
  referrerHost?: string
  eventAt: string
  payload?: Record<string, string | number | boolean>
}

type FetchLike = (input: string, init: RequestInit) => Promise<{ ok?: boolean }>

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

export async function postPublicAnalyticsEvent(
  event: PublicAnalyticsEvent,
  fetchImpl: FetchLike | null = (globalThis as { fetch?: FetchLike }).fetch ?? null
): Promise<boolean> {
  if (typeof fetchImpl !== 'function') return false
  try {
    const response = await fetchImpl(`${API_BASE_URL}/public/analytics/events`, {
      method: 'POST',
      keepalive: true,
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(event)
    })
    return Boolean(response?.ok)
  } catch {
    return false
  }
}
