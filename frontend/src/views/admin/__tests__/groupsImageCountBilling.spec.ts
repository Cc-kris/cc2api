import { describe, expect, it } from 'vitest'

import { imageCountBillingAllowed, normalizeImageCountBilling } from '../groupsImageCountBilling'

describe('group image count billing', () => {
  it('is allowed only for openai groups with image generation enabled', () => {
    expect(imageCountBillingAllowed('openai', true)).toBe(true)
    expect(imageCountBillingAllowed('openai', false)).toBe(false)
    expect(imageCountBillingAllowed('gemini', true)).toBe(false)
  })

  it('normalizes the form value to false when the switch does not apply', () => {
    expect(normalizeImageCountBilling({ platform: 'openai', allow_image_generation: true, image_count_billing: true })).toBe(true)
    expect(normalizeImageCountBilling({ platform: 'openai', allow_image_generation: true, image_count_billing: false })).toBe(false)
    expect(normalizeImageCountBilling({ platform: 'openai', allow_image_generation: false, image_count_billing: true })).toBe(false)
    expect(normalizeImageCountBilling({ platform: 'grok', allow_image_generation: true, image_count_billing: true })).toBe(false)
  })
})
