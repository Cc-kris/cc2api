import { expect, test } from '@playwright/test'

test.describe('anonymous CCAI content routes', () => {
  for (const path of ['/', '/news', '/articles', '/docs']) {
    test(`${path} is crawlable without login`, async ({ page }) => {
      await page.goto(path)
      await expect(page).not.toHaveURL(/\/login(?:\?|$)/)
      await expect(page.locator('body')).toContainText('CCAI')
      await expect(page.getByRole('link', { name: /模型广场/ })).toHaveCount(0)
    })
  }
})
