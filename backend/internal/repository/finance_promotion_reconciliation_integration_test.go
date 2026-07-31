//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPromotionCreditReconciliationResolveUpdatesBalanceAndAudit(t *testing.T) {
	require.NotNil(t, integrationDB)
	ctx := context.Background()
	suffix := fmt.Sprintf("promotion-reconciliation-%d", time.Now().UnixNano())
	var userID, operatorID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, suffix+"@example.test").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role) VALUES($1,'test','admin') RETURNING id`, "operator-"+suffix+"@example.test").Scan(&operatorID))
	cutover := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO user_promotion_credit_balances(user_id,remaining_amount) VALUES($1,9)
	ON CONFLICT(user_id) DO UPDATE SET remaining_amount=EXCLUDED.remaining_amount`, userID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO user_promotion_credit_reconciliations(user_id,detected_historical_bonus,status,cutover_at)
VALUES($1,12.5,'requires_reconciliation',$2)
ON CONFLICT(user_id) DO UPDATE SET detected_historical_bonus=EXCLUDED.detected_historical_bonus,status='requires_reconciliation',cutover_at=EXCLUDED.cutover_at,resolved_at=NULL,resolved_by=NULL,confirmed_remaining_amount=NULL,notes=NULL`, userID, cutover)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='promotion_credit_reconciliation' AND entity_id=$1`, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM user_promotion_credit_reconciliations WHERE user_id=$1`, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM user_promotion_credit_balances WHERE user_id=$1`, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, userID, operatorID)
	})

	svc := service.NewPromotionCreditReconciliationService(NewPromotionCreditReconciliationRepository(integrationDB))
	items, total, err := svc.List(ctx, service.PromotionCreditReconciliationListRequest{Status: service.PromotionCreditReconciliationRequired, Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(1))
	var pending *service.PromotionCreditReconciliation
	for index := range items {
		if items[index].UserID == userID {
			pending = &items[index]
			break
		}
	}
	require.NotNil(t, pending)
	require.Equal(t, suffix+"@example.test", pending.UserEmail)
	require.Equal(t, "12.5000000000", pending.DetectedHistoricalBonus)
	require.Equal(t, "9.0000000000", pending.CurrentRemainingAmount)
	require.Equal(t, service.PromotionCreditReconciliationRequired, pending.Status)

	resolved, err := svc.Resolve(ctx, userID, service.ResolvePromotionCreditReconciliationRequest{ConfirmedRemainingAmount: "2.5", Note: "已与客户台账核对"}, operatorID)
	require.NoError(t, err)
	require.Equal(t, service.PromotionCreditReconciliationResolved, resolved.Status)
	require.NotNil(t, resolved.ConfirmedRemainingAmount)
	require.Equal(t, "2.5000000000", *resolved.ConfirmedRemainingAmount)
	require.Equal(t, "2.5000000000", resolved.CurrentRemainingAmount)
	require.Equal(t, "已与客户台账核对", resolved.Notes)

	var balance string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT remaining_amount::text FROM user_promotion_credit_balances WHERE user_id=$1`, userID).Scan(&balance))
	require.Equal(t, "2.5000000000", balance)
	var auditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_calculation_revisions WHERE entity_type='promotion_credit_reconciliation' AND entity_id=$1 AND operator_id=$2`, userID, operatorID).Scan(&auditCount))
	require.Equal(t, 1, auditCount)

	_, err = svc.Resolve(ctx, userID, service.ResolvePromotionCreditReconciliationRequest{ConfirmedRemainingAmount: "1", Note: "重复处理"}, operatorID)
	require.ErrorIs(t, err, service.ErrPromotionCreditReconciliationResolved)
}

func TestPromotionCreditReconciliationRetriesConcurrentUsageDeduction(t *testing.T) {
	require.NotNil(t, integrationDB)
	ctx := context.Background()
	suffix := fmt.Sprintf("promotion-reconciliation-race-%d", time.Now().UnixNano())
	var userID, operatorID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, suffix+"@example.test").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role) VALUES($1,'test','admin') RETURNING id`, "operator-"+suffix+"@example.test").Scan(&operatorID))
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO user_promotion_credit_balances(user_id,remaining_amount) VALUES($1,9)`, userID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO user_promotion_credit_reconciliations(user_id,detected_historical_bonus,status,cutover_at) VALUES($1,12.5,'requires_reconciliation',$2)`, userID, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='promotion_credit_reconciliation' AND entity_id=$1`, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM user_promotion_credit_reconciliations WHERE user_id=$1`, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM user_promotion_credit_balances WHERE user_id=$1`, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, userID, operatorID)
	})

	billingTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	used, err := consumeUsageBillingPromotionCredit(ctx, billingTx, userID, 1)
	require.NoError(t, err)
	require.Equal(t, "1", used.String())

	type resolveResult struct {
		item *service.PromotionCreditReconciliation
		err  error
	}
	resultCh := make(chan resolveResult, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		item, resolveErr := service.NewPromotionCreditReconciliationService(NewPromotionCreditReconciliationRepository(integrationDB)).Resolve(
			ctx, userID, service.ResolvePromotionCreditReconciliationRequest{ConfirmedRemainingAmount: "2.5", Note: "并发核对"}, operatorID,
		)
		resultCh <- resolveResult{item: item, err: resolveErr}
	}()
	<-started
	select {
	case early := <-resultCh:
		require.Failf(t, "reconciliation must wait for billing balance lock", "returned early: item=%v err=%v", early.item, early.err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, billingTx.Commit())

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		require.NotNil(t, result.item)
		require.Equal(t, service.PromotionCreditReconciliationResolved, result.item.Status)
	case <-time.After(5 * time.Second):
		require.Fail(t, "reconciliation did not finish after billing committed")
	}
	var balance string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT remaining_amount::text FROM user_promotion_credit_balances WHERE user_id=$1`, userID).Scan(&balance))
	require.Equal(t, "2.5000000000", balance)
}
