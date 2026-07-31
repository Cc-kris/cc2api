import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import FinanceDataQualityPanel from '../FinanceDataQualityPanel.vue'

const api = vi.hoisted(() => ({
  getPromotionCreditReconciliations: vi.fn(),
  resolvePromotionCreditReconciliation: vi.fn()
}))
vi.mock('@/api/admin', () => ({ adminAPI: { finance: api } }))

const data = {
  quality: { status: 'partial', exact_count: 1, estimated_count: 0, missing_price_count: 0, missing_multiplier_count: 0, missing_usage_count: 0, non_billable_count: 0, unpriced_revenue: '0', cost_coverage_rate: '1' },
  trend: [], items: [], total: 0, page: 1, page_size: 100
}

describe('FinanceDataQualityPanel', () => {
  beforeEach(() => {
    api.getPromotionCreditReconciliations.mockReset().mockResolvedValue({
      items: [{ user_id: 8, user_email: 'finance@example.test', username: '财务客户', detected_historical_bonus: '12.5', current_remaining_amount: '9', confirmed_remaining_amount: null, status: 'requires_reconciliation', cutover_at: '2026-07-01T00:00:00Z', created_at: '2026-07-01T00:00:00Z', resolved_at: null, resolved_by: null }],
      total: 1, page: 1, page_size: 100
    })
    api.resolvePromotionCreditReconciliation.mockReset().mockResolvedValue({ user_id: 8, status: 'resolved' })
  })

  it('shows pending historical promotion credits and resolves them from the finance page', async () => {
    const wrapper = mount(FinanceDataQualityPanel, { props: { data: data as never } })
    await flushPromises()

    expect(wrapper.text()).toContain('历史优惠待核对')
    expect(wrapper.text()).toContain('财务客户')
    await wrapper.get('input[aria-label="确认剩余额度"]').setValue('2.5')
    await wrapper.get('input[aria-label="核对说明"]').setValue('已与客户台账核对')
    await wrapper.findAll('button').find(button => button.text() === '确认并入账')!.trigger('click')
    await flushPromises()

    expect(api.resolvePromotionCreditReconciliation).toHaveBeenCalledWith(8, { confirmed_remaining_amount: '2.5', note: '已与客户台账核对' })
    expect(api.getPromotionCreditReconciliations).toHaveBeenCalledTimes(2)
  })

  it('allows an administrator to confirm that no historical promotion credit remains', async () => {
    const wrapper = mount(FinanceDataQualityPanel, { props: { data: data as never } })
    await flushPromises()
    await wrapper.get('input[aria-label="确认剩余额度"]').setValue('0')
    await wrapper.get('input[aria-label="核对说明"]').setValue('客户已使用完毕')
    const button = wrapper.findAll('button').find(item => item.text() === '确认并入账')!
    expect(button.attributes('disabled')).toBeUndefined()
    await button.trigger('click')
    await flushPromises()
    expect(api.resolvePromotionCreditReconciliation).toHaveBeenCalledWith(8, { confirmed_remaining_amount: '0', note: '客户已使用完毕' })
  })
})
