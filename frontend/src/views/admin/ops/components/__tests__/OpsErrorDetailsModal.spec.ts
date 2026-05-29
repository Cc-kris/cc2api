import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import OpsErrorDetailsModal from '../OpsErrorDetailsModal.vue'
import { opsAPI } from '@/api/admin/ops'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listRequestErrors: vi.fn(),
    listUpstreamErrors: vi.fn(),
  },
}))

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /></div>',
})

const SelectStub = defineComponent({
  name: 'Select',
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: null },
    options: { type: Array, default: () => [] },
  },
  emits: ['update:modelValue'],
  template: '<select />',
})

const OpsErrorLogTableStub = defineComponent({
  name: 'OpsErrorLogTable',
  props: {
    rows: { type: Array, default: () => [] },
    total: { type: Number, default: 0 },
    loading: { type: Boolean, default: false },
    page: { type: Number, default: 1 },
    pageSize: { type: Number, default: 10 },
  },
  template: '<div class="ops-error-log-table-stub" />',
})

describe('OpsErrorDetailsModal', () => {
  it('SLA 明细使用自定义时间窗口并携带 SLA 过滤条件', async () => {
    vi.mocked(opsAPI.listRequestErrors).mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 10,
      pages: 1,
    } as any)

    const wrapper = mount(OpsErrorDetailsModal, {
      props: {
        show: false,
        timeRange: 'custom',
        customStartTime: '2026-05-28T00:00:00.000Z',
        customEndTime: '2026-05-28T01:00:00.000Z',
        platform: 'openai',
        groupId: 12,
        errorType: 'request',
        preset: {
          title: 'SLA 明细',
          impactPlatformSla: true,
          view: 'all',
        },
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          OpsErrorLogTable: OpsErrorLogTableStub,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(opsAPI.listRequestErrors).toHaveBeenCalledWith(expect.objectContaining({
      start_time: '2026-05-28T00:00:00.000Z',
      end_time: '2026-05-28T01:00:00.000Z',
      impact_platform_sla: '1',
      view: 'all',
      platform: 'openai',
      group_id: 12,
    }))
    expect(vi.mocked(opsAPI.listRequestErrors).mock.calls[0][0]).not.toHaveProperty('time_range', 'custom')
  })
})
