import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import type { PaymentOrder } from '@/types/payment'
import AdminRefundDialog from '../AdminRefundDialog.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

const order: PaymentOrder = {
  id: 7, user_id: 10, amount: 100, pay_amount: 100, fee_rate: 0,
  payment_type: 'stripe', out_trade_no: 'T1', status: 'COMPLETED',
  order_type: 'balance', created_at: '2026-11-01', expires_at: '2026-11-02',
  refund_amount: 0,
}

async function openDialog(props: Record<string, unknown> = {}) {
  const wrapper = mount(AdminRefundDialog, {
    props: { show: false, order, userBalance: 30, ...props },
    global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
  })
  await wrapper.setProps({ show: true })
  await wrapper.get('textarea').setValue('user asked')
  return wrapper
}

// V03-AC07/AC08（前端）：后端返回 require_force 后显示强制选项，未勾选不能提交，勾选后带 force=true 重提。
describe('refund require_force', () => {
  it('hides the force checkbox until the backend asks for it', async () => {
    const wrapper = await openDialog()
    expect(wrapper.find('#force-refund').exists()).toBe(false)
  })

  it('blocks submit until force is checked, then submits force=true', async () => {
    const wrapper = await openDialog({ requireForce: true, warning: 'payment.admin.refundRequireForce' })
    expect(wrapper.text()).toContain('payment.admin.refundRequireForce')
    const submit = wrapper.get('button[type="submit"]')
    expect(submit.attributes('disabled')).toBeDefined()

    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('confirm')).toBeUndefined()

    await wrapper.get('#force-refund').setValue(true)
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('form').trigger('submit')
    const events = wrapper.emitted('confirm')
    expect(events).toHaveLength(1)
    expect(events?.[0]?.[0]).toMatchObject({ amount: 100, deduct_balance: true, force: true, reason: 'user asked' })
  })

  it('shows REFUND_PENDING refund_amount as not yet refunded', async () => {
    const wrapper = await openDialog({ order: { ...order, status: 'REFUND_PENDING', refund_amount: 100 } })
    expect(wrapper.text()).not.toContain('payment.admin.alreadyRefunded')
  })
})
