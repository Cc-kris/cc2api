import { expect, test, type TestInfo } from '@playwright/test'
import { login, navigate, screenshot } from '../helpers/ui'
import { anthropicBody, gatewayTry } from '../helpers/api'
import { scenario, mockRequests } from '../helpers/mock'
import { scalar } from '../helpers/db'
import { seed } from '../helpers/seed'

function localeFor(t: TestInfo): 'zh-CN' | 'en-US' { return t.project.name === 'mobile-en' ? 'en-US' : 'zh-CN' }

test('AC05 任意错误换号：同账号重试成功，输入仍按正常价', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac05', balance: 10, group: { failover_on_any_error: true } })
  const key = s.upstreamKey
  // 隔离环境本轮将 failover_same_account_retries 设为 2，匹配文档前置条件。
  await scenario(key, { fail_first_n: 1, fail_mode: 'http_error', fail_status: 500, usage: { input: 1000, output: 200 }, stream: false })
  const r = await gatewayTry({ path: '/v1/messages', apiKey: s.key.key, body: anthropicBody('claude-sonnet-4-6') })
  expect(r.status, r.text).toBe(200)
  await expect.poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id}`)), { timeout: 20000 }).toBe(1)
  const id = scalar(`select id from usage_logs where api_key_id=${s.key.id} order by id desc limit 1`)
  expect(Number(scalar(`select actual_cost from usage_logs where id=${id}`)).toFixed(8)).toBe('0.00600000')
  expect(Number(scalar(`select input_tokens from usage_logs where id=${id}`))).toBe(1000)
  expect(Number(scalar(`select cache_read_tokens from usage_logs where id=${id}`))).toBe(0)
  const reqs = await mockRequests()
  expect(reqs.filter(x => x.key === key && x.path.startsWith('/v1/messages')).length).toBe(2)
  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.00600000')
  await screenshot(page, testInfo, 'AC05-same-account-normal-input-price')
})

test('AC23 Anthropic Fast：两倍费用；倍率 1.5 时费用 0.09000000', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac23', balance: 10, rate: 1.5 })
  await scenario(s.upstreamKey, { usage: { input: 1000, output: 1000 }, stream: false })
  const body = { ...anthropicBody('claude-opus-5'), speed: 'fast' }
  const r = await gatewayTry({ path: '/v1/messages', apiKey: s.key.key, body })
  expect(r.status, r.text).toBe(200)
  await expect.poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id}`)), { timeout: 20000 }).toBe(1)
  expect(scalar(`select service_tier from usage_logs where api_key_id=${s.key.id}`)).toBe('fast')
  expect(Number(scalar(`select actual_cost from usage_logs where api_key_id=${s.key.id}`)).toFixed(8)).toBe('0.09000000')
  expect(Number(scalar(`select balance from users where id=${s.user.id}`)).toFixed(8)).toBe('9.91000000')
  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.09000000')
  await page.locator('.cursor-help').last().scrollIntoViewIfNeeded()
  await page.locator('.cursor-help').last().hover({ force: true })
  await expect(page.locator('body')).toContainText(/fast|快速/i)
  await screenshot(page, testInfo, 'AC23-fast-tier-090')
})

test('AC24 上游 standard 只降不升；unknown 档按请求 fast，不任意加价', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac24', balance: 10 })
  for (const [speed, expectedCost, expectedTier] of [['standard', '0.03000000', 'standard'], ['ultrafast', '0.06000000', 'fast']] as const) {
    await scenario(s.upstreamKey, { usage: { input: 1000, output: 1000 }, speed, stream: false })
    const r = await gatewayTry({ path: '/v1/messages', apiKey: s.key.key, body: { ...anthropicBody('claude-opus-5'), speed: 'fast' } })
    expect(r.status, r.text).toBe(200)
    await expect.poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id} and service_tier='${expectedTier}'`)), { timeout: 20000 }).toBe(1)
    const id = scalar(`select id from usage_logs where api_key_id=${s.key.id} order by id desc limit 1`)
    expect(Number(scalar(`select actual_cost from usage_logs where id=${id}`)).toFixed(8)).toBe(expectedCost)
  }
  expect(Number(scalar(`select balance from users where id=${s.user.id}`)).toFixed(8)).toBe('9.91000000')
  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.03000000')
  await expect(page.locator('body')).toContainText('$0.06000000')
  await screenshot(page, testInfo, 'AC24-downgrade-standard-and-unknown')
})
