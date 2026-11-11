import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../GroupsView.vue'),
  'utf8'
)

describe('GroupsView model pricing contract', () => {
  it('exposes long-context pricing and validates interval JSON before save', () => {
    expect(source).toContain('v-model="createForm.long_context_pricing_enabled"')
    expect(source).toContain('v-model="createForm.model_pricing_json"')
    expect(source).toContain('parseGroupModelPricing(createForm.model_pricing_json)')
    expect(source).toContain('parseGroupModelPricing(editForm.model_pricing_json)')
  })

  // V03-T15：按张计费开关只在 openai 分组且已允许图片生成时出现。
  it('shows per-image billing only when image generation is allowed on openai groups', () => {
    for (const form of ['createForm', 'editForm']) {
      expect(source).toContain(`v-model="${form}.image_count_billing"`)
      expect(source).toContain(`v-if="${form}.allow_image_generation && ${form}.platform === 'openai'"`)
    }
    expect(source).toContain('editForm.image_count_billing = group.image_count_billing ?? false')
  })

  // 按张计费标签单行显示，提示放在下一行并通过 aria-describedby 关联，不再挤在标签里。
  it('renders the per-image billing hint below a single-line label', () => {
    for (const prefix of ['create', 'edit']) {
      expect(source).toContain(`aria-describedby="${prefix}-image-count-billing-hint"`)
      expect(source).toContain(`id="${prefix}-image-count-billing-hint"`)
      expect(source).toContain(`for="${prefix}-image-count-billing-input"`)
    }
    expect(source).not.toContain(":title=\"t('admin.groups.imagePricing.imageCountBillingHint')\"")
  })
})
