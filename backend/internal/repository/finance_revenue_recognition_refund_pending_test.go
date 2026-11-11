//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// V03-AC13：处理中的订阅退款订单（REFUND_PENDING，refund_amount 已写、refund_at 为空）仍参与当日订阅收入确认，
// 确认额与同金额的 COMPLETED 订单一致，pending 的退款额在 refund_at 写入前不影响确认额。
func TestFinanceRevenueRecognitionIncludesRefundPendingOrders(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	// 远期日期，避免和其他测试的订单落在同一天。
	paidAt := time.Date(2047, 3, 1, 8, 0, 0, 0, time.UTC)
	recognitionDate := time.Date(2047, 3, 5, 0, 0, 0, 0, time.UTC)

	var userID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, "revenue-pending-"+suffix+"@example.test").Scan(&userID))

	insertOrder := func(status string, refundAmount float64) int64 {
		var id int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO payment_orders(
 user_id,amount,pay_amount,order_type,subscription_days,status,expires_at,paid_at,completed_at,refund_amount,refund_at
) VALUES($1,30,30,'subscription',30,$2,$3,$4,$4,$5,NULL) RETURNING id`,
			userID, status, paidAt.AddDate(0, 2, 0), paidAt, refundAmount).Scan(&id))
		return id
	}
	pendingID := insertOrder(service.OrderStatusRefundPending, 30)
	completedID := insertOrder(service.OrderStatusCompleted, 0)
	ids := pq.Array([]int64{pendingID, completedID})
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM usage_revenue_allocations WHERE source_type='subscription_recognition' AND source_id IN (SELECT id FROM subscription_revenue_recognitions WHERE payment_order_id = ANY($1))`, ids)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM subscription_revenue_recognitions WHERE payment_order_id = ANY($1)`, ids)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM payment_orders WHERE id = ANY($1)`, ids)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM users WHERE id=$1`, userID)
	})

	repo := NewFinanceRevenueRecognitionRepository(integrationDB)
	orders, err := repo.ListSubscriptionOrdersForDate(ctx, recognitionDate, "UTC")
	require.NoError(t, err)
	listed := map[int64]service.FinanceSubscriptionOrder{}
	for _, o := range orders {
		listed[o.OrderID] = o
	}
	pending, ok := listed[pendingID]
	require.True(t, ok, "REFUND_PENDING subscription order must stay in revenue recognition")
	require.Nil(t, pending.RefundDate)
	_, ok = listed[completedID]
	require.True(t, ok)

	svc := service.NewFinanceRevenueRecognitionService(repo, nil)
	_, err = svc.RecognizeDate(ctx, recognitionDate, "UTC")
	require.NoError(t, err)

	readRecognition := func(orderID int64) (string, string) {
		var recognized, reduction string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT recognized_revenue::text,refund_reduction::text
FROM subscription_revenue_recognitions WHERE payment_order_id=$1 AND recognition_date=$2::date`,
			orderID, recognitionDate.Format("2006-01-02")).Scan(&recognized, &reduction))
		return recognized, reduction
	}
	pendingRecognized, pendingReduction := readRecognition(pendingID)
	completedRecognized, completedReduction := readRecognition(completedID)
	require.Equal(t, "1.0000000000", completedRecognized)
	require.Equal(t, completedRecognized, pendingRecognized)
	require.Equal(t, completedReduction, pendingReduction)
	require.Equal(t, "0.0000000000", pendingReduction)
}
