import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import OpsRequestDetailsModal from '../OpsRequestDetailsModal.vue'
import { opsAPI } from '@/api/admin/ops'
import { useAuthStore } from '@/stores/auth'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key}:${JSON.stringify(params)}`
      },
    }),
  }
})

vi.mock('@/api/admin/ops', () => {
  const opsAPI = {
    listRequestDetails: vi.fn(),
  }

  return {
    opsAPI,
    default: opsAPI,
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true),
  }),
}))

vi.mock('../../utils/opsFormatters', () => ({
  parseTimeRangeMinutes: () => 60,
  formatDateTime: (value: string) => value,
}))

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' },
  },
  template: '<div v-if="show"><slot /></div>',
})

const PaginationStub = defineComponent({
  name: 'Pagination',
  props: {
    total: { type: Number, default: 0 },
    page: { type: Number, default: 1 },
    pageSize: { type: Number, default: 10 },
  },
  template: '<div class="pagination-stub" />',
})

describe('OpsRequestDetailsModal', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const authStore = useAuthStore()
    authStore.user = { id: 1, email: 'admin@example.com', role: 'admin' } as any
  })

  it('失败请求列表展示分组、请求账号、上游账号和模型，并可点击进入错误详情', async () => {
    vi.mocked(opsAPI.listRequestDetails).mockResolvedValueOnce({
      items: [
        {
          kind: 'error',
          created_at: '2026-05-28T01:00:00Z',
          request_id: 'req-1',
          platform: 'openai',
          model: 'gpt-5.4-upstream',
          requested_model: 'gpt-5.4',
          upstream_model: 'gpt-5.4-upstream',
          duration_ms: 321,
          status_code: 500,
          error_id: 99,
          user_id: 11,
          user_email: 'user@example.com',
          api_key_id: 22,
          account_id: 42,
          account_name: '上游账号A',
          group_id: 33,
          group_name: '默认分组',
          stream: false,
        },
      ],
      total: 1,
      page: 1,
      page_size: 10,
    } as any)

    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: '1h',
        preset: { title: '失败请求', kind: 'error' },
        platform: '',
        groupId: null,
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Pagination: PaginationStub,
        },
      },
    })

    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(wrapper.text()).toContain('gpt-5.4')
    expect(wrapper.text()).toContain('gpt-5.4-upstream')
    expect(wrapper.text()).toContain('默认分组')
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('上游账号A')

    const row = wrapper.find('tbody tr')
    expect(row.classes()).toContain('cursor-pointer')
    await row.trigger('click')

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([false])
    expect(wrapper.emitted('openErrorDetail')?.at(-1)).toEqual([99])
  })

  it('自定义时间范围下使用 start_time/end_time 拉取请求明细', async () => {
    vi.mocked(opsAPI.listRequestDetails).mockResolvedValueOnce({
      items: [],
      total: 0,
      page: 1,
      page_size: 10,
      pages: 1,
    } as any)

    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: 'custom',
        customStartTime: '2026-05-28T00:00:00.000Z',
        customEndTime: '2026-05-28T01:00:00.000Z',
        preset: { title: '请求明细', kind: 'all' },
        platform: 'openai',
        groupId: 12,
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Pagination: PaginationStub,
        },
      },
    })

    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(opsAPI.listRequestDetails).toHaveBeenCalledWith(expect.objectContaining({
      start_time: '2026-05-28T00:00:00.000Z',
      end_time: '2026-05-28T01:00:00.000Z',
      platform: 'openai',
      group_id: 12,
      kind: 'all',
    }))
    expect(vi.mocked(opsAPI.listRequestDetails).mock.calls[0][0]).not.toHaveProperty('time_range', 'custom')
  })
})
