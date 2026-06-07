package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildIncidentImpactWhereIncludesModelAndSLA(t *testing.T) {
	start := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	groupID := int64(7)
	where, args := buildIncidentImpactWhere(&service.OpsDashboardFilter{
		Platform: " OpenAI ",
		Model:    "gpt-5.5",
		GroupID:  &groupID,
	}, start, end, 1)

	require.Len(t, args, 5)
	require.Contains(t, where, "e.created_at >= $1")
	require.Contains(t, where, "e.created_at < $2")
	require.Contains(t, where, "COALESCE(e.is_count_tokens,false) = false")
	require.Contains(t, where, "COALESCE(e.status_code, 0) >= 400")
	require.Contains(t, where, "e.group_id = $3")
	require.Contains(t, where, "e.platform = $4")
	require.Contains(t, where, "COALESCE(e.model,'') = $5")
	if strings.Contains(where, " status_code") && !strings.Contains(where, "e.status_code") {
		t.Fatalf("where should qualify status_code: %s", where)
	}
}

func TestOpsRepositoryGetIncidentImpact(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	groupID := int64(9)
	filter := &service.OpsDashboardFilter{StartTime: start, EndTime: end, Platform: "openai", Model: "gpt-5.5", GroupID: &groupID}

	mock.ExpectQuery(`(?s)SELECT\s+COUNT\(DISTINCT COALESCE\(e\.user_id, ak\.user_id\)\).*FROM ops_error_logs e\s+LEFT JOIN api_keys ak ON e\.api_key_id = ak\.id`).
		WithArgs(start, end, groupID, "openai", "gpt-5.5").WillReturnRows(sqlmock.NewRows([]string{"users", "api_keys"}).AddRow(int64(2), int64(3)))
	mock.ExpectQuery(`(?s)SELECT DISTINCT COALESCE\(e\.model, ''\) AS model\s+FROM ops_error_logs e`).
		WithArgs(start, end, groupID, "openai", "gpt-5.5").WillReturnRows(sqlmock.NewRows([]string{"model"}).AddRow("gpt-5.5").AddRow("gpt-4.1"))
	mock.ExpectQuery(`(?s)SELECT DISTINCT e\.account_id, COALESCE\(a\.name, ''\) AS account_name\s+FROM ops_error_logs e\s+LEFT JOIN accounts a ON e\.account_id = a\.id`).
		WithArgs(start, end, groupID, "openai", "gpt-5.5").WillReturnRows(sqlmock.NewRows([]string{"account_id", "account_name"}).AddRow(int64(11), "upstream-main"))

	got, err := repo.GetIncidentImpact(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.AffectedUsers)
	require.Equal(t, int64(3), got.AffectedAPIKeys)
	require.Equal(t, []string{"gpt-5.5", "gpt-4.1"}, got.AffectedModels)
	require.Len(t, got.AffectedAccounts, 1)
	require.Equal(t, int64(11), got.AffectedAccounts[0].ID)
	require.Equal(t, "upstream-main", got.AffectedAccounts[0].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}
