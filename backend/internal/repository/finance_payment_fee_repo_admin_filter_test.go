package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListPaymentFeesRequiresMatchedNonAdminOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewFinancePaymentFeeRepository(db)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery(`(?s)FROM payment_provider_fee_events fee JOIN payment_orders po ON po\.id=fee\.payment_order_id.*finance_admin\.role=\$3.*FROM payment_orders po.*LIMIT \$4 OFFSET \$5`).
		WithArgs(start, end, service.RoleAdmin, 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "payment_order_id", "order_no", "provider", "bill_event_id", "gross_amount", "fee_amount", "net_amount",
			"currency", "fx_rate_to_usd", "fee_usd_amount", "status", "occurred_at", "total",
		}))

	items, total, err := repo.ListPaymentFees(t.Context(), service.FinanceReportFilter{StartAt: start, EndBefore: end}, service.FinancePaymentFeeListRequest{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
