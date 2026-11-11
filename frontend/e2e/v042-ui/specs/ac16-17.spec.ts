import { expect, test, type TestInfo } from '@playwright/test'
import { login, navigate, screenshot } from '../helpers/ui'
import { anthropicBody, gatewayTry } from '../helpers/api'
import { scenario } from '../helpers/mock'
import { scalar } from '../helpers/db'
import { seed } from '../helpers/seed'

function localeFor(t: TestInfo): 'zh-CN' | 'en-US' {
  return t.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'
}

test('AC16 流中断但已产生用量：记账一次 0.00750000，余额 9.99250000', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac16', balance: 10, rate: 1 })
  await scenario(s.upstreamKey, { usage: { input: 1000, output: 300 }, stream: true, mode: 'disconnect_after_usage' })

  const res = await gatewayTry({
    path: '/v1/messages',
    apiKey: s.key.key,
    body: anthropicBody('claude-sonnet-4-6', true),
  })
  // 客户端侧会看到错误/断流；关键是服务端仍按已观测用量记账。
  expect(res.threw || res.status >= 400 || !res.text.includes('message_stop'), 'client should observe interruption').toBeTruthy()

  await expect
    .poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id}`)), { timeout: 20000 })
    .toBe(1)

  expect(Number(scalar(`select actual_cost from usage_logs where api_key_id=${s.key.id}`)).toFixed(8)).toBe('0.00750000')
  expect(Number(scalar(`select output_tokens from usage_logs where api_key_id=${s.key.id}`)).toFixed(0)).toBe('300')
  expect(Number(scalar(`select balance from users where id=${s.user.id}`)).toFixed(8)).toBe('9.99250000')

  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.00750000')
  await expect(page.locator('body')).toContainText('$9.99250000')
  await screenshot(page, testInfo, 'AC16-partial-usage')
})

test('AC17 message_start 前断流：不写用量、余额不变', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac17', balance: 10, rate: 1 })
  await scenario(s.upstreamKey, { stream: true, mode: 'disconnect_before_start' })

  const res = await gatewayTry({
    path: '/v1/messages',
    apiKey: s.key.key,
    body: anthropicBody('claude-sonnet-4-6', true),
  })
  expect(res.threw || res.status >= 400, 'client should see an error').toBeTruthy()

  await page.waitForTimeout(2500)
  expect(Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id}`))).toBe(0)
  expect(Number(scalar(`select balance from users where id=${s.user.id}`)).toFixed(8)).toBe('10.00000000')

  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$10.00000000')
  await expect(page.locator('body')).not.toContainText('$0.00750000')
  await screenshot(page, testInfo, 'AC17-no-usage')
})
