import { apiClient } from '../client'

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

export interface CacheStatsExportParams {
  time_range?: string
  start_time?: string
  end_time?: string
  platform?: string
  model?: string
  api_key_id?: number
  group_id?: number
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

export const cacheAPI = {
  getConfig() {
    return apiClient.get<CacheManagementConfig>('/admin/cache/config')
  },

  updateConfig(data: CacheManagementConfig) {
    return apiClient.put<CacheManagementConfig>('/admin/cache/config', data)
  },

  exportStats(params?: CacheStatsExportParams) {
    return apiClient.get<Blob>('/admin/cache/stats/export', {
      params,
      responseType: 'blob'
    })
  }
}

export default cacheAPI
