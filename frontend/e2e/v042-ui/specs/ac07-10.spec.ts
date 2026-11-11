import { expect, test } from '@playwright/test'
import { adminLogin, createUser, setBalance, userLogin } from '../helpers/api'
import { completedOrder, openOrder, queryRefund, submitRefund } from '../helpers/ac07-10'
import { paymentConfig } from '../helpers/mock'
import { scalar } from '../helpers/db'
import { screenshot } from '../helpers/ui'

// AC07–09 uses the unmodified EasyPay protocol against a local mock.
// AC10 uses an explicitly tagged E2E-only transport to 127.0.0.1; see run.sh.
// Provider domain allow-listing, refund service logic, frontend, database and Redis are unchanged.
test('AC07/08/09 管理员退款强制确认、强制扣款及正常退款', async ({ page }, testInfo) => {
  const admin = await adminLogin()
  const user = await createUser(admin, { email: `ac07-${Date.now()}@v042e2e.local`, password: 'E2e-Passw0rd!', balance: 0 })
  const token = await userLogin(user.email, 'E2e-Passw0rd!')
  const order = await completedOrder(token, '100.00000000', user.id)
  await setBalance(admin, user.id, 30)

  let row = await openOrder(page, order.order_id, 'zh-CN')
  await row.getByRole('button', { name: /^退款$|^Refund$/ }).click()
  await submitRefund(page)
  const dialog = page.getByRole('dialog').last()
  await expect(dialog.locator('#force-refund')).toBeVisible()
  await expect(dialog.locator('button[form="refund-form"]')).toBeDisabled()
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('30.00000000')
  expect(scalar(`select status from payment_orders where id=${order.order_id}`)).toBe('COMPLETED')
  await screenshot(page, testInfo, 'AC07-insufficient-balance-requires-force')

  await submitRefund(page, true)
  await expect(dialog).not.toBeVisible()
  await expect.poll(() => scalar(`select status from payment_orders where id=${order.order_id}`)).toBe('REFUNDED')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('0.00000000')
  expect(scalar(`select detail::text from payment_audit_logs where order_id='${order.order_id}' and action='REFUND_SUCCESS'`)).toContain('"balanceDeducted":30')
  expect(scalar(`select detail::text from payment_audit_logs where order_id='${order.order_id}' and action='REFUND_SUCCESS'`)).toContain('"force":true')

  const second = await completedOrder(token, '100.00000000', user.id)
  await setBalance(admin, user.id, 150)
  row = await openOrder(page, second.order_id, 'zh-CN')
  await row.getByRole('button', { name: /^退款$|^Refund$/ }).click()
  await submitRefund(page)
  await expect(page.getByRole('dialog').last()).not.toBeVisible()
  await expect.poll(() => scalar(`select status from payment_orders where id=${second.order_id}`)).toBe('REFUNDED')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('50.00000000')
  await screenshot(page, testInfo, 'AC09-normal-refund')
})

test('AC10 pending 查询成功/失败恢复余额，且可重新退款', async ({ page }, testInfo) => {
  const admin = await adminLogin()
  const user = await createUser(admin, { email: `ac10-${Date.now()}@v042e2e.local`, password: 'E2e-Passw0rd!', balance: 0 })
  const token = await userLogin(user.email, 'E2e-Passw0rd!')
  const order = await completedOrder(token, '100.00000000', user.id, 'airwallex')
  await setBalance(admin, user.id, 150)

  let row = await openOrder(page, order.order_id, 'zh-CN')
  await row.getByRole('button', { name: /^退款$|^Refund$/ }).click()
  await submitRefund(page)
  await expect.poll(() => scalar(`select status from payment_orders where id=${order.order_id}`)).toBe('REFUND_PENDING')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('50.00000000')
  expect(scalar(`select coalesce(refund_at::text,'') from payment_orders where id=${order.order_id}`)).toBe('')
  await screenshot(page, testInfo, 'AC10-refund-pending')

  await paymentConfig({ queryMode: 'success' })
  await queryRefund(page, order.order_id)
  await expect.poll(() => scalar(`select status from payment_orders where id=${order.order_id}`)).toBe('REFUNDED')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('50.00000000')
  expect(scalar(`select coalesce(refund_at::text,'') from payment_orders where id=${order.order_id}`)).not.toBe('')

  const failed = await completedOrder(token, '150.00000000', user.id, 'airwallex')
  row = await openOrder(page, failed.order_id, 'zh-CN')
  await row.getByRole('button', { name: /^退款$|^Refund$/ }).click()
  await submitRefund(page)
  await expect.poll(() => scalar(`select status from payment_orders where id=${failed.order_id}`)).toBe('REFUND_PENDING')
  await paymentConfig({ queryMode: 'fail' })
  await queryRefund(page, failed.order_id)
  await expect.poll(() => scalar(`select status from payment_orders where id=${failed.order_id}`)).toBe('REFUND_FAILED')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('150.00000000')
  expect(scalar(`select coalesce(refund_at::text,'') from payment_orders where id=${failed.order_id}`)).toBe('')
  await screenshot(page, testInfo, 'AC10-refund-failed-balance-restored')

  row = await openOrder(page, failed.order_id, 'zh-CN')
  await row.getByRole('button', { name: /重试退款|Retry Refund/ }).click()
  await submitRefund(page)
  await expect.poll(() => scalar(`select status from payment_orders where id=${failed.order_id}`)).toBe('REFUND_PENDING')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('50.00000000')
  await paymentConfig({ queryMode: 'success' })
  await queryRefund(page, failed.order_id)
  await expect.poll(() => scalar(`select status from payment_orders where id=${failed.order_id}`)).toBe('REFUNDED')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('50.00000000')
  expect(scalar(`select coalesce(refund_at::text,'') from payment_orders where id=${failed.order_id}`)).not.toBe('')
})
