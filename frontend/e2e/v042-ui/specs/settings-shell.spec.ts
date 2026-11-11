import { expect, test } from '@playwright/test'
import { BASE } from '../helpers/ui'

test('Public settings injection survives repeated conditional HTML reloads under CSP', async ({ page }) => {
  const blockedInlineScripts: string[] = []
  page.on('console', message => {
    if (/Executing inline script violates/.test(message.text())) blockedInlineScripts.push(message.text())
  })
  let response = await page.goto(BASE + '/login')
  let previousNonce = ''
  for (let i = 0; i < 3; i++) {
    expect(response?.status()).toBe(200)
    // Playwright text locators deliberately ignore script contents. Inspect the
    // real DOM script property instead (nonce attributes are hidden by browsers).
    const nonce = await page.evaluate(() => Array.from(document.scripts)
      .find(script => script.textContent?.includes('window.__APP_CONFIG__='))?.nonce)
    if (!nonce) throw new Error('Public settings script is missing its CSP nonce')
    expect(nonce).not.toBe(previousNonce)
    expect(response?.headers()['content-security-policy']).toContain(`'nonce-${nonce}'`)
    await expect.poll(() => page.evaluate(() =>
      typeof (window as unknown as { __APP_CONFIG__?: { payment_enabled?: boolean } }).__APP_CONFIG__?.payment_enabled)).toBe('boolean')
    previousNonce = nonce
    if (i < 2) response = await page.reload()
  }
  expect(blockedInlineScripts).toEqual([])
})
