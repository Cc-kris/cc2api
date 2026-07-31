package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceSyncRepositorySchedulesAccountUsageOnlyForPublishedCapableProtocols(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &upstreamFinanceSyncRepository{db: db}
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT wallet_id,sync_type.*p.status='published'.*v.published_at IS NOT NULL.*v.validation_status='valid'.*config->'capabilities' \? 'account_usage'.*config->'operations' \? 'account_usage'.*a.status='active'.*r.sync_type='account_usage'`).
		WithArgs(now).
		WillReturnRows(sqlmock.NewRows([]string{"wallet_id", "sync_type"}).AddRow(int64(11), service.UpstreamFinanceSyncAccountUsage))

	requests, err := repo.ListDueSyncRequests(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, []service.UpstreamFinanceSyncRequest{{WalletID: 11, SyncType: service.UpstreamFinanceSyncAccountUsage}}, requests)
	require.NoError(t, mock.ExpectationsWereMet())
}
