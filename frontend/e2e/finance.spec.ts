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
  let settlementRevision = 2
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const path = url.pathname
    if (path.endsWith('/auth/me')) return fulfill(route, admin)
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
    if (path.endsWith('/admin/finance/funds')) return fulfill(route, { wallet_cash: [{ wallet_id: 5, wallet_name: '现金钱包 A', balance_scope_key: 'shared-main', balance: '260', currency: 'USD', daily_cost: '10', available_days: '26', collected_at: '2026-07-27T08:00:00Z', sync_status: 'success', included_in_total: false, stale: true }], token_quota: [{ wallet_id: 6, wallet_name: '配额钱包 A', total_quota: '1000', used_quota: '100', remaining_quota: '900', currency: 'Token', collected_at: '2026-07-27T08:00:00Z', sync_status: 'success' }], customer_cash: { payment: '100', refund: '10', payment_fees: '2', net_cash: '88' }, upstream_cash: { topup: '60', refund: '5', adjustment: '0', recharge_bonus_income: '5', net_cash: '-55' }, stale_wallet_count: 1, failed_sync_count: 0 })
    if (path.endsWith('/admin/finance/data-quality')) return fulfill(route, { quality, trend: [], items: [{ usage_log_id: 88, issue_type: 'missing_multiplier', related_type: 'account', related_id: 4, exposed_revenue: '12.5', first_detected_at: '2026-07-26T08:00:00Z', last_scanned_at: '2026-07-27T08:00:00Z', recalculable: true }], total: 1, page: 1, page_size: 100 })
    if (path.endsWith('/admin/finance/promotion-credit-reconciliations') && route.request().method() === 'GET') return fulfill(route, { items: [{ user_id: 8, user_email: 'finance@example.test', username: '历史优惠客户', detected_historical_bonus: '12.5', current_remaining_amount: '9', confirmed_remaining_amount: null, status: 'requires_reconciliation', cutover_at: '2026-07-01T00:00:00Z', created_at: '2026-07-01T00:00:00Z', resolved_at: null, resolved_by: null }], total: 1, page: 1, page_size: 100 })
    if (/\/admin\/finance\/promotion-credit-reconciliations\/\d+\/resolve$/.test(path)) return fulfill(route, { user_id: 8, status: 'resolved', confirmed_remaining_amount: '2.5' })
    if (path.endsWith('/admin/finance/alerts') && route.request().method() === 'GET') return fulfill(route, { items: [{ id: 9, alert_type: 'negative_profit', severity: 'critical', title: '持续亏损', description: '账号连续出现负毛利', dimension_type: 'account', dimension_id: 4, impact_amount: '5', request_count: 2, occurrence_count: 2, status: 'open', first_occurred_at: '2026-07-26T08:00:00Z', last_occurred_at: '2026-07-27T08:00:00Z', assignee_id: null, handled_by: null, handled_at: null }], total: 1, page: 1, page_size: 100 })
    if (/\/admin\/finance\/alerts\/\d+$/.test(path)) return fulfill(route, { id: 9, status: 'acknowledged' })
    const settlement = { id: 7, owner_type: 'account', owner_id: 12, account_id: 12, scope_key: 'account-12', previous_snapshot_id: 100, current_snapshot_id: 101, period_start: '2026-07-29T00:00:00Z', period_end: '2026-07-30T00:00:00Z', unit_semantics: 'fiat_currency', currency: 'USD', list_cost_delta: '10', actual_cost_delta: '2.2', observed_multiplier: '0.22', status: 'settled', current_revision: settlementRevision, request_count: 1, segment_count: 1, standard_cost_total: '10', allocated_cost_total: '2.2', difference_amount: '0' }
    const allocations = [{ id: 31, settlement_interval_id: 7, usage_log_id: 88, request_id: 'req-settlement', attempt_no: 1, revision: settlementRevision, standard_cost_weight: '10', allocation_rate: '1', allocated_cost: '2.2', created_at: '2026-07-30T00:01:00Z' }]
    if (path.endsWith('/admin/finance/settlements') && route.request().method() === 'GET') return fulfill(route, { items: [settlement], total: 1, page: 1, page_size: 20 })
    if (/\/admin\/finance\/settlements\/\d+\/reallocate$/.test(path)) { settlementRevision += 1; return fulfill(route, { interval: { ...settlement, current_revision: settlementRevision }, allocations: [{ ...allocations[0], revision: settlementRevision }] }) }
    if (/\/admin\/finance\/settlements\/\d+$/.test(path) && route.request().method() === 'GET') return fulfill(route, { interval: settlement, allocations })
    return fulfill(route, {})
  })
}

test.describe('Finance owner report', () => {
  test.beforeEach(async ({ page }) => { await mockFinance(page) })

  test('shows current, historical loss, funds risk and auditable drill-down', async ({ page }) => {
    await page.goto('/admin/finance')
    await expect(page.getByRole('heading', { name: '财务管理' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('今日账面毛利')).toBeVisible()
    await expect(page.getByText('综合利润', { exact: true })).toBeVisible()
    await expect(page.getByText('历史累计账面亏损')).toBeVisible()
    await expect(page.getByText('待结算成本')).toBeVisible()
    await expect(page.getByText('待闭合真实成本')).toBeVisible()
    await expect(page.getByText('成本覆盖率 98.00%')).toBeVisible()

    await page.getByRole('button', { name: '亏损追踪' }).click()
    await expect(page.getByText('亏损客户')).toBeVisible()
    await expect(page.getByText('上游账号 A')).toBeVisible()

    await page.getByRole('button', { name: '资金余额' }).click()
    await expect(page.getByText('共享余额，不重复汇总')).toBeVisible()
    await expect(page.getByText('已过期')).toBeVisible()
    await expect(page.getByText('配额仅反映可用调用额度，不作为现金资产。')).toBeVisible()

    await page.getByRole('button', { name: '成本结算' }).click()
    await expect(page.getByText('0.22x')).toBeVisible()
    await page.getByRole('button', { name: '查看分摊' }).click()
    await expect(page.getByText('req-settlement')).toBeVisible()
    await page.getByPlaceholder('说明重新分摊的业务原因').fill('修正标准成本权重')
    await page.getByRole('button', { name: '创建新修订' }).click()
    await expect(page.getByText('当前修订 v3')).toBeVisible()

    await page.getByRole('button', { name: '数据质量' }).click()
    await expect(page.getByText('历史优惠客户')).toBeVisible()
    await page.getByRole('spinbutton', { name: '确认剩余额度' }).fill('2.5')
    await page.getByRole('textbox', { name: '核对说明' }).fill('已与客户台账核对')
    await page.getByRole('button', { name: '确认并入账' }).click()
    await expect(page.getByText('历史优惠待核对')).toBeVisible()

    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
    expect(results.violations.filter(item => ['critical', 'serious'].includes(item.impact || ''))).toEqual([])
  })

  test('keeps the owner summary usable on mobile', async ({ page, isMobile }) => {
    test.skip(!isMobile, 'mobile report acceptance')
    await page.goto('/admin/finance')
    await expect(page.getByText('今日账面毛利')).toBeVisible()
    await page.getByRole('button', { name: '财务预警' }).click()
    await expect(page.getByRole('heading', { name: '持续亏损' })).toBeVisible()
  })
})
