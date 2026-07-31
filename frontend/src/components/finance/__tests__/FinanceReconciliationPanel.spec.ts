import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import FinanceReconciliationPanel from '../FinanceReconciliationPanel.vue'

const api = vi.hoisted(() => ({ getReconciliations: vi.fn(), updateReconciliation: vi.fn(), importReconciliation: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { finance: api } }))

describe('FinanceReconciliationPanel', () => {
  beforeEach(() => {
    Object.values(api).forEach(mock => mock.mockReset())
    api.getReconciliations.mockResolvedValue({ items: [{ id: 1, wallet_id: 2, wallet_name: '主钱包', upstream_id: 3, upstream_name: '上游', period_start: '2026-07-01T00:00:00Z', period_end: '2026-08-01T00:00:00Z', upstream_bill_amount: '101', system_cost_amount: '100', difference_amount: '1', difference_rate: '0.01', currency: 'USD', status: 'difference', handled_at: null }], total: 1 })
    api.updateReconciliation.mockResolvedValue({ id: 1, status: 'confirmed' })
  })

  it('shows a difference and requires a handling note before status update', async () => {
    const wrapper = mount(FinanceReconciliationPanel)
    await flushPromises()
    expect(wrapper.text()).toContain('存在差额')
    expect(wrapper.text()).toContain('US$1.00')
    const save = wrapper.findAll('button').find(button => button.text() === '保存')!
    expect(save.attributes('disabled')).toBeDefined()
    await wrapper.get('input[placeholder="处理说明"]').setValue('已核对上游账单')
    await save.trigger('click')
    await flushPromises()
    expect(api.updateReconciliation).toHaveBeenCalledWith(1, { status: 'pending', note: '已核对上游账单' })
  })
})
