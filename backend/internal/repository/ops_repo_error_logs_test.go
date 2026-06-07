package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListErrorLogs_PlatformSLADetailsScansRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 5, 28, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	createdAt := start.Add(10 * time.Minute)
	impactSLA := true

	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\) FROM ops_error_logs e .*COALESCE\(e\.status_code, 0\) >= 400 AND \(e\.error_owner = 'platform'.*e\.error_owner = 'provider'`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "status_code", "client_status_code", "platform", "model",
		"resolved", "resolved_at", "resolved_by_user_id", "resolved_by_user_email", "client_request_id", "request_id", "error_message",
		"user_id", "user_email", "api_key_id", "account_id", "account_name", "group_id", "group_name", "client_ip", "request_path", "stream",
		"inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model", "request_type",
		"upstream_status_code", "error_body", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms",
	}).AddRow(
		int64(7), createdAt, "upstream", "upstream_http_error", "provider", "upstream_http", "error", 500, 500, "openai", "gpt-5.4-upstream",
		false, nil, nil, "", "client-req-1", "req-1", "provider failed",
		int64(11), "user@example.com", int64(22), int64(42), "上游账号A", int64(33), "默认分组", "127.0.0.1", "/v1/chat/completions", true,
		"/v1/chat/completions", "/v1/responses", "gpt-5.4", "gpt-5.4-upstream", int64(2),
		500, "", "provider failed", "", "", false, nil, nil, nil, nil, nil,
	)
	mock.ExpectQuery(`(?s)SELECT\s+e\.id,.*e\.api_key_id.*e\.account_id.*COALESCE\(a\.name, ''\).*e\.group_id.*COALESCE\(g\.name, ''\).*COALESCE\(e\.inbound_endpoint, ''\).*COALESCE\(e\.upstream_endpoint, ''\).*COALESCE\(e\.requested_model, ''\).*COALESCE\(e\.upstream_model, ''\).*e\.request_type.*FROM ops_error_logs e .*COALESCE\(e\.status_code, 0\) >= 400 AND \(e\.error_owner = 'platform'.*e\.error_owner = 'provider'`).
		WithArgs(start, end, 10, 0).
		WillReturnRows(rows)

	got, err := repo.ListErrorLogs(context.Background(), &service.OpsErrorLogFilter{
		StartTime:         &start,
		EndTime:           &end,
		ImpactPlatformSLA: &impactSLA,
		View:              "all",
		Page:              1,
		PageSize:          10,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, got)
	require.Equal(t, 1, got.Total)
	require.Len(t, got.Errors, 1)
	item := got.Errors[0]
	require.Equal(t, int64(7), item.ID)
	require.Equal(t, "upstream_http_error", item.Type)
	require.Equal(t, "provider", item.Owner)
	require.Equal(t, "上游账号A", item.AccountName)
	require.Equal(t, "默认分组", item.GroupName)
	require.Equal(t, "/v1/chat/completions", item.InboundEndpoint)
	require.Equal(t, "/v1/responses", item.UpstreamEndpoint)
	require.Equal(t, "gpt-5.4", item.RequestedModel)
	require.Equal(t, "gpt-5.4-upstream", item.UpstreamModel)
	require.NotNil(t, item.RequestType)
	require.Equal(t, int16(2), *item.RequestType)
	require.NotNil(t, item.UserID)
	require.Equal(t, int64(11), *item.UserID)
	require.Equal(t, "user@example.com", item.UserEmail)
	require.NotNil(t, item.AccountID)
	require.Equal(t, int64(42), *item.AccountID)
}

func TestListErrorLogs_BackfillsRequesterFromAPIKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	createdAt := time.Date(2026, 6, 2, 6, 30, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\) FROM ops_error_logs e .*COALESCE\(e\.status_code, 0\) >= 400`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "status_code", "client_status_code", "platform", "model",
		"resolved", "resolved_at", "resolved_by_user_id", "resolved_by_user_email", "client_request_id", "request_id", "error_message",
		"user_id", "user_email", "api_key_id", "account_id", "account_name", "group_id", "group_name", "client_ip", "request_path", "stream",
		"inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model", "request_type",
		"upstream_status_code", "error_body", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms",
	}).AddRow(
		int64(8), createdAt, "routing", "forbidden_error", "platform", "gateway", "error", 403, 403, "openai", "",
		false, nil, nil, "", "client-req-2", "req-2", "API Key 所属分组已删除",
		int64(11), "3238607507@qq.com", int64(95), nil, "", nil, "", "127.0.0.1", "/v1/responses", false,
		"/v1/responses", "", "", "", nil,
		nil, "", "", "", "", false, nil, nil, nil, nil, nil,
	)
	mock.ExpectQuery(`(?s)SELECT\s+e\.id,.*COALESCE\(e\.user_id, ak\.user_id\).*COALESCE\(u\.email, ''\).*LEFT JOIN api_keys ak ON e\.api_key_id = ak\.id.*LEFT JOIN users u ON COALESCE\(e\.user_id, ak\.user_id\) = u\.id`).
		WithArgs(20, 0).
		WillReturnRows(rows)

	got, err := repo.ListErrorLogs(context.Background(), &service.OpsErrorLogFilter{Page: 1, PageSize: 20})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, got)
	require.Len(t, got.Errors, 1)
	item := got.Errors[0]
	require.NotNil(t, item.UserID)
	require.Equal(t, int64(11), *item.UserID)
	require.Equal(t, "3238607507@qq.com", item.UserEmail)
	require.NotNil(t, item.APIKeyID)
	require.Equal(t, int64(95), *item.APIKeyID)
}

func TestGetErrorLogByID_BackfillsRequesterFromAPIKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	createdAt := time.Date(2026, 6, 2, 6, 45, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "status_code", "client_status_code", "platform", "model",
		"resolved", "resolved_at", "resolved_by_user_id", "client_request_id", "request_id", "error_message", "error_body",
		"upstream_status_code", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"user_id", "user_email", "api_key_id", "account_id", "account_name", "group_id", "group_name", "client_ip",
		"request_path", "stream", "inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model", "request_type", "user_agent",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms",
	}).AddRow(
		int64(8), createdAt, "routing", "forbidden_error", "platform", "gateway", "error", 403, 403, "openai", "",
		false, nil, nil, "client-req-2", "req-2", "API Key 所属分组已删除", "{}",
		nil, "", "", "", false,
		int64(11), "3238607507@qq.com", int64(95), nil, "", nil, "", "127.0.0.1",
		"/v1/responses", false, "/v1/responses", "", "", "", nil, "codex",
		nil, nil, nil, nil, nil,
	)
	mock.ExpectQuery(`(?s)SELECT\s+e\.id,.*COALESCE\(e\.user_id, ak\.user_id\).*COALESCE\(u\.email, ''\).*FROM ops_error_logs e.*LEFT JOIN api_keys ak ON e\.api_key_id = ak\.id.*LEFT JOIN users u ON COALESCE\(e\.user_id, ak\.user_id\) = u\.id.*WHERE e\.id = \$1`).
		WithArgs(int64(8)).
		WillReturnRows(rows)

	got, err := repo.GetErrorLogByID(context.Background(), 8)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, got)
	require.NotNil(t, got.UserID)
	require.Equal(t, int64(11), *got.UserID)
	require.Equal(t, "3238607507@qq.com", got.UserEmail)
	require.NotNil(t, got.APIKeyID)
	require.Equal(t, int64(95), *got.APIKeyID)
}

func TestBuildOpsErrorLogsWhere_ProviderOwnerDoesNotForceUpstreamPhase(t *testing.T) {
	filter := &service.OpsErrorLogFilter{
		Owner: "provider",
		View:  "all",
	}

	where, args := buildOpsErrorLogsWhere(filter)

	require.Contains(t, where, "LOWER(COALESCE(e.error_owner,'')) = $1")
	require.NotContains(t, where, "e.error_phase")
	require.Equal(t, []any{"provider"}, args)
}

func TestListAndDetailErrorClassificationUseSameEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	createdAt := time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\) FROM ops_error_logs e`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	listRows := sqlmock.NewRows([]string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "status_code", "client_status_code", "platform", "model",
		"resolved", "resolved_at", "resolved_by_user_id", "resolved_by_user_email", "client_request_id", "request_id", "error_message",
		"user_id", "user_email", "api_key_id", "account_id", "account_name", "group_id", "group_name", "client_ip", "request_path", "stream",
		"inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model", "request_type",
		"upstream_status_code", "error_body", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms",
	}).AddRow(
		int64(9), createdAt, "upstream", "api_error", "provider", "upstream_http", "error", 500, 500, "openai", "gpt-5.5",
		false, nil, nil, "", "client-req-9", "req-9", "upstream failed",
		int64(6), "user@example.com", int64(10), int64(88), "上游账号A", int64(3), "VIP", "127.0.0.1", "/v1/responses", false,
		"/v1/responses", "/v1/responses", "gpt-5.5", "gpt-5.5", nil,
		500, "", "insufficient balance", "", "", false, nil, nil, nil, nil, nil,
	)
	mock.ExpectQuery(`(?s)SELECT\s+e\.id,.*e\.upstream_error_message.*FROM ops_error_logs e`).
		WithArgs(20, 0).
		WillReturnRows(listRows)

	list, err := repo.ListErrorLogs(context.Background(), &service.OpsErrorLogFilter{Page: 1, PageSize: 20, View: "all"})
	require.NoError(t, err)
	require.Len(t, list.Errors, 1)
	require.Equal(t, service.OpsErrorCategoryBalance, list.Errors[0].ErrorCategory)
	require.Equal(t, "upstream_balance_error", list.Errors[0].ErrorSubcategory)

	detailRows := sqlmock.NewRows([]string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "status_code", "client_status_code", "platform", "model",
		"resolved", "resolved_at", "resolved_by_user_id", "client_request_id", "request_id", "error_message", "error_body",
		"upstream_status_code", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"user_id", "user_email", "api_key_id", "account_id", "account_name", "group_id", "group_name", "client_ip",
		"request_path", "stream", "inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model", "request_type", "user_agent",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms",
	}).AddRow(
		int64(9), createdAt, "upstream", "api_error", "provider", "upstream_http", "error", 500, 500, "openai", "gpt-5.5",
		false, nil, nil, "client-req-9", "req-9", "upstream failed", "",
		500, "insufficient balance", "", "", false,
		int64(6), "user@example.com", int64(10), int64(88), "上游账号A", int64(3), "VIP", "127.0.0.1",
		"/v1/responses", false, "/v1/responses", "/v1/responses", "gpt-5.5", "gpt-5.5", nil, "codex",
		nil, nil, nil, nil, nil,
	)
	mock.ExpectQuery(`(?s)SELECT\s+e\.id,.*e\.upstream_error_message.*WHERE e\.id = \$1`).
		WithArgs(int64(9)).
		WillReturnRows(detailRows)

	detail, err := repo.GetErrorLogByID(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, list.Errors[0].ErrorCategory, detail.ErrorCategory)
	require.Equal(t, list.Errors[0].ErrorSubcategory, detail.ErrorSubcategory)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUnifiedErrorsFiltersSortsAndCountsSameKind(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	userID := int64(6)

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "client_status_code", "status_code", "platform", "model",
		"client_request_id", "request_id", "error_message", "error_body", "upstream_status_code", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"user_id", "user_email", "api_key_id", "api_key_name", "account_id", "account_name", "group_id", "group_name",
		"request_path", "inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms", "ai_analysis_status", "same_kind_count", "total_count",
	}).AddRow(
		int64(1), start.Add(2*time.Minute), "upstream", "api_error", "provider", "upstream_http", "P1", 500, 500, "openai", "gpt-5.5",
		"client-1", "req-1", "upstream failed", "", 500, "insufficient balance", "", "", false,
		userID, "user@example.com", int64(12), "prod-key", int64(88), "Op01", int64(3), "VIP",
		"/v1/responses", "/v1/responses", "/v1/responses", "gpt-5.5", "gpt-5.5",
		nil, nil, nil, nil, nil, "completed", 2, 2,
	).AddRow(
		int64(2), start.Add(time.Minute), "upstream", "api_error", "provider", "upstream_http", "P1", 500, 500, "openai", "gpt-5.5",
		"client-2", "req-2", "upstream failed again", "", 500, "insufficient balance", "", "", false,
		userID, "user@example.com", int64(12), "prod-key", int64(88), "Op01", int64(3), "VIP",
		"/v1/responses", "/v1/responses", "/v1/responses", "gpt-5.5", "gpt-5.5",
		nil, nil, nil, nil, nil, "completed", 2, 2,
	)

	mock.ExpectQuery(`(?s)WITH base AS .*ops_ai_analysis_tasks.*FROM ops_error_logs e.*COALESCE\(e\.user_id, ak\.user_id\) = \$3.*COALESCE\(e\.upstream_status_code, e\.status_code, 0\) = ANY\(\$5\).*error_category = ANY\(\$6\).*ORDER BY same_kind_count DESC.*LIMIT \$7 OFFSET \$8`).
		WithArgs(start, end, userID, "openai", sqlmock.AnyArg(), sqlmock.AnyArg(), 20, 0).
		WillReturnRows(rows)

	got, err := repo.ListUnifiedErrors(context.Background(), &service.OpsUnifiedErrorListFilter{
		StartTime:       &start,
		EndTime:         &end,
		UserID:          &userID,
		Platform:        "openai",
		StatusCodes:     []int{400, 500},
		ErrorCategories: []string{service.OpsErrorCategoryBalance},
		AIAnalysis:      service.OpsUnifiedAIAnalysisAnalyzed,
		SortBy:          "same_kind_count",
		SortOrder:       "desc",
		Page:            1,
		PageSize:        20,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, 2, got.Total)
	require.Len(t, got.Items, 2)
	for _, item := range got.Items {
		require.Equal(t, service.OpsErrorCategoryBalance, item.ErrorCategory)
		require.Equal(t, "upstream_balance_error", item.ErrorSubcategory)
		require.Nil(t, item.ClientErrorSubcategory)
		require.Equal(t, service.OpsUnifiedErrorResultFinalFailed, item.ErrorResult)
		require.Equal(t, 2, item.SameKindCount)
		require.Equal(t, "completed", item.AIAnalysisStatus)
		require.NotNil(t, item.User)
		require.NotNil(t, item.APIKey)
		require.NotNil(t, item.UpstreamAccount)
	}
}

func TestListUnifiedErrorsClassifiesRecoveredByClientFinalStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "client_status_code", "status_code", "platform", "model",
		"client_request_id", "request_id", "error_message", "error_body", "upstream_status_code", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"user_id", "user_email", "api_key_id", "api_key_name", "account_id", "account_name", "group_id", "group_name",
		"request_path", "inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms", "ai_analysis_status", "same_kind_count", "total_count",
	}).AddRow(
		int64(11), start.Add(time.Minute), "upstream", "rate_limit_error", "provider", "upstream_http", "P1", 200, 429, "openai", "gpt-5.5",
		"client-11", "req-11", "upstream recovered after retry", "", 429, "rate limit", "", "", false,
		nil, "", nil, "", int64(88), "Op01", nil, "",
		"/v1/responses", "/v1/responses", "/v1/responses", "gpt-5.5", "gpt-5.5",
		nil, nil, nil, nil, nil, "not_analyzed", 1, 1,
	)
	mock.ExpectQuery(`(?s)WITH base AS .*COALESCE\(e\.status_code, 0\) AS client_status_code.*COALESCE\(e\.upstream_status_code, e\.status_code, 0\) AS effective_status_code.*FROM ops_error_logs e.*error_result = ANY\(\$3\).*LIMIT \$4 OFFSET \$5`).
		WithArgs(start, end, sqlmock.AnyArg(), 20, 0).
		WillReturnRows(rows)

	got, err := repo.ListUnifiedErrors(context.Background(), &service.OpsUnifiedErrorListFilter{
		StartTime:    &start,
		EndTime:      &end,
		ErrorResults: []string{service.OpsUnifiedErrorResultRecovered},
		Page:         1,
		PageSize:     20,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, 1, got.Total)
	require.Len(t, got.Items, 1)
	require.Equal(t, 429, got.Items[0].StatusCode)
	require.Equal(t, service.OpsUnifiedErrorResultRecovered, got.Items[0].ErrorResult)
}

func TestListUnifiedErrorsOutOfRangePageKeepsTotal(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	cols := []string{
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "client_status_code", "status_code", "platform", "model",
		"client_request_id", "request_id", "error_message", "error_body", "upstream_status_code", "upstream_error_message", "upstream_error_detail", "upstream_errors", "is_business_limited",
		"user_id", "user_email", "api_key_id", "api_key_name", "account_id", "account_name", "group_id", "group_name",
		"request_path", "inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model",
		"auth_latency_ms", "routing_latency_ms", "upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms", "ai_analysis_status", "same_kind_count", "total_count",
	}
	mock.ExpectQuery(`(?s)WITH base AS .*LIMIT \$3 OFFSET \$4`).
		WithArgs(start, end, 20, 19960).
		WillReturnRows(sqlmock.NewRows(cols))

	firstPageRows := sqlmock.NewRows(cols).AddRow(
		int64(21), start.Add(time.Minute), "upstream", "api_error", "provider", "upstream_http", "P1", 500, 500, "openai", "gpt-5.5",
		"client-21", "req-21", "upstream failed", "", 500, "service unavailable", "", "", false,
		nil, "", nil, "", int64(88), "Op01", nil, "",
		"/v1/responses", "/v1/responses", "/v1/responses", "gpt-5.5", "gpt-5.5",
		nil, nil, nil, nil, nil, "not_analyzed", 1, 3,
	)
	mock.ExpectQuery(`(?s)WITH base AS .*LIMIT \$3 OFFSET \$4`).
		WithArgs(start, end, 20, 0).
		WillReturnRows(firstPageRows)

	got, err := repo.ListUnifiedErrors(context.Background(), &service.OpsUnifiedErrorListFilter{
		StartTime: &start,
		EndTime:   &end,
		Page:      999,
		PageSize:  20,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, got.Items)
	require.Equal(t, 3, got.Total)
	require.Equal(t, 999, got.Page)
}
