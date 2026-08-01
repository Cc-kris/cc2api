import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import FinanceInitializationPanel from '../FinanceInitializationPanel.vue'

const api = vi.hoisted(() => ({ scanInitialization: vi.fn(), applyInitialization: vi.fn(), syncFromAccounts: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { finance: api, upstreams: api } }))

describe('FinanceInitializationPanel', () => {
  beforeEach(() => {
    api.scanInitialization.mockReset()
    api.applyInitialization.mockReset()
    api.syncFromAccounts.mockReset()
    api.syncFromAccounts.mockResolvedValue({ created: 0 })
    api.scanInitialization.mockResolvedValue({
      accounts: [{ account_id: 7, account_name: '上游账号', platform: 'openai', upstream_id: 3, upstream_name: '测试上游', current_multiplier: '', finance_profile_ready: false, needs_multiplier_confirm: true }],
      upstreams: [{ upstream_id: 3, upstream_name: '测试上游', base_url: 'https://upstream.test', currency: 'USD', current_balance: 12, account_count: 1, finance_wallet_set: false }]
    })
    api.applyInitialization.mockResolvedValue({ initialized_accounts: 1, initialized_upstreams: 1, created_wallets: 1 })
  })

  it('only asks for the account multiplier and upstream current balance', async () => {
    const wrapper = mount(FinanceInitializationPanel)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(api.syncFromAccounts).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('账号上游倍率')
    expect(wrapper.text()).toContain('上游当前余额')
    expect(wrapper.text()).not.toContain('协议版本')
    expect(wrapper.text()).not.toContain('共享余额范围')

    await wrapper.get('input[aria-label="上游账号 上游倍率"]').setValue('0.22')
    await wrapper.get('input[aria-label="测试上游 当前余额"]').setValue('15')
    await wrapper.findAll('button').find(button => button.text() === '确认并初始化财务')!.trigger('click')
    await flushPromises()
    expect(api.applyInitialization).toHaveBeenCalledWith(expect.objectContaining({
      accounts: [{ account_id: 7, upstream_cost_multiplier: '0.22' }],
      upstreams: [{ upstream_id: 3, current_balance: 15 }]
    }))
    expect(wrapper.text()).toContain('初始化完成：1 个账号、1 个上游')
  })
})
