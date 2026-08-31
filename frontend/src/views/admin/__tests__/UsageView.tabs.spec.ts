import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../UsageView.vue'), 'utf8')

describe('UsageView tabs', () => {
  it('keeps details, errors and lazy ranking tabs with date handoff', () => {
    expect(source).toContain("type UsageTab = 'details' | 'errors' | 'ranking'")
    expect(source).toContain("activeTab === 'ranking'")
    expect(source).toContain('handleRankingUserClick')
    expect(source).toContain('startDate.value = rankingRange.value.start')
  })
})
