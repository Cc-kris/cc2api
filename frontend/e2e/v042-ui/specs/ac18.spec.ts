import { expect, test, type TestInfo } from '@playwright/test'
import { login, navigate, screenshot } from '../helpers/ui'
import { anthropicBody, gatewayTry } from '../helpers/api'
import { scenario } from '../helpers/mock'
import { rows, scalar } from '../helpers/db'
import { seed } from '../helpers/seed'

function localeFor(t: TestInfo): 'zh-CN' | 'en-US' {
  return t.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'
}

test('AC18 断流换号续写：只记 1 条用量、按 B 计费 0.01140000、余额只扣一次', async ({ page }, testInfo) => {
  const s = await seed({
    prefix: 'ac18',
    balance: 10,
    rate: 1,
    group: { failover_on_any_error: true },
    accounts: [{ priority: 1 }, { priority: 2 }],
  })
  expect(s.accounts.length).toBe(2)

  // 账号 A：正文输出到一半即断流（无 stop_reason），已观测输入 1000/输出 300。
  await scenario(s.accounts[0].upstreamKey, {
    usage: { input: 1000, output: 300 },
    stream: true,
    mode: 'disconnect_after_delta',
  })
  // 账号 B：续写并正常结束，上报输入 1300/输出 500。
  await scenario(s.accounts[1].upstreamKey, {
    usage: { input: 1300, output: 500 },
    stream: true,
  })

  const res = await gatewayTry({
    path: '/v1/messages',
    apiKey: s.key.key,
    body: anthropicBody('claude-sonnet-4-6', true),
  })
  expect(res.status, res.text).toBe(200)
  expect(res.text).toContain('message_stop')

  await expect
    .poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${s.key.id}`)), { timeout: 20000 })
    .toBe(1)

  const logId = scalar(`select id from usage_logs where api_key_id=${s.key.id}`)
  expect(Number(scalar(`select actual_cost from usage_logs where id=${logId}`)).toFixed(8)).toBe('0.01140000')
  expect(Number(scalar(`select input_tokens from usage_logs where id=${logId}`))).toBe(1300)
  expect(Number(scalar(`select output_tokens from usage_logs where id=${logId}`))).toBe(500)
  expect(Number(scalar(`select balance from users where id=${s.user.id}`)).toFixed(8)).toBe('9.98860000')

  // 本隔离环境显式配置 gateway.failover_same_account_retries=0，匹配 AC18 的两次换号尝试。
  const attempts = rows(`select account_id, input_tokens, output_tokens from usage_upstream_attempts where usage_log_id=${logId} order by attempt_no`)
  expect(attempts.length).toBe(2)
  expect(Number(attempts[0][0])).toBe(Number(s.accounts[0].id))
  expect([Number(attempts[0][1]), Number(attempts[0][2])]).toEqual([1000, 300])
  expect(Number(attempts[1][0])).toBe(Number(s.accounts[1].id))
  expect([Number(attempts[1][1]), Number(attempts[1][2])]).toEqual([1300, 500])
  expect(Number(scalar(`select count(*) from usage_billing_dedup where api_key_id=${s.key.id}`))).toBe(1)

  await login(page, s.email, s.password, localeFor(testInfo))
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.01140000')
  await expect(page.locator('body')).toContainText('$9.98860000')
  await screenshot(page, testInfo, 'AC18-continuation-single-charge')
})
