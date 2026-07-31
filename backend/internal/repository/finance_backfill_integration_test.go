//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinanceBackfillCandidatesPreserveRecordedClassificationWithoutProjection(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("classification-%d", time.Now().UnixNano())
	createdAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	var userID, accountID, keyID, promotionUsageID, adminUsageID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, suffix+"@example.test").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO accounts(name,platform,type) VALUES($1,'openai','apikey') RETURNING id`, suffix).Scan(&accountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'backfill-classification') RETURNING id`, userID, "sk-"+suffix).Scan(&keyID))
	promotionRequestID := "promo-" + suffix
	adminRequestID := "admin-" + suffix
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,actual_cost,total_cost,created_at)
VALUES($1,$2,$3,$4,'classification-model',1,1,$5) RETURNING id`,
		userID, keyID, accountID, promotionRequestID, createdAt).Scan(&promotionUsageID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,actual_cost,total_cost,created_at)
VALUES($1,$2,$3,$4,'classification-model',1,1,$5) RETURNING id`,
		userID, keyID, accountID, adminRequestID, createdAt.Add(time.Second)).Scan(&adminUsageID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_billing_dedup(
 request_id,api_key_id,request_fingerprint,finance_business_type,promotion_credit_used,
 finance_excluded,finance_exclusion_reason,finance_classification_recorded,created_at
) VALUES
 ($1,$3,repeat('a',64),'promotion',1,FALSE,NULL,TRUE,$4),
 ($2,$3,repeat('b',64),'admin',0,TRUE,'trusted_test',TRUE,$4)`,
		promotionRequestID, adminRequestID, keyID, createdAt)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_billing_dedup WHERE api_key_id=$1`, keyID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_logs WHERE id=ANY($1)`, pq.Array([]int64{promotionUsageID, adminUsageID}))
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM api_keys WHERE id=$1`, keyID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, accountID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	repo := NewFinanceBackfillRepository(integrationDB)
	candidates, err := repo.ListFinanceBackfillCandidates(ctx, service.FinanceBackfillRequest{
		StartDate: "2026-07-15", EndDate: "2026-07-15",
	}, service.FinanceBackfillCursor{}, 100)
	require.NoError(t, err)
	byID := make(map[int64]service.FinanceBackfillCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.UsageLog.ID] = candidate
	}
	require.False(t, byID[promotionUsageID].HasProjection)
	require.Equal(t, "promotion", byID[promotionUsageID].UsageLog.FinanceBusinessTypeSnapshot)
	require.False(t, byID[adminUsageID].HasProjection)
	require.Equal(t, "admin", byID[adminUsageID].UsageLog.FinanceBusinessTypeSnapshot)
	require.True(t, byID[adminUsageID].UsageLog.FinanceExcluded)
	require.Equal(t, "trusted_test", byID[adminUsageID].UsageLog.FinanceExclusionReason)
}

func TestFinanceBackfillPersistsResumesAndAppendsRevision(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	createdAt := time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC)
	var userID, accountID, keyID, upstreamID, walletID, usageID, recordID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, "backfill-"+suffix+"@example.test").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO accounts(name,platform,type,upstream_cost_multiplier) VALUES($1,'openai','apikey',9.0000) RETURNING id`, "backfill-"+suffix).Scan(&accountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'backfill') RETURNING id`, userID, "sk-backfill-"+suffix).Scan(&keyID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstreams(base_url,normalized_base_url,name)
VALUES($1,$1,$2) RETURNING id`, "https://backfill-"+suffix+".example.test", "backfill-"+suffix).Scan(&upstreamID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstream_wallets(upstream_id,name,currency)
VALUES($1,$2,'USD') RETURNING id`, upstreamID, "wallet-"+suffix).Scan(&walletID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO upstream_wallet_accounts(wallet_id,account_id,effective_from,reason)
VALUES($1,$2,$3,'integration fixture')`, walletID, accountID, createdAt.Add(-time.Hour))
	require.NoError(t, err)
	model := "backfill-model-" + suffix
	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO system_model_price_versions(catalog_checksum,provider,model_name,billing_mode,price_detail,effective_from)
VALUES($1,'openai',$2,'token','{"input":"2"}'::jsonb,$3)`, "catalog-"+suffix, model, createdAt.Add(-time.Hour))
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO usage_logs(
 user_id,api_key_id,account_id,request_id,model,requested_model,upstream_model,
 input_tokens,billing_mode,actual_cost,usage_list_value,upstream_cost_multiplier,created_at
) VALUES($1,$2,$3,$4,$5,$5,$5,1000000,'token',3,3,0.5000,$6) RETURNING id`,
		userID, keyID, accountID, "req-"+suffix, model, createdAt).Scan(&usageID))
	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO usage_upstream_attempts(
 usage_log_id,request_id,attempt_no,account_id,upstream_model,input_tokens,request_count,
 upstream_cost_multiplier,billable,created_at
) VALUES($1,$2,1,$3,$4,1000000,1,0.5000,TRUE,$5)`, usageID, "req-"+suffix, accountID, model, createdAt)
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO usage_finance_records(
 usage_log_id,user_id,account_id,wallet_id,upstream_id,usage_created_at,requested_model,upstream_model,
 billing_type,business_type,usage_list_value,upstream_cost,cost_status,pricing_source,
 upstream_cost_multiplier_snapshot,current_revision,calculation_detail,calculated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$7,'token','balance',3,99,'estimated','estimated_system',0.5000,1,'{}',$6)
RETURNING id`, usageID, userID, accountID, walletID, upstreamID, createdAt, model).Scan(&recordID))

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id=$1`, recordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_backfill_jobs WHERE async_job_id IN (SELECT id FROM finance_async_jobs WHERE job_type=$1 AND operator_id=$2)`, service.FinanceBackfillJobType, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_async_jobs WHERE job_type=$1 AND operator_id=$2`, service.FinanceBackfillJobType, userID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_finance_cost_segments WHERE usage_finance_record_id=$1`, recordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_finance_records WHERE id=$1`, recordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_upstream_attempts WHERE usage_log_id=$1`, usageID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_logs WHERE id=$1`, usageID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM system_model_price_versions WHERE model_name=$1`, model)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_wallet_accounts WHERE wallet_id=$1`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_wallets WHERE id=$1`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstreams WHERE id=$1`, upstreamID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM api_keys WHERE id=$1`, keyID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, accountID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	backfillRepo := NewFinanceBackfillRepository(integrationDB)
	ledgerRepo := NewFinanceLedgerRepository(integrationEntClient, integrationDB)
	priceSelector := service.NewFinancePriceSelector(NewFinancePriceLookupRepository(integrationEntClient))
	scanner := service.NewFinanceUsageScanner(ledgerRepo, priceSelector, service.NewFinanceCostCalculator())
	creator := service.NewFinanceBackfillService(backfillRepo, ledgerRepo, scanner, nil)
	request := service.FinanceBackfillRequest{
		StartDate: "2026-07-01", EndDate: "2026-07-01",
		Scope:         service.FinanceBackfillScope{CostStatus: []string{"estimated"}, AccountIDs: []int64{accountID}},
		PricingPolicy: service.FinanceBackfillPricingHistorical, DryRunSampleSize: 100, Reason: "integration historical correction",
	}

	var revisionsBefore, jobsBefore int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id=$1`, recordID).Scan(&revisionsBefore))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_async_jobs WHERE job_type=$1 AND operator_id=$2`, service.FinanceBackfillJobType, userID).Scan(&jobsBefore))
	preview, err := creator.Preview(ctx, request)
	require.NoError(t, err)
	require.Equal(t, int64(1), preview.EstimatedRecords)
	require.Zero(t, preview.ExactRepairable)
	require.Equal(t, int64(1), preview.EstimatedOnly)
	var revisionsAfterPreview, jobsAfterPreview int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id=$1`, recordID).Scan(&revisionsAfterPreview))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_async_jobs WHERE job_type=$1 AND operator_id=$2`, service.FinanceBackfillJobType, userID).Scan(&jobsAfterPreview))
	require.Equal(t, revisionsBefore, revisionsAfterPreview)
	require.Equal(t, jobsBefore, jobsAfterPreview)

	request.PreviewToken = preview.PreviewToken
	job, err := creator.Run(ctx, request, userID)
	require.NoError(t, err)
	require.Equal(t, "queued", job.Status)
	paused, err := creator.Pause(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "paused", paused.Status)
	resumed, err := creator.Resume(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "queued", resumed.Status)
	claimed, err := backfillRepo.ClaimFinanceBackfillJob(ctx, "integration-worker", time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, "running", claimed.Status)
	pausedRunning, err := creator.Pause(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "paused", pausedRunning.Status)
	_, err = creator.Resume(ctx, job.ID)
	require.Error(t, err, "a paused running job must keep its lease until the active worker acknowledges the pause")
	require.NoError(t, backfillRepo.AcknowledgeFinanceBackfillPause(ctx, job.ID, "integration-worker", time.Now().UTC()))
	resumed, err = creator.Resume(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "queued", resumed.Status)

	// A new service instance proves execution state is durable and does not rely
	// on the creator process's in-memory cursor or preview signing key.
	worker := service.NewFinanceBackfillService(backfillRepo, ledgerRepo, scanner, nil)
	require.NoError(t, worker.RunNextBatch(ctx))
	completed, err := worker.Get(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Status)
	require.Equal(t, int64(1), completed.ProcessedCount)

	var currentCost decimal.Decimal
	var currentRevision int
	var usageMultiplier decimal.Decimal
	var currentStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT upstream_cost,cost_status,current_revision FROM usage_finance_records WHERE id=$1`, recordID).Scan(&currentCost, &currentStatus, &currentRevision))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT upstream_cost_multiplier FROM usage_logs WHERE id=$1`, usageID).Scan(&usageMultiplier))
	require.True(t, currentCost.Equal(decimal.NewFromInt(1)))
	require.Equal(t, "estimated", currentStatus)
	require.Equal(t, 2, currentRevision)
	require.True(t, usageMultiplier.Equal(decimal.RequireFromString("0.5")))

	var revisionCount int
	var revisionJobID, revisionOperatorID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*),MAX(job_id),MAX(operator_id)
FROM finance_calculation_revisions
WHERE entity_type='usage_finance_record' AND entity_id=$1`, recordID).Scan(&revisionCount, &revisionJobID, &revisionOperatorID))
	require.Equal(t, 1, revisionCount)
	require.Equal(t, job.ID, revisionJobID)
	require.Equal(t, userID, revisionOperatorID)

	_, err = worker.Resume(ctx, job.ID)
	require.True(t, service.IsFinanceBackfillError(err, "JOB_STATE_CONFLICT"))
}
