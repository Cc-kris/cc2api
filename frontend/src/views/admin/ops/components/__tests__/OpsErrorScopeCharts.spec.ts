import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import OpsErrorDistributionChart from '../OpsErrorDistributionChart.vue'
import OpsErrorTrendChart from '../OpsErrorTrendChart.vue'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'

vi.mock('chart.js', () => ({
  Chart: { register: vi.fn() },
  ArcElement: {},
  CategoryScale: {},
  Filler: {},
  Legend: {},
  LineElement: {},
  LinearScale: {},
  PointElement: {},
  Title: {},
  Tooltip: {},
}))

vi.mock('vue-chartjs', async () => {
  const { defineComponent } = await import('vue')

  return {
    Doughnut: defineComponent({
      name: 'Doughnut',
      props: {
        data: { type: Object, required: true },
        options: { type: Object, default: () => ({}) },
      },
      template: '<div class="doughnut-stub" />',
    }),
    Line: defineComponent({
      name: 'LineChartStub',
      props: {
        data: { type: Object, required: true },
        options: { type: Object, default: () => ({}) },
      },
      template: '<div class="line-stub" />',
    }),
  }
})

vi.mock('../../utils/opsFormatters', () => ({
  formatHistoryLabel: (date: string | undefined) => date ?? '',
  sumNumbers: (values: Array<number | null | undefined>) =>
    values.reduce<number>((total, value) => total + (typeof value === 'number' && Number.isFinite(value) ? value : 0), 0),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/api', () => ({
  adminAPI: {
    groups: {
      getAll: vi.fn().mockResolvedValue([]),
    },
  },
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRealtimeTrafficSummary: vi.fn().mockResolvedValue({
      summary: {
        window: '1min',
        start_time: '2026-05-18T00:00:00Z',
        end_time: '2026-05-18T00:01:00Z',
        qps: { current: 0, peak: 0, avg: 0 },
        tps: { current: 0, peak: 0, avg: 0 },
      },
    }),
  },
}))

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => ({
    opsRealtimeMonitoringEnabled: true,
    setOpsRealtimeMonitoringEnabledLocal: vi.fn(),
  }),
}))

const HelpTooltipStub = defineComponent({
  name: 'HelpTooltip',
  props: {
    content: { type: String, default: '' },
  },
  template: '<span class="help-tooltip-stub" />',
})

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' },
  },
  template: '<div class="empty-state-stub" />',
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: null },
    options: { type: Array, default: () => [] },
  },
  emits: ['update:modelValue'],
  template: '<select />',
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const IconStub = defineComponent({
  name: 'Icon',
  template: '<span class="icon-stub" />',
})

const globalStubs = {
  stubs: {
    HelpTooltip: HelpTooltipStub,
    EmptyState: EmptyStateStub,
    Select: SelectStub,
    BaseDialog: BaseDialogStub,
    Icon: IconStub,
  },
}

describe('Ops SLA-scoped error charts', () => {
  it('错误分布图按错误归属拆分展示客户端、上游、平台与业务限制', () => {
    const wrapper = mount(OpsErrorDistributionChart, {
      props: {
        loading: false,
        data: {
          total: 10,
          items: [
            { status_code: 400, total: 2, sla: 0, business_limited: 0, category: 'client_error' },
            { status_code: 503, total: 3, sla: 3, business_limited: 0, category: 'upstream_error' },
            { status_code: 500, total: 1, sla: 1, business_limited: 0, category: 'platform_error' },
            { status_code: 429, total: 4, sla: 0, business_limited: 4, category: 'business_limited' },
          ],
        },
      },
      global: globalStubs,
    })

    const doughnut = wrapper.findComponent({ name: 'Doughnut' })
    expect(doughnut.exists()).toBe(true)
    expect(doughnut.props('data')).toMatchObject({
      labels: ['admin.ops.platformErrors', 'admin.ops.upstreamErrors', 'admin.ops.clientErrors', 'admin.ops.businessLimitedDetails'],
      datasets: [{ data: [1, 3, 2, 4] }],
    })
  })

  it('错误分布图在只有业务限制错误时仍展示独立业务限制分布', () => {
    const wrapper = mount(OpsErrorDistributionChart, {
      props: {
        loading: false,
        data: {
          total: 4,
          items: [{ status_code: 429, total: 4, sla: 0, business_limited: 4, category: 'business_limited' }],
        },
      },
      global: globalStubs,
    })

    const doughnut = wrapper.findComponent({ name: 'Doughnut' })
    expect(doughnut.exists()).toBe(true)
    expect(doughnut.props('data')).toMatchObject({
      labels: ['admin.ops.businessLimitedDetails'],
      datasets: [{ data: [4] }],
    })
  })

  it('错误趋势图的请求错误详情按钮只按 SLA 错误启用', () => {
    const wrapper = mount(OpsErrorTrendChart, {
      props: {
        loading: false,
        timeRange: '1h',
        points: [
          {
            bucket_start: '2026-05-18T00:00:00Z',
            error_count_total: 5,
            business_limited_count: 5,
            error_count_sla: 0,
            upstream_error_count_excl_429_529: 0,
            upstream_429_count: 0,
            upstream_529_count: 0,
          },
        ],
      },
      global: globalStubs,
    })

    const requestErrorsButton = wrapper.findAll('button')[0]
    expect(requestErrorsButton.attributes('disabled')).toBeDefined()
  })

  it('用户成功率卡片详情打开失败请求列表，而不是全部请求列表', async () => {
    const wrapper = mount(OpsDashboardHeader, {
      props: {
        overview: {
          start_time: '2026-05-18T00:00:00Z',
          end_time: '2026-05-18T01:00:00Z',
          platform: '',
          success_count: 96,
          error_count_total: 4,
          business_limited_count: 0,
          error_count_sla: 0,
          request_count_total: 100,
          request_count_sla: 96,
          token_consumed: 0,
          sla: 1,
          user_success_rate: 0.96,
          error_rate: 0,
          upstream_error_rate: 0,
          upstream_error_count_excl_429_529: 0,
          upstream_429_count: 0,
          upstream_529_count: 0,
          qps: { current: 0, peak: 0, avg: 0 },
          tps: { current: 0, peak: 0, avg: 0 },
          duration: {},
          ttft: {},
        },
        platform: '',
        groupId: null,
        timeRange: '1h',
        queryMode: 'auto',
        loading: false,
        lastUpdated: null,
      },
      global: globalStubs,
    })

    const detailsButtons = wrapper.findAll('button').filter((button) => button.text() === 'admin.ops.requestDetails.details')
    expect(detailsButtons.length).toBeGreaterThanOrEqual(2)
    await detailsButtons[2].trigger('click')

    expect(wrapper.emitted('openRequestDetails')?.at(-1)).toEqual([
      {
        title: 'admin.ops.totalFailures',
        kind: 'error',
      },
    ])
  })
})
