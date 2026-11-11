//go:build integration

package repository

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type refundIntegrationFixture struct {
	userID  int64
	orderID int64
	instID  int64
}

// createRefundIntegrationFixture 建一个余额充值订单；tradeNo 为空时退款跳过网关（REFUND_NO_TRADE_NO）。
func createRefundIntegrationFixture(t *testing.T, ctx context.Context, balance, amount float64, tradeNo string) refundIntegrationFixture {
	t.Helper()
	client := testEntClient(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	user, err := client.User.Create().
		SetEmail("refund-it-" + suffix + "@example.test").
		SetPasswordHash("hash").
		SetUsername("refund-it-" + suffix).
		SetBalance(balance).
		Save(ctx)
	require.NoError(t, err)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-refund-it-" + suffix).
		SetConfig("{}").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(amount).
		SetPayAmount(amount).
		SetFeeRate(0).
		SetRechargeCode("REFUND-IT-" + suffix).
		SetOutTradeNo("refund_it_" + suffix).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo(tradeNo).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeStripe).
		Save(ctx)
	require.NoError(t, err)

	fx := refundIntegrationFixture{userID: user.ID, orderID: order.ID, instID: inst.ID}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM payment_audit_logs WHERE order_id = $1`, strconv.FormatInt(fx.orderID, 10))
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM payment_orders WHERE id = $1`, fx.orderID)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM payment_provider_instances WHERE id = $1`, fx.instID)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM users WHERE id = $1`, fx.userID)
	})
	return fx
}

func newRefundIntegrationPaymentService(t *testing.T) *service.PaymentService {
	t.Helper()
	client := testEntClient(t)
	userRepo := newUserRepositoryWithSQL(client, integrationDB)
	return service.NewPaymentService(client, payment.NewRegistry(), nil, nil, nil, nil, userRepo, nil, nil)
}

func userBalanceForRefundTest(t *testing.T, ctx context.Context, userID int64) float64 {
	t.Helper()
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = $1`, userID).Scan(&balance))
	return balance
}

// V03-AC12：余额 150、订单 100，两个管理员并发退款。两边都通过 PrepareRefund 后同时执行，
// 只有一个能认领订单；另一个返回 CONFLICT，余额只扣一次，是 50 而不是 -50。
func TestExecuteRefund_ConcurrentOnlyOneWins(t *testing.T) {
	ctx := context.Background()
	fx := createRefundIntegrationFixture(t, ctx, 150, 100, "")
	svc := newRefundIntegrationPaymentService(t)

	const admins = 2
	plans := make([]*service.RefundPlan, admins)
	for i := range plans {
		plan, early, err := svc.PrepareRefund(ctx, fx.orderID, 100, fmt.Sprintf("admin-%d", i), false, true)
		require.NoError(t, err)
		require.Nil(t, early)
		require.InDelta(t, 100.0, plan.BalanceToDeduct, 1e-9)
		plans[i] = plan
	}

	type outcome struct {
		result *service.RefundResult
		err    error
	}
	outcomes := make([]outcome, admins)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range plans {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			res, err := svc.ExecuteRefund(context.Background(), plans[i])
			outcomes[i] = outcome{result: res, err: err}
		}(i)
	}
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, o := range outcomes {
		switch {
		case o.err == nil && o.result != nil && o.result.Success:
			successes++
			require.InDelta(t, 100.0, o.result.BalanceDeducted, 1e-9)
		case o.err != nil && infraerrors.Reason(o.err) == "CONFLICT":
			conflicts++
			require.Contains(t, o.err.Error(), "order status changed")
		default:
			t.Fatalf("unexpected refund outcome: result=%+v err=%v", o.result, o.err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	require.InDelta(t, 50.0, userBalanceForRefundTest(t, ctx, fx.userID), 1e-9)

	order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusRefunded, order.Status)
	successAudits, err := testEntClient(t).PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(fx.orderID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, successAudits)
}

// V03-AC07/AC08/AC09 端到端（真实 Postgres + 真实 userRepo）。
func TestRefundBalanceDeduction_RequireForceAndForcedClamp(t *testing.T) {
	ctx := context.Background()

	t.Run("insufficient balance requires force and changes nothing", func(t *testing.T) {
		fx := createRefundIntegrationFixture(t, ctx, 30, 100, "")
		svc := newRefundIntegrationPaymentService(t)

		plan, early, err := svc.PrepareRefund(ctx, fx.orderID, 100, "", false, true)
		require.NoError(t, err)
		require.Nil(t, plan)
		require.NotNil(t, early)
		require.False(t, early.Success)
		require.True(t, early.RequireForce)
		require.InDelta(t, 30.0, userBalanceForRefundTest(t, ctx, fx.userID), 1e-9)
		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusCompleted, order.Status)
	})

	t.Run("forced refund deducts only available balance", func(t *testing.T) {
		fx := createRefundIntegrationFixture(t, ctx, 30, 100, "")
		svc := newRefundIntegrationPaymentService(t)

		plan, early, err := svc.PrepareRefund(ctx, fx.orderID, 100, "", true, true)
		require.NoError(t, err)
		require.Nil(t, early)
		require.InDelta(t, 100.0, plan.GatewayAmount, 1e-9)
		result, err := svc.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		require.True(t, result.Success)
		require.InDelta(t, 30.0, result.BalanceDeducted, 1e-9)
		require.InDelta(t, 0.0, userBalanceForRefundTest(t, ctx, fx.userID), 1e-9)

		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefunded, order.Status)
		require.True(t, order.ForceRefund)
		audit, err := testEntClient(t).PaymentAuditLog.Query().
			Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(fx.orderID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
			Only(ctx)
		require.NoError(t, err)
		require.Contains(t, audit.Detail, `"balanceDeducted":30`)
		require.Contains(t, audit.Detail, `"force":true`)
	})

	t.Run("enough balance deducts full refund", func(t *testing.T) {
		fx := createRefundIntegrationFixture(t, ctx, 150, 100, "")
		svc := newRefundIntegrationPaymentService(t)

		plan, early, err := svc.PrepareRefund(ctx, fx.orderID, 100, "", false, true)
		require.NoError(t, err)
		require.Nil(t, early)
		result, err := svc.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		require.True(t, result.Success)
		require.InDelta(t, 50.0, userBalanceForRefundTest(t, ctx, fx.userID), 1e-9)
		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefunded, order.Status)
	})
}
