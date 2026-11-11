import { expect, test, type TestInfo } from '@playwright/test'
import { login, navigate, screenshot } from '../helpers/ui'
import { gatewayOpenAITry, openaiChatBody } from '../helpers/api'
import { scenario } from '../helpers/mock'
import { scalar } from '../helpers/db'
import { seed } from '../helpers/seed'

const localeFor = (t: TestInfo): 'zh-CN' | 'en-US' => t.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'

test('AC24 OpenAI priority 请求由上游 default 服务时只降不升', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac24openai', platform: 'openai', balance: 10 })
  for (const [observed, tier, expected] of [['priority', 'priority', '0.03500000'], ['default', 'default', '0.01750000']] as const) {
    await scenario(s.upstreamKey, { usage: { input: 1000, output: 1000 }, service_tier: observed })
    const r = await gatewayOpenAITry({ path: '/v1/chat/completions', apiKey: s.key.key, body: openaiChatBody('gpt-5.4', { service_tier: 'priority' }) })
    expect(r.status, r.text).toBe(200)
    await expect.poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id} and service_tier='${tier}'`)), { timeout: 20000 }).toBe(1)
    const id = scalar(`select id from usage_logs where api_key_id=${s.key.id} order by id desc limit 1`)
    expect(Number(scalar(`select actual_cost from usage_logs where id=${id}`)).toFixed(8)).toBe(expected)
  }
  expect(Number(scalar(`select balance from users where id=${s.user.id}`)).toFixed(8)).toBe('9.94750000')
  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.03500000')
  await expect(page.locator('body')).toContainText('$0.01750000')
  const latestCost = page.locator('tr').filter({ hasText: '$0.01750000' }).first()
    .or(page.locator('div.rounded-lg.border').filter({ hasText: '$0.01750000' }).first())
  const costInfo = latestCost.locator('.cursor-help').last()
  await costInfo.scrollIntoViewIfNeeded()
  await costInfo.hover({ force: true })
  await expect(page.locator('body')).toContainText(/Standard|标准/i)
  await screenshot(page, testInfo, 'AC24-openai-priority-downgraded-default')
})
