import { describe, expect, it } from 'vitest'
import { formatDuration, latencyHealth } from '../duration'

describe('duration formatting', () => {
  it('uses seconds, minutes and hours consistently', () => {
    expect(formatDuration(null)).toBe('-')
    expect(formatDuration(500)).toBe('0.5s')
    expect(formatDuration(1500)).toBe('1.5s')
    expect(formatDuration(61_000)).toBe('1m 1s')
    expect(formatDuration(3_661_000)).toBe('1h 1m')
  })

  it('classifies latency at the documented thresholds', () => {
    expect(latencyHealth(null).state).toBe('missing')
    expect(latencyHealth(5000).state).toBe('good')
    expect(latencyHealth(5001).state).toBe('slow')
    expect(latencyHealth(30000).state).toBe('slow')
    expect(latencyHealth(30001).state).toBe('critical')
    expect(latencyHealth(15000).width).toBe(50)
    expect(latencyHealth(40000).width).toBe(100)
    expect(latencyHealth(1000, true).state).toBe('error')
  })
})
