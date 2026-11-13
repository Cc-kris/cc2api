import { describe, expect, it } from 'vitest'
import { parseSchedulingThresholdReason } from '../schedulingThreshold'
import { defaultSchedulerOverrides, schedulerOverridesValid } from '../openaiSchedulerSettings'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

describe('scheduling threshold account reasons', () => {
  it('recognizes detailed pause reasons and rejects malformed or other sources', () => {
    expect(parseSchedulingThresholdReason('{')).toBeNull()
    expect(parseSchedulingThresholdReason('{"source":"other"}')).toBeNull()
    expect(parseSchedulingThresholdReason(JSON.stringify({ source: 'account_scheduling_threshold', window: '5h', threshold_percent: 90, used_percent: 95 }))).toEqual({ source: 'account_scheduling_threshold', platform: undefined, window: '5h', threshold_percent: 90, used_percent: 95 })
  })
  it('provides localized badge and reason text', () => {
    expect(zh.admin.accounts.status.usageThreshold).toBe('用量阈值停调')
    expect(en.admin.accounts.status.usageThreshold).toBe('Usage Threshold Pause')
    expect(zh.admin.accounts.status.usageThresholdDetail).toContain('{threshold}')
    expect(en.admin.accounts.status.usageThresholdDetail).toContain('{window}')
  })
  it('validates scheduler override boundaries without inventing headroom settings', () => {
    const values = defaultSchedulerOverrides()
    expect(values).not.toHaveProperty('openai_advanced_scheduler_weight_quota_headroom')
    expect(schedulerOverridesValid(values)).toBe(true)
    values.openai_advanced_scheduler_lb_top_k = '1.5'
    expect(schedulerOverridesValid(values)).toBe(false)
    values.openai_advanced_scheduler_lb_top_k = '10'
    values.openai_advanced_scheduler_weight_load = '-1'
    expect(schedulerOverridesValid(values)).toBe(false)
  })
})
