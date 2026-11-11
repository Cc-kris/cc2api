import { expect, test, type Page } from '@playwright/test'
import { api, createGroup, createUser } from '../helpers/api'
import { sql, scalar } from '../helpers/db'
import { seed } from '../helpers/seed'
import { login, navigate, screenshot } from '../helpers/ui'

async function redeemThroughUI(page: Page, code: string) {
  await page.locator('#code').fill(code)
  const [response] = await Promise.all([
    page.waitForResponse(r => r.request().method() === 'POST' && /\/redeem-codes\/redeem$|\/redeem$/.test(new URL(r.url()).pathname)),
    page.locator('button[type="submit"]').click(),
  ])
  expect(response.status(), await response.text()).toBeLessThan(300)
  await expect(page.locator('body')).toContainText(/Code Redeemed Successfully|Redeemed successfully|兑换成功/i)
}

test('AC14 十个独立兑换码经真实界面连续续期累计 300 天', async ({ page }, testInfo) => {
  // Exact ten-way concurrent renewal arithmetic is additionally covered by the
  // PostgreSQL integration suite. This spec verifies the supported UI workflow.
  const s = await seed({ prefix: 'ac14' })
  // The subscription-only UI flow must not require an API key for a group that
  // users cannot bind until they have an active subscription.
  const group = await createGroup(s.admin, {
    name: 'subscription-' + s.suffix,
    platform: 'anthropic',
    rate_multiplier: 1,
    subscription_type: 'subscription',
  })
  const email = 'ac14-subscription-' + Date.now() + '@v042e2e.local'
  const user = await createUser(s.admin, {
    email,
    password: 'E2e-Passw0rd!',
    balance: 0,
    allowed_groups: [],
  })
  const codes = await api('POST', '/admin/redeem-codes/generate', {
    count: 10, type: 'subscription', value: 0, group_id: group.id, validity_days: 30,
  }, s.admin) as Array<{ code: string }>
  expect(codes).toHaveLength(10)
  expect(new Set(codes.map(c => c.code)).size).toBe(10)
  const locale = testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'
  await login(page, email, 'E2e-Passw0rd!', locale)
  await navigate(page, '/redeem')
  for (const { code } of codes) {
    await redeemThroughUI(page, code)
    await navigate(page, '/redeem')
  }
  await expect.poll(() => scalar(`select count(*) from user_subscriptions where user_id=${user.id} and group_id=${group.id}`)).toBe('1')
  const fields = sql(`select concat_ws('|', extract(epoch from starts_at)::bigint, extract(epoch from expires_at)::bigint) from user_subscriptions where user_id=${user.id} and group_id=${group.id}`).split('|').map(Number)
  expect(fields[1] - fields[0]).toBe(300 * 86400)
  expect(scalar(`select count(*) from redeem_codes where type='subscription' and group_id=${group.id} and used_by=${user.id}`)).toBe('10')
  await screenshot(page, testInfo, 'AC14-ten-real-ui-renewals')
})
