//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// V03-AC10 后半：REFUND_PENDING 订单的查询收尾，走真实 QueryAndFinalizeRefund、真实 userRepo
// 和 Postgres 事务。假网关退款返回 pending，查询结果由用例控制。

// pendingFinalizeGateway 实现 payment.RefundQueryProvider；并发用例下两个管理员共用，计数用原子操作。
type pendingFinalizeGateway struct {
	queryStatus string
	queryErr    error
	refundCalls atomic.Int32
	queryCalls  atomic.Int32
	// queryBarrier 非空时，每次 QueryRefund 先到达屏障再返回，用来让并发查询都读到 REFUND_PENDING。
	queryBarrier *sync.WaitGroup
}

func (g *pendingFinalizeGateway) Name() string        { return "pending-finalize-it" }
func (g *pendingFinalizeGateway) ProviderKey() string { return payment.TypeStripe }
func (g *pendingFinalizeGateway) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}
func (g *pendingFinalizeGateway) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, errors.New("not used")
}
func (g *pendingFinalizeGateway) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, errors.New("not used")
}
func (g *pendingFinalizeGateway) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, errors.New("not used")
}
func (g *pendingFinalizeGateway) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	g.refundCalls.Add(1)
	return &payment.RefundResponse{RefundID: "re_it_pending", Status: payment.ProviderStatusPending}, nil
}
func (g *pendingFinalizeGateway) QueryRefund(context.Context, payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	g.queryCalls.Add(1)
	if g.queryBarrier != nil {
		g.queryBarrier.Done()
		g.queryBarrier.Wait()
	}
	if g.queryErr != nil {
		return nil, g.queryErr
	}
	return &payment.RefundResponse{RefundID: "re_it_pending", Status: g.queryStatus}, nil
}

var _ payment.RefundQueryProvider = (*pendingFinalizeGateway)(nil)

// pendingFinalizeLoadBalancer 只为 createProviderFromInstance 提供空配置。
type pendingFinalizeLoadBalancer struct{}

func (pendingFinalizeLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return map[string]string{}, nil
}
func (pendingFinalizeLoadBalancer) SelectInstance(context.Context, string, payment.PaymentType, payment.Strategy, float64) (*payment.InstanceSelection, error) {
	return nil, errors.New("not used")
}

func newPendingFinalizePaymentService(t *testing.T, gw *pendingFinalizeGateway) *service.PaymentService {
	t.Helper()
	restore := service.SetPaymentProviderFactoryForTest(func(string, string, map[string]string) (payment.Provider, error) {
		return gw, nil
	})
	t.Cleanup(restore)
	client := testEntClient(t)
	userRepo := newUserRepositoryWithSQL(client, integrationDB)
	return service.NewPaymentService(client, payment.NewRegistry(), pendingFinalizeLoadBalancer{}, nil, nil, nil, userRepo, nil, nil)
}

// enterRefundPending 余额 150、订单 100：管理员退款，网关返回 pending。
func enterRefundPending(t *testing.T, ctx context.Context, svc *service.PaymentService, gw *pendingFinalizeGateway) refundIntegrationFixture {
	t.Helper()
	fx := createRefundIntegrationFixture(t, ctx, 150, 100, "pi_it_pending")
	plan, early, err := svc.PrepareRefund(ctx, fx.orderID, 100, "AC10", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	result, err := svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "pending confirmation")
	require.Equal(t, int32(1), gw.refundCalls.Load())

	order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusRefundPending, order.Status)
	require.Nil(t, order.RefundAt, "pending 时不写 refund_at")
	require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID), "pending 期间扣减保留")
	require.Equal(t, 1, refundAuditCount(t, ctx, fx.orderID, "REFUND_PENDING"))
	return fx
}

// userBalanceTextForRefundTest 按 decimal(20,8) 原样取余额，对齐 AC 的 8 位口径。
func userBalanceTextForRefundTest(t *testing.T, ctx context.Context, userID int64) string {
	t.Helper()
	var balance string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance::text FROM users WHERE id = $1`, userID).Scan(&balance))
	return balance
}

func refundAuditCount(t *testing.T, ctx context.Context, orderID int64, action string) int {
	t.Helper()
	n, err := testEntClient(t).PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)), paymentauditlog.ActionEQ(action)).
		Count(ctx)
	require.NoError(t, err)
	return n
}

func TestQueryAndFinalizeRefund_AC10_EndToEnd(t *testing.T) {
	ctx := context.Background()

	t.Run("gateway query succeeds: REFUNDED, refund_at written, balance unchanged", func(t *testing.T) {
		gw := &pendingFinalizeGateway{queryStatus: payment.ProviderStatusSuccess}
		svc := newPendingFinalizePaymentService(t, gw)
		fx := enterRefundPending(t, ctx, svc, gw)

		result, err := svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.NoError(t, err)
		require.True(t, result.Success)
		require.InDelta(t, 100.0, result.BalanceDeducted, 1e-9)

		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefunded, order.Status)
		require.NotNil(t, order.RefundAt)
		require.InDelta(t, 100.0, order.RefundAmount, 1e-9)
		require.Nil(t, order.FailedAt)
		require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID), "保留的扣减就是最终扣减，不能再扣一次")

		require.Equal(t, 1, refundAuditCount(t, ctx, fx.orderID, "REFUND_SUCCESS"))
		success, err := testEntClient(t).PaymentAuditLog.Query().
			Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(fx.orderID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
			Only(ctx)
		require.NoError(t, err)
		require.Contains(t, success.Detail, `"balanceDeducted":100`)
		require.Contains(t, success.Detail, `"refundAmount":100`)
		require.Zero(t, refundAuditCount(t, ctx, fx.orderID, "REFUND_FAILED"))
		require.Zero(t, refundAuditCount(t, ctx, fx.orderID, "REFUND_ROLLBACK_FAILED"))
		require.Equal(t, int32(1), gw.refundCalls.Load(), "收尾不重复调用网关退款")

		_, err = svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.Equal(t, "INVALID_STATUS", infraerrors.Reason(err))
		require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))
	})

	t.Run("gateway query fails: balance restored in one tx, order refundable again", func(t *testing.T) {
		gw := &pendingFinalizeGateway{queryStatus: payment.ProviderStatusFailed}
		svc := newPendingFinalizePaymentService(t, gw)
		fx := enterRefundPending(t, ctx, svc, gw)

		result, err := svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.NoError(t, err)
		require.False(t, result.Success)
		require.Contains(t, result.Warning, "gateway refund failed")
		require.NotContains(t, result.Warning, "rollback failed")

		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefundFailed, order.Status)
		require.Nil(t, order.RefundAt, "失败不写 refund_at")
		require.NotNil(t, order.FailedAt)
		require.NotNil(t, order.FailedReason)
		require.Equal(t, "150.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))

		require.Equal(t, 1, refundAuditCount(t, ctx, fx.orderID, "REFUND_FAILED"))
		require.Zero(t, refundAuditCount(t, ctx, fx.orderID, "REFUND_ROLLBACK_FAILED"))
		require.Zero(t, refundAuditCount(t, ctx, fx.orderID, "REFUND_SUCCESS"))

		// 回到可退款状态：重新退款会重新扣减（不是跳过扣减），且只扣一次。
		gw.queryStatus = payment.ProviderStatusSuccess
		plan, early, err := svc.PrepareRefund(ctx, fx.orderID, 100, "retry", false, true)
		require.NoError(t, err)
		require.Nil(t, early)
		require.InDelta(t, 100.0, plan.BalanceToDeduct, 1e-9)
		_, err = svc.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))
		result, err = svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))
	})

	t.Run("retry that fails again restores once more and stays refundable", func(t *testing.T) {
		gw := &pendingFinalizeGateway{queryStatus: payment.ProviderStatusFailed}
		svc := newPendingFinalizePaymentService(t, gw)
		fx := enterRefundPending(t, ctx, svc, gw)
		_, err := svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, "150.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))

		// 第二次退款：再次 pending、再次失败。迁移 131 限定每单每动作一行审计，第二条
		// REFUND_PENDING / REFUND_FAILED 不能让事务失败。
		plan, early, err := svc.PrepareRefund(ctx, fx.orderID, 100, "retry", false, true)
		require.NoError(t, err)
		require.Nil(t, early)
		result, err := svc.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		require.Contains(t, result.Warning, "pending confirmation")
		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefundPending, order.Status)
		require.Nil(t, order.FailedAt, "重新进入 pending 清掉上一次失败")
		require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))

		pending, err := testEntClient(t).PaymentAuditLog.Query().
			Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(fx.orderID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
			Only(ctx)
		require.NoError(t, err)
		require.Contains(t, pending.Detail, `"reason":"retry"`, "收尾读取的是本次尝试的保留扣减")
		require.Contains(t, pending.Detail, `"supersedes"`, "上一次尝试的审计内容保留")

		result, err = svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.NoError(t, err)
		require.False(t, result.Success)
		order, err = testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefundFailed, order.Status)
		require.Equal(t, "150.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))
		require.Equal(t, 1, refundAuditCount(t, ctx, fx.orderID, "REFUND_FAILED"))
		require.Equal(t, int32(2), gw.refundCalls.Load())
	})

	t.Run("audit insert failure rolls back the balance restore with the status change", func(t *testing.T) {
		gw := &pendingFinalizeGateway{queryStatus: payment.ProviderStatusFailed}
		svc := newPendingFinalizePaymentService(t, gw)
		fx := enterRefundPending(t, ctx, svc, gw)

		// 让同事务内最后一步（写 REFUND_FAILED 审计）失败：若余额归还不在同一事务，余额会停在 150。
		trigger := fmt.Sprintf("ac10_fail_refund_failed_audit_%d", fx.orderID)
		_, err := integrationDB.ExecContext(ctx, fmt.Sprintf(`
			CREATE FUNCTION %[1]s() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				IF NEW.order_id = '%[2]d' AND NEW.action IN ('REFUND_FAILED', 'REFUND_ROLLBACK_FAILED') THEN
					RAISE EXCEPTION 'ac10 injected audit failure';
				END IF;
				RETURN NEW;
			END $$;
			CREATE TRIGGER %[1]s BEFORE INSERT ON payment_audit_logs FOR EACH ROW EXECUTE FUNCTION %[1]s();`, trigger, fx.orderID))
		require.NoError(t, err)
		dropTrigger := func() {
			_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %[1]s ON payment_audit_logs; DROP FUNCTION IF EXISTS %[1]s();`, trigger))
		}
		t.Cleanup(dropTrigger)

		_, err = svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.Error(t, err)
		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefundPending, order.Status, "事务整体回滚，订单保持 pending")
		require.Nil(t, order.FailedAt)
		require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID), "余额归还随事务回滚")

		dropTrigger()
		result, err := svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.NoError(t, err)
		require.False(t, result.Success)
		require.Equal(t, "150.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))
		require.Equal(t, 1, refundAuditCount(t, ctx, fx.orderID, "REFUND_FAILED"))
	})

	t.Run("query transport error leaves pending untouched", func(t *testing.T) {
		gw := &pendingFinalizeGateway{queryErr: errors.New("gateway timeout")}
		svc := newPendingFinalizePaymentService(t, gw)
		fx := enterRefundPending(t, ctx, svc, gw)

		_, err := svc.QueryAndFinalizeRefund(ctx, fx.orderID)
		require.Error(t, err)
		order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefundPending, order.Status, "查不到结果不能当作失败归还余额")
		require.Equal(t, "50.00000000", userBalanceTextForRefundTest(t, ctx, fx.userID))
		require.Zero(t, refundAuditCount(t, ctx, fx.orderID, "REFUND_FAILED"))
	})
}

// 两个管理员同时查询同一笔处理中退款：只收尾一次，余额不被恢复两次、也不被扣两次。
func TestQueryAndFinalizeRefund_ConcurrentAdminsFinalizeOnce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name        string
		queryStatus string
		wantStatus  string
		wantBalance string
		wantAudit   string
	}{
		{name: "both see success", queryStatus: payment.ProviderStatusSuccess, wantStatus: service.OrderStatusRefunded, wantBalance: "50.00000000", wantAudit: "REFUND_SUCCESS"},
		{name: "both see failure", queryStatus: payment.ProviderStatusFailed, wantStatus: service.OrderStatusRefundFailed, wantBalance: "150.00000000", wantAudit: "REFUND_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gw := &pendingFinalizeGateway{queryStatus: tc.queryStatus}
			svc := newPendingFinalizePaymentService(t, gw)
			fx := enterRefundPending(t, ctx, svc, gw)

			const admins = 2
			barrier := &sync.WaitGroup{}
			barrier.Add(admins)
			gw.queryBarrier = barrier // 两边都已读到 REFUND_PENDING、拿到网关结果后才同时收尾

			type outcome struct {
				result *service.RefundResult
				err    error
			}
			outcomes := make([]outcome, admins)
			var wg sync.WaitGroup
			for i := 0; i < admins; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					res, err := svc.QueryAndFinalizeRefund(context.Background(), fx.orderID)
					outcomes[i] = outcome{result: res, err: err}
				}(i)
			}
			wg.Wait()
			require.Equal(t, int32(admins), gw.queryCalls.Load())

			finalized, conflicts := 0, 0
			for _, o := range outcomes {
				switch {
				case o.err == nil && o.result != nil:
					finalized++
				case o.err != nil && infraerrors.Reason(o.err) == "CONFLICT":
					conflicts++
				default:
					t.Fatalf("unexpected finalize outcome: result=%+v err=%v", o.result, o.err)
				}
			}
			require.Equal(t, 1, finalized)
			require.Equal(t, 1, conflicts)

			order, err := testEntClient(t).PaymentOrder.Get(ctx, fx.orderID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, order.Status)
			require.Equal(t, tc.wantBalance, userBalanceTextForRefundTest(t, ctx, fx.userID))
			require.Equal(t, 1, refundAuditCount(t, ctx, fx.orderID, tc.wantAudit))
			require.Zero(t, refundAuditCount(t, ctx, fx.orderID, "REFUND_ROLLBACK_FAILED"))
			require.Equal(t, int32(1), gw.refundCalls.Load())
		})
	}
}
