import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e/v042-ui/specs',
  outputDir: './e2e/v042-ui/test-results',
  globalSetup: './e2e/v042-ui/setup/global-setup.ts',
  timeout: 180000,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:8280',
    headless: true,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'desktop-zh', use: { viewport: { width: 1440, height: 900 }, locale: 'zh-CN' } },
    { name: 'mobile-en', use: { viewport: { width: 390, height: 844 }, locale: 'en-US' } },
  ],
})
