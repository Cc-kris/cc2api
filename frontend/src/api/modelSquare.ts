import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export type ModelSquarePriceUnit =
  | 'per_1m_tokens'
  | 'per_1m_cache_tokens'
  | 'per_request'
  | 'per_image'
  | 'per_second'

export interface ModelSquareUnitPrice {
  original: string
  multiplier_price: string
  unit: ModelSquarePriceUnit
}

export interface ModelSquareFastPrices {
  input: ModelSquareUnitPrice | null
  output: ModelSquareUnitPrice | null
  cache_read: ModelSquareUnitPrice | null
  cache_write_5m: ModelSquareUnitPrice | null
  cache_write_1h: ModelSquareUnitPrice | null
}

export interface ModelSquareTierPrice {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  sort_order: number
  input: ModelSquareUnitPrice | null
  output: ModelSquareUnitPrice | null
  cache_read: ModelSquareUnitPrice | null
  cache_write: ModelSquareUnitPrice | null
  per_request: ModelSquareUnitPrice | null
}

export interface ModelSquarePrices {
  input: ModelSquareUnitPrice | null
  output: ModelSquareUnitPrice | null
  cache_read: ModelSquareUnitPrice | null
  cache_write_5m: ModelSquareUnitPrice | null
  cache_write_1h: ModelSquareUnitPrice | null
  image_output?: ModelSquareUnitPrice | null
  per_request?: ModelSquareUnitPrice | null
  per_second?: ModelSquareUnitPrice | null
}

export interface ModelSquareModel {
  name: string
  billing_mode: BillingMode
  pricing_source?: 'system' | 'channel' | 'mixed'
  prices: ModelSquarePrices
  fast_prices: ModelSquareFastPrices | null
  tiers: ModelSquareTierPrice[]
}

export interface ModelSquareGroup {
  id: number
  name: string
  platform: string
  subscription_type: string
  default_multiplier: string
  effective_multiplier: string
  has_custom_multiplier: boolean
  model_count: number
}

export interface ModelSquareGroupsResult {
  groups: ModelSquareGroup[]
  catalog_updated_at: string
}

export interface ModelSquareModelsResult {
  group_id: number
  group_name: string
  effective_multiplier: string
  items: ModelSquareModel[]
  next_cursor: string | null
  catalog_updated_at: string
}

export interface ModelSquareModelsQuery {
  q?: string
  cursor?: string
  page_size?: number
  catalog_updated_at?: string
  signal?: AbortSignal
}

export async function listModelSquareGroups(signal?: AbortSignal): Promise<ModelSquareGroupsResult> {
  const { data } = await apiClient.get<ModelSquareGroupsResult>('/model-square/groups', { signal })
  return data
}

export async function listModelSquareModels(
  groupId: number,
  query: ModelSquareModelsQuery = {},
): Promise<ModelSquareModelsResult> {
  const { signal, ...params } = query
  const { data } = await apiClient.get<ModelSquareModelsResult>(`/model-square/groups/${groupId}/models`, {
    params,
    signal,
  })
  return data
}

export const modelSquareAPI = {
  listGroups: listModelSquareGroups,
  listModels: listModelSquareModels,
}

export default modelSquareAPI
