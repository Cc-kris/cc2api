import type { PublicAnalyticsEvent } from '@/api/publicAnalyticsV04'

/** Backend rejects durations outside [0, 4h]; clamp locally so a bad clock
 * cannot cost us the whole view (R11-02). */
export const ANALYTICS_MAX_DURATION_MS = 4 * 60 * 60 * 1000
/** A "read" is a view that stayed at least 10s (PRD R11 dimension 1). */
export const ANALYTICS_READ_THRESHOLD_MS = 10_000
const DEFAULT_HEARTBEAT_MS = 15_000
const MAX_IN_FLIGHT = 4

export type AnalyticsContentType = 'news' | 'article' | 'page'
export type AnalyticsEventName = PublicAnalyticsEvent['eventName']

export interface AnalyticsPageContext {
  contentType: AnalyticsContentType
  contentId?: number
  locale?: string
  pagePath: string
  referrerHost?: string
}

export interface ContentAnalyticsOptions {
  send: (event: PublicAnalyticsEvent) => unknown
  now?: () => number
  newId?: () => string
  heartbeatMs?: number
  readThresholdMs?: number
}

export interface ContentAnalyticsTracker {
  readonly viewId: string
  start(context: AnalyticsPageContext): void
  impression(placement: string, contentId?: number): void
  click(placement: string, contentId?: number): void
  heartbeat(): void
  pause(): void
  resume(): void
  leave(): void
  dispose(): void
}

export function referrerHostOf(referrer: string, selfHost = ''): string | undefined {
  if (!referrer) return undefined
  try {
    const host = new URL(referrer).host
    return host && host !== selfHost ? host : undefined
  } catch {
    return undefined
  }
}

/**
 * Page-scoped BI tracker. Every send is fire-and-forget; a failing transport
 * is dropped (no retry storm, R11-06) and never throws into the click path.
 * One tracker == one view: `view_id` is stable for all heartbeat/leave events
 * so the backend can keep the maximum valid duration per view (R11-02/03).
 */
export function createContentAnalyticsTracker(options: ContentAnalyticsOptions): ContentAnalyticsTracker {
  const now = options.now ?? (() => Date.now())
  const newId = options.newId ?? (() => `${now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`)
  const heartbeatMs = options.heartbeatMs ?? DEFAULT_HEARTBEAT_MS
  const readThresholdMs = options.readThresholdMs ?? ANALYTICS_READ_THRESHOLD_MS

  let viewId = newId()
  let context: AnalyticsPageContext | null = null
  let activeSince: number | null = null
  let accumulated = 0
  let lastDuration = 0
  let readSent = false
  let left = false
  let timer: ReturnType<typeof setInterval> | null = null
  let inFlight = 0
  const seen = new Set<string>()

  const elapsed = () => {
    const live = activeSince === null ? 0 : Math.max(0, now() - activeSince)
    lastDuration = Math.min(ANALYTICS_MAX_DURATION_MS, Math.max(lastDuration, accumulated + live))
    return lastDuration
  }

  const emit = (
    eventName: AnalyticsEventName,
    payload: Record<string, string | number | boolean>,
    placement?: string,
    contentId?: number
  ) => {
    if (!context || left || inFlight >= MAX_IN_FLIGHT) return
    const event: PublicAnalyticsEvent = {
      eventId: `ev-${newId()}`,
      eventName,
      contentType: context.contentType,
      pagePath: context.pagePath,
      eventAt: new Date(now()).toISOString(),
      payload
    }
    if (contentId ?? context.contentId) event.contentId = contentId ?? context.contentId
    if (context.locale) event.locale = context.locale
    if (context.referrerHost) event.referrerHost = context.referrerHost
    if (placement) event.placement = placement
    try {
      const result = options.send(event)
      if (result && typeof (result as Promise<unknown>).then === 'function') {
        inFlight += 1
        void (result as Promise<unknown>).then(
          () => { inFlight -= 1 },
          () => { inFlight -= 1 }
        )
      }
    } catch {
      // Analytics must never break interaction.
    }
  }

  const stopTimer = () => {
    if (timer !== null) clearInterval(timer)
    timer = null
  }

  const heartbeat = () => {
    if (!context || left) return
    emit('page_heartbeat', { view_id: viewId, duration_ms: elapsed() })
    if (!readSent && lastDuration >= readThresholdMs) {
      readSent = true
      emit('content_read', {
        read_seconds: Math.round(lastDuration / 1000),
        section: context.contentType === 'page' ? 'list' : 'detail'
      })
    }
  }

  const tracker: ContentAnalyticsTracker = {
    get viewId() {
      return viewId
    },
    start(next) {
      tracker.leave()
      viewId = newId()
      context = { ...next }
      accumulated = 0
      activeSince = now()
      lastDuration = 0
      readSent = false
      left = false
      seen.clear()
      emit('page_view', { view_id: viewId })
      stopTimer()
      timer = setInterval(heartbeat, heartbeatMs)
    },
    impression(placement, contentId) {
      if (!context || left) return
      const key = `${placement}:${contentId ?? context.contentId ?? ''}`
      if (seen.has(key)) return
      seen.add(key)
      emit('content_impression', { view_id: viewId, position: placement }, placement, contentId)
    },
    click(placement, contentId) {
      // Clicks are counted per occurrence: repeating a click is a real signal.
      emit('content_click', { target: placement, position: placement }, placement, contentId)
    },
    heartbeat,
    pause() {
      if (activeSince === null) return
      accumulated = elapsed()
      activeSince = null
    },
    resume() {
      if (!context || left || activeSince !== null) return
      activeSince = now()
    },
    leave() {
      if (!context || left) return
      const duration = elapsed()
      const payload: Record<string, string | number | boolean> = {
        view_id: viewId,
        duration_ms: duration
      }
      if (!readSent && duration >= readThresholdMs) {
        readSent = true
        emit('content_read', {
          read_seconds: Math.round(duration / 1000),
          section: context.contentType === 'page' ? 'list' : 'detail'
        })
      }
      emit('page_leave', payload)
      left = true
      activeSince = null
      stopTimer()
    },
    dispose() {
      stopTimer()
      context = null
      left = true
    }
  }
  return tracker
}
