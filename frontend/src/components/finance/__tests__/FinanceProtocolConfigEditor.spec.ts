import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import FinanceProtocolConfigEditor from '../FinanceProtocolConfigEditor.vue'

describe('FinanceProtocolConfigEditor', () => {
  it('builds a generic account usage operation without collecting credentials', async () => {
    const wrapper = mount(FinanceProtocolConfigEditor, {
      props: { modelValue: { capabilities: [], authentication: { type: 'none' }, operations: {}, cost_mode: 'manual', unit_semantics: 'none' } },
    })

    const checkbox = wrapper.findAll('input[type="checkbox"]').find(item => item.element.parentElement?.textContent?.includes('账号累计费用'))
    expect(checkbox).toBeTruthy()
    await checkbox!.setValue(true)
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('账号累计费用')
    expect(wrapper.text()).toContain('钱包财务凭据')
    expect(wrapper.text()).toContain('不填写或保存真实密钥')
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted?.length).toBeGreaterThan(0)
    const latest = emitted!.at(-1)![0] as Record<string, any>
    expect(latest.capabilities).toContain('account_usage')
    expect(latest.operations.account_usage).toMatchObject({ method: 'GET', mapping: {} })
  })

  it('serializes mappings, pagination and redaction paths', async () => {
    const wrapper = mount(FinanceProtocolConfigEditor, {
      props: {
        modelValue: {
          capabilities: ['funding_transactions'], cost_mode: 'manual', unit_semantics: 'fiat_currency',
          authentication: { type: 'bearer', credential_source: 'wallet_finance_credential' },
          operations: { funding_transactions: { method: 'GET', path: '/funds', mapping: { transactions: '$.data' }, pagination: { type: 'page', page_parameter: 'page', max_pages: 5 } } },
          redact_paths: ['$.secret'],
        },
      },
    })

    expect(wrapper.text()).toContain('充值交易')
    const redaction = wrapper.findAll('textarea').find(item => item.element.value.includes('$.secret'))
    expect(redaction).toBeTruthy()
    await redaction!.setValue('$.secret\n$.private_token')
    const emitted = wrapper.emitted('update:modelValue')!
    const latest = emitted.at(-1)![0] as Record<string, any>
    expect(latest.redact_paths).toEqual(['$.secret', '$.private_token'])
    expect(latest.operations.funding_transactions.pagination).toMatchObject({ type: 'page', page_parameter: 'page', max_pages: 5 })
  })
})
