import apiClient from '../client'

export interface UpstreamModelRate {
  id?: number
  model: string
  rate_multiplier: number
}

export interface Upstream {
  id: number
  base_url: string
  normalized_base_url: string
  name: string
  rate_multiplier: number
  model_rates: UpstreamModelRate[]
  initial_balance: number
  consumed_balance: number
  current_balance: number
  account_count: number
  balance_alert_enabled: boolean
  alert_balance?: number | null
  alert_email_sent_at?: string | null
  alert_last_balance?: number | null
  notes: string
  created_at: string
  updated_at: string
}

export interface UpstreamInput {
  base_url: string
  name: string
  rate_multiplier: number
  model_rates: UpstreamModelRate[]
  initial_balance: number
  balance_alert_enabled: boolean
  alert_balance?: number | null
  notes: string
}

export interface UpstreamStatsSummary {
  upstream_count: number
  total_current_balance: number
  total_initial_balance: number
  total_consumed_balance: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_write_tokens: number
  total_cache_read_tokens: number
  total_tokens: number
}

export interface UpstreamCostPoint {
  bucket: string
  upstream_id?: number
  upstream_name?: string
  consumed_balance: number
  input_tokens: number
  output_tokens: number
  cache_write_tokens: number
  cache_read_tokens: number
  total_tokens: number
}

export interface UpstreamStatsResponse {
  summary: UpstreamStatsSummary
  cost_bars: UpstreamCostPoint[]
  token_trend: UpstreamCostPoint[]
  start_date: string
  end_date: string
  granularity: string
  updated_at: string
}

export interface FinanceStatsSummary {
  user_recharge_total: number
  upstream_recharge_total: number
  user_consumed_amount: number
  upstream_consumed_amount: number
  consumed_profit: number
  consumed_profit_rate: number
}

export interface FinanceTrendPoint {
  bucket: string
  profit: number
  upstream_cost: number
  user_recharge: number
  user_consumed_amount: number
  upstream_consumed_amount: number
}

export interface FinanceStatsResponse {
  summary: FinanceStatsSummary
  trend: FinanceTrendPoint[]
  start_date: string
  end_date: string
  granularity: string
  updated_at: string
}

export interface StatsParams {
  start_date?: string
  end_date?: string
  granularity?: 'hour' | 'day' | 'month'
}

export async function list(): Promise<Upstream[]> {
  const { data } = await apiClient.get<{ items: Upstream[] }>('/admin/upstreams')
  return data.items || []
}

export async function create(payload: UpstreamInput): Promise<Upstream> {
  const { data } = await apiClient.post<Upstream>('/admin/upstreams', payload)
  return data
}

export async function update(id: number, payload: UpstreamInput): Promise<Upstream> {
  const { data } = await apiClient.put<Upstream>(`/admin/upstreams/${id}`, payload)
  return data
}

export async function deleteUpstream(id: number): Promise<void> {
  await apiClient.delete(`/admin/upstreams/${id}`)
}

export async function syncFromAccounts(): Promise<{ created: number }> {
  const { data } = await apiClient.post<{ created: number }>('/admin/upstreams/sync-from-accounts')
  return data
}

export async function getStats(params?: StatsParams): Promise<UpstreamStatsResponse> {
  const { data } = await apiClient.get<UpstreamStatsResponse>('/admin/upstreams/stats', { params })
  return data
}

export async function getFinanceStats(params?: StatsParams): Promise<FinanceStatsResponse> {
  const { data } = await apiClient.get<FinanceStatsResponse>('/admin/finance/stats', { params })
  return data
}

export default { list, create, update, deleteUpstream, syncFromAccounts, getStats, getFinanceStats }
