import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { readinessMock, updateMock, successMock, errorMock } = vi.hoisted(() => ({
  readinessMock: vi.fn(), updateMock: vi.fn(), successMock: vi.fn(), errorMock: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getFinanceReadiness: readinessMock, updateFinanceProfile: updateMock } },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: successMock, showError: errorMock }) }))

import AccountFinanceProfilePanel from '../AccountFinanceProfilePanel.vue'

describe('AccountFinanceProfilePanel', () => {
	beforeEach(() => vi.clearAllMocks())
  it('shows readiness and saves a new immutable version', async () => {
    readinessMock.mockResolvedValue({
      account_id: 9, status: 'ready_contract', issues: [], actions: [],
      profile: {
        id: 1, account_id: 9, cost_mode: 'contract_multiplier', endpoint_source: 'account_base_url',
        endpoint_base_url_snapshot: 'https://upstream.example.com', credential_source: 'account_api_key',
        counter_scope: 'account', balance_unit_semantics: 'none', contract_multiplier: '0.22',
        readiness_status: 'ready_contract', readiness_detail: {}, version: 1,
        effective_from: '2026-07-30T00:00:00Z', reason: '初始化配置',
      },
    })
    updateMock.mockResolvedValue({ version: 2 })
    const wrapper = mount(AccountFinanceProfilePanel, { props: { accountId: 9 } })
    await flushPromises()

    expect(wrapper.text()).toContain('合同成本就绪')
    const reason = wrapper.find('textarea')
    await reason.setValue('上游合同价格发生变化')
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(updateMock).toHaveBeenCalledWith(9, expect.objectContaining({ expected_version: 1, reason: '上游合同价格发生变化' }))
    expect(successMock).toHaveBeenCalled()
  })

  it('requires wallet and protocol version for cumulative settlement', async () => {
    readinessMock.mockResolvedValue({ account_id: 9, status: 'unconfigured', issues: [], actions: [] })
    const wrapper = mount(AccountFinanceProfilePanel, { props: { accountId: 9 } })
    await flushPromises()
    await wrapper.find('select').setValue('cumulative_actual')
    await wrapper.find('textarea').setValue('切换累计实扣模式')
    await wrapper.get('button').trigger('click')
    expect(errorMock).toHaveBeenCalledWith('累计结算模式必须填写钱包和协议版本')
    expect(updateMock).not.toHaveBeenCalled()
  })

  it('submits a blank contract override as fallback to the account upstream multiplier', async () => {
    readinessMock.mockResolvedValue({ account_id: 9, status: 'unconfigured', issues: [], actions: [] })
    updateMock.mockResolvedValue({ version: 1 })
    const wrapper = mount(AccountFinanceProfilePanel, { props: { accountId: 9 } })
    await flushPromises()

    expect(wrapper.text()).toContain('留空使用账号上游倍率')
    await wrapper.find('textarea').setValue('使用账号上游倍率计费')
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(updateMock).toHaveBeenCalledWith(9, expect.objectContaining({
      contract_multiplier: undefined,
      contract_type: undefined,
    }))
  })
})
