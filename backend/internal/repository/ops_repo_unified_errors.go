package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type opsUnifiedErrorCandidate struct {
	item                 service.OpsUnifiedErrorItem
	errorType            string
	errorPhase           string
	errorSource          string
	errorOwner           string
	errorBody            string
	upstreamStatusCode   *int
	upstreamErrorMessage string
	upstreamErrorDetail  string
	upstreamErrors       string
	isBusinessLimited    bool
	authLatencyMs        *int64
	routingLatencyMs     *int64
	upstreamLatencyMs    *int64
	responseLatencyMs    *int64
	timeToFirstTokenMs   *int64
	requestPath          string
	inboundEndpoint      string
	upstreamEndpoint     string
	requestedModel       string
	upstreamModel        string
	rawSeverity          string
	clientStatusCode     int
	message              string
}

func (r *opsRepository) ListUnifiedErrors(ctx context.Context, filter *service.OpsUnifiedErrorListFilter) (*service.OpsUnifiedErrorList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.OpsUnifiedErrorListFilter{}
	}
	page, pageSize := normalizeUnifiedErrorPagination(filter.Page, filter.PageSize)
	baseWhere, args := buildUnifiedErrorCandidateWhere(filter)
	classifiedWhere, args := buildUnifiedErrorClassifiedWhere(filter, args)
	sortExpr := unifiedErrorSortExpr(filter.SortBy)
	sortOrder := "DESC"
	if strings.EqualFold(strings.TrimSpace(filter.SortOrder), "asc") {
		sortOrder = "ASC"
	}
	args = append(args, pageSize, (page-1)*pageSize)
	limitArg := itoa(len(args) - 1)
	offsetArg := itoa(len(args))

	q := `
WITH base AS (
  SELECT
    e.id,
    e.created_at,
    e.error_phase,
    e.error_type,
    COALESCE(e.error_owner, '') AS error_owner,
    COALESCE(e.error_source, '') AS error_source,
    e.severity,
    COALESCE(e.status_code, 0) AS client_status_code,
    COALESCE(e.upstream_status_code, e.status_code, 0) AS effective_status_code,
    COALESCE(e.platform, '') AS platform,
    COALESCE(e.model, '') AS model,
    COALESCE(e.client_request_id, '') AS client_request_id,
    COALESCE(e.request_id, '') AS request_id,
    COALESCE(e.error_message, '') AS error_message,
    COALESCE(e.error_body, '') AS error_body,
    e.upstream_status_code,
    COALESCE(e.upstream_error_message, '') AS upstream_error_message,
    COALESCE(e.upstream_error_detail, '') AS upstream_error_detail,
    COALESCE(e.upstream_errors::text, '') AS upstream_errors,
    COALESCE(e.is_business_limited, false) AS is_business_limited,
    COALESCE(e.user_id, ak.user_id) AS user_id,
    COALESCE(u.email, '') AS user_email,
    e.api_key_id,
    COALESCE(ak.name, '') AS api_key_name,
    e.account_id,
    COALESCE(a.name, '') AS account_name,
    e.group_id,
    COALESCE(g.name, '') AS group_name,
    COALESCE(e.request_path, '') AS request_path,
    COALESCE(e.inbound_endpoint, '') AS inbound_endpoint,
    COALESCE(e.upstream_endpoint, '') AS upstream_endpoint,
    COALESCE(e.requested_model, '') AS requested_model,
    COALESCE(e.upstream_model, '') AS upstream_model,
    e.auth_latency_ms,
    e.routing_latency_ms,
    e.upstream_latency_ms,
    e.response_latency_ms,
    e.time_to_first_token_ms,
    COALESCE((
      SELECT t.status
      FROM ops_ai_analysis_tasks t
      WHERE t.source_type IN ('unified_errors','manual_filter')
        AND (t.filters @> jsonb_build_object('error_id', e.id) OR t.filters @> jsonb_build_object('error_ids', jsonb_build_array(e.id)))
      ORDER BY t.created_at DESC
      LIMIT 1
    ), 'not_analyzed') AS ai_analysis_status,
    LOWER(COALESCE(e.error_type,'') || ' ' || COALESCE(e.error_phase,'') || ' ' || COALESCE(e.error_source,'') || ' ' || COALESCE(e.error_owner,'') || ' ' || COALESCE(e.error_message,'') || ' ' || COALESCE(e.error_body,'') || ' ' || COALESCE(e.upstream_error_message,'') || ' ' || COALESCE(e.upstream_error_detail,'') || ' ' || COALESCE(e.upstream_errors::text,'') || ' ' || COALESCE(e.request_path,'') || ' ' || COALESCE(e.inbound_endpoint,'') || ' ' || COALESCE(e.upstream_endpoint,'') || ' ' || COALESCE(e.requested_model,'') || ' ' || COALESCE(e.upstream_model,'') || ' ' || COALESCE(e.model,'')) AS text_blob
  FROM ops_error_logs e
  LEFT JOIN api_keys ak ON e.api_key_id = ak.id
  LEFT JOIN users u ON COALESCE(e.user_id, ak.user_id) = u.id
  LEFT JOIN accounts a ON e.account_id = a.id
  LEFT JOIN groups g ON e.group_id = g.id
  ` + baseWhere + `
), classified AS (
  SELECT
    base.*,
    ` + unifiedErrorCategorySQL() + ` AS error_category,
    ` + unifiedErrorSubcategorySQL() + ` AS error_subcategory
  FROM base
), enriched AS (
  SELECT
    classified.*,
    CASE WHEN error_category = 'client' THEN error_subcategory ELSE NULL END AS client_error_subcategory,
    ` + unifiedErrorResultSQL() + ` AS error_result,
    ` + unifiedSeveritySQL() + ` AS unified_severity
  FROM classified
), filtered AS (
  SELECT
    enriched.*,
    COUNT(*) OVER() AS total_count,
    COUNT(*) OVER(PARTITION BY error_category, error_subcategory, COALESCE(client_error_subcategory,''), platform, model, effective_status_code) AS same_kind_count
  FROM enriched
  ` + classifiedWhere + `
)
SELECT
  id,
  created_at,
  error_phase,
  error_type,
  error_owner,
  error_source,
  severity,
  client_status_code,
  effective_status_code,
  platform,
  model,
  client_request_id,
  request_id,
  error_message,
  error_body,
  upstream_status_code,
  upstream_error_message,
  upstream_error_detail,
  upstream_errors,
  is_business_limited,
  user_id,
  user_email,
  api_key_id,
  api_key_name,
  account_id,
  account_name,
  group_id,
  group_name,
  request_path,
  inbound_endpoint,
  upstream_endpoint,
  requested_model,
  upstream_model,
  auth_latency_ms,
  routing_latency_ms,
  upstream_latency_ms,
  response_latency_ms,
  time_to_first_token_ms,
  ai_analysis_status,
  same_kind_count,
  total_count
FROM filtered
ORDER BY ` + sortExpr + ` ` + sortOrder + `, created_at DESC
LIMIT $` + limitArg + ` OFFSET $` + offsetArg

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsUnifiedErrorItem, 0, pageSize)
	total := 0
	for rows.Next() {
		candidate, sameKindCount, rowTotal, err := scanUnifiedErrorCandidate(rows)
		if err != nil {
			return nil, err
		}
		applyUnifiedErrorClassification(&candidate)
		candidate.item.SameKindCount = sameKindCount
		item := candidate.item
		items = append(items, &item)
		if rowTotal > total {
			total = rowTotal
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if total == 0 && page > 1 {
		firstPageFilter := *filter
		firstPageFilter.Page = 1
		firstPageFilter.PageSize = pageSize
		firstPage, err := r.ListUnifiedErrors(ctx, &firstPageFilter)
		if err != nil {
			return nil, err
		}
		total = firstPage.Total
	}
	return &service.OpsUnifiedErrorList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func buildUnifiedErrorCandidateWhere(filter *service.OpsUnifiedErrorListFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	if filter == nil {
		return "WHERE " + strings.Join(clauses, " AND "), args
	}
	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, filter.StartTime.UTC())
		clauses = append(clauses, "e.created_at >= $"+itoa(len(args)))
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, filter.EndTime.UTC())
		clauses = append(clauses, "e.created_at < $"+itoa(len(args)))
	}
	if filter.UserID != nil && *filter.UserID > 0 {
		args = append(args, *filter.UserID)
		clauses = append(clauses, "COALESCE(e.user_id, ak.user_id) = $"+itoa(len(args)))
	}
	if filter.APIKeyID != nil && *filter.APIKeyID > 0 {
		args = append(args, *filter.APIKeyID)
		clauses = append(clauses, "e.api_key_id = $"+itoa(len(args)))
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, *filter.GroupID)
		clauses = append(clauses, "e.group_id = $"+itoa(len(args)))
	}
	if filter.UpstreamAccountID != nil && *filter.UpstreamAccountID > 0 {
		args = append(args, *filter.UpstreamAccountID)
		clauses = append(clauses, "e.account_id = $"+itoa(len(args)))
	}
	if platform := strings.TrimSpace(filter.Platform); platform != "" {
		args = append(args, strings.ToLower(platform))
		clauses = append(clauses, "LOWER(COALESCE(e.platform,'')) = $"+itoa(len(args)))
	}
	if model := strings.TrimSpace(filter.Model); model != "" {
		args = append(args, "%"+model+"%")
		n := itoa(len(args))
		clauses = append(clauses, "(COALESCE(e.model,'') ILIKE $"+n+" OR COALESCE(e.requested_model,'') ILIKE $"+n+" OR COALESCE(e.upstream_model,'') ILIKE $"+n+")")
	}
	if len(filter.StatusCodes) > 0 {
		args = append(args, pq.Array(filter.StatusCodes))
		clauses = append(clauses, "COALESCE(e.upstream_status_code, e.status_code, 0) = ANY($"+itoa(len(args))+")")
	}
	if requestID := strings.TrimSpace(filter.RequestID); requestID != "" {
		args = append(args, requestID)
		n := itoa(len(args))
		clauses = append(clauses, "(COALESCE(e.request_id,'') = $"+n+" OR COALESCE(e.client_request_id,'') = $"+n+")")
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		args = append(args, "%"+keyword+"%")
		n := itoa(len(args))
		clauses = append(clauses, "(COALESCE(e.error_message,'') ILIKE $"+n+" OR COALESCE(e.upstream_error_message,'') ILIKE $"+n+" OR COALESCE(e.upstream_error_detail,'') ILIKE $"+n+" OR COALESCE(e.request_id,'') ILIKE $"+n+" OR COALESCE(e.client_request_id,'') ILIKE $"+n+")")
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildUnifiedErrorClassifiedWhere(filter *service.OpsUnifiedErrorListFilter, args []any) (string, []any) {
	clauses := []string{"1=1"}
	if filter == nil {
		return "WHERE " + strings.Join(clauses, " AND "), args
	}
	if len(filter.ErrorCategories) > 0 {
		args = append(args, pq.Array(filter.ErrorCategories))
		clauses = append(clauses, "error_category = ANY($"+itoa(len(args))+")")
	}
	if len(filter.ErrorSubcategories) > 0 {
		args = append(args, pq.Array(filter.ErrorSubcategories))
		clauses = append(clauses, "error_subcategory = ANY($"+itoa(len(args))+")")
	}
	if len(filter.ClientErrorSubcategories) > 0 {
		args = append(args, pq.Array(filter.ClientErrorSubcategories))
		clauses = append(clauses, "client_error_subcategory = ANY($"+itoa(len(args))+")")
	}
	if len(filter.ErrorResults) > 0 {
		args = append(args, pq.Array(filter.ErrorResults))
		clauses = append(clauses, "error_result = ANY($"+itoa(len(args))+")")
	}
	if len(filter.Severities) > 0 {
		args = append(args, pq.Array(filter.Severities))
		clauses = append(clauses, "unified_severity = ANY($"+itoa(len(args))+")")
	}
	switch strings.TrimSpace(filter.AIAnalysis) {
	case "", service.OpsUnifiedAIAnalysisAll:
	case service.OpsUnifiedAIAnalysisAnalyzed:
		clauses = append(clauses, "ai_analysis_status = 'completed'")
	case service.OpsUnifiedAIAnalysisNotAnalyzed:
		clauses = append(clauses, "ai_analysis_status = 'not_analyzed'")
	default:
		clauses = append(clauses, "1=0")
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func unifiedErrorCategorySQL() string {
	return `CASE
      WHEN text_blob LIKE '%context canceled%' OR text_blob LIKE '%client canceled%' OR text_blob LIKE '%request canceled%' OR text_blob LIKE '%cancelled%' OR text_blob LIKE '%broken pipe%' OR text_blob LIKE '%connection reset%' OR text_blob LIKE '%client disconnected%' THEN 'client'
      WHEN text_blob LIKE '%no available accounts%' OR text_blob LIKE '%no available account%' OR text_blob LIKE '%account pool%' OR text_blob LIKE '%账号池%' OR text_blob LIKE '%账号不可用%' OR text_blob LIKE '%无可用账号%' OR text_blob LIKE '%account scheduler%' OR text_blob LIKE '%scheduling account%' THEN 'account_pool'
      WHEN ` + unifiedSQLClientSideExpr() + ` THEN 'client'
      WHEN effective_status_code = 429 OR text_blob LIKE '%rate limit%' OR text_blob LIKE '%rate_limit%' OR text_blob LIKE '%too many requests%' OR text_blob LIKE '%rpm%' OR text_blob LIKE '%tpm%' OR text_blob LIKE '%concurrency%' OR text_blob LIKE '%限流%' OR text_blob LIKE '%频率限制%' THEN 'rate_limit'
      WHEN effective_status_code IN (401,403) OR text_blob LIKE '%permission%' OR text_blob LIKE '%unauthorized%' OR text_blob LIKE '%forbidden%' OR text_blob LIKE '%access denied%' OR text_blob LIKE '%invalid api key%' OR text_blob LIKE '%invalid_api_key%' OR text_blob LIKE '%权限%' OR text_blob LIKE '%鉴权%' THEN 'permission'
      WHEN text_blob LIKE '%insufficient balance%' OR text_blob LIKE '%insufficient_balance%' OR text_blob LIKE '%balance%' OR text_blob LIKE '%quota%' OR text_blob LIKE '%credit%' OR text_blob LIKE '%usage limit%' OR text_blob LIKE '%subscription%' OR text_blob LIKE '%余额%' OR text_blob LIKE '%额度%' THEN 'balance'
      WHEN text_blob LIKE '%model mapping%' OR text_blob LIKE '%no mapping%' OR text_blob LIKE '%mapped model%' OR text_blob LIKE '%channel config%' OR text_blob LIKE '%config%' OR text_blob LIKE '%cache config%' OR text_blob LIKE '%ai config%' OR text_blob LIKE '%配置%' OR text_blob LIKE '%映射%' THEN 'config'
      WHEN text_blob LIKE '%slow%' OR text_blob LIKE '%p99%' OR text_blob LIKE '%ttft%' OR text_blob LIKE '%time to first token%' OR text_blob LIKE '%latency%' OR text_blob LIKE '%耗时%' OR text_blob LIKE '%慢请求%' OR COALESCE(time_to_first_token_ms,0) >= 30000 OR COALESCE(response_latency_ms,0) >= 120000 OR COALESCE(upstream_latency_ms,0) >= 120000 THEN 'slow_request'
      WHEN ` + unifiedSQLHasUpstreamEvidenceExpr() + ` OR effective_status_code >= 500 OR ((LOWER(error_owner) <> 'platform') AND (text_blob LIKE '%timeout%' OR text_blob LIKE '%overloaded%' OR text_blob LIKE '%unavailable%' OR text_blob LIKE '%bad gateway%' OR text_blob LIKE '%service unavailable%' OR text_blob LIKE '%gateway timeout%')) THEN 'upstream'
      WHEN text_blob LIKE '%panic%' OR text_blob LIKE '%internal%' OR text_blob LIKE '%database%' OR text_blob LIKE '%redis%' OR text_blob LIKE '%gateway%' OR text_blob LIKE '%platform%' OR text_blob LIKE '%平台%' OR LOWER(error_owner) = 'platform' THEN 'platform'
      ELSE 'unknown'
    END`
}

func unifiedErrorSubcategorySQL() string {
	return `CASE
      WHEN text_blob LIKE '%context canceled%' OR text_blob LIKE '%client canceled%' OR text_blob LIKE '%request canceled%' OR text_blob LIKE '%cancelled%' OR text_blob LIKE '%broken pipe%' OR text_blob LIKE '%connection reset%' OR text_blob LIKE '%client disconnected%' THEN 'client_disconnect_error'
      WHEN text_blob LIKE '%no available accounts%' OR text_blob LIKE '%no available account%' OR text_blob LIKE '%account pool%' OR text_blob LIKE '%账号池%' OR text_blob LIKE '%账号不可用%' OR text_blob LIKE '%无可用账号%' OR text_blob LIKE '%account scheduler%' OR text_blob LIKE '%scheduling account%' THEN 'account_pool_empty'
      WHEN ` + unifiedSQLClientSideExpr() + ` THEN ` + unifiedClientSubcategorySQL() + `
      WHEN effective_status_code = 429 OR text_blob LIKE '%rate limit%' OR text_blob LIKE '%rate_limit%' OR text_blob LIKE '%too many requests%' OR text_blob LIKE '%rpm%' OR text_blob LIKE '%tpm%' OR text_blob LIKE '%concurrency%' OR text_blob LIKE '%限流%' OR text_blob LIKE '%频率限制%' THEN 'upstream_rate_limit'
      WHEN effective_status_code IN (401,403) OR text_blob LIKE '%permission%' OR text_blob LIKE '%unauthorized%' OR text_blob LIKE '%forbidden%' OR text_blob LIKE '%access denied%' OR text_blob LIKE '%invalid api key%' OR text_blob LIKE '%invalid_api_key%' OR text_blob LIKE '%权限%' OR text_blob LIKE '%鉴权%' THEN 'upstream_permission_error'
      WHEN text_blob LIKE '%insufficient balance%' OR text_blob LIKE '%insufficient_balance%' OR text_blob LIKE '%balance%' OR text_blob LIKE '%quota%' OR text_blob LIKE '%credit%' OR text_blob LIKE '%usage limit%' OR text_blob LIKE '%subscription%' OR text_blob LIKE '%余额%' OR text_blob LIKE '%额度%' THEN 'upstream_balance_error'
      WHEN text_blob LIKE '%model mapping%' OR text_blob LIKE '%no mapping%' OR text_blob LIKE '%mapped model%' OR text_blob LIKE '%channel config%' OR text_blob LIKE '%config%' OR text_blob LIKE '%cache config%' OR text_blob LIKE '%ai config%' OR text_blob LIKE '%配置%' OR text_blob LIKE '%映射%' THEN 'config_model_mapping_error'
      WHEN text_blob LIKE '%slow%' OR text_blob LIKE '%p99%' OR text_blob LIKE '%ttft%' OR text_blob LIKE '%time to first token%' OR text_blob LIKE '%latency%' OR text_blob LIKE '%耗时%' OR text_blob LIKE '%慢请求%' OR COALESCE(time_to_first_token_ms,0) >= 30000 OR COALESCE(response_latency_ms,0) >= 120000 OR COALESCE(upstream_latency_ms,0) >= 120000 THEN 'slow_response'
      WHEN ` + unifiedSQLHasUpstreamEvidenceExpr() + ` OR effective_status_code >= 500 OR ((LOWER(error_owner) <> 'platform') AND (text_blob LIKE '%timeout%' OR text_blob LIKE '%overloaded%' OR text_blob LIKE '%unavailable%' OR text_blob LIKE '%bad gateway%' OR text_blob LIKE '%service unavailable%' OR text_blob LIKE '%gateway timeout%')) THEN CASE WHEN text_blob LIKE '%timeout%' OR text_blob LIKE '%deadline%' OR text_blob LIKE '%gateway timeout%' OR effective_status_code = 504 THEN 'upstream_timeout' WHEN effective_status_code IN (502,503) OR text_blob LIKE '%overloaded%' OR text_blob LIKE '%unavailable%' OR text_blob LIKE '%bad gateway%' OR text_blob LIKE '%service unavailable%' THEN 'upstream_unavailable' ELSE 'upstream_error' END
      WHEN text_blob LIKE '%panic%' OR text_blob LIKE '%internal%' OR text_blob LIKE '%database%' OR text_blob LIKE '%redis%' OR text_blob LIKE '%gateway%' OR text_blob LIKE '%platform%' OR text_blob LIKE '%平台%' OR LOWER(error_owner) = 'platform' THEN CASE WHEN text_blob LIKE '%database%' OR text_blob LIKE '%redis%' OR text_blob LIKE '%dependency%' OR text_blob LIKE '%依赖%' THEN 'platform_dependency_error' ELSE 'platform_internal_error' END
      ELSE 'unknown_insufficient_evidence'
    END`
}

func unifiedClientSubcategorySQL() string {
	return `CASE
        WHEN effective_status_code IN (401,403) OR text_blob LIKE '%invalid api key%' OR text_blob LIKE '%invalid_api_key%' OR text_blob LIKE '%api_key_required%' OR text_blob LIKE '%api key required%' OR text_blob LIKE '%api_key_disabled%' OR text_blob LIKE '%api_key_expired%' OR text_blob LIKE '%key disabled%' OR text_blob LIKE '%key missing%' OR text_blob LIKE '%unauthorized%' OR text_blob LIKE '%forbidden%' OR text_blob LIKE '%鉴权%' OR text_blob LIKE '%key 无效%' OR text_blob LIKE '%key 禁用%' THEN 'client_auth_error'
        WHEN effective_status_code = 429 OR text_blob LIKE '%rate limit%' OR text_blob LIKE '%rate_limit%' OR text_blob LIKE '%too many requests%' OR text_blob LIKE '%user rate%' OR text_blob LIKE '%key rate%' OR text_blob LIKE '%group rate%' OR text_blob LIKE '%rpm%' OR text_blob LIKE '%tpm%' OR text_blob LIKE '%concurrency%' OR text_blob LIKE '%pending%' OR text_blob LIKE '%queue%' OR text_blob LIKE '%用户限流%' OR text_blob LIKE '%key 限流%' THEN 'client_rate_limit_error'
        WHEN is_business_limited OR text_blob LIKE '%insufficient balance%' OR text_blob LIKE '%insufficient_balance%' OR text_blob LIKE '%insufficient quota%' OR text_blob LIKE '%quota exhausted%' OR text_blob LIKE '%api_key_quota_exhausted%' OR text_blob LIKE '%balance%' OR text_blob LIKE '%余额不足%' OR text_blob LIKE '%额度不足%' OR text_blob LIKE '%配额耗尽%' THEN 'client_balance_error'
        WHEN text_blob LIKE '%context length%' OR text_blob LIKE '%context window%' OR text_blob LIKE '%maximum context%' OR text_blob LIKE '%max_tokens%' OR text_blob LIKE '%input tokens%' OR text_blob LIKE '%output tokens%' OR text_blob LIKE '%token limit%' OR text_blob LIKE '%上下文%' OR text_blob LIKE '%超限%' THEN 'client_context_error'
        WHEN text_blob LIKE '%model not found%' OR text_blob LIKE '%model unavailable%' OR text_blob LIKE '%model does not exist%' OR text_blob LIKE '%unsupported model%' OR text_blob LIKE '%no mapping%' OR text_blob LIKE '%model mapping%' OR text_blob LIKE '%no available channel%' OR text_blob LIKE '%无可用渠道%' OR text_blob LIKE '%模型不存在%' OR text_blob LIKE '%模型不可用%' OR text_blob LIKE '%模型权限%' THEN 'client_model_error'
        WHEN effective_status_code IN (404,405) OR text_blob LIKE '%not found%' OR text_blob LIKE '%route not found%' OR text_blob LIKE '%method not allowed%' OR text_blob LIKE '%unsupported method%' OR text_blob LIKE '%路径不存在%' OR text_blob LIKE '%方法不支持%' THEN 'client_path_error'
        WHEN effective_status_code IN (400,422) OR text_blob LIKE '%invalid request%' OR text_blob LIKE '%invalid_request%' OR text_blob LIKE '%validation%' OR text_blob LIKE '%missing required%' OR text_blob LIKE '%bad request%' OR text_blob LIKE '%json%' OR text_blob LIKE '%request body%' OR text_blob LIKE '%parameter%' OR text_blob LIKE '%param%' OR text_blob LIKE '%参数%' OR text_blob LIKE '%请求体%' THEN 'client_parameter_error'
        ELSE 'client_insufficient_evidence'
      END`
}

func unifiedErrorResultSQL() string {
	return `CASE
      WHEN error_subcategory = 'client_disconnect_error' THEN 'client_aborted'
      WHEN client_status_code >= 400 THEN 'final_failed'
      WHEN client_status_code > 0 AND client_status_code < 400 THEN 'recovered'
      ELSE 'unknown'
    END`
}

func unifiedSeveritySQL() string {
	return `CASE
      WHEN UPPER(COALESCE(severity,'')) IN ('P0','P1','P2') THEN UPPER(severity)
      WHEN UPPER(COALESCE(severity,'')) = 'P3' OR LOWER(COALESCE(severity,'')) = 'observe' THEN 'observe'
      WHEN effective_status_code >= 500 OR effective_status_code = 429 THEN 'P1'
      WHEN effective_status_code >= 400 THEN 'P2'
      ELSE 'normal'
    END`
}

func unifiedSQLHasUpstreamEvidenceExpr() string {
	return `(upstream_status_code IS NOT NULL OR text_blob LIKE '%upstream_http%' OR text_blob LIKE '%provider%' OR text_blob LIKE '%upstream error%' OR text_blob LIKE '%upstream_error%' OR text_blob LIKE '%upstream_status%' OR text_blob LIKE '%upstream status%' OR LOWER(error_owner) = 'provider' OR LOWER(error_source) = 'upstream_http' OR LOWER(error_phase) = 'upstream')`
}

func unifiedSQLClientSideExpr() string {
	return `((NOT ` + unifiedSQLHasUpstreamEvidenceExpr() + ` OR LOWER(error_owner) = 'client' OR LOWER(error_source) = 'client_request') AND (LOWER(error_owner) = 'client' OR LOWER(error_source) = 'client_request' OR LOWER(error_phase) IN ('auth','request')))`
}

func unifiedErrorSortExpr(sortBy string) string {
	switch strings.TrimSpace(sortBy) {
	case "status_code":
		return "effective_status_code"
	case "severity":
		return "CASE unified_severity WHEN 'P0' THEN 5 WHEN 'P1' THEN 4 WHEN 'P2' THEN 3 WHEN 'observe' THEN 2 WHEN 'normal' THEN 1 ELSE 0 END"
	case "same_kind_count":
		return "same_kind_count"
	case "occurred_at":
		fallthrough
	default:
		return "created_at"
	}
}
func scanUnifiedErrorCandidate(rows *sql.Rows) (opsUnifiedErrorCandidate, int, int, error) {
	var c opsUnifiedErrorCandidate
	var userID, apiKeyID, accountID, groupID sql.NullInt64
	var userEmail, apiKeyName, accountName, groupName string
	var upstreamStatusCode sql.NullInt64
	var authLatency, routingLatency, upstreamLatency, responseLatency, ttft sql.NullInt64
	var sameKindCount int
	var totalCount int
	if err := rows.Scan(
		&c.item.ID,
		&c.item.OccurredAt,
		&c.errorPhase,
		&c.errorType,
		&c.errorOwner,
		&c.errorSource,
		&c.rawSeverity,
		&c.clientStatusCode,
		&c.item.StatusCode,
		&c.item.Platform,
		&c.item.Model,
		new(string),
		new(string),
		&c.message,
		&c.errorBody,
		&upstreamStatusCode,
		&c.upstreamErrorMessage,
		&c.upstreamErrorDetail,
		&c.upstreamErrors,
		&c.isBusinessLimited,
		&userID,
		&userEmail,
		&apiKeyID,
		&apiKeyName,
		&accountID,
		&accountName,
		&groupID,
		&groupName,
		&c.requestPath,
		&c.inboundEndpoint,
		&c.upstreamEndpoint,
		&c.requestedModel,
		&c.upstreamModel,
		&authLatency,
		&routingLatency,
		&upstreamLatency,
		&responseLatency,
		&ttft,
		&c.item.AIAnalysisStatus,
		&sameKindCount,
		&totalCount,
	); err != nil {
		return c, 0, 0, err
	}
	if upstreamStatusCode.Valid && upstreamStatusCode.Int64 > 0 {
		v := int(upstreamStatusCode.Int64)
		c.upstreamStatusCode = &v
	}
	c.authLatencyMs = sqlNullInt64Ptr(authLatency)
	c.routingLatencyMs = sqlNullInt64Ptr(routingLatency)
	c.upstreamLatencyMs = sqlNullInt64Ptr(upstreamLatency)
	c.responseLatencyMs = sqlNullInt64Ptr(responseLatency)
	c.timeToFirstTokenMs = sqlNullInt64Ptr(ttft)
	if userID.Valid {
		c.item.User = &service.OpsUnifiedEntityRef{ID: userID.Int64, Email: userEmail}
	}
	if apiKeyID.Valid {
		c.item.APIKey = &service.OpsUnifiedEntityRef{ID: apiKeyID.Int64, Name: apiKeyName, Display: opsUnifiedAPIKeyDisplay(apiKeyID.Int64, apiKeyName)}
	}
	if accountID.Valid {
		c.item.UpstreamAccount = &service.OpsUnifiedEntityRef{ID: accountID.Int64, Name: accountName}
	}
	if groupID.Valid {
		c.item.Group = &service.OpsUnifiedEntityRef{ID: groupID.Int64, Name: groupName}
	}
	c.upstreamErrors = normalizedOpsErrorJSONText(c.upstreamErrors)
	if c.item.AIAnalysisStatus == "" {
		c.item.AIAnalysisStatus = service.OpsUnifiedAIAnalysisNotAnalyzed
	}
	return c, sameKindCount, totalCount, nil
}

func applyUnifiedErrorClassification(c *opsUnifiedErrorCandidate) {
	classification := service.ClassifyOpsError(service.OpsErrorClassificationInput{
		StatusCode:           c.item.StatusCode,
		UpstreamStatusCode:   c.upstreamStatusCode,
		ErrorType:            c.errorType,
		ErrorPhase:           c.errorPhase,
		ErrorSource:          c.errorSource,
		ErrorOwner:           c.errorOwner,
		ErrorMessage:         c.message,
		ErrorBody:            c.errorBody,
		UpstreamErrorMessage: c.upstreamErrorMessage,
		UpstreamErrorDetail:  c.upstreamErrorDetail,
		UpstreamErrors:       c.upstreamErrors,
		RequestPath:          c.requestPath,
		InboundEndpoint:      c.inboundEndpoint,
		UpstreamEndpoint:     c.upstreamEndpoint,
		RequestedModel:       c.requestedModel,
		UpstreamModel:        c.upstreamModel,
		Model:                c.item.Model,
		IsBusinessLimited:    c.isBusinessLimited,
		AuthLatencyMs:        c.authLatencyMs,
		RoutingLatencyMs:     c.routingLatencyMs,
		UpstreamLatencyMs:    c.upstreamLatencyMs,
		ResponseLatencyMs:    c.responseLatencyMs,
		TimeToFirstTokenMs:   c.timeToFirstTokenMs,
	})
	c.item.ErrorCategory = classification.ErrorCategory
	c.item.ErrorSubcategory = classification.ErrorSubcategory
	if classification.ClientErrorSubcategory != "" {
		subcategory := classification.ClientErrorSubcategory
		c.item.ClientErrorSubcategory = &subcategory
	}
	c.item.ErrorResult = unifiedErrorResultFor(c.clientStatusCode, classification)
	c.item.Severity = unifiedSeverityFor(c.rawSeverity, c.item.StatusCode)
	c.item.Summary = unifiedErrorSummary(classification.ClassificationReason, c.message, c.upstreamErrorMessage)
}

func normalizeUnifiedErrorPagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	switch pageSize {
	case 20, 50, 100:
		return page, pageSize
	case 0:
		return page, 20
	default:
		if pageSize < 20 {
			return page, 20
		}
		if pageSize > 100 {
			return page, 100
		}
		return page, 20
	}
}

func unifiedErrorResultFor(statusCode int, classification service.OpsErrorClassification) string {
	if classification.ClientErrorSubcategory == service.OpsClientErrorSubcategoryDisconnect {
		return service.OpsUnifiedErrorResultClientAborted
	}
	if statusCode >= 400 {
		return service.OpsUnifiedErrorResultFinalFailed
	}
	if statusCode > 0 && statusCode < 400 {
		return service.OpsUnifiedErrorResultRecovered
	}
	return service.OpsUnifiedErrorResultUnknown
}

func unifiedSeverityFor(raw string, statusCode int) string {
	raw = strings.TrimSpace(raw)
	switch strings.ToUpper(raw) {
	case "P0", "P1", "P2":
		return strings.ToUpper(raw)
	case "P3":
		return "observe"
	}
	if strings.EqualFold(raw, "observe") {
		return "observe"
	}
	if statusCode >= 500 || statusCode == 429 {
		return "P1"
	}
	if statusCode >= 400 {
		return "P2"
	}
	return "normal"
}

func unifiedErrorSummary(reason, message, upstreamMessage string) string {
	for _, candidate := range []string{reason, message, upstreamMessage} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			if len([]rune(candidate)) > 160 {
				return string([]rune(candidate)[:160])
			}
			return candidate
		}
	}
	return "暂无摘要"
}

func opsUnifiedAPIKeyDisplay(id int64, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("API Key #%d", id)
	}
	return fmt.Sprintf("%s #%d", name, id)
}
