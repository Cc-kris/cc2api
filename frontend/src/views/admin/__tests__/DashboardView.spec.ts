import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { DashboardStats } from '@/types'
import DashboardView from '../DashboardView.vue'

const {
  getSnapshotV2,
  getUserUsageTrend,
  getUserSpendingRanking,
  getRevenueOverview,
  getRepurchaseDistribution
} = vi.hoisted(() => ({
  getSnapshotV2: vi.fn(),
  getUserUsageTrend: vi.fn(),
  getUserSpendingRanking: vi.fn(),
  getRevenueOverview: vi.fn(),
  getRepurchaseDistribution: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getSnapshotV2,
      getUserUsageTrend,
      getUserSpendingRanking,
      getRevenueOverview,
      getRepurchaseDistribution
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn()
  })
}))


vi.mock('vue-chartjs', () => ({
  Line: { name: 'Line', template: '<div data-testid="line-chart" />' },
  Bar: { name: 'Bar', template: '<div data-testid="bar-chart" />' }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const createDashboardStats = (): DashboardStats => ({
  total_users: 0,
  today_new_users: 0,
  active_users: 0,
  hourly_active_users: 0,
  stats_updated_at: '',
  stats_stale: false,
  total_api_keys: 0,
  active_api_keys: 0,
  total_accounts: 0,
  normal_accounts: 0,
  error_accounts: 0,
  ratelimit_accounts: 0,
  overload_accounts: 0,
  total_requests: 0,
  total_input_tokens: 0,
  total_output_tokens: 0,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 0,
  total_cost: 0,
  total_actual_cost: 0,
  total_account_cost: 0,
  today_requests: 0,
  today_input_tokens: 0,
  today_output_tokens: 0,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 0,
  today_cost: 0,
  today_actual_cost: 0,
  today_account_cost: 0,
  average_duration_ms: 0,
  uptime: 0,
  rpm: 0,
  tpm: 0
})

describe('admin DashboardView', () => {
  beforeEach(() => {
    getSnapshotV2.mockReset()
    getUserUsageTrend.mockReset()
    getUserSpendingRanking.mockReset()
    getRevenueOverview.mockReset()
    getRepurchaseDistribution.mockReset()

    getSnapshotV2.mockResolvedValue({
      stats: createDashboardStats(),
      trend: [],
      models: []
    })
    getUserUsageTrend.mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'hour'
    })
    getUserSpendingRanking.mockResolvedValue({
      ranking: [],
      total_actual_cost: 0,
      total_requests: 0,
      total_tokens: 0,
      start_date: '',
      end_date: ''
    })
    getRevenueOverview.mockResolvedValue({
      total_credit_amount: '10000.00',
      used_amount: '3200.00',
      unused_amount: '6800.00',
      non_admin_user_count: 200,
      credited_user_count: 120,
      is_estimated: false,
      updated_at: '2026-06-07T12:00:00+08:00'
    })
    getRepurchaseDistribution.mockResolvedValue({
      buckets: [
        { bucket: 'zero', label: '零购', user_count: 100, ratio: 50 },
        { bucket: 'one', label: '一购', user_count: 60, ratio: 30 },
        { bucket: 'two', label: '二购', user_count: 20, ratio: 10 },
        { bucket: 'three', label: '三购', user_count: 10, ratio: 5 },
        { bucket: 'three_plus', label: '三购以上', user_count: 10, ratio: 5 }
      ],
      updated_at: '2026-06-07T12:00:00+08:00'
    })
  })

  it('uses last 24 hours as default dashboard range', async () => {
    mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          Line: true,
          Bar: true
        }
      }
    })

    await flushPromises()

    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))
  })

  it('loads and renders revenue and repurchase metrics', async () => {
    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          Line: true,
          Bar: true
        }
      }
    })

    await flushPromises()

    expect(getRevenueOverview).toHaveBeenCalledTimes(1)
    expect(getRepurchaseDistribution).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('admin.dashboard.revenueOverviewTitle')
    expect(wrapper.text()).toContain('¥10,000.00')
    expect(wrapper.text()).toContain('admin.dashboard.repurchaseDistributionTitle')
    expect(wrapper.text()).toContain('120')
    expect(wrapper.text()).toContain('40')
  })

})
