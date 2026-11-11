import { expect, test } from '@playwright/test'
import { scalar } from '../helpers/db'
import { ADMIN_EMAIL, ADMIN_PASSWORD, BASE, login, navigate, screenshot } from '../helpers/ui'

async function enableInvitationRegistration(page: any, locale: 'zh-CN' | 'en-US') {
  await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, locale)
  await navigate(page, '/admin/settings')
  await page.locator('#settings-tab-security').click()
  for (const [pattern, checked] of [
    [/Enable Registration|开放注册|允许注册/, true],
    [/Email Verification|邮箱验证/, false],
    [/Invitation Code Registration|邀请码注册/, true],
  ] as Array<[RegExp, boolean]>) {
    const row = page.locator('div.flex.items-center.justify-between')
      .filter({ hasText: pattern }).filter({ has: page.getByRole('switch') }).first()
    const toggle = row.getByRole('switch')
    await expect(toggle).toBeVisible()
    if ((await toggle.getAttribute('aria-checked') === 'true') !== checked) await toggle.click()
    await expect(toggle).toHaveAttribute('aria-checked', String(checked))
  }
  const [saved] = await Promise.all([
    page.waitForResponse((r: any) => r.request().method() === 'PUT' && /\/admin\/settings$/.test(new URL(r.url()).pathname)),
    page.locator('form button[type="submit"]').last().click(),
  ])
  expect(saved.status(), await saved.text()).toBeLessThan(300)
}

test('AC15 同一邀请码第二个真实浏览器注册被界面拒绝且没有残留用户', async ({ page }, testInfo) => {
  // The exact 20-way registration/OAuth race is covered by the PostgreSQL integration suite.
  // Here the second browser exercises the real invitation validation and submit blocking.
  const locale = testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'
  await enableInvitationRegistration(page, locale)
  await navigate(page, '/admin/redeem')
  await page.getByRole('button', { name: /Generate Codes|生成兑换码/, exact: true }).click()
  // The form is the stable dialog container on both viewport layouts; the
  // heading is a sibling of the form, so a descendant locator cannot find it.
  const dialog = page.locator('div.fixed.inset-0.z-50').filter({
    has: page.getByRole('heading', { name: /Generate Redeem Codes|Generate Codes|生成兑换码/ }),
  }).last()
  await dialog.getByRole('button', { name: 'Select option', exact: true }).click()
  await page.getByRole('option', { name: /^Invitation$|^邀请码$/ }).click()
  const [generated] = await Promise.all([
    page.waitForResponse((r: any) => r.request().method() === 'POST' && /\/admin\/redeem-codes\/generate$/.test(new URL(r.url()).pathname)),
    dialog.locator('button[type="submit"]').filter({ hasText: /Generate$|生成$/ }).click(),
  ])
  expect(generated.status(), await generated.text()).toBeLessThan(300)
  const code = (await page.locator('textarea[readonly]').inputValue()).trim()
  expect(code).not.toBe('')
  const prefix = `ac15-${Date.now()}`
  const browser = page.context().browser()!
  const first = await browser.newContext({ viewport: page.viewportSize() || undefined })
  const second = await browser.newContext({ viewport: page.viewportSize() || undefined })
  try {
    for (const [context, email] of [[first, `${prefix}-1@v042e2e.local`], [second, `${prefix}-2@v042e2e.local`]] as const) {
      const p = await context.newPage()
      await p.addInitScript((l) => localStorage.setItem('sub2api_locale', l), locale === 'zh-CN' ? 'zh' : 'en')
      await p.goto(BASE + '/register')
      await p.locator('#email').fill(email)
      await p.locator('#password').fill('E2e-Passw0rd!')
      await p.locator('#invitation_code').fill(code)
      if (context === first) {
        await expect(p.locator('body')).toContainText(/Invitation code is valid|邀请码有效/)
        const [response] = await Promise.all([
          p.waitForResponse(r => r.request().method() === 'POST' && /\/auth\/register$/.test(new URL(r.url()).pathname)),
          p.locator('button[type="submit"]').click(),
        ])
        expect(response.status(), await response.text()).toBeLessThan(300)
        await p.waitForURL(u => u.pathname === '/dashboard')
      } else {
        let registrations = 0
        p.on('request', r => {
          if (r.method() === 'POST' && /\/auth\/register$/.test(new URL(r.url()).pathname)) registrations++
        })
        await expect(p.locator('body')).toContainText(/Invalid or used invitation code|邀请码无效或已被使用/)
        await expect(p.locator('button[type="submit"]')).toBeEnabled()
        await p.locator('button[type="submit"]').click()
        await expect(p.locator('body')).toContainText(/Invalid or used invitation code|邀请码无效或已被使用/)
        await screenshot(p, testInfo, 'AC15-second-registration-rejected')
        expect(registrations).toBe(0)
      }
    }
    expect(scalar(`select count(*) from users where email like '${prefix}-%@v042e2e.local'`)).toBe('1')
    expect(scalar(`select used_by=(select id from users where email='${prefix}-1@v042e2e.local') from redeem_codes where code='${code}'`)).toBe('t')
  } finally {
    await first.close()
    await second.close()
  }
})
