//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinanceReconciliationImportIsIdempotentAndAuditable(t *testing.T) {
	require.NotNil(t, integrationDB)
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var upstreamID, walletID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstreams(base_url,normalized_base_url,name)
VALUES($1,$1,$2) RETURNING id`, "https://reconciliation-"+suffix+".example.test", "reconciliation-"+suffix).Scan(&upstreamID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstream_wallets(upstream_id,name,currency)
VALUES($1,$2,'USD') RETURNING id`, upstreamID, "wallet-"+suffix).Scan(&walletID))

	usageID := insertFinanceFixture(t, "exact", "12", "9", now.Add(-time.Hour))
	var actorID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT user_id FROM usage_finance_records WHERE usage_log_id=$1`, usageID).Scan(&actorID))
	_, err := integrationDB.ExecContext(ctx, `UPDATE usage_finance_records SET wallet_id=$1,upstream_id=$2 WHERE usage_log_id=$3`, walletID, upstreamID, usageID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `UPDATE usage_finance_records SET wallet_id=NULL,upstream_id=NULL WHERE usage_log_id=$1`, usageID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_alerts WHERE aggregation_key=$1`, fmt.Sprintf("reconciliation_difference:%d", walletID))
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_async_jobs WHERE job_type='upstream_bill_reconciliation' AND parameters->>'wallet_id'=$1`, fmt.Sprintf("%d", walletID))
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='upstream_bill_reconciliation' AND entity_id IN (SELECT id FROM upstream_bill_reconciliations WHERE wallet_id=$1)`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_bill_reconciliations WHERE wallet_id=$1`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_wallets WHERE id=$1`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstreams WHERE id=$1`, upstreamID)
	})

	repo := NewFinanceReconciliationRepository(integrationDB)
	reconciliation := service.NewFinanceReconciliationService(repo)
	periodStart := now.Add(-24 * time.Hour)
	periodEnd := now.Add(time.Hour)
	content := []byte("item,amount\nusage,10.00\n")

	first, err := reconciliation.ImportCSV(ctx, walletID, periodStart, periodEnd, "USD", "upstream-bill-1", "bill.csv", content, actorID)
	require.NoError(t, err)
	require.False(t, first.Duplicate)
	require.Positive(t, first.JobID)
	require.Equal(t, "completed", first.JobStatus)
	require.True(t, decimal.RequireFromString(first.Reconciliation.UpstreamBillAmount).Equal(decimal.NewFromInt(10)))
	require.True(t, decimal.RequireFromString(first.Reconciliation.SystemCostAmount).Equal(decimal.NewFromInt(9)))
	require.True(t, decimal.RequireFromString(first.Reconciliation.DifferenceAmount).Equal(decimal.NewFromInt(1)))
	require.Equal(t, service.FinanceReconciliationDifference, first.Reconciliation.Status)
	require.NotNil(t, first.Reconciliation.DifferenceRate)
	require.True(t, decimal.RequireFromString(*first.Reconciliation.DifferenceRate).Equal(decimal.RequireFromString("0.1")))

	duplicate, err := reconciliation.ImportCSV(ctx, walletID, periodStart, periodEnd, "USD", "different-reference", "renamed.csv", content, actorID)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Equal(t, first.Reconciliation.ID, duplicate.Reconciliation.ID)
	require.Equal(t, first.JobID, duplicate.JobID)
	require.Equal(t, "upstream-bill-1", duplicate.Reconciliation.SourceReference)
	var duplicateAlertCount, duplicateOccurrenceCount, duplicateRequestCount int64
	var duplicateImpact string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*),MAX(occurrence_count),MAX(request_count),MAX(impact_amount)::text
		FROM finance_alerts WHERE aggregation_key=$1
	`, fmt.Sprintf("reconciliation_difference:%d", walletID)).Scan(
		&duplicateAlertCount, &duplicateOccurrenceCount, &duplicateRequestCount, &duplicateImpact,
	))
	require.Equal(t, int64(1), duplicateAlertCount)
	require.Equal(t, int64(1), duplicateOccurrenceCount)
	require.Equal(t, int64(1), duplicateRequestCount)
	require.Equal(t, "1.0000000000", duplicateImpact)

	matched, err := reconciliation.ImportCSV(
		ctx, walletID, now.Add(-48*time.Hour), now.Add(-25*time.Hour), "USD",
		"upstream-bill-zero", "zero.csv", []byte("amount\n0\n"), actorID,
	)
	require.NoError(t, err)
	require.Equal(t, service.FinanceReconciliationMatched, matched.Reconciliation.Status)
	require.True(t, decimal.RequireFromString(matched.Reconciliation.DifferenceAmount).IsZero())
	require.Nil(t, matched.Reconciliation.DifferenceRate)

	items, total, err := reconciliation.List(ctx, service.FinanceReconciliationListRequest{
		UpstreamID: &upstreamID,
		WalletID:   &walletID,
		Status:     service.FinanceReconciliationDifference,
		Page:       1,
		PageSize:   20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "bill.csv", items[0].SourceFileName)

	updated, err := reconciliation.UpdateStatus(ctx, first.Reconciliation.ID, service.FinanceReconciliationStatusUpdate{
		Status: service.FinanceReconciliationConfirmed,
		Note:   "confirmed against upstream statement",
	}, actorID)
	require.NoError(t, err)
	require.Equal(t, service.FinanceReconciliationConfirmed, updated.Status)
	var auditCount int
	var oldStatus, newStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*),MAX(old_result->>'status'),MAX(new_result->>'status')
FROM finance_calculation_revisions
WHERE entity_type='upstream_bill_reconciliation' AND entity_id=$1`, first.Reconciliation.ID).Scan(&auditCount, &oldStatus, &newStatus))
	require.Equal(t, 1, auditCount)
	require.Equal(t, service.FinanceReconciliationDifference, oldStatus)
	require.Equal(t, service.FinanceReconciliationConfirmed, newStatus)
	require.Equal(t, "confirmed against upstream statement", updated.HandledNote)
	require.NotNil(t, updated.HandledBy)
	require.Equal(t, actorID, *updated.HandledBy)
	require.NotNil(t, updated.HandledAt)

	var reconciliationCount, alertCount, jobCount int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_bill_reconciliations WHERE wallet_id=$1`, walletID).Scan(&reconciliationCount))
	require.Equal(t, int64(2), reconciliationCount)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_alerts WHERE aggregation_key=$1`, fmt.Sprintf("reconciliation_difference:%d", walletID)).Scan(&alertCount))
	require.Equal(t, int64(1), alertCount)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_async_jobs WHERE job_type='upstream_bill_reconciliation' AND request_checksum=$1`, first.Reconciliation.SourceFileChecksum).Scan(&jobCount))
	require.Equal(t, int64(1), jobCount)
}
