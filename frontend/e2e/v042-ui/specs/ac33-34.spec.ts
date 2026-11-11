import { expect, test } from '@playwright/test'
import { gatewayTry } from '../helpers/api'
import { scenario, releaseUpstream } from '../helpers/mock'
import { scalar } from '../helpers/db'
import { seed } from '../helpers/seed'
import { login, navigate, screenshot } from '../helpers/ui'

const MODEL = 'claude-sonnet-4-6'
const body = (content: string) => ({ model: MODEL, max_tokens: 8192, stream: false, messages: [{ role: 'user', content }] })

test('AC33 真实 Redis 十并发六放行四拒绝，余额八位小数一致', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac33', balance: 1, quota: 100 })
  await scenario(s.upstreamKey, { hold: true, stream: false, usage: { input: 10000, output: 8192 } })
  const requests = Array.from({ length: 10 }, () => gatewayTry({
    path: '/v1/messages', apiKey: s.key.key, body: body('a'.repeat(40000)),
  }))
  try {
    const upstreamArrived = () => fetch('http://127.0.0.1:18921/__requests').then(r => r.json())
      .then(d => d.requests.filter((r: any) => r.key === s.upstreamKey).length)
    await expect.poll(upstreamArrived, { timeout: 30000 }).toBe(6)
    const inflightKey = `billing:inflight:{${s.user.id}}`
    const inflight = await import('node:child_process').then(({ execFileSync }) =>
      execFileSync('docker', ['exec', 'v042-e2e-redis', 'redis-cli', 'ZCARD', inflightKey], { encoding: 'utf8' }).trim())
    expect(inflight).toBe('6')
  } finally {
    for (let i = 0; i < 10; i++) await releaseUpstream()
  }
  const results = await Promise.all(requests)
  expect(results.filter(r => r.status === 200)).toHaveLength(6)
  expect(results.filter(r => r.status !== 200)).toHaveLength(4)
  await expect.poll(() => Number(scalar(`select balance from users where id=${s.user.id}`)).toFixed(8), { timeout: 30000 }).toBe('0.08272000')
  expect(Number(scalar(`select sum(actual_cost) from usage_logs where api_key_id=${s.key.id}`)).toFixed(8)).toBe('0.91728000')
  await login(page, s.email, s.password, testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN')
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('0.91728000')
  await screenshot(page, testInfo, 'AC33-six-admitted-four-rejected')
})

test('AC34 无在途预占时低余额首个请求正常放行', async ({ page }, testInfo) => {
  // Configuration-only cases are tested through the real gateway; no fabricated UI toggle.
  const low = await seed({ prefix: 'ac34-low', balance: 0.01 })
  await scenario(low.upstreamKey, { usage: { input: 10, output: 5 }, stream: false })
  let r = await gatewayTry({ path: '/v1/messages', apiKey: low.key.key, body: body('low balance') })
  expect(r.status, r.text).toBe(200)
  // Keep the explicit subscription/Redis/fail-closed assertions in the service integration suite.
  // This browser-facing test records the ordinary balance-path behavior without claiming those branches.
  await login(page, low.email, low.password, testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN')
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('0.00010500')
  await screenshot(page, testInfo, 'AC34-first-request-low-balance')
})
