import { expect, type Page } from '@playwright/test'
import { adminLogin, api, raw } from './api'
import { paymentConfig } from './mock'
import { scalar } from './db'
import { login, navigate, ADMIN_EMAIL, ADMIN_PASSWORD } from './ui'

const MOCK = 'http://127.0.0.1:18921'

/** 使用真实支付 API 和真实 webhook 构造已完成的余额充值订单。 */
export async function completedOrder(userToken: string, expectedBalance: string, userId: number, provider = 'easypay') {
  const adminToken = await adminLogin()
  const admin = { access_token: adminToken }
  if (provider === 'easypay') {
    const existing = await api('GET', '/admin/payment/providers', undefined, admin.access_token)
    for (const instance of existing) {
      if (instance.enabled) await api('PUT', `/admin/payment/providers/${instance.id}`, { enabled: false }, admin.access_token)
    }
    await api('POST', '/admin/payment/providers', {
      provider_key: 'easypay', name: 'v042-isolated-easypay', enabled: true,
      supported_types: ['alipay'], refund_enabled: true,
      config: { pid: '1001', pkey: 'v042e2epkey', apiBase: MOCK, notifyUrl: 'http://127.0.0.1:8280/api/v1/payment/webhook/easypay', returnUrl: 'http://127.0.0.1:8280/payment/result' },
    }, admin.access_token)
  } else {
    await api('POST', '/admin/payment/providers', {
      provider_key: 'airwallex', name: 'v042-isolated-airwallex', enabled: true,
      supported_types: ['airwallex'], refund_enabled: true,
      config: {
        clientId: 'v042-e2e-client', apiKey: 'v042-e2e-api-key',
        webhookSecret: 'v042-e2e-airwallex-webhook',
        apiBase: 'https://api-demo.airwallex.com/api/v1', currency: 'CNY', countryCode: 'CN',
      },
    }, admin.access_token)
  }
  const paymentType = provider === 'easypay' ? 'alipay' : 'airwallex'
  await api('PUT', '/admin/payment/config', {
    enabled: true, min_amount: 1, max_amount: 1000, daily_limit: 1000,
    balance_recharge_multiplier: 1, recharge_fee_rate: 0,
    enabled_payment_types: [paymentType],
  }, admin.access_token)
  await paymentConfig({ webhookSecret: 'v042-e2e-airwallex-webhook', queryMode: 'success' })
  const order = await api('POST', '/payment/orders', {
    amount: 100, payment_type: paymentType, order_type: 'balance', is_mobile: false,
  }, userToken)
  const notification = await raw('POST', MOCK + (provider === 'easypay' ? '/__payment/notify' : '/__payment/notify-airwallex'), { out_trade_no: order.out_trade_no })
  expect(notification.status, notification.text).toBe(200)
  expect(notification.json.ok, notification.text).toBe(true)
  await expect.poll(() => scalar(`select status from payment_orders where id=${order.order_id}`)).toBe('COMPLETED')
  expect(Number(scalar(`select balance from users where id=${userId}`)).toFixed(8)).toBe(expectedBalance)
  return order
}

export async function openOrder(page: Page, orderId: number, locale: 'zh-CN' | 'en-US') {
  await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, locale)
  // The API fixture may enable payment after the first public-settings response.
  // A full page reload must happen before route guards consult the cached flag.
  await page.goto('http://127.0.0.1:8280/admin/dashboard')
  if (new URL(page.url()).pathname === '/login') await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, locale)
  if (new URL(page.url()).pathname === '/login') await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, locale)
  await expect.poll(async () => {
    // The login shell may have injected settings before the API fixture enabled
    // payments. Reload the real page; waiting on that old object cannot refresh it.
    await page.reload({ waitUntil: 'domcontentloaded' })
    return page.evaluate(() => (window as any).__APP_CONFIG__?.payment_enabled === true)
  }, { timeout: 15000 }).toBe(true)
  await navigate(page, '/admin/orders')
  const row = page.locator('tr').filter({ hasText: `#${orderId}` })
    .or(page.locator('div.rounded-lg.border').filter({ hasText: `#${orderId}` }))
  await expect(row.first()).toBeVisible()
  return row.first()
}

export async function submitRefund(page: Page, force = false) {
  const dialog = page.getByRole('dialog').last()
  await expect(dialog).toBeVisible()
  await dialog.locator('#deduct-balance').check()
  await dialog.locator('textarea').fill('V03 isolated refund acceptance')
  if (force) await dialog.locator('#force-refund').check()
  await dialog.locator('button[form="refund-form"]').click()
}

export async function queryRefund(page: Page, orderId: number) {
  const row = await openOrder(page, orderId, 'zh-CN')
  await row.getByRole('button', { name: /查询退款状态|Query refund status/ }).click()
}
