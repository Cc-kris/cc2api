import apiClient from "../client";

export type FinanceGranularity = "hour" | "day" | "week" | "month";
export type FinanceBreakdownDimension =
  | "user"
  | "group"
  | "requested_model"
  | "upstream_model"
  | "channel"
  | "upstream"
  | "wallet"
  | "account"
  | "billing_type"
  | "business_type";

export interface FinanceFilterParams {
  start_date: string;
  end_date: string;
  timezone: string;
  granularity: FinanceGranularity;
  user_id?: number;
  group_id?: number;
  channel_id?: number;
  upstream_id?: number;
  wallet_id?: number;
  account_id?: number;
  requested_model?: string;
  upstream_model?: string;
  billing_type?: string;
  business_type?: string;
  cost_status?: string;
  data_scope?: "all" | "exact_only";
}

export interface FinanceQuality {
  status: string;
  exact_count: number;
  estimated_count: number;
  missing_profile_count?: number;
  missing_price_count: number;
  missing_multiplier_count: number;
  missing_usage_count: number;
  unsupported_usage_count?: number;
  non_billable_count: number;
  excluded_count?: number;
  unpriced_revenue: string | null;
  cost_coverage_rate: string | null;
}

export interface FinanceOverviewMetric {
  amount: string | null;
  currency: string;
  previous_amount: string | null;
  change_rate: string | null;
  status: string;
}

export interface FinanceOverview {
  range: { start_date: string; end_date: string; timezone: string };
  revenue: FinanceOverviewMetric;
  upstream_cost: FinanceOverviewMetric;
  profit: FinanceOverviewMetric;
	 recharge_bonus_income: FinanceOverviewMetric;
	 combined_profit: FinanceOverviewMetric;
  margin_rate: string | null;
  loss_amount: string | null;
  loss_request_count: number;
  payment_net_cash: string | null;
  upstream_net_cash: string | null;
  wallet_cash_total: string | null;
  token_quota_wallet_count: number;
  quality: FinanceQuality;
  open_alert_count: number;
  generated_at: string;
  today_profit: FinanceOverviewMetric;
  month_profit: FinanceOverviewMetric;
  historical_profit: FinanceOverviewMetric;
  historical_combined_profit: FinanceOverviewMetric;
  historical_loss_amount: string | null;
  settled_profit: FinanceOverviewMetric;
  pending_settlement_cost: string | null;
  unconfigured_revenue_exposure: string | null;
  settlement_coverage_rate: string | null;
  estimated_cost_risk: string | null;
  unconfirmed_exact_cost?: string | null;
  unpriced_revenue_risk: string | null;
}

export interface FinanceTrendItem {
  bucket_start: string;
  bucket_end: string;
  revenue: string | null;
  covered_revenue?: string | null;
  upstream_cost: string | null;
	 recharge_bonus_income?: string | null;
  profit: string | null;
	 combined_profit?: string | null;
  cumulative_profit?: string | null;
	 cumulative_combined_profit?: string | null;
  cost_coverage_rate?: string | null;
  loss_amount: string | null;
  margin_rate: string | null;
  request_count: number;
  quality: FinanceQuality;
}

export interface FinanceBreakdownItem {
  dimension_key: string;
  dimension_name: string;
  revenue: string | null;
  upstream_cost: string | null;
  profit: string | null;
  margin_rate: string | null;
  loss_amount: string | null;
  request_count: number;
  exact_count: number;
  estimated_count: number;
  missing_count: number;
  input_cost: string;
  output_cost: string;
  cache_cost: string;
  fast_cost: string;
  image_cost: string;
  video_cost: string;
  other_cost: string;
}

export interface FinanceDetailItem {
  usage_log_id: number;
  request_id: string;
  usage_created_at: string;
  user_id: number;
  user_name: string;
  group_id: number | null;
  group_name: string;
  channel_id: number | null;
  channel_name: string;
  account_id: number | null;
  account_name: string;
  wallet_id: number | null;
  wallet_name: string;
  upstream_id: number | null;
  upstream_name: string;
  requested_model: string;
  upstream_model: string;
  sales_pricing_version: string;
  revenue: string | null;
  upstream_cost: string | null;
  profit: string | null;
  margin_rate: string | null;
  cost_status: string;
}

export interface FinanceLossItem extends FinanceDetailItem {
  loss_amount: string | null;
  loss_reason: string;
  alert_id: number | null;
  status: "open" | "acknowledged" | "resolved" | "ignored" | "untracked";
  assignee_id: number | null;
  handled_by: number | null;
  handled_note?: string;
  handled_at: string | null;
}

export interface FinanceWalletCashItem {
  wallet_id: number;
  wallet_name: string;
  balance_scope_key?: string;
  balance: string | null;
  currency: string;
  daily_cost: string | null;
  available_days: string | null;
  collected_at: string;
  sync_status: string;
  included_in_total?: boolean;
  stale?: boolean;
}

export interface FinanceTokenQuotaItem {
  wallet_id: number;
  wallet_name: string;
  total_quota: string | null;
  used_quota: string | null;
  remaining_quota: string | null;
  currency: string;
  collected_at: string;
  sync_status: string;
}

export interface FinanceFunds {
  wallet_cash: FinanceWalletCashItem[] | null;
  token_quota: FinanceTokenQuotaItem[] | null;
  customer_cash: {
    payment: string | null;
    refund: string | null;
    payment_fees: string | null;
    net_cash: string | null;
  };
  upstream_cash: {
    topup: string | null;
    refund: string | null;
    adjustment: string | null;
	 recharge_bonus_income: string | null;
    net_cash: string | null;
  };
  stale_wallet_count: number;
  failed_sync_count: number;
}

export interface FinanceQualityIssue {
  usage_log_id: number;
  issue_type: string;
  related_type: string;
  related_id: number | null;
  exposed_revenue: string | null;
  first_detected_at: string;
  last_scanned_at: string;
  recalculable: boolean;
}

export interface FinanceDataQuality {
  quality: FinanceQuality;
  trend: Array<{
    bucket_start: string;
    bucket_end: string;
    quality: FinanceQuality;
  }> | null;
  items: FinanceQualityIssue[] | null;
  total: number;
  page: number;
  page_size: number;
}

export interface FinanceAlert {
  id: number;
  alert_type: string;
  severity: "info" | "warning" | "critical";
  title: string;
  description: string;
  dimension_type?: string;
  dimension_id: number | null;
  impact_amount: string | null;
  request_count: number;
  occurrence_count: number;
  status: "open" | "acknowledged" | "resolved" | "ignored";
  first_occurred_at: string;
  last_occurred_at: string;
  assignee_id: number | null;
  handled_by: number | null;
  handled_note?: string;
  handled_at: string | null;
}

export interface FinancePaginatedResponse<T> {
  items: T[] | null;
  total: number;
  page: number;
  page_size: number;
  pages?: number;
}

export interface FinanceBreakdownParams extends FinanceFilterParams {
  dimension: FinanceBreakdownDimension;
  sort_by?:
    | "revenue"
    | "upstream_cost"
    | "profit"
    | "loss_amount"
    | "margin_rate"
    | "request_count";
  sort_order?: "asc" | "desc";
  page?: number;
  page_size?: number;
}

export interface FinanceLossParams extends FinanceFilterParams {
  loss_reason?: string;
  request_id?: string;
  status?: FinanceLossItem["status"];
  sort_by?:
    "usage_created_at" | "revenue" | "upstream_cost" | "profit" | "margin_rate";
  sort_order?: "asc" | "desc";
  page?: number;
  page_size?: number;
}

export interface FinanceAlertParams extends FinanceFilterParams {
  alert_type?: string;
  severity?: FinanceAlert["severity"];
  status?: FinanceAlert["status"];
  page?: number;
  page_size?: number;
}

export interface FinanceBackfillRequest {
  start_date: string;
  end_date: string;
  scope: { cost_status: string[]; account_ids: number[]; wallet_ids: number[] };
  pricing_policy: "historical_only";
  dry_run_sample_size: number;
  reason: string;
  preview_token?: string;
}

export interface FinanceBackfillPreview {
  estimated_records: number;
  exact_repairable: number;
  estimated_only: number;
  unrepairable: number;
  estimated_scan_bytes: number;
  sample_size: number;
  pending_models: string[] | null;
  ambiguous_account_ids: number[] | null;
  blockers: string[] | null;
  preview_token: string;
  expires_at: string;
}

export interface FinanceBackfillJob {
  job_id: number;
  status:
    "queued" | "running" | "paused" | "completed" | "failed" | "cancelled";
  progress: string;
  processed_count: number;
  success_count: number;
  failed_count: number;
  estimated_total: number;
  error_summary?: string | null;
}

export interface FinanceExportRequest {
  report: "breakdown";
  format: "csv";
  filters: {
    start_date: string;
    end_date: string;
    dimension: FinanceBreakdownDimension;
    data_scope: "all" | "exact_only";
    sort_by: "profit";
    sort_order: "asc";
  };
  timezone: string;
}

export interface FinanceExportJob {
  id: number;
  type: "finance_export";
  status: "queued" | "running" | "completed" | "failed";
  progress: string;
  processed_count: number;
  success_count: number;
  failed_count: number;
  report: "breakdown";
  format: "csv";
  file_size: number | null;
  row_count: number | null;
  expires_at: string | null;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
  error_summary: string | null;
  download_url?: string;
}

export interface FinanceReconciliation {
  id: number;
  wallet_id: number;
  wallet_name: string;
  upstream_id: number;
  upstream_name: string;
  period_start: string;
  period_end: string;
  upstream_bill_amount: string;
  system_cost_amount: string;
  difference_amount: string;
  difference_rate: string | null;
  currency: string;
  source_reference?: string;
  status: "pending" | "matched" | "difference" | "confirmed" | "ignored";
  handled_note?: string;
  handled_at: string | null;
}

export interface FinanceReconciliationParams {
  start_date?: string;
  end_date?: string;
  upstream_id?: number;
  wallet_id?: number;
  status?: string;
  page?: number;
  page_size?: number;
}

export type FinanceSettlementStatus =
  | "pending"
  | "settled"
  | "needs_review"
  | "failed";

export interface FinanceSettlementInterval {
  id: number;
  owner_type: "account" | "wallet";
  owner_id: number;
  account_id?: number;
  account_finance_profile_id?: number;
  wallet_id?: number;
  scope_key: string;
  previous_snapshot_id: number;
  current_snapshot_id: number;
  period_start: string;
  period_end: string;
  unit_semantics: "fiat_currency" | "platform_credit";
  currency?: string;
  fx_rate_version_id?: number;
  fx_rate_to_usd?: string;
  fx_source?: string;
  fx_observed_at?: string;
  list_cost_delta?: string;
  actual_cost_delta: string;
  observed_multiplier?: string;
  status: FinanceSettlementStatus;
  current_revision: number;
  request_count: number;
  segment_count: number;
  standard_cost_total?: string;
  allocated_cost_total?: string;
  difference_amount?: string;
  error_summary?: string;
  settled_at?: string;
}

export interface FinanceSettlementAllocation {
  id: number;
  settlement_interval_id: number;
  usage_log_id: number;
  request_id: string;
  attempt_no: number;
  revision: number;
  standard_cost_weight: string;
  allocation_rate: string;
  allocated_cost: string;
  invalidated_at?: string;
  created_at: string;
}

export interface FinanceSettlementDetail {
  interval: FinanceSettlementInterval;
  allocations: FinanceSettlementAllocation[] | null;
}

export interface PromotionCreditReconciliation {
  user_id: number;
  user_email: string;
  username: string;
  detected_historical_bonus: string;
  current_remaining_amount: string;
  confirmed_remaining_amount: string | null;
  status: "requires_reconciliation" | "resolved";
  cutover_at: string;
  created_at: string;
  resolved_at: string | null;
  resolved_by: number | null;
  notes?: string;
}

export interface FinanceFXRateVersion {
  id: number
  currency: string
  rate_to_usd: string
  source: string
  observed_at: string
  effective_from: string
  effective_to?: string | null
  checksum: string
  operator_id?: number | null
  change_reason?: string
  idempotency_key?: string
  created_at: string
}

export interface FinanceFXRateCreateInput {
  currency: string
  rate_to_usd: string
  source: string
  observed_at: string
  effective_from: string
  change_reason: string
}

export async function getOverview(
  params: FinanceFilterParams,
): Promise<FinanceOverview> {
  const { data } = await apiClient.get<FinanceOverview>(
    "/admin/finance/overview",
    { params },
  );
  return data;
}

export async function getTrend(
  params: FinanceFilterParams,
): Promise<FinanceTrendItem[]> {
  const { data } = await apiClient.get<{ items: FinanceTrendItem[] | null }>(
    "/admin/finance/trend",
    { params },
  );
  return data.items || [];
}

export async function getBreakdown(
  params: FinanceBreakdownParams,
): Promise<FinancePaginatedResponse<FinanceBreakdownItem>> {
  const { data } = await apiClient.get<
    FinancePaginatedResponse<FinanceBreakdownItem>
  >("/admin/finance/breakdown", { params });
  return data;
}

export async function getLosses(
  params: FinanceLossParams,
): Promise<FinancePaginatedResponse<FinanceLossItem>> {
  const { data } = await apiClient.get<
    FinancePaginatedResponse<FinanceLossItem>
  >("/admin/finance/losses", { params });
  return data;
}

export async function getFunds(
  params: FinanceFilterParams,
): Promise<FinanceFunds> {
  const { data } = await apiClient.get<FinanceFunds>("/admin/finance/funds", {
    params,
  });
  return data;
}

export async function getDataQuality(
  params: FinanceFilterParams,
): Promise<FinanceDataQuality> {
  const { data } = await apiClient.get<FinanceDataQuality>(
    "/admin/finance/data-quality",
    { params },
  );
  return data;
}

export async function getAlerts(
  params: FinanceAlertParams,
): Promise<FinancePaginatedResponse<FinanceAlert>> {
  const { data } = await apiClient.get<FinancePaginatedResponse<FinanceAlert>>(
    "/admin/finance/alerts",
    { params },
  );
  return data;
}

export async function updateAlert(
  id: number,
  payload: { status: FinanceAlert["status"]; note: string },
): Promise<FinanceAlert> {
  const { data } = await apiClient.put<FinanceAlert>(
    `/admin/finance/alerts/${id}`,
    payload,
  );
  return data;
}

export async function createExport(
  payload: FinanceExportRequest,
): Promise<FinanceExportJob> {
  const { data } = await apiClient.post<FinanceExportJob>(
    "/admin/finance/exports",
    payload,
    { headers: { "Idempotency-Key": crypto.randomUUID() } },
  );
  return data;
}

export async function getExport(id: number): Promise<FinanceExportJob> {
  const { data } = await apiClient.get<FinanceExportJob>(
    `/admin/finance/exports/${id}`,
  );
  return data;
}

export async function downloadExport(downloadURL: string): Promise<Blob> {
  const path = downloadURL.replace(/^\/api\/v1/, "");
  const { data } = await apiClient.get<Blob>(path, { responseType: "blob" });
  return data;
}

export async function previewBackfill(
  payload: FinanceBackfillRequest,
): Promise<FinanceBackfillPreview> {
  const { data } = await apiClient.post<FinanceBackfillPreview>(
    "/admin/finance/backfill/preview",
    payload,
  );
  return data;
}

export async function runBackfill(
  payload: FinanceBackfillRequest,
): Promise<FinanceBackfillJob> {
  const { data } = await apiClient.post<FinanceBackfillJob>(
    "/admin/finance/backfill/run",
    payload,
  );
  return data;
}

export async function getBackfill(jobId: number): Promise<FinanceBackfillJob> {
  const { data } = await apiClient.get<FinanceBackfillJob>(
    `/admin/finance/backfill/${jobId}`,
  );
  return data;
}

export async function pauseBackfill(
  jobId: number,
): Promise<FinanceBackfillJob> {
  const { data } = await apiClient.post<FinanceBackfillJob>(
    `/admin/finance/backfill/${jobId}/pause`,
  );
  return data;
}

export async function resumeBackfill(
  jobId: number,
): Promise<FinanceBackfillJob> {
  const { data } = await apiClient.post<FinanceBackfillJob>(
    `/admin/finance/backfill/${jobId}/resume`,
  );
  return data;
}

export async function getReconciliations(
  params: FinanceReconciliationParams,
): Promise<FinancePaginatedResponse<FinanceReconciliation>> {
  const { data } = await apiClient.get<
    FinancePaginatedResponse<FinanceReconciliation>
  >("/admin/finance/reconciliations", { params });
  return data;
}

export async function getSettlements(params: {
  status?: FinanceSettlementStatus;
  account_id?: number;
  page?: number;
  page_size?: number;
}): Promise<FinancePaginatedResponse<FinanceSettlementInterval>> {
  const { data } = await apiClient.get<
    FinancePaginatedResponse<FinanceSettlementInterval>
  >("/admin/finance/settlements", { params });
  return data;
}

export async function getSettlement(
  id: number,
): Promise<FinanceSettlementDetail> {
  const { data } = await apiClient.get<FinanceSettlementDetail>(
    `/admin/finance/settlements/${id}`,
  );
  return data;
}

export async function retrySettlement(
  id: number,
): Promise<FinanceSettlementDetail> {
  const { data } = await apiClient.post<FinanceSettlementDetail>(
    `/admin/finance/settlements/${id}/retry`,
  );
  return data;
}

export async function reallocateSettlement(
  id: number,
  payload: { expected_revision: number; reason: string },
): Promise<FinanceSettlementDetail> {
  const { data } = await apiClient.post<FinanceSettlementDetail>(
    `/admin/finance/settlements/${id}/reallocate`,
    payload,
  );
  return data;
}

export async function importReconciliation(
  form: FormData,
): Promise<{ reconciliation: FinanceReconciliation; duplicate: boolean }> {
  const { data } = await apiClient.post<{
    reconciliation: FinanceReconciliation;
    duplicate: boolean;
  }>("/admin/finance/reconciliations/import", form);
  return data;
}

export async function updateReconciliation(
  id: number,
  payload: { status: "confirmed" | "ignored" | "pending"; note: string },
): Promise<FinanceReconciliation> {
  const { data } = await apiClient.put<FinanceReconciliation>(
    `/admin/finance/reconciliations/${id}`,
    payload,
  );
  return data;
}

export async function getPromotionCreditReconciliations(params: {
  status?: PromotionCreditReconciliation["status"];
  page?: number;
  page_size?: number;
}): Promise<FinancePaginatedResponse<PromotionCreditReconciliation>> {
  const { data } = await apiClient.get<FinancePaginatedResponse<PromotionCreditReconciliation>>(
    "/admin/finance/promotion-credit-reconciliations",
    { params },
  );
  return data;
}

export async function resolvePromotionCreditReconciliation(
  userId: number,
  payload: { confirmed_remaining_amount: string; note: string },
): Promise<PromotionCreditReconciliation> {
  const { data } = await apiClient.post<PromotionCreditReconciliation>(
    `/admin/finance/promotion-credit-reconciliations/${userId}/resolve`,
    payload,
  );
  return data;
}

export async function getFXRates(params: { currency?: string; page?: number; page_size?: number } = {}): Promise<FinancePaginatedResponse<FinanceFXRateVersion>> {
  const { data } = await apiClient.get<FinancePaginatedResponse<FinanceFXRateVersion>>('/admin/finance/fx-rates', { params })
  return data
}

export async function createFXRate(payload: FinanceFXRateCreateInput): Promise<FinanceFXRateVersion> {
  const { data } = await apiClient.post<FinanceFXRateVersion>('/admin/finance/fx-rates', payload)
  return data
}

export default {
  getOverview,
  getTrend,
  getBreakdown,
  getLosses,
  getFunds,
  getDataQuality,
  getAlerts,
  updateAlert,
  createExport,
  getExport,
  downloadExport,
  previewBackfill,
  runBackfill,
  getBackfill,
  pauseBackfill,
  resumeBackfill,
  getReconciliations,
  getSettlements,
  getSettlement,
  retrySettlement,
  reallocateSettlement,
  importReconciliation,
  updateReconciliation,
  getPromotionCreditReconciliations,
  resolvePromotionCreditReconciliation,
  getFXRates,
  createFXRate,
};
