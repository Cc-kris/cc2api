import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const readSource = (relativePath: string) => readFileSync(resolve(process.cwd(), relativePath), 'utf8')

describe('Grok admin configuration flow', () => {
  it('exposes Grok in group creation and filtering', () => {
    const source = readSource('src/views/admin/GroupsView.vue')

    expect(source.match(/\{ value: "grok", label: "Grok" \}/g)).toHaveLength(2)
  })

  it('exposes Grok as a channel platform for mapping and pricing', () => {
    const source = readSource('src/views/admin/ChannelsView.vue')

    expect(source).toContain("'antigravity', 'grok', 'seedace'")
  })

  it('allows live upstream model synchronization for Grok accounts', () => {
    const source = readSource('src/components/account/ModelWhitelistSelector.vue')

    expect(source).toContain("'antigravity', 'grok', 'seedace'")
  })
})
