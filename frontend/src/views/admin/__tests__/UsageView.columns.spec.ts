import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'

// 管理员用量页列设置：新加入的默认隐藏列（上游请求 ID、会话 ID）对已保存设置也生效。
const { store, list, getStats, getSnapshotV2 } = vi.hoisted(() => {
  const store = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: vi.fn((key: string) => (store.has(key) ? store.get(key)! : null)),
    setItem: vi.fn((key: string, value: string) => { store.set(key, value) }),
    removeItem: vi.fn((key: string) => { store.delete(key) }),
  })
  return { store, list: vi.fn(), getStats: vi.fn(), getSnapshotV2: vi.fn() }
})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: { list, getStats },
    dashboard: { getModelStats: vi.fn().mockResolvedValue([]), getSnapshotV2 },
    users: { getById: vi.fn() },
  },
}))
vi.mock('@/api/admin/usage', () => ({ adminUsageAPI: { list: vi.fn() } }))
vi.mock('@/api/admin/ops', () => ({ listErrorLogs: vi.fn().mockResolvedValue({ items: [], total: 0 }) }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showWarning: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() }),
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const KEY = 'usage-hidden-columns'
const NEW_COLUMNS = ['upstream_request_id', 'session_id']

const UsageTableStub = {
  name: 'UsageTable',
  props: ['columns'],
  template: '<div data-test="usage-table">{{ columns.map((c) => c.key).join(",") }}</div>',
}

const mountView = async () => {
  const wrapper = mount(UsageView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        UsageFilters: { template: '<div><slot name="after-reset" /></div>' },
        UsageTable: UsageTableStub,
        UsageStatsCards: true,
        OpsErrorLogTable: true,
        OpsErrorDetailModal: true,
        UsageExportProgress: true,
        UsageCleanupDialog: true,
        UserBalanceHistoryModal: true,
        UserTokenRanking: true,
        Pagination: true,
        Select: true,
        DateRangePicker: true,
        Icon: true,
        TokenUsageTrend: true,
        ModelDistributionChart: true,
        GroupDistributionChart: true,
        EndpointDistributionChart: true,
      },
    },
  })
  await flushPromises()
  vi.advanceTimersByTime(120)
  await flushPromises()
  return wrapper
}

const visibleKeys = (wrapper: Awaited<ReturnType<typeof mountView>>) =>
  (wrapper.findComponent(UsageTableStub).props('columns') as Array<{ key: string }>).map((c) => c.key)

describe('admin UsageView column defaults', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    store.clear()
    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({ total_requests: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0 })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('hides the new audit columns when nothing is saved', async () => {
    const keys = visibleKeys(await mountView())
    for (const key of NEW_COLUMNS) expect(keys).not.toContain(key)
    expect(keys).toContain('request_id')
  })

  it('hides the new audit columns for an admin with an old array save', async () => {
    // 旧存档只隐藏了 model，并打开了默认隐藏的 user_agent。
    store.set(KEY, JSON.stringify(['model', 'reasoning_effort']))
    const keys = visibleKeys(await mountView())
    for (const key of NEW_COLUMNS) expect(keys).not.toContain(key)
    expect(keys).not.toContain('model')
    expect(keys).toContain('user_agent')
  })

  it('keeps a new column open after the admin turned it on', async () => {
    store.set(KEY, JSON.stringify(['model', 'reasoning_effort']))
    const wrapper = await mountView()

    // 在列设置里打开「上游请求 ID」。
    await wrapper.find('button.btn-secondary').trigger('click')
    const item = wrapper.findAll('button').find((b) => b.text() === 'admin.usage.upstreamRequestId')
    expect(item).toBeTruthy()
    await item!.trigger('click')
    expect(visibleKeys(wrapper)).toContain('upstream_request_id')

    const saved = JSON.parse(store.get(KEY)!)
    expect(saved.v).toBe(2)
    expect(saved.known).toEqual(expect.arrayContaining(NEW_COLUMNS))
    expect(saved.hidden).not.toContain('upstream_request_id')
    expect(saved.hidden).toContain('session_id')
    wrapper.unmount()

    // 重新进入页面，用户打开的列仍打开，未打开的新列仍隐藏。
    const keys = visibleKeys(await mountView())
    expect(keys).toContain('upstream_request_id')
    expect(keys).not.toContain('session_id')
    expect(keys).not.toContain('model')
  })
})
