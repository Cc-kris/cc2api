import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(process.cwd(), 'src/views/admin/ops/OpsAIAnalysisView.vue'), 'utf8')

describe('OpsAIAnalysisView tabs', () => {
  it('separates configuration and result views', () => {
    expect(source).toContain("const activeTab = ref<AIAnalysisTab>('config')")
    expect(source).toContain("t('admin.ops.aiAnalysis.tabs.configuration')")
    expect(source).toContain("t('admin.ops.aiAnalysis.tabs.results')")
    expect(source).toContain('v-if="activeTab === \'config\'"')
    expect(source).toContain('<section v-else class="space-y-6">')
  })
})
