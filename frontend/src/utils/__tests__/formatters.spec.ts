import { describe, expect, it } from 'vitest'
import { formatFixedMultiplier } from '../formatters'

describe('formatFixedMultiplier', () => {
  it('keeps model square multipliers at two decimal places', () => {
    expect(formatFixedMultiplier('1.2345')).toBe('1.23')
    expect(formatFixedMultiplier(1)).toBe('1.00')
    expect(formatFixedMultiplier('0.005')).toBe('0.01')
  })
})
