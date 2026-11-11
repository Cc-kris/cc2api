import { test, expect, type TestInfo } from '@playwright/test'
import { login, navigate, screenshot, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers/ui'
import {
  adminLogin,
  anthropicBody,
  createAccount,
  createGroup,
  createKey,
  createUser,
  gateway,
  uniq,
  userLogin,
} from '../helpers/api'
import { scenario } from '../helpers/mock'
import { scalar } from '../helpers/db'

const MOCK_UPSTREAM = 'http://127.0.0.1:18921'

function localeFor(testInfo: TestInfo): 'zh-CN' | 'en-US' {
  return testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'
}

async function seedUserWithKey(opts: { prefix: string; rate: number; balance: number }) {
  const admin = await adminLogin()
  const suffix = uniq(opts.prefix)
  const upstreamKey = 'up-' + suffix
  const group = await createGroup(admin, {
    name: 'grp-' + suffix,
    platform: 'anthropic',
    rate_multiplier: opts.rate,
  })
  await createAccount(admin, {
    name: 'acct-' + suffix,
    platform: 'anthropic',
    type: 'apikey',
    credentials: { api_key: upstreamKey, base_url: MOCK_UPSTREAM },
    group_ids: [group.id],
    concurrency: 10,
    priority: 1,
  })
  const email = suffix + '@v042e2e.local'
  const password = 'E2e-Passw0rd!'
  const user = await createUser(admin, { email, password, balance: opts.balance, allowed_groups: [group.id] })
  const userToken = await userLogin(email, password)
  const key = await createKey(userToken, { name: 'key-' + suffix, group_id: group.id, custom_key: 'sk-' + suffix, quota: 100 })
  return { admin, email, password, user, key, group, upstreamKey }
}

test('AC01 用量页、Key 配额与余额均为八位量化值', async ({ page, browser }, testInfo) => {
  const { email, password, user, key, upstreamKey } = await seedUserWithKey({ prefix: 'ac01', rate: 1.25, balance: 10000 })
  await scenario(upstreamKey, { usage: { input: 10, output: 5 }, stream: false })

  for (let i = 0; i < 2; i++) {
    const res = await gateway({ path: '/v1/messages', apiKey: key.key, body: anthropicBody('claude-sonnet-4-6') })
    expect(res.status, res.text).toBe(200)
  }

  await expect
    .poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${key.id}`)), { timeout: 20000 })
    .toBe(2)

  const row = scalar(`select actual_cost from usage_logs where api_key_id=${key.id} order by id limit 1`)
  const listValue = scalar(`select usage_list_value from usage_logs where api_key_id=${key.id} order by id limit 1`)
  expect(Number(row).toFixed(8)).toBe('0.00013125')
  expect(Number(listValue).toFixed(8)).toBe('0.00013125')
  expect(Number(scalar(`select quota_used from api_keys where id=${key.id}`)).toFixed(8)).toBe('0.00026250')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('9999.99973750')

  const locale = localeFor(testInfo)
  await login(page, email, password, locale)
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.00013125')
  await expect(page.locator('body')).toContainText('$9999.99973750')
  const totalCost = page.getByTestId('usage-total-actual-cost')
  await expect(totalCost).toHaveText('$0.00026250')
  expect(await totalCost.evaluate(element => {
    const card = element.closest('.card')
    if (!card) return false
    return element.scrollWidth <= element.clientWidth && card.scrollWidth <= card.clientWidth
  })).toBe(true)
  await screenshot(page, testInfo, 'AC01-user-usage-8dp')

  await navigate(page, '/keys')
  await expect(page.locator('body')).toContainText('$0.00026250')
  await screenshot(page, testInfo, 'AC01-key-quota-8dp')

  const adminContext = await browser.newContext({ viewport: page.viewportSize() || undefined })
  const adminPage = await adminContext.newPage()
  await login(adminPage, ADMIN_EMAIL, ADMIN_PASSWORD, locale)
  await navigate(adminPage, '/admin/usage')
  await expect(adminPage.locator('body')).toContainText('$0.00013125')
  await screenshot(adminPage, testInfo, 'AC01-admin-usage-8dp')
  await adminContext.close()
})

test('AC01-doc gemini-2.5-pro 倍率 1.25 的文档数值', async ({ page }, testInfo) => {
  const { email, password, user, key, upstreamKey } = await seedUserWithKey({ prefix: 'ac01doc', rate: 1.25, balance: 10000 })
  await scenario(upstreamKey, { usage: { input: 10, output: 5 }, stream: false })

  const res = await gateway({ path: '/v1/messages', apiKey: key.key, body: anthropicBody('gemini-2.5-pro') })
  expect(res.status, res.text).toBe(200)

  await expect
    .poll(() => Number(scalar(`select count(*) from usage_logs where api_key_id=${key.id}`)), { timeout: 20000 })
    .toBe(1)

  expect(Number(scalar(`select actual_cost from usage_logs where api_key_id=${key.id}`)).toFixed(8)).toBe('0.00007813')
  expect(Number(scalar(`select balance from users where id=${user.id}`)).toFixed(8)).toBe('9999.99992187')
  expect(Number(scalar(`select quota_used from api_keys where id=${key.id}`)).toFixed(8)).toBe('0.00007813')

  const locale = localeFor(testInfo)
  await login(page, email, password, locale)
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.00007813')
  await expect(page.locator('body')).toContainText('$9999.99992187')
  await screenshot(page, testInfo, 'AC01doc-user-usage-8dp')
})
