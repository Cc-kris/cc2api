import { describe, expect, it, vi } from 'vitest'
import {
  ANALYTICS_MAX_DURATION_MS,
  createContentAnalyticsTracker,
  referrerHostOf,
  type AnalyticsPageContext
} from '../contentAnalyticsV04'
import { postPublicAnalyticsEvent, type PublicAnalyticsEvent } from '@/api/publicAnalyticsV04'

const detailContext: AnalyticsPageContext = {
  contentType: 'article',
  contentId: 42,
  locale: 'en',
  pagePath: '/articles/hello',
  referrerHost: 'google.com'
}

function setup(send?: (event: PublicAnalyticsEvent) => unknown) {
  let clock = 1_000
  const events: PublicAnalyticsEvent[] = []
  let seq = 0
  const tracker = createContentAnalyticsTracker({
    now: () => clock,
    newId: () => `id-${++seq}`,
    heartbeatMs: 60 * 60 * 1000,
    send: send ?? ((event) => { events.push(event) })
  })
  return { tracker, events, advance: (ms: number) => { clock += ms }, setClock: (value: number) => { clock = value } }
}

const names = (events: PublicAnalyticsEvent[]) => events.map((event) => event.eventName)

describe('content analytics tracker', () => {
  it('keeps one view_id for page_view, heartbeats and page_leave', () => {
    const { tracker, events, advance } = setup()
    tracker.start(detailContext)
    advance(20_000)
    tracker.heartbeat()
    advance(5_000)
    tracker.leave()

    expect(names(events)).toEqual(['page_view', 'page_heartbeat', 'content_read', 'page_leave'])
    const viewIds = events.map((event) => event.payload?.view_id).filter(Boolean)
    expect(new Set(viewIds).size).toBe(1)
    expect(events[0].payload?.view_id).toBe(tracker.viewId)
    expect(events[0].contentId).toBe(42)
    expect(events[0].referrerHost).toBe('google.com')
  })

  it('reports a monotonically growing duration and flags reads at 10s', () => {
    const { tracker, events, advance, setClock } = setup()
    tracker.start(detailContext)
    advance(5_000)
    tracker.heartbeat()
    advance(20_000)
    tracker.heartbeat()
    setClock(0)
    advance(0)
    tracker.leave()

    const durations = events
      .filter((event) => event.payload && 'duration_ms' in event.payload)
      .map((event) => Number(event.payload?.duration_ms))
    expect(durations).toEqual([5_000, 25_000, 25_000])
    expect(events.filter((event) => event.eventName === 'content_read')).toHaveLength(1)
    expect(events.find((event) => event.eventName === 'content_read')?.payload?.read_seconds).toBe(25)
  })

  it('clamps absurd durations to the backend 4h ceiling', () => {
    const { tracker, events, advance } = setup()
    tracker.start(detailContext)
    advance(ANALYTICS_MAX_DURATION_MS * 3)
    tracker.leave()
    const leave = events.find((event) => event.eventName === 'page_leave')
    expect(leave?.payload?.duration_ms).toBe(ANALYTICS_MAX_DURATION_MS)
  })

  it('de-duplicates impressions per placement but counts every click', () => {
    const { tracker, events } = setup()
    tracker.start({ ...detailContext, contentType: 'page', pagePath: '/articles' })
    tracker.impression('articles_list', 1)
    tracker.impression('articles_list', 1)
    tracker.impression('articles_list', 2)
    tracker.click('articles_list', 1)
    tracker.click('articles_list', 1)

    expect(events.filter((event) => event.eventName === 'content_impression')).toHaveLength(2)
    expect(events.filter((event) => event.eventName === 'content_click')).toHaveLength(2)
    expect(events.find((event) => event.eventName === 'content_click')?.placement).toBe('articles_list')
  })

  it('sends page_leave only once and ignores later events', () => {
    const { tracker, events } = setup()
    tracker.start(detailContext)
    tracker.leave()
    tracker.leave()
    tracker.click('article_detail_related', 9)
    expect(events.filter((event) => event.eventName === 'page_leave')).toHaveLength(1)
    expect(events.filter((event) => event.eventName === 'content_click')).toHaveLength(0)
  })

  it('never throws nor retries when the transport fails', async () => {
    const failing = vi.fn(() => { throw new Error('offline') })
    const { tracker } = setup(failing)
    expect(() => {
      tracker.start(detailContext)
      tracker.click('home_hero', 1)
    }).not.toThrow()

    const rejecting = vi.fn(() => Promise.reject(new Error('offline')))
    const retried = setup(rejecting)
    retried.tracker.start(detailContext)
    for (let i = 0; i < 3; i += 1) retried.tracker.click('home_hero', 1)
    await Promise.resolve()
    await Promise.resolve()
    expect(rejecting).toHaveBeenCalledTimes(4)
  })

  it('caps concurrent sends instead of queueing a storm', () => {
    const pending = vi.fn(() => new Promise(() => {}))
    const { tracker } = setup(pending)
    tracker.start(detailContext)
    for (let i = 0; i < 10; i += 1) tracker.click('home_list', i)
    expect(pending.mock.calls.length).toBeLessThanOrEqual(4)
  })

  it('derives referrer hosts safely', () => {
    expect(referrerHostOf('https://news.example.com/a', 'app.example.com')).toBe('news.example.com')
    expect(referrerHostOf('https://app.example.com/a', 'app.example.com')).toBeUndefined()
    expect(referrerHostOf('not a url')).toBeUndefined()
    expect(referrerHostOf('')).toBeUndefined()
  })
})

describe('public analytics transport', () => {
  const event: PublicAnalyticsEvent = {
    eventId: 'ev-1',
    eventName: 'page_view',
    pagePath: '/articles/hello',
    eventAt: '2026-10-27T00:00:00.000Z',
    payload: { view_id: 'v-1' }
  }

  it('posts keepalive JSON to the public ingest endpoint', async () => {
    const fetchImpl = vi.fn(async () => ({ ok: true }))
    await expect(postPublicAnalyticsEvent(event, fetchImpl)).resolves.toBe(true)
    const [url, init] = fetchImpl.mock.calls[0] as unknown as [string, RequestInit]
    expect(url.endsWith('/public/analytics/events')).toBe(true)
    expect(init.keepalive).toBe(true)
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toMatchObject({ eventId: 'ev-1', pagePath: '/articles/hello' })
  })

  it('returns false instead of throwing when the network fails', async () => {
    const fetchImpl = vi.fn(async () => { throw new Error('offline') })
    await expect(postPublicAnalyticsEvent(event, fetchImpl)).resolves.toBe(false)
    await expect(postPublicAnalyticsEvent(event, null)).resolves.toBe(false)
  })
})
