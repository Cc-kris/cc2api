package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepository_GetDashboardRevenueOverview(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := newUsageLogRepositoryWithSQL(nil, db)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT COUNT(*)::bigint AS non_admin_user_count,
       COALESCE(SUM(GREATEST(balance, 0)), 0)::float8 AS unused_amount
FROM users
WHERE role <> $1`)).
		WithArgs(service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"non_admin_user_count", "unused_amount"}).AddRow(int64(4), 80.25))

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT COALESCE(SUM(rc.value), 0)::float8 AS total_credit_amount,
       COUNT(DISTINCT rc.used_by)::bigint AS credited_user_count
FROM redeem_codes rc
JOIN users u ON u.id = rc.used_by
WHERE u.role <> $1
  AND rc.status = $2
  AND rc.value > 0
  AND rc.type IN ($3, $4)
  AND rc.used_by IS NOT NULL`)).
		WithArgs(service.RoleAdmin, service.StatusUsed, service.RedeemTypeBalance, service.AdjustmentTypeAdminBalance).
		WillReturnRows(sqlmock.NewRows([]string{"total_credit_amount", "credited_user_count"}).AddRow(125.50, int64(2)))

	got, err := repo.GetDashboardRevenueOverview(context.Background())
	require.NoError(t, err)
	require.Equal(t, "125.50", got.TotalCreditAmount)
	require.Equal(t, "45.25", got.UsedAmount)
	require.Equal(t, "80.25", got.UnusedAmount)
	require.Equal(t, int64(4), got.NonAdminUserCount)
	require.Equal(t, int64(2), got.CreditedUserCount)
	require.True(t, got.IsEstimated)
	require.NotEmpty(t, got.UpdatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepository_GetDashboardRevenueOverview_ClampsNegativeUsedAmount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := newUsageLogRepositoryWithSQL(nil, db)
	mock.ExpectQuery("FROM users").
		WithArgs(service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"non_admin_user_count", "unused_amount"}).AddRow(int64(1), 200.00))
	mock.ExpectQuery("FROM redeem_codes rc").
		WithArgs(service.RoleAdmin, service.StatusUsed, service.RedeemTypeBalance, service.AdjustmentTypeAdminBalance).
		WillReturnRows(sqlmock.NewRows([]string{"total_credit_amount", "credited_user_count"}).AddRow(50.00, int64(1)))

	got, err := repo.GetDashboardRevenueOverview(context.Background())
	require.NoError(t, err)
	require.Equal(t, "0.00", got.UsedAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepository_GetDashboardRepurchaseDistribution(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := newUsageLogRepositoryWithSQL(nil, db)
	mock.ExpectQuery("WITH non_admin_users").
		WithArgs(service.RoleAdmin, service.StatusUsed, service.RedeemTypeBalance, service.AdjustmentTypeAdminBalance).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_users",
			"zero_count",
			"one_count",
			"two_count",
			"three_count",
			"three_plus_count",
		}).AddRow(int64(10), int64(4), int64(3), int64(2), int64(1), int64(0)))

	got, err := repo.GetDashboardRepurchaseDistribution(context.Background())
	require.NoError(t, err)
	require.Len(t, got.Buckets, 5)
	require.Equal(t, service.DashboardRepurchaseBucket{Bucket: "zero", Label: "零购", UserCount: 4, Ratio: 40}, got.Buckets[0])
	require.Equal(t, service.DashboardRepurchaseBucket{Bucket: "one", Label: "一购", UserCount: 3, Ratio: 30}, got.Buckets[1])
	require.Equal(t, service.DashboardRepurchaseBucket{Bucket: "two", Label: "二购", UserCount: 2, Ratio: 20}, got.Buckets[2])
	require.Equal(t, service.DashboardRepurchaseBucket{Bucket: "three", Label: "三购", UserCount: 1, Ratio: 10}, got.Buckets[3])
	require.Equal(t, service.DashboardRepurchaseBucket{Bucket: "three_plus", Label: "三购以上", UserCount: 0, Ratio: 0}, got.Buckets[4])
	require.NotEmpty(t, got.UpdatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepository_GetDashboardRepurchaseDistribution_ZeroUsers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := newUsageLogRepositoryWithSQL(nil, db)
	mock.ExpectQuery("WITH non_admin_users").
		WithArgs(service.RoleAdmin, service.StatusUsed, service.RedeemTypeBalance, service.AdjustmentTypeAdminBalance).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_users",
			"zero_count",
			"one_count",
			"two_count",
			"three_count",
			"three_plus_count",
		}).AddRow(int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)))

	got, err := repo.GetDashboardRepurchaseDistribution(context.Background())
	require.NoError(t, err)
	for _, bucket := range got.Buckets {
		require.Zero(t, bucket.UserCount)
		require.Zero(t, bucket.Ratio)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
