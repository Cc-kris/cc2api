import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ default: { get, post, put } }))

import financeAPI from '@/api/admin/finance'

const params = { start_date: '2026-07-01', end_date: '2026-07-27', timezone: 'Asia/Shanghai', granularity: 'day' as const }

describe('admin finance API', () => {
  beforeEach(() => { get.mockReset(); post.mockReset(); put.mockReset() })

  it('uses the finance endpoints and preserves the unified filter object', async () => {
    get.mockResolvedValueOnce({ data: { items: null, total: 0, page: 1, page_size: 100 } })
    const result = await financeAPI.getLosses({ ...params, page: 1, page_size: 100 })
    expect(get).toHaveBeenCalledWith('/admin/finance/losses', { params: { ...params, page: 1, page_size: 100 } })
    expect(result.items).toBeNull()
  })

  it('normalizes a null trend list to an empty list', async () => {
    get.mockResolvedValueOnce({ data: { items: null } })
    await expect(financeAPI.getTrend(params)).resolves.toEqual([])
  })

  it('updates an alert with status and handling note', async () => {
    put.mockResolvedValueOnce({ data: { id: 9, status: 'resolved' } })
    await financeAPI.updateAlert(9, { status: 'resolved', note: '已完成核对' })
    expect(put).toHaveBeenCalledWith('/admin/finance/alerts/9', { status: 'resolved', note: '已完成核对' })
  })

  it('lists and resolves historical promotion credit reconciliations', async () => {
    get.mockResolvedValueOnce({ data: { items: [], total: 0, page: 1, page_size: 100 } })
    await financeAPI.getPromotionCreditReconciliations({ status: 'requires_reconciliation', page: 1, page_size: 100 })
    expect(get).toHaveBeenCalledWith('/admin/finance/promotion-credit-reconciliations', { params: { status: 'requires_reconciliation', page: 1, page_size: 100 } })

    post.mockResolvedValueOnce({ data: { user_id: 8, status: 'resolved' } })
    await financeAPI.resolvePromotionCreditReconciliation(8, { confirmed_remaining_amount: '2.5', note: '已与客户记录核对' })
    expect(post).toHaveBeenCalledWith('/admin/finance/promotion-credit-reconciliations/8/resolve', { confirmed_remaining_amount: '2.5', note: '已与客户记录核对' })
  })

  it('lists, retries, and reallocates settlement intervals with revision protection', async () => {
    get.mockResolvedValueOnce({ data: { items: [], total: 0, page: 1, page_size: 20 } })
    await financeAPI.getSettlements({ status: 'needs_review', account_id: 12, page: 1, page_size: 20 })
    expect(get).toHaveBeenCalledWith('/admin/finance/settlements', { params: { status: 'needs_review', account_id: 12, page: 1, page_size: 20 } })

    post.mockResolvedValueOnce({ data: { interval: { id: 7, status: 'settled' }, allocations: [] } })
    await financeAPI.retrySettlement(7)
    expect(post).toHaveBeenCalledWith('/admin/finance/settlements/7/retry')

    post.mockResolvedValueOnce({ data: { interval: { id: 7, current_revision: 3 }, allocations: [] } })
    await financeAPI.reallocateSettlement(7, { expected_revision: 2, reason: '修正标准成本权重' })
    expect(post).toHaveBeenCalledWith('/admin/finance/settlements/7/reallocate', { expected_revision: 2, reason: '修正标准成本权重' })
  })
})
