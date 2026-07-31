import apiClient from '../client'

export type UpstreamWalletAdapter = 'newapi' | 'legacy_openai_billing' | 'protocol' | 'manual'
export type UpstreamWalletBalanceKind = 'wallet_cash' | 'token_quota'

export interface UpstreamWallet {
  id: number
  upstream_id: number
  name: string
  adapter_type: UpstreamWalletAdapter
  base_url?: string
  credential_configured: boolean
  currency: string
  balance_kind: UpstreamWalletBalanceKind
  balance_scope_key?: string
  pricing_group?: string
  protocol_version_id?: number
  enabled: boolean
  last_pricing_sync_at: string | null
  pricing_sync_status: string
  pricing_sync_error?: string
  last_balance_sync_at: string | null
  balance_sync_status: string
  balance_sync_error?: string
  last_quota_sync_at: string | null
  quota_sync_status: string
  quota_sync_error?: string
  assigned_account_count: number
  created_at: string
  updated_at: string
  deleted_at: string | null
}

export interface UpstreamWalletInput {
  name: string
  adapter_type: UpstreamWalletAdapter
  base_url: string
  credential?: string
  currency: string
  balance_kind: UpstreamWalletBalanceKind
  balance_scope_key?: string
  pricing_group?: string
  protocol_version_id?: number
  enabled?: boolean
}

export interface UpstreamFinanceProbe {
  reachable: boolean
  adapter_type: string
  capabilities: { pricing: string; wallet_balance: string; token_quota: string }
  latency_ms: number
  probed_at: string
  error_summary?: string
}

export interface UpstreamFinanceSyncJob {
  id: number
  wallet_id: number
  sync_type: string
  status: string
  progress: string
  error_summary?: string
  created_at: string
}

export interface UpstreamFinancePriceVersion {
  id: number
  model_pattern: string
  billing_mode: string
  service_tier?: string
  price_detail: Record<string, unknown>
  currency: string
  source: string
  effective_from: string
  effective_to: string | null
}

export interface UpstreamFinancePriceInput {
  model_pattern: string
  is_wildcard?: boolean
  billing_mode: 'token' | 'per_request' | 'image' | 'per_second'
  service_tier?: string
  price_detail: Record<string, unknown>
  currency: string
  effective_at?: string
}

export interface UpstreamFundEventInput {
  event_type: 'opening_balance' | 'topup' | 'refund' | 'adjustment'
  original_amount: string
  currency: string
  fx_rate_to_usd: string
  fx_source?: string
  fx_observed_at?: string
  usd_amount: string
	base_credit_units?: string
	bonus_credit_units?: string
	reversed_event_id?: number
  occurred_at: string
  reference_no?: string
  note: string
}

export interface UpstreamFundEvent {
  id: number
  wallet_id: number
  event_type: 'opening_balance' | 'topup' | 'refund' | 'adjustment'
  original_amount: string
  currency: string
  fx_rate_to_usd: string
  fx_source: string
  fx_observed_at: string
  usd_amount: string
  base_credit_units?: string
  bonus_credit_units?: string
  total_credit_units?: string
  base_recharge_ratio?: string
  effective_recharge_ratio?: string
  bonus_income_original?: string
  bonus_income_usd?: string
  bonus_status: 'not_applicable' | 'confirmed' | 'unresolved' | 'reversed'
  reversed_event_id?: number
  occurred_at: string
  reference_no?: string
  note: string
  created_at: string
}

export interface UpstreamFinanceSyncHistory {
  id: number
  sync_type: string
  status: string
  collected_count: number
  skipped_count: number
  duration_ms: number | null
  error_summary?: string
  started_at: string
  finished_at: string | null
}

export interface Paginated<T> { items: T[]; total: number; page: number; page_size: number }

export interface UpstreamFinanceProtocol {
  id: number
  code: string
  name: string
  status: 'draft' | 'published' | 'disabled'
  current_version_id?: number
}

export async function listPublishedProtocols(): Promise<UpstreamFinanceProtocol[]> {
  const { data } = await apiClient.get<Paginated<UpstreamFinanceProtocol>>('/admin/upstream-finance-protocols', { params: { status: 'published', page: 1, page_size: 100 } })
  return data.items || []
}

export async function list(upstreamId: number): Promise<UpstreamWallet[]> {
  const { data } = await apiClient.get<UpstreamWallet[]>(`/admin/upstreams/${upstreamId}/wallets`)
  return Array.isArray(data) ? data : []
}

export async function create(upstreamId: number, payload: UpstreamWalletInput): Promise<UpstreamWallet> {
  const { data } = await apiClient.post<UpstreamWallet>(`/admin/upstreams/${upstreamId}/wallets`, payload)
  return data
}

export async function update(id: number, payload: UpstreamWalletInput): Promise<UpstreamWallet> {
  const { data } = await apiClient.put<UpstreamWallet>(`/admin/upstream-wallets/${id}`, payload)
  return data
}

export async function deleteWallet(id: number): Promise<void> { await apiClient.delete(`/admin/upstream-wallets/${id}`) }

export async function assignAccounts(id: number, accountIds: number[], effectiveAt: string, reason: string): Promise<void> {
  await apiClient.post(`/admin/upstream-wallets/${id}/accounts`, { account_ids: accountIds, effective_at: effectiveAt, reason })
}

export async function probe(id: number): Promise<UpstreamFinanceProbe> {
  const { data } = await apiClient.post<UpstreamFinanceProbe>(`/admin/upstream-wallets/${id}/probe`)
  return data
}

export async function sync(id: number, type: 'pricing' | 'balance' | 'quota' | 'funding' | 'account-usage'): Promise<{ job: UpstreamFinanceSyncJob; created: boolean }> {
  const { data } = await apiClient.post<{ job: UpstreamFinanceSyncJob; created: boolean }>(`/admin/upstream-wallets/${id}/sync-${type}`)
  return data
}

export async function listPrices(id: number): Promise<Paginated<UpstreamFinancePriceVersion>> {
  const { data } = await apiClient.get<Paginated<UpstreamFinancePriceVersion>>(`/admin/upstream-wallets/${id}/prices`, { params: { page: 1, page_size: 100 } })
  return data
}

export async function listSyncHistory(id: number): Promise<Paginated<UpstreamFinanceSyncHistory>> {
  const { data } = await apiClient.get<Paginated<UpstreamFinanceSyncHistory>>(`/admin/upstream-wallets/${id}/sync-history`, { params: { page: 1, page_size: 100 } })
  return data
}

export async function importPrices(id: number, prices: UpstreamFinancePriceInput[], effectiveAt?: string): Promise<{ created_count: number; skipped_count: number }> {
  const { data } = await apiClient.post(`/admin/upstream-wallets/${id}/prices/import`, { prices, effective_at: effectiveAt })
  return data
}

export async function createFundEvent(id: number, payload: UpstreamFundEventInput, idempotencyKey: string): Promise<unknown> {
  const { data } = await apiClient.post(`/admin/upstream-wallets/${id}/fund-events`, payload, { headers: { 'Idempotency-Key': idempotencyKey } })
  return data
}

export async function listFundEvents(id: number): Promise<Paginated<UpstreamFundEvent>> {
  const { data } = await apiClient.get<Paginated<UpstreamFundEvent>>(`/admin/upstream-wallets/${id}/fund-events`, { params: { page: 1, page_size: 100 } })
  return data
}

export default { list, create, update, deleteWallet, assignAccounts, probe, sync, listPrices, listSyncHistory, importPrices, createFundEvent, listFundEvents, listPublishedProtocols }
