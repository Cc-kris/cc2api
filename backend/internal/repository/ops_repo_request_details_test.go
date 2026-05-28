package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListRequestDetails_ReturnsAccountName(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 5, 22, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	createdAt := start.Add(5 * time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1) FROM combined")).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"kind", "created_at", "request_id", "platform", "model", "requested_model", "upstream_model", "duration_ms", "status_code", "error_id",
		"phase", "severity", "message", "user_id", "user_email", "api_key_id", "account_id", "account_name", "group_id", "group_name", "stream",
	}).AddRow(
		"success", createdAt, "req-1", "openai", "gpt-5.4", "gpt-5.4", "gpt-5.4-upstream", 123, nil, nil,
		nil, nil, nil, int64(11), "user@example.com", int64(22), int64(42), "上游账号", int64(33), "默认分组", false,
	)
	mock.ExpectQuery("(?s)SELECT\\s+kind,.*account_name.*FROM combined").
		WithArgs(start, end, 10, 0).
		WillReturnRows(rows)

	got, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Page:      1,
		PageSize:  10,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, int64(1), total)
	require.Len(t, got, 1)
	require.Equal(t, "user@example.com", got[0].UserEmail)
	require.Equal(t, "上游账号", got[0].AccountName)
	require.Equal(t, "默认分组", got[0].GroupName)
	require.Equal(t, "gpt-5.4", got[0].RequestedModel)
	require.Equal(t, "gpt-5.4-upstream", got[0].UpstreamModel)
	require.NotNil(t, got[0].AccountID)
	require.Equal(t, int64(42), *got[0].AccountID)
}
