import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UpstreamWalletManager from '../UpstreamWalletManager.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), update: vi.fn(), create: vi.fn(), deleteWallet: vi.fn(), assignAccounts: vi.fn(),
  probe: vi.fn(), sync: vi.fn(), listPrices: vi.fn(), listSyncHistory: vi.fn(), listFundEvents: vi.fn(), accountList: vi.fn(),
  importPrices: vi.fn(), createFundEvent: vi.fn(), listPublishedProtocols: vi.fn(),
  success: vi.fn(), error: vi.fn()
}))

vi.mock('@/api/admin', () => ({ adminAPI: { upstreamWallets: {
  list: mocks.list, update: mocks.update, create: mocks.create, deleteWallet: mocks.deleteWallet,
  assignAccounts: mocks.assignAccounts, probe: mocks.probe, sync: mocks.sync,
  listPrices: mocks.listPrices, listSyncHistory: mocks.listSyncHistory, listFundEvents: mocks.listFundEvents,
  importPrices: mocks.importPrices, createFundEvent: mocks.createFundEvent, listPublishedProtocols: mocks.listPublishedProtocols
}, accounts: { list: mocks.accountList } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))

const upstream = { id: 3, name: 'NewAPI 上游', base_url: 'https://upstream.test', normalized_base_url: 'https://upstream.test' }
const wallet = {
  id: 21, upstream_id: 3, name: '主钱包', adapter_type: 'newapi', base_url: 'https://upstream.test',
  credential_configured: true, currency: 'USD', balance_kind: 'wallet_cash', balance_scope_key: 'main-shared',
  pricing_group: 'default', enabled: true, last_pricing_sync_at: null, pricing_sync_status: 'success',
  last_balance_sync_at: null, balance_sync_status: 'success', last_quota_sync_at: null, quota_sync_status: 'idle',
  assigned_account_count: 2, created_at: '2026-07-27T00:00:00Z', updated_at: '2026-07-27T00:00:00Z', deleted_at: null
}

describe('UpstreamWalletManager', () => {
  beforeEach(() => {
    Object.values(mocks).forEach(mock => mock.mockReset())
    mocks.list.mockResolvedValue([wallet])
    mocks.sync.mockResolvedValue({ created: true, job: { id: 1 } })
    mocks.probe.mockResolvedValue({ reachable: true, latency_ms: 12, capabilities: {} })
    mocks.listPrices.mockResolvedValue({ items: [], total: 0 })
    mocks.listSyncHistory.mockResolvedValue({ items: [], total: 0 })
    mocks.listFundEvents.mockResolvedValue({ items: [], total: 0 })
    mocks.listPublishedProtocols.mockResolvedValue([])
    mocks.accountList.mockResolvedValue({ items: [] })
  })

  it('shows finance scope and triggers deduplicated pricing sync', async () => {
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never] } })
    await flushPromises()

    expect(wrapper.text()).toContain('财务结算钱包')
    expect(wrapper.text()).toContain('main-shared')
    expect(wrapper.text()).toContain('凭据已配置')
    await wrapper.findAll('button').find(button => button.text() === '同步定价')!.trigger('click')
    await flushPromises()

    expect(mocks.sync).toHaveBeenCalledWith(21, 'pricing')
    expect(mocks.success).toHaveBeenCalledWith('同步任务已创建')
  })

  it('preserves an existing credential when edit is submitted empty', async () => {
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never] } })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑')!.trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    const payload = mocks.update.mock.calls[0][1]
    expect(payload).not.toHaveProperty('credential')
    expect(payload.balance_scope_key).toBe('main-shared')
  })

  it('labels NewAPI credentials and only lists accounts from the selected upstream', async () => {
    mocks.accountList.mockResolvedValue({ items: [
      { id: 1, name: '同上游账号', platform: 'openai', type: 'apikey', credentials: { base_url: 'https://upstream.test/' } },
      { id: 2, name: '其他上游账号', platform: 'openai', type: 'apikey', credentials: { base_url: 'https://other.test' } }
    ] })
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never] } })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑')!.trigger('click')
    expect(wrapper.text()).toContain('NewAPI 用户中心 Access Token，不是模型 API Key')
    await wrapper.findAll('button').find(button => button.text() === '取消')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === '账号归属')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('同上游账号')
    expect(wrapper.text()).not.toContain('其他上游账号')
  })

  it('reuses the same idempotency key when a manual fund event is retried', async () => {
    const manualWallet = { ...wallet, adapter_type: 'manual', pricing_adapter: 'manual', balance_adapter: 'manual', quota_adapter: 'none' }
    mocks.list.mockResolvedValue([manualWallet])
    mocks.createFundEvent.mockRejectedValueOnce(new Error('temporary failure')).mockResolvedValueOnce({ id: 1 })
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never] } })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '记录资金')!.trigger('click')
    const inputs = wrapper.get('form').findAll('input')
    await inputs[0].setValue('10')
    await inputs[1].setValue('USD')
    await inputs[2].setValue('1')
    await inputs[3].setValue('manual-test')
    await inputs[4].setValue('10')
    await inputs[6].setValue('upstream-tx-1')
    await inputs[9].setValue('人工充值记录')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.createFundEvent).toHaveBeenCalledTimes(2)
    const firstKey = mocks.createFundEvent.mock.calls[0][2]
    expect(firstKey).toBeTruthy()
    expect(mocks.createFundEvent.mock.calls[1][2]).toBe(firstKey)
  })

  it('offers protocol funding synchronization without coupling it to account multiplier', async () => {
    mocks.list.mockResolvedValue([{ ...wallet, adapter_type: 'protocol', protocol_version_id: 91 }])
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never] } })
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === '同步充值')!.trigger('click')
    await flushPromises()

    expect(mocks.sync).toHaveBeenCalledWith(21, 'funding')
  })

  it('offers protocol account usage synchronization for automatic multiplier observation', async () => {
    mocks.list.mockResolvedValue([{ ...wallet, adapter_type: 'protocol', protocol_version_id: 91 }])
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never] } })
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === '同步账号倍率')!.trigger('click')
    await flushPromises()

    expect(mocks.sync).toHaveBeenCalledWith(21, 'account-usage')
  })

  it('opens the requested upstream finance detail instead of defaulting to another upstream', async () => {
    const anotherUpstream = { ...upstream, id: 32, name: '另一个上游' }
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never, anotherUpstream as never], activeUpstreamId: 32 } })
    await flushPromises()

    expect(wrapper.get('[data-testid="upstream-finance-details"] select').element.value).toBe('32')
    expect(mocks.list).toHaveBeenCalledWith(32)
  })

  it('allows account-credential protocols to create a wallet without a shared wallet credential', async () => {
    mocks.list.mockResolvedValue([{ ...wallet, adapter_type: 'protocol', protocol_version_id: 91 }])
    mocks.listPublishedProtocols.mockResolvedValue([{ id: 9, code: 'generic', name: '通用协议', status: 'published', current_version_id: 91 }])
    const wrapper = mount(UpstreamWalletManager, { props: { upstreams: [upstream as never] } })
    await flushPromises()

    expect(wrapper.text()).toContain('通用财务协议')
    await wrapper.findAll('button').find(button => button.text() === '新增钱包')!.trigger('click')
    const form = wrapper.get('form')
    await form.findAll('select')[0].setValue('protocol')
    await flushPromises()

    const credential = form.get('input[type="password"]')
    expect(credential.attributes('required')).toBeUndefined()
    expect(wrapper.text()).toContain('协议使用账号凭据时留空')
  })
})
