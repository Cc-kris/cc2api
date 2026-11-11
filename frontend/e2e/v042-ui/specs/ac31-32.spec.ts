import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { login, navigate, screenshot, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers/ui'
import { anthropicBody, gatewayTry, raw } from '../helpers/api'
import { scenario } from '../helpers/mock'
import { scalar, sql } from '../helpers/db'
import { seed } from '../helpers/seed'
import { ac31Labels, localeFor, selectUsageFilter, setInputValueRaw, trackAccountWrites, type Ac31Locale } from '../helpers/ac31-32'

const require = createRequire(import.meta.url)
const XLSX = require('xlsx') as typeof import('xlsx')

async function accountRow(page: Page, name: string) {
  const tableRow = page.locator('tr').filter({ hasText: name })
  const card = page.locator('div.rounded-lg.border').filter({ hasText: name })
    .filter({ has: page.getByRole('button', { name: /编辑|Edit/i }) })
  return tableRow.or(card)
}

async function adminPage(page: Page, path: string, locale: Ac31Locale) {
  await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, locale)
  await navigate(page, path)
  await expect(path === '/admin/usage'
    ? page.locator('[data-test="usage-content-ready"]')
    : page.getByRole('main')).toBeVisible()
}

test('AC31 审计字段、请求 ID 查询、模型筛选、列设置和真实 Excel 导出', async ({ page }, testInfo) => {
  const locale = localeFor(testInfo)
  const l = ac31Labels(locale)
  const s = await seed({ prefix: 'ac31', balance: 10 })
  const requestId = 'req_abc_' + s.suffix
  const sessionId = 'sess-audit-' + s.suffix
  await scenario(s.upstreamKey, {
    request_id: requestId,
    model: 'claude-sonnet-4-5-audit',
    usage: { input: 1000, output: 200 },
    stream: false,
  })
  const r = await gatewayTry({
    path: '/v1/messages', apiKey: s.key.key,
    headers: { session_id: sessionId },
    body: { ...anthropicBody('claude-sonnet-4-6'), output_config: { effort: 'max' } },
  })
  expect(r.status, r.text).toBe(200)
  await expect.poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id} and upstream_request_id='${requestId}'`)), { timeout: 20000 }).toBe(1)
  const id = Number(scalar(`select id from usage_logs where api_key_id=${s.key.id}`))
  const fields = sql(`select concat_ws('|', coalesce(session_id,'∅'), coalesce(upstream_response_model,'∅'), coalesce(upstream_model_mismatch::text,'∅'), coalesce(upstream_request_id,'∅'), coalesce(requested_reasoning_effort,'∅')) from usage_logs where id=${id}`)
  expect(fields).toBe(`${sessionId}|claude-sonnet-4-5-audit|true|${requestId}|max`)
  expect(Number(scalar(`select actual_cost from usage_logs where id=${id}`)).toFixed(8)).toBe('0.00600000')

  await adminPage(page, '/admin/usage', locale)
  const requestFilter = page.getByTestId('usage-filter-upstream-request-id')
  await requestFilter.scrollIntoViewIfNeeded()
  await requestFilter.fill(requestId)
  await requestFilter.press('Enter')
  await expect(page.getByText(requestId, { exact: true }).first()).toBeVisible()
  await selectUsageFilter(page, 'usage-filter-upstream-model-mismatch', l.mismatchOnly)
  await expect(page.getByText(l.mismatchBadge, { exact: true }).first()).toBeVisible()
  await selectUsageFilter(page, 'usage-filter-upstream-model-mismatch', l.mismatchMatched)
  await expect(page.getByRole('heading', { name: locale === 'zh-CN' ? '暂无数据' : 'No data', exact: true })).toBeVisible()
  await selectUsageFilter(page, 'usage-filter-upstream-model-mismatch', l.mismatchOnly)
  await expect(page.getByText(l.mismatchBadge, { exact: true }).first()).toBeVisible()

  await page.getByRole('button', { name: l.columnSettings }).click()
  await page.getByRole('button', { name: l.sessionId, exact: true }).click()
  await expect(page.getByText(sessionId, { exact: true })).toBeVisible()
  // 列设置菜单会覆盖手机端导出按钮；通过同一真实界面控件关闭它。
  await page.getByRole('button', { name: l.columnSettings }).click()
  const exported = page.waitForEvent('download')
  await page.getByRole('button', { name: l.exportButton, exact: true }).click()
  const download = await exported
  const file = await download.path()
  expect(file).not.toBeNull()
  const workbook = XLSX.read(readFileSync(file!), { type: 'buffer' })
  const sheet = workbook.Sheets[workbook.SheetNames[0]]
  const rows = XLSX.utils.sheet_to_json<unknown[]>(sheet, { header: 1, defval: '' })
  expect(rows).toHaveLength(2)
  const row = Object.fromEntries(rows[0].map((header, i) => [header, rows[1][i]]))
  expect(row[l.upstreamRequestId]).toBe(requestId)
  expect(row[l.sessionIdHeader]).toBe(sessionId)
  expect(row[l.upstreamResponseModelHeader]).toBe('claude-sonnet-4-5-audit')
  expect(row[l.mismatchHeader]).toBe(l.no)
  expect(row[l.requestedEffortHeader]).toBe('Max')
  await screenshot(page, testInfo, 'AC31-audit-search-columns-export')

  // 只有粘性会话种子而没有显式会话头：审计 session_id 必须为空。
  await scenario(s.upstreamKey, { usage: { input: 1000, output: 200 }, stream: false })
  const sticky = await gatewayTry({ path: '/v1/messages', apiKey: s.key.key, body: {
    ...anthropicBody('claude-sonnet-4-6'), prompt_cache_key: 'sticky-only-' + s.suffix,
    metadata: { user_id: JSON.stringify({ session_id: '11111111-2222-3333-4444-555555555555' }) },
  } })
  expect(sticky.status, sticky.text).toBe(200)
  await expect.poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id}`))).toBe(2)
  const stickyId = scalar(`select id from usage_logs where api_key_id=${s.key.id} order by id desc limit 1`)
  expect(scalar(`select coalesce(session_id,'') from usage_logs where id=${stickyId}`)).toBe('')
})

test('AC32 单个及批量界面拒绝非法请求 ID 头名，后端同样拒绝', async ({ page }, testInfo) => {
  const locale = localeFor(testInfo)
  const l = ac31Labels(locale)
  const s = await seed({ prefix: 'ac32' })
  await adminPage(page, '/admin/accounts', locale)
  const search = page.getByPlaceholder(/Search accounts|搜索账号/i)
  await search.fill(s.account.name)
  await search.press('Enter')
  const writes = trackAccountWrites(page)
  await (await accountRow(page, s.account.name)).getByRole('button', { name: /编辑|Edit/, exact: true }).click()
  const dialog = page.getByRole('dialog').last()
  await expect(dialog).toBeVisible()
  for (const invalid of ['x'.repeat(65), 'bad\nheader']) {
    await setInputValueRaw(page, '[data-testid="upstream-request-id-header"]', invalid)
    await expect(dialog.getByTestId('upstream-request-id-header-error')).toHaveText(l.upstreamRequestIdHeaderInvalid)
    await dialog.locator('button[form="edit-account-form"]').click()
    await expect(dialog).toBeVisible()
  }
  expect(writes).toEqual([])
  await dialog.getByRole('button', { name: l.cancelButton }).click()

  await page.getByTestId('bulk-edit-filtered').click()
  const bulk = page.getByRole('dialog').last()
  await expect(bulk).toBeVisible()
  await bulk.locator('#bulk-edit-upstream-request-id-header-enabled').check()
  await bulk.locator('#bulk-edit-upstream-request-id-header').fill('bad:header')
  await expect(bulk.getByTestId('upstream-request-id-header-error')).toHaveText(l.upstreamRequestIdHeaderInvalid)
  await bulk.locator('button[form="bulk-edit-account-form"]').click()
  await expect(bulk).toBeVisible()
  expect(writes).toEqual([])
  await screenshot(page, testInfo, 'AC32-account-header-validation')

  const backend = await raw('PUT', `http://127.0.0.1:8280/api/v1/admin/accounts/${s.account.id}`,
    { extra: { upstream_request_id_header: 'bad\nheader' } }, s.admin)
  expect(backend.status).toBe(400)
  expect(backend.text).toContain('INVALID_UPSTREAM_REQUEST_ID_HEADER')
})
