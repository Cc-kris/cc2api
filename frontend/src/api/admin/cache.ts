import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface CacheManagementPlatformConfig {
  enabled: boolean
}

export interface CacheManagementPlatformsConfig {
  openai: CacheManagementPlatformConfig
  claude: CacheManagementPlatformConfig
  gemini: CacheManagementPlatformConfig
}

export interface CacheManagementBypassHeader {
  name: string
  value: string
}

export interface CacheManagementConfig {
  global_enabled: boolean
  platforms: CacheManagementPlatformsConfig
  ttl_seconds: number
  max_request_bytes: number
  max_response_bytes: number
  max_temperature: number
  model_allowlist: string[]
  model_blocklist: string[]
  bypass_header: CacheManagementBypassHeader
}


export interface CacheStatsParams {
  time_range?: string
  start_time?: string
  end_time?: string
  platform?: string
  model?: string
  api_key_id?: number
  group_id?: number
}

export interface CacheStatsSummary {
  total_requests: number
  candidate_requests: number
  hit_requests: number
  miss_requests: number
  bypass_requests: number
  store_success: number
  store_skip: number
  request_hit_rate: number
  input_tokens: number
  output_tokens: number
  hit_tokens: number
  candidate_tokens: number
  tokens_hit_rate: number
  overall_tokens_coverage: number
  estimated_saved_amount: string
}

export interface CacheStatsModelRow {
  platform: string
  model: string
  total_requests: number
  candidate_requests: number
  hit_requests: number
  miss_requests: number
  bypass_requests: number
  store_success: number
  store_skip: number
  input_tokens: number
  output_tokens: number
  hit_tokens: number
  candidate_tokens: number
  all_request_tokens: number
  request_hit_rate: number
  tokens_hit_rate: number
  top_bypass_reason?: string
  top_store_skip_reason?: string
  estimated_saved_amount: string
}

export interface CacheStatsReasonRow {
  reason: string
  count: number
  percent: number
}

export interface CacheStatsResponse {
  summary: CacheStatsSummary
  model_rows: CacheStatsModelRow[]
  bypass_reasons: CacheStatsReasonRow[]
  store_skip_reasons: CacheStatsReasonRow[]
}

export type CacheClearType =
  | 'all'
  | 'by_platform'
  | 'by_model'
  | 'by_group'
  | 'by_api_key'
  | 'by_time'
  | 'expired'

export interface CacheClearScope {
  platforms?: string[]
  models?: string[]
  group_ids?: number[]
  api_key_ids?: number[]
  start_time?: string
  end_time?: string
}

export interface CacheClearRequest {
  clear_type: CacheClearType
  scope: CacheClearScope
  confirm_text?: string
}

export interface CacheClearResult {
  clear_type: CacheClearType
  scope: CacheClearScope
  matched_keys: number
  deleted_keys: number
  status: 'success' | 'failed' | 'partial_success'
  error_message?: string
}

export interface CacheClearAuditRecord extends CacheClearResult {
  id: number
  operator_user_id?: number | null
  created_at: string
}

export interface CacheClearAuditFilter {
  page?: number
  page_size?: number
  start_time?: string
  end_time?: string
  operator_user_id?: number
  clear_type?: CacheClearType
  status?: CacheClearResult['status']
}

export const defaultCacheManagementConfig = (): CacheManagementConfig => ({
  global_enabled: false,
  platforms: {
    openai: { enabled: false },
    claude: { enabled: false },
    gemini: { enabled: false }
  },
  ttl_seconds: 600,
  max_request_bytes: 256 * 1024,
  max_response_bytes: 512 * 1024,
  max_temperature: 0.3,
  model_allowlist: [],
  model_blocklist: [],
  bypass_header: {
    name: 'X-Sub2API-Cache-Control',
    value: 'bypass'
  }
})

const sanitizeCacheStatsParams = (params?: CacheStatsParams): Record<string, string | number> | undefined => {
  if (!params) return undefined

  const query: Record<string, string | number> = {}
  if (typeof params.time_range === 'string' && params.time_range.trim()) {
    query.time_range = params.time_range.trim()
  }
  if (typeof params.start_time === 'string' && params.start_time.trim()) {
    query.start_time = params.start_time.trim()
  }
  if (typeof params.end_time === 'string' && params.end_time.trim()) {
    query.end_time = params.end_time.trim()
  }
  if (typeof params.platform === 'string' && params.platform.trim()) {
    query.platform = params.platform.trim()
  }
  if (typeof params.model === 'string' && params.model.trim()) {
    query.model = params.model.trim()
  }
  if (typeof params.api_key_id === 'number' && Number.isFinite(params.api_key_id) && params.api_key_id > 0) {
    query.api_key_id = params.api_key_id
  }
  if (typeof params.group_id === 'number' && Number.isFinite(params.group_id) && params.group_id > 0) {
    query.group_id = params.group_id
  }
  return Object.keys(query).length > 0 ? query : undefined
}

export const cacheAPI = {
  getConfig() {
    return apiClient.get<CacheManagementConfig>('/admin/cache/config')
  },

  updateConfig(data: CacheManagementConfig) {
    return apiClient.put<CacheManagementConfig>('/admin/cache/config', data)
  },

  getStats(params?: CacheStatsParams) {
    return apiClient.get<CacheStatsResponse>('/admin/cache/stats', { params: sanitizeCacheStatsParams(params) })
  },

  exportStats(params?: CacheStatsParams) {
    return apiClient.get<Blob>('/admin/cache/stats/export', {
      params: sanitizeCacheStatsParams(params),
      responseType: 'blob'
    })
  },

  clearLocalResponseCache(data: CacheClearRequest) {
    return apiClient.post<CacheClearResult>('/admin/cache/clear', data)
  },

  listClearAudits(params?: CacheClearAuditFilter) {
    return apiClient.get<PaginatedResponse<CacheClearAuditRecord> | { items: CacheClearAuditRecord[]; total: number; page: number; page_size: number }>(
      '/admin/cache/clear-audits',
      { params }
    )
  }
}

export default cacheAPI
