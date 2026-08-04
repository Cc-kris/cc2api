import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const admin = { id: 1, username: 'owner', email: 'owner@example.com', role: 'admin', balance: 0, status: 'active' }
const quality = { status: 'partial', exact_count: 90, estimated_count: 4, missing_profile_count: 1, missing_price_count: 2, missing_multiplier_count: 1, missing_usage_count: 1, unsupported_usage_count: 0, non_billable_count: 0, excluded_count: 2, unpriced_revenue: '12.5', cost_coverage_rate: '0.98' }
const metric = (amount: string) => ({ amount, currency: 'USD', previous_amount: '80', change_rate: '0.25', status: 'complete' })

async function fulfill(route: Route, data: unknown) { await route.fulfill({ json: { code: 0, message: 'success', data } }) }

async function mockFinance(page: Page) {
  await page.addInitScript((savedAdmin) => {
    localStorage.setItem('auth_token', 'finance-e2e-token')
    localStorage.setItem('auth_user', JSON.stringify(savedAdmin))
    localStorage.setItem('admin_guide_1_admin_v4_interactive', 'true')
    ;(window as any).__APP_CONFIG__ = { site_name: 'CCAI', version: 'e2e', model_square_enabled: true, custom_menu_items: [] }
  }, admin)
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const path = url.pathname
    if (path.endsWith('/auth/me')) return fulfill(route, admin)
    if (path.endsWith('/admin/upstreams') && route.request().method() === 'GET') return fulfill(route, { items: [{ id: 6, base_url: 'https://upstream.example', normalized_base_url: 'https://upstream.example', name: '上游 A', rate_multiplier: 1, platform_rates: [], initial_balance: 300, consumed_balance: 40, current_balance: 260, account_count: 1, balance_alert_enabled: false, alert_balance: null, notes: '', created_at: '2026-07-01T00:00:00Z', updated_at: '2026-07-27T08:00:00Z' }] })
    if (/\/admin\/upstreams\/\d+\/wallets$/.test(path) && route.request().method() === 'GET') return fulfill(route, [])
    if (path.endsWith('/admin/upstream-finance-protocols') && route.request().method() === 'GET') return fulfill(route, { items: [], total: 0, page: 1, page_size: 100 })
    if (path.endsWith('/admin/accounts') && route.request().method() === 'GET') return fulfill(route, { items: [], total: 0, page: 1, page_size: 1000 })
    if (path.endsWith('/admin/finance/overview')) return fulfill(route, {
      range: { start_date: '2026-07-01', end_date: '2026-07-27', timezone: 'Asia/Shanghai' },
      revenue: metric('100'), upstream_cost: metric('70'), profit: metric('30'), recharge_bonus_income: metric('5'), combined_profit: metric('35'), margin_rate: '0.3',
      today_profit: metric('-2'), month_profit: metric('20'), historical_profit: metric('300'), historical_combined_profit: metric('320'), historical_loss_amount: '25',
      estimated_cost_risk: '3', unconfirmed_exact_cost: '1.5', unpriced_revenue_risk: '12.5', loss_amount: '5', loss_request_count: 2,
      payment_net_cash: '50', upstream_net_cash: '-20', wallet_cash_total: '260', token_quota_wallet_count: 1,
      quality, open_alert_count: 1, generated_at: '2026-07-27T08:00:00Z'
    })
    if (path.endsWith('/admin/finance/trend')) return fulfill(route, { items: [{ bucket_start: '2026-07-26T00:00:00Z', bucket_end: '2026-07-27T00:00:00Z', revenue: '100', covered_revenue: '98', upstream_cost: '70', recharge_bonus_income: '5', profit: '30', combined_profit: '35', cumulative_profit: '300', cumulative_combined_profit: '320', cost_coverage_rate: '0.98', loss_amount: '5', margin_rate: '0.3', request_count: 10, quality }] })
    if (path.endsWith('/admin/finance/breakdown')) return fulfill(route, { items: [{ dimension_key: '7', dimension_name: '重点客户', revenue: '100', upstream_cost: '70', profit: '30', margin_rate: '0.3', loss_amount: '5', request_count: 10, exact_count: 9, estimated_count: 0, missing_count: 1 }], total: 1, page: 1, page_size: 100 })
    if (path.endsWith('/admin/finance/losses')) return fulfill(route, { items: [{ usage_log_id: 88, request_id: 'req-loss', usage_created_at: '2026-07-26T08:00:00Z', user_id: 7, user_name: '亏损客户', group_id: 2, group_name: '企业组', channel_id: 3, channel_name: '渠道 A', account_id: 4, account_name: '上游账号 A', wallet_id: 5, wallet_name: '钱包 A', upstream_id: 6, upstream_name: '上游 A', requested_model: 'gpt-5', upstream_model: 'gpt-5', sales_pricing_version: 'v2', revenue: '1', upstream_cost: '2', profit: '-1', margin_rate: '-1', cost_status: 'exact', loss_amount: '1', loss_reason: 'upstream_multiplier_increased', alert_id: 9, status: 'open' }], total: 1, page: 1, page_size: 100 })
    if (path.endsWith('/admin/finance/funds')) return fulfill(route, { wallet_cash: [{ wallet_id: 5, wallet_name: '现金钱包 A', balance_scope_key: 'shared-main', balance: '260', currency: 'USD', daily_cost: '10', available_days: '26', collected_at: '2026-07-27T08:00:00Z', sync_status: 'success', included_in_total: false, stale: true }], token_quota: [{ wallet_id: 6, wallet_name: '配额钱包 A', total_quota: '1000', used_quota: '100', remaining_quota: '900', currency: 'Token', collected_at: '2026-07-27T08:00:00Z', sync_status: 'success' }], customer_balance: '42', customer_cash: { payment: '100', refund: '10', payment_fees: '2', net_cash: '88' }, upstream_cash: { topup: '60', topup_available: true, topup_event_count: 1, net_cash_available: true, event_count: 1, refund: '5', adjustment: '0', recharge_bonus_income: '5', net_cash: '-55' }, stale_wallet_count: 1, failed_sync_count: 0 })
    return fulfill(route, {})
  })
}

test.describe('Finance owner report', () => {
  test.beforeEach(async ({ page }) => { await mockFinance(page) })

  test('shows current, historical loss, funds risk and auditable drill-down', async ({ page }) => {
    await page.goto('/admin/finance')
    await expect(page.getByRole('main').getByRole('heading', { name: '经营与财务' })).toBeVisible({ timeout: 15_000 })
    await expect(page.locator('p').filter({ hasText: /^客户本期计费$/ })).toBeVisible()
    await expect(page.getByText('预计赚 / 亏', { exact: true })).toBeVisible()
    await expect(page.getByText('本期倒贴 / 少收', { exact: true })).toBeVisible()
    await expect(page.getByText('还没核准的上游成本', { exact: true })).toBeVisible()
    await expect(page.getByText('成本覆盖率 98.00%')).toBeVisible()

    await page.getByRole('button', { name: '收入和亏损' }).click()
    await expect(page.getByText('亏损客户')).toBeVisible()
    await expect(page.getByText('上游账号 A')).toBeVisible()

    await page.getByRole('button', { name: '上游与资金' }).click()
    await expect(page.getByText('共享余额，不重复汇总', { exact: true })).toBeVisible()
    await expect(page.getByText('已过期', { exact: true })).toBeVisible()
    await expect(page.getByText('配额仅反映可用调用额度，不作为现金资产。', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: '上游财务资料' })).toBeVisible()
    await expect(page.getByText('上游 A', { exact: true }).first()).toBeVisible()

    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
    expect(results.violations.filter(item => ['critical', 'serious'].includes(item.impact || ''))).toEqual([])
  })

  test('keeps the owner summary usable on mobile', async ({ page, isMobile }) => {
    test.skip(!isMobile, 'mobile report acceptance')
    await page.goto('/admin/finance')
    await expect(page.locator('p').filter({ hasText: /^客户本期计费$/ })).toBeVisible()
    await page.getByRole('button', { name: '上游与资金' }).click()
    await expect(page.getByRole('heading', { name: '上游财务资料' })).toBeVisible()
  })
})
