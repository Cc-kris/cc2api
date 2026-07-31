import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ default: { get, post } }))

import upstreamWalletsAPI from '@/api/admin/upstreamWallets'

describe('admin upstream wallet API', () => {
  beforeEach(() => { get.mockReset(); post.mockReset() })

  it('sends the caller-owned idempotency key for a manual fund event', async () => {
    post.mockResolvedValueOnce({ data: { id: 1 } })
    const payload = { event_type: 'topup' as const, original_amount: '10', currency: 'USD', fx_rate_to_usd: '1', usd_amount: '10', occurred_at: '2026-07-28T00:00:00Z', note: '人工充值记录' }
    await upstreamWalletsAPI.createFundEvent(21, payload, 'fund-event-key-1')
    expect(post).toHaveBeenCalledWith('/admin/upstream-wallets/21/fund-events', payload, { headers: { 'Idempotency-Key': 'fund-event-key-1' } })
  })

  it('loads manual fund events through the wallet-scoped endpoint', async () => {
    get.mockResolvedValueOnce({ data: { items: [{ id: 1, bonus_status: 'confirmed' }], total: 1, page: 1, page_size: 100 } })
    const result = await upstreamWalletsAPI.listFundEvents(21)
    expect(get).toHaveBeenCalledWith('/admin/upstream-wallets/21/fund-events', { params: { page: 1, page_size: 100 } })
    expect(result.items[0].bonus_status).toBe('confirmed')
  })

  it('loads published protocol versions for generic wallets', async () => {
    get.mockResolvedValueOnce({ data: { items: [{ id: 9, code: 'vendor_x', name: 'Vendor X', status: 'published', current_version_id: 91 }], total: 1, page: 1, page_size: 100 } })
    const result = await upstreamWalletsAPI.listPublishedProtocols()
    expect(get).toHaveBeenCalledWith('/admin/upstream-finance-protocols', { params: { status: 'published', page: 1, page_size: 100 } })
    expect(result[0].current_version_id).toBe(91)
  })

  it('enqueues a funding transaction sync for a protocol wallet', async () => {
    post.mockResolvedValueOnce({ data: { job: { id: 8, sync_type: 'funding' }, created: true } })
    const result = await upstreamWalletsAPI.sync(21, 'funding')
    expect(post).toHaveBeenCalledWith('/admin/upstream-wallets/21/sync-funding')
    expect(result.job.sync_type).toBe('funding')
  })

  it('enqueues account usage synchronization for automatic multiplier observation', async () => {
    post.mockResolvedValueOnce({ data: { job: { id: 9, sync_type: 'account_usage' }, created: true } })
    const result = await upstreamWalletsAPI.sync(21, 'account-usage')
    expect(post).toHaveBeenCalledWith('/admin/upstream-wallets/21/sync-account-usage')
    expect(result.job.sync_type).toBe('account_usage')
  })
})
