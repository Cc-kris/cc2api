import { expect, test } from '@playwright/test'

test('public home stays within the Core Web Vitals budget', async ({ page }) => {
  await page.goto('/')
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(1200)
  const metrics = await page.evaluate(() => {
    const paints = performance.getEntriesByType('paint') as PerformancePaintTiming[]
    const firstContentfulPaint = paints.find((entry) => entry.name === 'first-contentful-paint')?.startTime ?? 0
    const lcp = (performance.getEntriesByType('largest-contentful-paint').at(-1) as PerformanceEntry | undefined)?.startTime ?? 0
    let cls = 0
    for (const entry of performance.getEntriesByType('layout-shift') as Array<PerformanceEntry & { hadRecentInput?: boolean; value?: number }>) {
      if (!entry.hadRecentInput) cls += entry.value ?? 0
    }
    return { firstContentfulPaint, lcp, cls }
  })
  test.info().annotations.push({ type: 'performance', description: JSON.stringify(metrics) })
  expect(metrics.firstContentfulPaint).toBeGreaterThan(0)
  expect(metrics.cls).toBeLessThanOrEqual(0.1)
  if (metrics.lcp > 0) expect(metrics.lcp).toBeLessThanOrEqual(2500)
})
