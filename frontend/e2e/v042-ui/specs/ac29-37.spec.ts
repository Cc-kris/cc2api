import { expect, test, type TestInfo, type Page } from '@playwright/test'
import { login, navigate, screenshot, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers/ui'
import { anthropicBody, gatewayTry, gatewayOpenAITry, openaiChatBody, updateGroup, raw } from '../helpers/api'
import { scenario } from '../helpers/mock'
import { scalar } from '../helpers/db'
import { seed } from '../helpers/seed'
import { assertLatestCost } from '../helpers/ac25-28'

const localeFor = (t: TestInfo): 'zh-CN'|'en-US' => t.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'
const money = (v: string) => Number(v).toFixed(8)

async function setLongContextUI(page: Page, s: Awaited<ReturnType<typeof seed>>, enabled: boolean, t: TestInfo) {
  await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, localeFor(t))
  await navigate(page, '/admin/groups')
  // Group rows are windowed; search before locating a row outside the viewport.
  await page.getByRole('textbox', { name: /搜索分组|Search groups/i }).fill(s.group.name)
  const row = page.locator('tr').filter({ hasText: s.group.name })
  const card = page.locator('div.rounded-lg.border').filter({ hasText: s.group.name })
    .filter({ has: page.getByRole('button', { name: /编辑|Edit/i }) })
  await row.or(card).getByRole('button', { name: /编辑|Edit/i }).click()
  const dialog = page.getByRole('dialog').last()
  await expect(dialog).toBeVisible()
  const target = dialog.locator('label').filter({ hasText: /长上下文|Long.context/i }).locator('input[type="checkbox"]')
  await target.scrollIntoViewIfNeeded()
  if (enabled) await target.check(); else await target.uncheck()
  await screenshot(page, t, `long-context-ui-${enabled}`)
  const [response] = await Promise.all([
    page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith(`/admin/groups/${s.group.id}`)),
    dialog.getByRole('button', { name: /更新|Update/i }).click(),
  ])
  expect(response.status(), await response.text()).toBe(200)
  await expect(dialog).toBeHidden()
}

async function assertCost(s: Awaited<ReturnType<typeof seed>>, expected: string) {
  await expect.poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id}`)), { timeout: 20000 }).toBe(1)
  expect(money(scalar(`select actual_cost from usage_logs where api_key_id=${s.key.id}`))).toBe(expected)
}

test('AC29 gpt-5.4 长上下文开关控制整请求阶梯价', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac29', platform: 'openai', balance: 100, group: { long_context_pricing_enabled: false } })
  await scenario(s.upstreamKey, { usage: { input: 300000, output: 1000 } })
  let r = await gatewayOpenAITry({ path: '/v1/chat/completions', apiKey: s.key.key, body: openaiChatBody('gpt-5.4') })
  expect(r.status, r.text).toBe(200)
  await assertCost(s, '0.76500000')
  await setLongContextUI(page, s, true, testInfo)
  r = await gatewayOpenAITry({ path: '/v1/chat/completions', apiKey: s.key.key, body: openaiChatBody('gpt-5.4') })
  expect(r.status, r.text).toBe(200)
  await assertLatestCost(s.key.id, '1.52250000')
  await screenshot(page, testInfo, 'AC29-group-long-context-enabled')
})

test('AC37 Gemini 原生与兼容接口长上下文均按整请求计价', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac37', platform: 'gemini', balance: 100, group: { long_context_pricing_enabled: true } })
  await scenario(s.upstreamKey, { usage: { input: 300000, output: 1000 } })
  let r = await raw('POST', 'http://127.0.0.1:8280/v1beta/models/gemini-2.5-pro:generateContent', { contents: [{ role: 'user', parts: [{ text: 'hello' }] }] }, undefined, { authorization: 'Bearer ' + s.key.key })
  expect(r.status, r.text).toBe(200)
  await assertCost(s, '0.76500000')
  await updateGroup(s.admin, s.group.id, { long_context_pricing_enabled: false })
  r = await raw('POST', 'http://127.0.0.1:8280/v1beta/models/gemini-2.5-pro:generateContent', { contents: [{ role: 'user', parts: [{ text: 'hello' }] }] }, undefined, { authorization: 'Bearer ' + s.key.key })
  expect(r.status, r.text).toBe(200)
  let id = scalar(`select id from usage_logs where api_key_id=${s.key.id} order by id desc limit 1`)
  expect(money(scalar(`select actual_cost from usage_logs where id=${id}`))).toBe('0.38500000')
  await updateGroup(s.admin, s.group.id, { long_context_pricing_enabled: true })
  const compat = await gatewayOpenAITry({ path: '/v1/chat/completions', apiKey: s.key.key, body: openaiChatBody('gemini-2.5-pro') })
  expect(compat.status, compat.text).toBe(200)
  id = scalar(`select id from usage_logs where api_key_id=${s.key.id} order by id desc limit 1`)
  expect(money(scalar(`select actual_cost from usage_logs where id=${id}`))).toBe('0.76500000')
  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.76500000')
  await expect(page.locator('body')).toContainText('$0.38500000')
  await screenshot(page, testInfo, 'AC37-gemini-native-compatible-long-context')
})

test('AC37 claude-sonnet-4-5 历史 200k 阶梯整请求计价', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac37claude', balance: 100, group: { long_context_pricing_enabled: true } })
  await scenario(s.upstreamKey, { usage: { input: 300000, output: 1000 } })
  const r = await gatewayTry({ path: '/v1/messages', apiKey: s.key.key, body: anthropicBody('claude-sonnet-4-5') })
  expect(r.status, r.text).toBe(200)
  await assertCost(s, '1.82250000')
  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$1.82250000')
  await screenshot(page, testInfo, 'AC37-claude-sonnet45-long-context')
})
