import { apiClient } from '@/api'

export interface OpsAIAnalysisConfigSnapshot {
  enabled: boolean
  base_url: string
  api_key_masked?: string
  model: string
  manual_enabled: boolean
}

const manualAIAllowedRoles = new Set(['admin', 'ops', 'operation', 'operator'])

export function canManageManualAIAnalysis(role: string | null | undefined): boolean {
  return manualAIAllowedRoles.has(String(role || '').trim().toLowerCase())
}

export function isManualAIAnalysisConfigured(config: OpsAIAnalysisConfigSnapshot | null | undefined): boolean {
  if (!config) return false
  return Boolean(
    config.enabled &&
    config.manual_enabled &&
    String(config.base_url || '').trim() &&
    String(config.model || '').trim() &&
    String(config.api_key_masked || '').trim()
  )
}

export async function fetchOpsAIAnalysisConfig(): Promise<OpsAIAnalysisConfigSnapshot> {
  const { data } = await apiClient.get<OpsAIAnalysisConfigSnapshot>('/admin/ops/ai-analysis/config')
  return data
}
