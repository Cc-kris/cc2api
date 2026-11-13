export interface SchedulingThresholdReason {
  source: 'account_scheduling_threshold'
  platform?: string
  window?: string
  threshold_percent?: number
  used_percent?: number
}

export function parseSchedulingThresholdReason(raw: unknown): SchedulingThresholdReason | null {
  if (typeof raw !== 'string') return null
  try {
    const value = JSON.parse(raw)
    if (!value || value.source !== 'account_scheduling_threshold') return null
    return {
      source: value.source,
      platform: typeof value.platform === 'string' ? value.platform : undefined,
      window: typeof value.window === 'string' ? value.window : undefined,
      threshold_percent: typeof value.threshold_percent === 'number' && Number.isFinite(value.threshold_percent) ? value.threshold_percent : undefined,
      used_percent: typeof value.used_percent === 'number' && Number.isFinite(value.used_percent) ? value.used_percent : undefined
    }
  } catch { return null }
}
