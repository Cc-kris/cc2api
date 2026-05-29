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
		"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "status_code", "platform", "model",
		"resolved", "resolved_at", "resolved_by_user_id", "resolved_by_user_email", "client_request_id", "request_id", "error_message",
		"user_id", "user_email", "api_key_id", "account_id", "account_name", "group_id", "group_name", "client_ip", "request_path", "stream",
		"inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model", "request_type",
	}).AddRow(
		int64(7), createdAt, "upstream", "upstream_http_error", "provider", "upstream_http", "error", 500, "openai", "gpt-5.4-upstream",
		false, nil, nil, "", "client-req-1", "req-1", "provider failed",
		int64(11), "user@example.com", int64(22), int64(42), "上游账号A", int64(33), "默认分组", "127.0.0.1", "/v1/chat/completions", true,
		"/v1/chat/completions", "/v1/responses", "gpt-5.4", "gpt-5.4-upstream", int64(2),
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
	require.NotNil(t, item.AccountID)
	require.Equal(t, int64(42), *item.AccountID)
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
