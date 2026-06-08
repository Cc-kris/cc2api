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

export type SemanticCacheStage = 'observe' | 'review' | 'gray' | 'active' | 'rollback'

export interface SemanticCacheConfig {
  enabled: boolean
  stage: SemanticCacheStage
  platforms: string[]
  model_allowlist: string[]
  semantic_model_base_url: string
  semantic_api_key?: string
  semantic_api_key_masked: string
  semantic_model_name: string
  namespace: string
  embedding_dimension: number | null
  rule_version: string
  similarity_threshold: number
  max_reuse_minutes: number
  max_candidates: number
  gray_api_key_ids: number[]
  review_mode: boolean
  quality_rollback_threshold_percent: number
  auto_closed: boolean
  auto_close_reason: string | null
  auto_closed_at: string | null
}

export interface SemanticCacheConnectionTestResult {
  success: boolean
  status: 'success' | 'config_error' | 'auth_failed' | 'network_failed' | 'timeout' | 'failed'
  message: string
  semantic_model_base_url: string
  model: string
  embedding_dimension?: number | null
  duration_ms: number
  http_status?: number
}



export interface AdvancedCacheGrayScope {
  api_key_ids: number[]
  group_ids: number[]
  models: string[]
}

export interface AdvancedCacheConfig {
  advanced_cache_enabled: boolean
  gray_scope: AdvancedCacheGrayScope
  redis_capacity_mb: number
  memory_safe_limit_mb: number
  compression_enabled: boolean
  compression_threshold_kb: number
  eviction_policy: 'LRU' | 'LFU' | 'W-TinyLFU' | string
  hot_window: '15m' | '1h' | '6h' | '24h' | string
  hot_threshold: number
  cost_saving_enabled: boolean
  upstream_prompt_cache_enabled: boolean
}

export interface AdvancedCacheStatsParams extends CacheStatsParams {
  hotspot_limit?: number
}

export interface AdvancedCacheNameRef {
  id: number
  name?: string
  display?: string
}

export interface AdvancedCacheCapacityStats {
  current_used_bytes: number
  capacity_limit_bytes: number
  capacity_usage_rate: number
  memory_safe_limit_bytes: number
  eviction_policy: string
  recent_eviction_count: number
  last_evicted_at?: string | null
}

export interface AdvancedCacheCompressionStats {
  enabled: boolean
  raw_response_bytes: number
  stored_response_bytes: number
  compression_saved_bytes: number
  compression_saved_rate: number
  compressed_entry_count: number
  compression_failed_count: number
  decompression_failed_count: number
}

export interface AdvancedCacheHotspot {
  rank: number
  platform: string
  model: string
  group: AdvancedCacheNameRef
  api_key: AdvancedCacheNameRef
  hit_count: number
  hit_tokens: number
  last_hit_at?: string | null
}

export interface AdvancedCacheSavings {
  local_response_cache_saved_tokens: number
  local_response_cache_saved_amount?: string | null
  upstream_prompt_cache_read_tokens: number
  upstream_prompt_cache_write_tokens: number
  upstream_prompt_cache_saved_amount?: string | null
  total_estimated_saved_amount?: string | null
  price_missing: boolean
  price_missing_models: string[]
}

export interface AdvancedCacheEmptyStates {
  hotspots: boolean
  prompt_cache: boolean
  price: boolean
}

export interface AdvancedCacheFallback {
  advanced_cache_fallback_active: boolean
  fallback_reason?: string | null
  last_fallback_at?: string | null
}

export interface AdvancedCacheStatsResponse {
  capacity: AdvancedCacheCapacityStats
  compression: AdvancedCacheCompressionStats
  hotspots: AdvancedCacheHotspot[]
  savings: AdvancedCacheSavings
  empty_states: AdvancedCacheEmptyStates
  fallback: AdvancedCacheFallback
  updated_at: string
}


export interface AdvancedCacheGrayScope {
  api_key_ids: number[]
  group_ids: number[]
  models: string[]
}

export interface AdvancedCacheConfig {
  advanced_cache_enabled: boolean
  gray_scope: AdvancedCacheGrayScope
  redis_capacity_mb: number
  memory_safe_limit_mb: number
  compression_enabled: boolean
  compression_threshold_kb: number
  eviction_policy: 'LRU' | 'LFU' | 'W-TinyLFU' | string
  hot_window: '15m' | '1h' | '6h' | '24h' | string
  hot_threshold: number
  cost_saving_enabled: boolean
  upstream_prompt_cache_enabled: boolean
}

export interface AdvancedCacheStatsParams extends CacheStatsParams {
  hotspot_limit?: number
}

export interface AdvancedCacheNameRef {
  id: number
  name?: string
  display?: string
}

export interface AdvancedCacheCapacityStats {
  current_used_bytes: number
  capacity_limit_bytes: number
  capacity_usage_rate: number
  memory_safe_limit_bytes: number
  eviction_policy: string
  recent_eviction_count: number
  last_evicted_at?: string | null
}

export interface AdvancedCacheCompressionStats {
  enabled: boolean
  raw_response_bytes: number
  stored_response_bytes: number
  compression_saved_bytes: number
  compression_saved_rate: number
  compressed_entry_count: number
  compression_failed_count: number
  decompression_failed_count: number
}

export interface AdvancedCacheHotspot {
  rank: number
  platform: string
  model: string
  group: AdvancedCacheNameRef
  api_key: AdvancedCacheNameRef
  hit_count: number
  hit_tokens: number
  last_hit_at?: string | null
}

export interface AdvancedCacheSavings {
  local_response_cache_saved_tokens: number
  local_response_cache_saved_amount?: string | null
  upstream_prompt_cache_read_tokens: number
  upstream_prompt_cache_write_tokens: number
  upstream_prompt_cache_saved_amount?: string | null
  total_estimated_saved_amount?: string | null
  price_missing: boolean
  price_missing_models: string[]
}

export interface AdvancedCacheEmptyStates {
  hotspots: boolean
  prompt_cache: boolean
  price: boolean
}

export interface AdvancedCacheFallback {
  advanced_cache_fallback_active: boolean
  fallback_reason?: string | null
  last_fallback_at?: string | null
}

export interface AdvancedCacheStatsResponse {
  capacity: AdvancedCacheCapacityStats
  compression: AdvancedCacheCompressionStats
  hotspots: AdvancedCacheHotspot[]
  savings: AdvancedCacheSavings
  empty_states: AdvancedCacheEmptyStates
  fallback: AdvancedCacheFallback
  updated_at: string
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

export const defaultSemanticCacheConfig = (): SemanticCacheConfig => ({
  enabled: false,
  stage: 'observe',
  platforms: [],
  model_allowlist: [],
  semantic_model_base_url: '',
  semantic_api_key_masked: '',
  semantic_model_name: '',
  namespace: 'default',
  embedding_dimension: null,
  rule_version: 'v1',
  similarity_threshold: 0.98,
  max_reuse_minutes: 10,
  max_candidates: 20,
  gray_api_key_ids: [],
  review_mode: true,
  quality_rollback_threshold_percent: 1,
  auto_closed: false,
  auto_close_reason: null,
  auto_closed_at: null
})

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

const sanitizeCacheStatsParams = (params?: CacheStatsParams | AdvancedCacheStatsParams): Record<string, string | number> | undefined => {
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
  const hotspotLimit = (params as AdvancedCacheStatsParams).hotspot_limit
  if (typeof hotspotLimit === 'number' && Number.isFinite(hotspotLimit) && hotspotLimit > 0) {
    query.hotspot_limit = Math.min(100, Math.max(1, Math.round(hotspotLimit)))
  }
  return Object.keys(query).length > 0 ? query : undefined
}

export const defaultAdvancedCacheConfig = (): AdvancedCacheConfig => ({
  advanced_cache_enabled: false,
  gray_scope: {
    api_key_ids: [],
    group_ids: [],
    models: []
  },
  redis_capacity_mb: 512,
  memory_safe_limit_mb: 2048,
  compression_enabled: true,
  compression_threshold_kb: 64,
  eviction_policy: 'LRU',
  hot_window: '1h',
  hot_threshold: 5,
  cost_saving_enabled: true,
  upstream_prompt_cache_enabled: true
})

export const cacheAPI = {
  getConfig() {
    return apiClient.get<CacheManagementConfig>('/admin/cache/config')
  },

  updateConfig(data: CacheManagementConfig) {
    return apiClient.put<CacheManagementConfig>('/admin/cache/config', data)
  },

  getAdvancedConfig() {
    return apiClient.get<AdvancedCacheConfig>('/admin/cache/advanced-config')
  },

  updateAdvancedConfig(data: AdvancedCacheConfig) {
    return apiClient.put<AdvancedCacheConfig>('/admin/cache/advanced-config', data)
  },

  getSemanticConfig() {
    return apiClient.get<SemanticCacheConfig>('/admin/cache/semantic-config')
  },

  updateSemanticConfig(data: SemanticCacheConfig) {
    return apiClient.put<SemanticCacheConfig>('/admin/cache/semantic-config', data)
  },

  testSemanticConfig(data: SemanticCacheConfig) {
    return apiClient.post<SemanticCacheConnectionTestResult>('/admin/cache/semantic-config/test', data)
  },

  getStats(params?: CacheStatsParams) {
    return apiClient.get<CacheStatsResponse>('/admin/cache/stats', { params: sanitizeCacheStatsParams(params) })
  },

  getAdvancedStats(params?: AdvancedCacheStatsParams) {
    return apiClient.get<AdvancedCacheStatsResponse>('/admin/cache/advanced-stats', { params: sanitizeCacheStatsParams(params) })
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
