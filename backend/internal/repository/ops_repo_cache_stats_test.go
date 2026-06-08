package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryListCacheStatsRowsFiltersAndScans(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	apiKeyID := int64(12)
	groupID := int64(34)

	rows := sqlmock.NewRows([]string{
		"platform", "model", "total_requests", "candidate_requests", "hit_requests", "bypass_requests", "store_success", "store_skip",
		"input_tokens", "output_tokens", "hit_tokens", "candidate_tokens", "all_request_tokens", "bypass_reasons", "store_skip_reasons", "estimated_saved_amount",
	}).AddRow("anthropic", "claude-sonnet-4-5", int64(10), int64(8), int64(4), int64(2), int64(3), int64(1), int64(100), int64(50), int64(60), int64(120), int64(150), `{"disabled":2}`, `{"response_too_large":1}`, "1.50000000")

	mock.ExpectQuery(`FROM ops_cache_minute_stats`).
		WithArgs(start, end, "anthropic", "claude-sonnet-4-5", apiKeyID, groupID).
		WillReturnRows(rows)

	got, err := repo.ListCacheStatsRows(context.Background(), &service.CacheStatsFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  "claude",
		Model:     "claude-sonnet-4-5",
		APIKeyID:  &apiKeyID,
		GroupID:   &groupID,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "anthropic", got[0].Platform)
	require.Equal(t, int64(4), got[0].HitRequests)
	require.Equal(t, `{"disabled":2}`, got[0].BypassReasonsJSON)
	require.NoError(t, mock.ExpectationsWereMet())
}
