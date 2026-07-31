import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

test.describe('public shell', () => {
  test('renders the login page without critical accessibility violations', async ({ page }) => {
    await page.goto('/login')

    await expect(page.locator('form')).toBeVisible()

    const accessibilityScanResults = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
      .analyze()

    const blockingViolations = accessibilityScanResults.violations.filter((violation) =>
      ['critical', 'serious'].includes(violation.impact || '')
    )

    expect(blockingViolations).toEqual([])
  })
})
