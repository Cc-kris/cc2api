import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const readSource = (path: string) => readFileSync(resolve(path), 'utf8')

describe('admin platform filter coverage', () => {
  it('exposes every account platform in the account filter', () => {
    const source = readSource('src/components/admin/account/AccountTableFilters.vue')
    for (const platform of ['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'seedace', 'ollama', 'kimi', 'zhipu', 'deepseek']) {
      expect(source).toContain(`value: '${platform}'`)
    }
  })

  it('normalizes group platform labels before rendering them', () => {
    const source = readSource('src/views/admin/GroupsView.vue')
    const locale = readSource('src/i18n/locales/zh.ts')
    expect(source).toContain('const groupPlatformValue = (value: unknown): GroupPlatform | undefined')
    expect(source).toContain('{{ groupPlatformLabel(value) }}')
    expect(locale).toContain("grok: 'Grok'")
  })
})
