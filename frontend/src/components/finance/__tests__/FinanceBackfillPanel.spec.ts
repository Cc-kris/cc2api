import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import FinanceBackfillPanel from '../FinanceBackfillPanel.vue'

const api = vi.hoisted(() => ({ previewBackfill: vi.fn(), runBackfill: vi.fn(), getBackfill: vi.fn(), pauseBackfill: vi.fn(), resumeBackfill: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { finance: api } }))

describe('FinanceBackfillPanel', () => {
  beforeEach(() => {
    Object.values(api).forEach(mock => mock.mockReset())
    api.previewBackfill.mockResolvedValue({ estimated_records: 10, exact_repairable: 8, estimated_only: 1, unrepairable: 1, blockers: [], preview_token: 'signed', expires_at: '2026-07-27T09:00:00Z' })
    api.runBackfill.mockResolvedValue({ job_id: 7, status: 'queued', progress: '0', processed_count: 0, success_count: 0, failed_count: 0, estimated_total: 10 })
  })

  it('requires preview before starting a historical-only job', async () => {
    const wrapper = mount(FinanceBackfillPanel, { props: { startDate: '2026-01-01', endDate: '2026-07-27' } })
    await wrapper.get('input').setValue('补齐历史价格')
    await wrapper.findAll('button').find(button => button.text() === '预览回算')!.trigger('click')
    await flushPromises()
    expect(api.previewBackfill).toHaveBeenCalledWith(expect.objectContaining({ pricing_policy: 'historical_only', reason: '补齐历史价格' }))
    expect(wrapper.text()).toContain('可精确修复')

    await wrapper.findAll('button').find(button => button.text() === '启动回算')!.trigger('click')
    await flushPromises()
    expect(api.runBackfill).toHaveBeenCalledWith(expect.objectContaining({ preview_token: 'signed' }))
    expect(wrapper.text()).toContain('任务 #7')
  })
})
