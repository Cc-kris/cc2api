import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const GROUP_PRICING_KEYS = ['longContextPricing', 'modelPricingJson', 'modelPricingHint'] as const

describe('group pricing locale labels', () => {
  it.each([
    ['English', en],
    ['Chinese', zh]
  ])('provides the pricing form labels in %s', (_locale, messages) => {
    const form = messages.admin.groups.form

    for (const key of GROUP_PRICING_KEYS) {
      expect(form[key]).toEqual(expect.any(String))
      expect(form[key].trim().length).toBeGreaterThan(0)
    }
  })

  it('uses localized privacy filter labels rather than hard-coded abbreviations', () => {
    const source = readFileSync(
      resolve(process.cwd(), 'src/components/admin/account/AccountTableFilters.vue'),
      'utf8'
    )

    expect(source).toContain("t('admin.accounts.privacyTrainingOff')")
    expect(source).toContain("t('admin.accounts.privacyCfBlocked')")
    expect(source).toContain("t('admin.accounts.privacyFailed')")
    expect(source).not.toContain("label: 'Privacy'")
    expect(source).not.toContain("label: 'CF'")
    expect(source).not.toContain("label: 'Fail'")
    expect(en.admin.accounts.allPrivacyModes).toBe('All Privacy States')
    expect(zh.admin.accounts.allPrivacyModes).toBe('全部隐私状态')
  })
})
