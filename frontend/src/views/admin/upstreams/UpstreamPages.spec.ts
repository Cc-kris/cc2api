import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UpstreamManagementView from './UpstreamManagementView.vue'
import UpstreamStatsView from './UpstreamStatsView.vue'
import FinanceStatsView from '../FinanceStatsView.vue'

const { list, deleteUpstream, getStats, getFinanceStats, showSuccess, showError } = vi.hoisted(() => ({
  list: vi.fn(),
  deleteUpstream: vi.fn(),
  getStats: vi.fn(),
  getFinanceStats: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    upstreams: {
      list,
      create: vi.fn(),
      update: vi.fn(),
      deleteUpstream,
      syncFromAccounts: vi.fn(),
      getStats,
      getFinanceStats
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('vue-chartjs', () => ({
  Bar: { props: ['data', 'options'], template: '<div data-testid="bar-chart">{{ JSON.stringify(data) }}</div>' },
  Line: { props: ['data', 'options'], template: '<div data-testid="line-chart" :data-options="JSON.stringify(options)">{{ JSON.stringify(data) }}</div>' }
}))

vi.mock('chart.js', () => ({
  BarElement: {},
  CategoryScale: {},
  Chart: { register: vi.fn() },
  Legend: {},
  LineElement: {},
  LinearScale: {},
  PointElement: {},
  Tooltip: {}
}))

const mountOptions = {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' }
    }
  }
}

describe('admin upstream pages', () => {
  beforeEach(() => {
    list.mockReset()
    deleteUpstream.mockReset()
    getStats.mockReset()
    getFinanceStats.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('renders upstream management platform billing, required marks, and performs delete', async () => {
    list
      .mockResolvedValueOnce([
        {
          id: 1,
          base_url: 'https://api.anthropic.com',
          normalized_base_url: 'https://api.anthropic.com',
          name: 'Anthropic',
          rate_multiplier: 1,
          platform_rates: [{ id: 10, platform: 'anthropic', rate_multiplier: 0.8, image_unit_price: 0.08 }],
          initial_balance: 100,
          consumed_balance: 12,
          current_balance: 88,
          account_count: 2,
          balance_alert_enabled: true,
          alert_balance: 20,
          notes: 'main',
          created_at: '2026-06-18T00:00:00Z',
          updated_at: '2026-06-18T00:00:00Z'
        }
      ])
      .mockResolvedValueOnce([])
    deleteUpstream.mockResolvedValue(undefined)

    const wrapper = mount(UpstreamManagementView, mountOptions)
    await flushPromises()

    expect(wrapper.text()).toContain('平台计费')
    expect(wrapper.text()).toContain('anthropic × 0.8')
    expect(wrapper.text()).toContain('图片 0.0800/次')

    await wrapper.get('button.btn-primary').trigger('click')
    expect(wrapper.text()).toContain('* 为必填项。')
    expect(wrapper.text()).toContain('Base URL *')
    expect(wrapper.text()).toContain('余额 *')
    expect(wrapper.text()).toContain('按平台设置计费')

    await wrapper.findAll('button').find(button => button.text() === '删除')?.trigger('click')
    await flushPromises()

    expect(deleteUpstream).toHaveBeenCalledWith(1)
    expect(showSuccess).toHaveBeenCalledWith('删除成功')
  })

  it('renders one token trend line for each upstream', async () => {
    getStats.mockResolvedValue({
      summary: {
        upstream_count: 2,
        total_current_balance: 180,
        total_initial_balance: 200,
        total_consumed_balance: 20,
        total_input_tokens: 10,
        total_output_tokens: 20,
        total_cache_write_tokens: 0,
        total_cache_read_tokens: 0,
        total_tokens: 30
      },
      cost_bars: [
        { upstream_id: 1, upstream_name: 'Anthropic', consumed_balance: 12, input_tokens: 10, output_tokens: 20, cache_write_tokens: 0, cache_read_tokens: 0, total_tokens: 30 }
      ],
      token_trend: [
        { bucket: '2026-06-17T00:00:00Z', upstream_id: 1, upstream_name: 'Anthropic', consumed_balance: 1, input_tokens: 10, output_tokens: 0, cache_write_tokens: 0, cache_read_tokens: 0, total_tokens: 10 },
        { bucket: '2026-06-17T00:00:00Z', upstream_id: 2, upstream_name: 'OpenAI', consumed_balance: 2, input_tokens: 20, output_tokens: 0, cache_write_tokens: 0, cache_read_tokens: 0, total_tokens: 20 },
        { bucket: '2026-06-18T00:00:00Z', upstream_id: 1, upstream_name: 'Anthropic', consumed_balance: 3, input_tokens: 30, output_tokens: 0, cache_write_tokens: 0, cache_read_tokens: 0, total_tokens: 30 }
      ],
      start_date: '2026-06-17T00:00:00Z',
      end_date: '2026-06-18T00:00:00Z',
      granularity: 'day',
      updated_at: '2026-06-18T00:00:00Z'
    })

    const wrapper = mount(UpstreamStatsView, mountOptions)
    await flushPromises()

    const chartData = JSON.parse(wrapper.get('[data-testid="line-chart"]').text())
    expect(chartData.datasets.map((dataset: { label: string }) => dataset.label)).toEqual(['Anthropic', 'OpenAI'])
    expect(chartData.datasets[0].data).toEqual([10, 30])
    expect(chartData.datasets[1].data).toEqual([20, 0])
  })

  it('configures finance chart to show all three values on date hover', async () => {
    getFinanceStats.mockResolvedValue({
      summary: {
        user_recharge_total: 100,
        upstream_recharge_total: 50,
        user_consumed_amount: 40,
        upstream_consumed_amount: 10,
        consumed_profit: 30,
        consumed_profit_rate: 75
      },
      trend: [
        { bucket: '2026-06-18T08:00:00Z', profit: 30, upstream_cost: 10, user_recharge: 100, user_consumed_amount: 40, upstream_consumed_amount: 10 }
      ],
      start_date: '2026-06-18T00:00:00Z',
      end_date: '2026-06-19T00:00:00Z',
      granularity: 'day',
      updated_at: '2026-06-18T00:00:00Z'
    })

    const wrapper = mount(FinanceStatsView, mountOptions)
    await flushPromises()

    const chartData = JSON.parse(wrapper.get('[data-testid="line-chart"]').text())
    const chartOptions = JSON.parse(wrapper.get('[data-testid="line-chart"]').attributes('data-options') || '{}')
    expect(chartData.datasets.map((dataset: { label: string }) => dataset.label)).toEqual(['已消耗利润', '上游成本', '用户充值'])
    expect(chartData.labels[0]).not.toContain(':')
    expect(chartOptions.interaction.mode).toBe('index')
    expect(chartOptions.plugins.tooltip.mode).toBe('index')
  })
})
