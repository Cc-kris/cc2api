import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FinanceFXRatePanel from '../FinanceFXRatePanel.vue'

const { getFXRates, createFXRate } = vi.hoisted(() => ({
  getFXRates: vi.fn(),
  createFXRate: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { finance: { getFXRates, createFXRate } },
}))

describe('FinanceFXRatePanel', () => {
  beforeEach(() => {
    getFXRates.mockReset().mockResolvedValue({ items: [{ id: 1, currency: 'CNY', rate_to_usd: '0.138', source: 'manual_admin', observed_at: '2026-07-30T00:00:00Z', effective_from: '2026-07-30T00:00:00Z', checksum: 'x', created_at: '2026-07-30T00:00:00Z' }], total: 1, page: 1, page_size: 100 })
    createFXRate.mockReset().mockResolvedValue({ id: 2 })
  })

  it('lists frozen FX versions', async () => {
    const wrapper = mount(FinanceFXRatePanel)
    await flushPromises()
    expect(wrapper.text()).toContain('CNY')
    expect(wrapper.text()).toContain('0.138')
    expect(wrapper.text()).toContain('manual_admin')
  })

  it('creates a normalized FX version and refreshes the list', async () => {
    const wrapper = mount(FinanceFXRatePanel)
    await flushPromises()
    await wrapper.get('button').trigger('click')
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('cny')
    await inputs[1].setValue('0.139')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(createFXRate).toHaveBeenCalledWith(expect.objectContaining({ currency: 'CNY', rate_to_usd: '0.139' }))
    expect(getFXRates).toHaveBeenCalledTimes(2)
  })
})
