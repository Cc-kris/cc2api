export const schedulerWeightNames = ['priority', 'load', 'queue', 'error_rate', 'ttft', 'reset', 'previous_response', 'session_sticky'] as const
export const schedulerOverrideKeys = ['openai_advanced_scheduler_lb_top_k', ...schedulerWeightNames.map(name => `openai_advanced_scheduler_weight_${name}`)] as const
export interface OpenAISchedulerSettings {
  openai_advanced_scheduler_sticky_weighted_enabled?: boolean
  openai_advanced_scheduler_subscription_priority_enabled?: boolean
  openai_advanced_scheduler_lb_top_k?: string
  openai_advanced_scheduler_weight_priority?: string
  openai_advanced_scheduler_weight_load?: string
  openai_advanced_scheduler_weight_queue?: string
  openai_advanced_scheduler_weight_error_rate?: string
  openai_advanced_scheduler_weight_ttft?: string
  openai_advanced_scheduler_weight_reset?: string
  openai_advanced_scheduler_weight_previous_response?: string
  openai_advanced_scheduler_weight_session_sticky?: string
}
export const defaultSchedulerOverrides = () => Object.fromEntries(schedulerOverrideKeys.map(key => [key, ''])) as Record<string, string>
export function schedulerOverridesValid(values: Record<string, string>): boolean {
  return schedulerOverrideKeys.every(key => {
    const raw = values[key]?.trim() || ''
    if (!raw) return true
    const n = Number(raw)
    return Number.isFinite(n) && n >= 0 && (key !== 'openai_advanced_scheduler_lb_top_k' || (Number.isInteger(n) && n >= 1 && n <= 100))
  })
}
