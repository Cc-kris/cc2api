package repository

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFinanceReportWhereAlwaysExcludesAdminUsage(t *testing.T) {
	userID := int64(42)
	filter := service.FinanceReportFilter{
		StartAt:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		EndBefore: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
		UserID:    &userID,
	}

	where, args := financeReportWhere(filter, "ufr")

	require.Contains(t, where, "COALESCE(ufr.business_type,'') <> $3")
	require.Contains(t, where, "finance_admin.id=ufr.user_id AND finance_admin.role=$3")
	require.Contains(t, where, "ufr.user_id=$4")
	require.Len(t, args, 4)
	require.Equal(t, service.RoleAdmin, args[2])
	require.Equal(t, userID, args[3])
}

func TestGetFinanceFundsExcludesAdminFromCustomerAndWalletAmounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	repo := NewFinanceReportRepository(db)

	mock.ExpectQuery(`(?s)FROM usage_finance_records ufr.*finance_admin\.role=\$1.*ORDER BY l\.name,l\.wallet_id`).
		WithArgs(service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"wallet_id", "name", "balance", "currency", "collected_at", "sync_status", "cost", "scope", "included", "stale"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM payment_orders po
WHERE NOT EXISTS (SELECT 1 FROM users finance_admin WHERE finance_admin.id=po.user_id AND finance_admin.role=$3)`)).
		WithArgs(start, end, service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"payment", "refund"}).AddRow("0", "0"))
	mock.ExpectQuery(`(?s)FROM payment_provider_fee_events fee.*finance_admin\.role=\$3`).
		WithArgs(start, end, service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"fees"}).AddRow("0"))
	mock.ExpectQuery(`(?s)FROM upstream_fund_events WHERE occurred_at >= \$1 AND occurred_at < \$2`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"topup", "refund", "adjustment", "bonus", "topup_count", "event_count"}).AddRow("0", "0", "0", "0", int64(0), int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(SUM(balance),0)::text FROM users WHERE role<>$1 AND deleted_at IS NULL`)).
		WithArgs(service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("0"))
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\)::bigint FROM latest WHERE collected_at < NOW\(\)-INTERVAL '20 minutes'`).
		WillReturnRows(sqlmock.NewRows([]string{"stale_count"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)::bigint FROM upstream_wallets WHERE deleted_at IS NULL AND (pricing_sync_status='failed' OR balance_sync_status='failed' OR quota_sync_status='failed')`)).
		WillReturnRows(sqlmock.NewRows([]string{"failed_count"}).AddRow(int64(0)))

	result, err := repo.GetFinanceFunds(t.Context(), service.FinanceReportFilter{StartAt: start, EndBefore: end})
	require.NoError(t, err)
	require.True(t, result.CustomerBalance.IsZero())
	require.NoError(t, mock.ExpectationsWereMet())
}
