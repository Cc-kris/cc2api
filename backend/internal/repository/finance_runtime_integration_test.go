//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func insertFinanceFixture(t *testing.T, status string, revenue string, cost any, createdAt time.Time) int64 {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID, accountID, keyID, usageID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, "finance-"+suffix+"@example.test").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO accounts(name,platform,type) VALUES($1,'openai','apikey') RETURNING id`, "finance-"+suffix).Scan(&accountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'finance') RETURNING id`, userID, "sk-finance-"+suffix).Scan(&keyID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,created_at)
VALUES($1,$2,$3,$4,'gpt-test',$5) RETURNING id`, userID, keyID, accountID, "req-"+suffix, createdAt).Scan(&usageID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_records(
  usage_log_id,user_id,account_id,usage_created_at,requested_model,upstream_model,
  billing_type,business_type,usage_list_value,upstream_cost,cost_status,pricing_source,
  calculation_detail,calculated_at
) VALUES($1,$2,$3,$4,'gpt-test','gpt-test','token','balance',$5,$6,$7,'system','{}',$4)`,
		usageID, userID, accountID, createdAt, revenue, cost, status)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_finance_cost_segments WHERE usage_finance_record_id IN (SELECT id FROM usage_finance_records WHERE usage_log_id=$1)`, usageID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_finance_records WHERE usage_log_id=$1`, usageID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_logs WHERE id=$1`, usageID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM api_keys WHERE id=$1`, keyID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, accountID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	return usageID
}

func TestFinanceRepositoriesExecuteAgainstPostgreSQL(t *testing.T) {
	require.NotNil(t, integrationDB)
	ctx := context.Background()
	filter := service.FinanceReportFilter{
		StartDate:   "2026-07-01",
		EndDate:     "2026-07-27",
		Timezone:    "UTC",
		Location:    time.UTC,
		StartAt:     time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		EndBefore:   time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
		Granularity: "day",
		DataScope:   "exact_only",
	}

	reportRepo := NewFinanceReportRepository(integrationDB)
	_, err := reportRepo.SummarizeFinance(ctx, filter)
	require.NoError(t, err)
	_, err = reportRepo.ListFinanceTrend(ctx, filter)
	require.NoError(t, err)
	_, _, err = reportRepo.ListFinanceBreakdown(ctx, filter, service.FinanceBreakdownRequest{Dimension: "requested_model", SortBy: "profit", SortOrder: "desc", Page: 1, PageSize: 20})
	require.NoError(t, err)
	_, _, err = reportRepo.ListFinanceDetails(ctx, filter, service.FinanceDetailsRequest{SortBy: "profit", SortOrder: "desc", Page: 1, PageSize: 20})
	require.NoError(t, err)
	_, err = reportRepo.GetFinanceFunds(ctx, filter)
	require.NoError(t, err)
	_, _, err = reportRepo.ListFinanceQualityIssues(ctx, filter, "missing_price", 1, 20)
	require.NoError(t, err)
	_, err = reportRepo.GetFinanceCashFlow(ctx, filter, service.FinanceCashFlowRequest{Page: 1, PageSize: 20})
	require.NoError(t, err)

	alertRepo := NewFinanceAlertRepository(integrationDB)
	_, err = alertRepo.CollectFinanceAlertSignals(ctx, time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	syncRepo := NewUpstreamFinanceSyncRepository(integrationDB)
	_, err = syncRepo.ListDueSyncRequests(ctx, time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC))
	require.NoError(t, err)
}

func TestFinanceCashFlowKeepsOriginalPaymentAndRefundOnTheirOwnDates(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	paidAt := time.Date(2046, 7, 1, 8, 0, 0, 0, time.UTC)
	refundedAt := paidAt.Add(24 * time.Hour)
	var userID, orderID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, "finance-refund-"+suffix+"@example.test").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO payment_orders(
 user_id,amount,pay_amount,order_type,subscription_days,status,expires_at,paid_at,completed_at,refund_amount,refund_at
) VALUES($1,10,10,'balance',0,'REFUNDED',$2,$3,$3,4,$4) RETURNING id`, userID, refundedAt.AddDate(0, 1, 0), paidAt, refundedAt).Scan(&orderID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM payment_orders WHERE id=$1`, orderID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	repo := NewFinanceReportRepository(integrationDB)
	paidFilter := service.FinanceReportFilter{StartAt: paidAt.Truncate(24 * time.Hour), EndBefore: paidAt.Truncate(24 * time.Hour).Add(24 * time.Hour)}
	paidFacts, err := repo.GetFinanceCashFlow(ctx, paidFilter, service.FinanceCashFlowRequest{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, "10", paidFacts.CustomerPayments.String())
	require.True(t, paidFacts.CustomerRefunds.IsZero())

	refundFilter := service.FinanceReportFilter{StartAt: refundedAt.Truncate(24 * time.Hour), EndBefore: refundedAt.Truncate(24 * time.Hour).Add(24 * time.Hour)}
	refundFacts, err := repo.GetFinanceCashFlow(ctx, refundFilter, service.FinanceCashFlowRequest{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.True(t, refundFacts.CustomerPayments.IsZero())
	require.Equal(t, "4", refundFacts.CustomerRefunds.String())
}

func TestFinanceLossesReturnPersistedAlertWorkflow(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	usageID := insertFinanceFixture(t, "exact", "1", "2", createdAt)
	var alertID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO finance_alerts(
 alert_type,severity,aggregation_key,title,description,dimension_type,dimension_id,
 impact_amount,request_count,status,first_occurred_at,last_occurred_at,handled_note,handled_at
) VALUES('negative_profit','critical',$1,'loss','loss','usage_log',$2,1,1,'acknowledged',$3,$3,'checked',$3)
RETURNING id`, fmt.Sprintf("negative_profit:usage:%d", usageID), usageID, createdAt).Scan(&alertID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_alerts WHERE id=$1`, alertID)
	})

	filter := service.FinanceReportFilter{
		StartDate: "2026-07-27", EndDate: "2026-07-27", Timezone: "UTC", Location: time.UTC,
		StartAt: createdAt.Truncate(24 * time.Hour), EndBefore: createdAt.Truncate(24 * time.Hour).Add(24 * time.Hour),
		Granularity: "day", DataScope: "exact_only",
	}
	items, total, err := NewFinanceReportRepository(integrationDB).ListFinanceDetails(ctx, filter, service.FinanceDetailsRequest{
		ProfitDirection: "loss", LossStatus: "acknowledged", SortBy: "profit", SortOrder: "asc", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, &alertID, items[0].AlertID)
	require.Equal(t, "acknowledged", items[0].AlertStatus)
	require.Equal(t, "checked", items[0].HandledNote)
}

func TestResolvedUsageLossAlertDoesNotReopenAndSubscriptionRevenueMatchesReport(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Now().UTC().Add(-time.Hour)
	usageID := insertFinanceFixture(t, "exact", "7", "9", createdAt)
	_, err := integrationDB.ExecContext(ctx, `UPDATE usage_finance_records SET business_type='subscription' WHERE usage_log_id=$1`, usageID)
	require.NoError(t, err)
	var alertID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO finance_alerts(
 alert_type,severity,aggregation_key,title,description,dimension_type,dimension_id,
 impact_amount,request_count,status,first_occurred_at,last_occurred_at,handled_note,handled_at
) VALUES('negative_profit','critical',$1,'loss','loss','usage_log',$2,9,1,'resolved',$3,$3,'closed',$3)
RETURNING id`, fmt.Sprintf("negative_profit:usage:%d", usageID), usageID, createdAt).Scan(&alertID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_alerts WHERE id=$1`, alertID)
	})

	repo := NewFinanceAlertRepository(integrationDB)
	signals, err := repo.CollectFinanceAlertSignals(ctx, time.Now().UTC())
	require.NoError(t, err)
	var matched *service.FinanceAlertSignal
	for index := range signals {
		if signals[index].AggregationKey == fmt.Sprintf("negative_profit:usage:%d", usageID) {
			matched = &signals[index]
			break
		}
	}
	require.NotNil(t, matched)
	require.NotNil(t, matched.ImpactAmount)
	require.True(t, matched.ImpactAmount.Equal(decimal.NewFromInt(9)))
	require.NoError(t, repo.UpsertFinanceAlertSignals(ctx, []service.FinanceAlertSignal{*matched}))
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_alerts WHERE aggregation_key=$1`, matched.AggregationKey).Scan(&count))
	require.Equal(t, 1, count)
}

func TestFinanceMigrationAllowsZeroMultiplierAndNewStatuses(t *testing.T) {
	ctx := context.Background()
	for _, constraint := range []string{
		"accounts_upstream_cost_multiplier_check",
		"usage_logs_upstream_cost_multiplier_check",
		"usage_upstream_attempts_multiplier_check",
		"account_upstream_multiplier_changes_value_check",
		"usage_finance_records_cost_status_check",
		"usage_finance_cost_segments_status_check",
	} {
		var definition string
		err := integrationDB.QueryRowContext(ctx, `
SELECT pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conname = $1
ORDER BY oid DESC
LIMIT 1`, constraint).Scan(&definition)
		require.NoError(t, err, constraint)
		require.NotEmpty(t, definition, constraint)
		if constraint == "accounts_upstream_cost_multiplier_check" || constraint == "usage_logs_upstream_cost_multiplier_check" || constraint == "usage_upstream_attempts_multiplier_check" || constraint == "account_upstream_multiplier_changes_value_check" {
			require.Contains(t, definition, ">= (0)::numeric", constraint)
		} else {
			require.Contains(t, definition, "missing_profile", constraint)
			require.Contains(t, definition, "unsupported_usage", constraint)
			require.Contains(t, definition, "excluded", constraint)
		}
	}
}

func TestUpstreamFinanceSyncRequiresLeaseOwnerAndClosesRemovedPrices(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var upstreamID, walletID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstreams(base_url,normalized_base_url,name)
VALUES($1,$1,$2) RETURNING id`, "https://finance-sync-"+suffix+".example.test", "finance-sync-"+suffix).Scan(&upstreamID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstream_wallets(upstream_id,name,currency)
VALUES($1,$2,'USD') RETURNING id`, upstreamID, "finance-sync-wallet-"+suffix).Scan(&walletID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_model_price_versions WHERE wallet_id=$1`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_finance_sync_runs WHERE wallet_id=$1`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_async_jobs WHERE parameters->>'wallet_id'=$1`, fmt.Sprint(walletID))
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_wallets WHERE id=$1`, walletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstreams WHERE id=$1`, upstreamID)
	})

	repo := NewUpstreamFinanceSyncRepository(integrationDB)
	initialAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	prices := []service.UpstreamFinancePrice{
		{ModelPattern: "model-a", BillingMode: "token", PriceDetail: map[string]any{"input": "1"}, Currency: "USD", Source: "manual"},
		{ModelPattern: "model-b", BillingMode: "token", PriceDetail: map[string]any{"input": "2"}, Currency: "USD", Source: "manual"},
	}
	created, skipped, err := repo.ImportPriceVersions(ctx, walletID, prices, initialAt)
	require.NoError(t, err)
	require.Equal(t, int64(2), created)
	require.Zero(t, skipped)

	job, createdJob, err := repo.CreateOrGetActiveSyncJob(ctx, walletID, service.UpstreamFinanceSyncPricing, nil)
	require.NoError(t, err)
	require.True(t, createdJob)
	claimed, err := repo.ClaimNextSyncJob(ctx, "sync-owner", time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, job.ID, claimed.ID)
	require.NoError(t, repo.RenewSyncJobLease(ctx, job.ID, "sync-owner", time.Now().UTC().Add(2*time.Minute)))

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	err = repo.CompletePricingSync(ctx, job, "wrong-owner", prices[:1], finishedAt)
	require.ErrorIs(t, err, service.ErrUpstreamFinanceSyncLeaseLost)
	require.NoError(t, repo.CompletePricingSync(ctx, job, "sync-owner", prices[:1], finishedAt))

	var activeA, activeB int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM upstream_model_price_versions
WHERE wallet_id=$1 AND model_pattern='model-a' AND effective_to IS NULL`, walletID).Scan(&activeA))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM upstream_model_price_versions
WHERE wallet_id=$1 AND model_pattern='model-b' AND effective_to IS NULL`, walletID).Scan(&activeB))
	require.Equal(t, 1, activeA)
	require.Zero(t, activeB)
	var modelBEffectiveTo time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT effective_to FROM upstream_model_price_versions
WHERE wallet_id=$1 AND model_pattern='model-b' ORDER BY id DESC LIMIT 1`, walletID).Scan(&modelBEffectiveTo))
	require.WithinDuration(t, finishedAt, modelBEffectiveTo, time.Microsecond)
}

func TestFinanceReportReconcilesRepresentativeCostStates(t *testing.T) {
	createdAt := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	exactUsageID := insertFinanceFixture(t, "exact", "10", "12", createdAt)
	estimatedUsageID := insertFinanceFixture(t, "estimated", "10", "5", createdAt)
	insertFinanceFixture(t, "missing_price", "10", nil, createdAt)
	var estimatedRecordID, estimatedAccountID int64
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
SELECT id,account_id FROM usage_finance_records WHERE usage_log_id=$1`, estimatedUsageID).Scan(&estimatedRecordID, &estimatedAccountID))
	_, err := integrationDB.ExecContext(context.Background(), `
INSERT INTO usage_finance_cost_segments(
 usage_finance_record_id,attempt_no,account_id,upstream_model,usage_detail,
 pricing_source,cost_status,cost_amount,calculation_detail,created_at
) VALUES
 ($1,1,$2,'estimated-model','{}','upstream_exact','exact',2,'{}',$3),
 ($1,2,$2,'estimated-model','{}','estimated_system','estimated',3,'{}',$3)`, estimatedRecordID, estimatedAccountID, createdAt)
	require.NoError(t, err)

	filter := service.FinanceReportFilter{
		StartDate: "2026-07-27", EndDate: "2026-07-27", Timezone: "UTC", Location: time.UTC,
		StartAt: createdAt.Truncate(24 * time.Hour), EndBefore: createdAt.Truncate(24 * time.Hour).Add(24 * time.Hour),
		Granularity: "day", DataScope: "exact_only",
	}
	repo := NewFinanceReportRepository(integrationDB)
	summary, err := repo.SummarizeFinance(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, "30", summary.Revenue.String())
	require.Equal(t, "10", summary.CoveredRevenue.String())
	require.Equal(t, "12", summary.UpstreamCost.String())
	require.Equal(t, "2", summary.LossAmount.String())
	require.Equal(t, int64(1), summary.ExactCount)
	require.Equal(t, int64(1), summary.EstimatedCount)
	require.Equal(t, int64(1), summary.MissingPriceCount)

	allFilter := filter
	allFilter.DataScope = "all"
	allSummary, err := repo.SummarizeFinance(context.Background(), allFilter)
	require.NoError(t, err)
	require.Equal(t, "17", allSummary.UpstreamCost.String())
	require.Equal(t, "3", allSummary.EstimatedCost.String())
	require.Equal(t, "2", allSummary.UnconfirmedExactCost.String())

	details, total, err := repo.ListFinanceDetails(context.Background(), filter, service.FinanceDetailsRequest{
		SortBy: "profit", SortOrder: "desc", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, details, 3)
	foundExact := false
	for _, detail := range details {
		if detail.UsageLogID == exactUsageID {
			foundExact = true
			require.Equal(t, "exact", detail.CostStatus)
		}
	}
	require.True(t, foundExact)
}

func TestFinanceBreakdownReturnsAuditableCostComponents(t *testing.T) {
	createdAt := time.Date(2045, 8, 19, 3, 0, 0, 0, time.UTC)
	usageID := insertFinanceFixture(t, "exact", "100", "66", createdAt)
	ctx := context.Background()

	var recordID, accountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT id,account_id FROM usage_finance_records WHERE usage_log_id=$1`, usageID,
	).Scan(&recordID, &accountID))
	standardDetail := `{"items":[
		{"item":"input","amount":"1"},{"item":"output","amount":"2"},
		{"item":"cache_read","amount":"3"},{"item":"cache_write_5m","amount":"4"},{"item":"cache_write_1h","amount":"5"},
		{"item":"image_output","amount":"6"},{"item":"per_image","amount":"7"},
		{"item":"per_second","amount":"8"},{"item":"per_request","amount":"9"}
	]}`
	fastDetail := `{"items":[{"item":"input","amount":"10"},{"item":"cache_read","amount":"11"}]}`
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_cost_segments(
 usage_finance_record_id,attempt_no,account_id,upstream_model,service_tier,usage_detail,
 upstream_cost_multiplier_snapshot,pricing_source,cost_status,cost_amount,calculation_detail,created_at
) VALUES
 ($1,1,$2,'gpt-test','standard','{}',1,'system','exact',45,$3::jsonb,$5),
 ($1,2,$2,'gpt-test','priority','{}',1,'system','exact',21,$4::jsonb,$5)`,
		recordID, accountID, standardDetail, fastDetail, createdAt)
	require.NoError(t, err)

	filter := service.FinanceReportFilter{
		StartDate: "2045-08-19", EndDate: "2045-08-19", Timezone: "UTC", Location: time.UTC,
		StartAt: createdAt.Truncate(24 * time.Hour), EndBefore: createdAt.Truncate(24 * time.Hour).Add(24 * time.Hour),
		Granularity: "day", DataScope: "exact_only",
	}
	items, total, err := NewFinanceReportRepository(integrationDB).ListFinanceBreakdown(ctx, filter, service.FinanceBreakdownRequest{
		Dimension: "requested_model", SortBy: "profit", SortOrder: "desc", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	item := items[0]
	require.Equal(t, "1", item.InputCost.String())
	require.Equal(t, "2", item.OutputCost.String())
	require.Equal(t, "12", item.CacheCost.String())
	require.Equal(t, "21", item.FastCost.String())
	require.Equal(t, "13", item.ImageCost.String())
	require.Equal(t, "8", item.VideoCost.String())
	require.Equal(t, "9", item.OtherCost.String())
	componentTotal := item.InputCost.Add(item.OutputCost).Add(item.CacheCost).Add(item.FastCost).Add(item.ImageCost).Add(item.VideoCost).Add(item.OtherCost)
	require.True(t, item.UpstreamCost.Equal(componentTotal), "component total %s must equal upstream cost %s", componentTotal, item.UpstreamCost)
}

func TestFinanceBreakdownClassifiesCalculatorImageAndVideoItems(t *testing.T) {
	createdAt := time.Date(2045, 8, 20, 3, 0, 0, 0, time.UTC)
	usageID := insertFinanceFixture(t, "exact", "10", "5", createdAt)
	ctx := context.Background()
	var recordID, accountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT id,account_id FROM usage_finance_records WHERE usage_log_id=$1`, usageID).Scan(&recordID, &accountID))
	calculator := service.NewFinanceCostCalculator()
	multiplier := decimal.NewFromInt(1)
	perImage := decimal.RequireFromString("1.5")
	perSecond := decimal.RequireFromString("0.5")
	quote := func(card service.FinanceRateCard) *service.FinancePriceQuote {
		return &service.FinancePriceQuote{VersionID: 1, Source: service.FinancePricingSourceUpstreamExact, BillingMode: "image", Currency: "USD", USDExchangeRate: decimal.NewFromInt(1), Detail: service.FinancePriceDetail{Standard: card}}
	}
	image := calculator.Calculate(service.FinanceCostCalculatorInput{
		Attempt: service.UsageUpstreamAttempt{ImageCount: 2, UpstreamCostMultiplier: &multiplier, Billable: true}, BillingMode: "image",
		Price: quote(service.FinanceRateCard{PerImage: &perImage}),
	})
	video := calculator.Calculate(service.FinanceCostCalculatorInput{
		Attempt: service.UsageUpstreamAttempt{VideoSeconds: 4, UpstreamCostMultiplier: &multiplier, Billable: true}, BillingMode: "per_second",
		Price: quote(service.FinanceRateCard{PerSecond: &perSecond}),
	})
	require.Equal(t, "per_image", image.Items[0].Item)
	require.Equal(t, "per_second", video.Items[0].Item)
	imageJSON, err := json.Marshal(map[string]any{"items": image.Items})
	require.NoError(t, err)
	videoJSON, err := json.Marshal(map[string]any{"items": video.Items})
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_cost_segments(
 usage_finance_record_id,attempt_no,account_id,upstream_model,usage_detail,
 pricing_source,cost_status,cost_amount,calculation_detail,created_at
) VALUES
 ($1,1,$2,'image-model','{}','upstream_exact','exact',3,$3::jsonb,$5),
 ($1,2,$2,'video-model','{}','upstream_exact','exact',2,$4::jsonb,$5)`,
		recordID, accountID, imageJSON, videoJSON, createdAt)
	require.NoError(t, err)
	filter := service.FinanceReportFilter{StartDate: "2045-08-20", EndDate: "2045-08-20", Timezone: "UTC", Location: time.UTC,
		StartAt: createdAt.Truncate(24 * time.Hour), EndBefore: createdAt.Truncate(24 * time.Hour).Add(24 * time.Hour), Granularity: "day", DataScope: "exact_only"}
	items, total, err := NewFinanceReportRepository(integrationDB).ListFinanceBreakdown(ctx, filter, service.FinanceBreakdownRequest{
		Dimension: "requested_model", SortBy: "profit", SortOrder: "desc", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "3", items[0].ImageCost.String())
	require.Equal(t, "2", items[0].VideoCost.String())
	require.True(t, items[0].OtherCost.IsZero())
}

func TestSubscriptionRevenueRecognitionPersistsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	usageID := insertFinanceFixture(t, "exact", "3", "1", date)
	_, err := integrationDB.ExecContext(ctx, `UPDATE usage_finance_records SET business_type='subscription' WHERE usage_log_id=$1`, usageID)
	require.NoError(t, err)
	var userID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT user_id FROM usage_finance_records WHERE usage_log_id=$1`, usageID).Scan(&userID))
	var orderID int64
	err = integrationDB.QueryRowContext(ctx, `
INSERT INTO payment_orders(
 user_id,amount,pay_amount,order_type,subscription_days,status,expires_at,paid_at,completed_at
) VALUES($1,30,30,'subscription',30,'COMPLETED',$2,$3,$3) RETURNING id`,
		userID, date.AddDate(0, 1, 0), date).Scan(&orderID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_revenue_allocations WHERE source_type='subscription_recognition' AND source_id IN (SELECT id FROM subscription_revenue_recognitions WHERE payment_order_id=$1)`, orderID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM subscription_revenue_recognitions WHERE payment_order_id=$1`, orderID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM payment_orders WHERE id=$1`, orderID)
	})

	revenueRepo := NewFinanceRevenueRecognitionRepository(integrationDB)
	oldest, err := revenueRepo.OldestUnrecognizedSubscriptionDate(ctx, date, "UTC")
	require.NoError(t, err)
	require.NotNil(t, oldest)
	require.Equal(t, "2026-07-27", oldest.Format("2006-01-02"))
	svc := service.NewFinanceRevenueRecognitionService(revenueRepo, nil)
	processed, err := svc.RecognizeDate(ctx, date, "UTC")
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	processed, err = svc.RecognizeDate(ctx, date, "UTC")
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var recognized, allocated string
	var revision int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT recognized_revenue::text,allocated_revenue::text,current_revision
FROM subscription_revenue_recognitions WHERE payment_order_id=$1`, orderID).Scan(&recognized, &allocated, &revision))
	require.Equal(t, "1.0000000000", recognized)
	require.Equal(t, "1.0000000000", allocated)
	require.Equal(t, 1, revision)
	var allocationCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM usage_revenue_allocations
WHERE usage_log_id=$1 AND source_type='subscription_recognition' AND invalidated_at IS NULL`, usageID).Scan(&allocationCount))
	require.Equal(t, 1, allocationCount)
}

func TestSubscriptionRevenueRecognitionDateLockSerializesInstances(t *testing.T) {
	repo := NewFinanceRevenueRecognitionRepository(integrationDB)
	ctx := context.Background()
	date := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	releaseFirst, err := repo.AcquireSubscriptionRevenueDateLock(ctx, date, "UTC")
	require.NoError(t, err)

	acquired := make(chan func() error, 1)
	errors := make(chan error, 1)
	go func() {
		releaseSecond, lockErr := repo.AcquireSubscriptionRevenueDateLock(ctx, date, "UTC")
		if lockErr != nil {
			errors <- lockErr
			return
		}
		acquired <- releaseSecond
	}()

	select {
	case <-acquired:
		t.Fatal("second revenue recognizer acquired the same date lock before the first released it")
	case lockErr := <-errors:
		require.NoError(t, lockErr)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, releaseFirst())
	select {
	case releaseSecond := <-acquired:
		require.NoError(t, releaseSecond())
	case lockErr := <-errors:
		require.NoError(t, lockErr)
	case <-time.After(2 * time.Second):
		t.Fatal("second revenue recognizer did not acquire the date lock after release")
	}
}

func TestFinanceRetryQueueReplaysUsageBehindCursor(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, 7, 20, 3, 0, 0, 0, time.UTC)
	usageID := insertFinanceFixture(t, "missing_price", "1", nil, createdAt)
	_, err := integrationDB.ExecContext(ctx, `DELETE FROM usage_finance_records WHERE usage_log_id=$1`, usageID)
	require.NoError(t, err)
	repo := NewFinanceLedgerRepository(integrationEntClient, integrationDB)
	failureAt := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	require.NoError(t, repo.RecordFinanceProjectionFailure(ctx, usageID, "price unavailable", failureAt))
	_, err = integrationDB.ExecContext(ctx, `UPDATE finance_ledger_retries SET next_retry_at=NOW()-INTERVAL '1 second' WHERE usage_log_id=$1`, usageID)
	require.NoError(t, err)

	logs, err := repo.ListPendingUsage(ctx, service.FinanceUsageCursor{CreatedAt: createdAt.AddDate(0, 0, 10), ID: usageID + 1000}, 20)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, usageID, logs[0].ID)

	require.NoError(t, repo.ResolveFinanceProjectionFailure(ctx, usageID, failureAt.Add(time.Minute)))
	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status FROM finance_ledger_retries WHERE usage_log_id=$1`, usageID).Scan(&status))
	require.Equal(t, "resolved", status)
}

func TestFinanceFundsDeduplicatesSharedBalanceScope(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var upstreamID, walletA, walletB int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstreams(base_url,normalized_base_url,name)
VALUES($1,$1,$2) RETURNING id`, "https://finance-scope-"+suffix+".example.test", "finance-scope-"+suffix).Scan(&upstreamID))
	for index, target := range []*int64{&walletA, &walletB} {
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO upstream_wallets(upstream_id,name,balance_scope_key,currency,balance_kind)
VALUES($1,$2,'shared-main','USD','wallet_cash') RETURNING id`, upstreamID, fmt.Sprintf("shared-wallet-%d-%s", index, suffix)).Scan(target))
		_, err := integrationDB.ExecContext(ctx, `
INSERT INTO upstream_balance_snapshots(wallet_id,dedupe_key,balance_kind,balance_amount,currency,source,collected_at,sync_status)
VALUES($1,$2,'wallet_cash',100,'USD','newapi',NOW(),'success')`, *target, "snapshot-"+suffix+fmt.Sprint(index))
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_balance_snapshots WHERE wallet_id IN ($1,$2)`, walletA, walletB)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_wallets WHERE id IN ($1,$2)`, walletA, walletB)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstreams WHERE id=$1`, upstreamID)
	})

	filter := service.FinanceReportFilter{
		StartDate: "2026-07-27", EndDate: "2026-07-27", Timezone: "UTC", Location: time.UTC,
		StartAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), EndBefore: time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
		Granularity: "day", DataScope: "exact_only",
	}
	repo := NewFinanceReportRepository(integrationDB)
	funds, err := repo.GetFinanceFunds(ctx, filter)
	require.NoError(t, err)
	included := 0
	shared := 0
	for _, item := range funds.WalletCash {
		if item.BalanceScopeKey != "shared-main" {
			continue
		}
		shared++
		if item.IncludedInTotal {
			included++
		}
	}
	require.Equal(t, 2, shared)
	require.Equal(t, 1, included)

	summary, err := repo.SummarizeFinance(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, "100", summary.WalletCashTotal.String())
}

func TestFinanceOverviewIncludesUnallocatedSubscriptionRevenueWithoutUsingNominalUsageValue(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID, orderID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, "subscription-unallocated-"+suffix+"@example.test").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO payment_orders(user_id,amount,pay_amount,order_type,subscription_days,status,expires_at,paid_at,completed_at)
VALUES($1,30,30,'subscription',30,'COMPLETED',$2,$3,$3) RETURNING id`, userID, date.AddDate(0, 1, 0), date).Scan(&orderID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM subscription_revenue_recognitions WHERE payment_order_id=$1`, orderID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM payment_orders WHERE id=$1`, orderID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	recognition := service.NewFinanceRevenueRecognitionService(NewFinanceRevenueRecognitionRepository(integrationDB), nil)
	processed, err := recognition.RecognizeDate(ctx, date, "UTC")
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	filter := service.FinanceReportFilter{
		StartDate: "2026-07-27", EndDate: "2026-07-27", Timezone: "UTC", Location: time.UTC,
		StartAt: date.Truncate(24 * time.Hour), EndBefore: date.Truncate(24 * time.Hour).Add(24 * time.Hour),
		Granularity: "day", DataScope: "exact_only", UserID: &userID,
	}
	summary, err := NewFinanceReportRepository(integrationDB).SummarizeFinance(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, "1", summary.Revenue.String())
	require.Equal(t, "0", summary.CoveredRevenue.String())
}

func TestFinanceSettlementPersistsExactCostWithoutRewritingRequestMultiplier(t *testing.T) {
	ctx := context.Background()
	periodStart := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	usageCreatedAt := periodStart.Add(30 * time.Minute)
	periodEnd := periodStart.Add(time.Hour)
	usageID := insertFinanceFixture(t, string(service.FinanceCostStatusEstimated), "12", "5", usageCreatedAt)

	var accountID, recordID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT account_id FROM usage_logs WHERE id=$1`, usageID).Scan(&accountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
UPDATE usage_finance_records
SET upstream_cost_multiplier_snapshot=0.5
WHERE usage_log_id=$1
RETURNING id`, usageID).Scan(&recordID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_cost_segments(
 usage_finance_record_id,attempt_no,account_id,upstream_model,usage_detail,
 upstream_cost_multiplier_snapshot,pricing_source,cost_status,cost_amount,calculation_detail
) VALUES($1,1,$2,'gpt-test','{}',0.5,'system','estimated',5,
  '{"items":[{"amount_before_multiplier":"10"}]}'::jsonb)`, recordID, accountID)
	require.NoError(t, err)

	scopeKey := fmt.Sprintf("settlement-account-%d", accountID)
	var previousSnapshotID, currentSnapshotID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(
 account_id,scope_key,idempotency_key,list_cost_total,actual_cost_total,unit_code,
 unit_semantics,currency,collected_at,safe_snapshot,checksum,derivation_status
) VALUES($1,$2,$3,100,50,'USD','fiat_currency','USD',$4,'{}',$3,'baseline')
RETURNING id`, accountID, scopeKey, scopeKey+"-previous", periodStart).Scan(&previousSnapshotID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(
 account_id,scope_key,idempotency_key,list_cost_total,actual_cost_total,unit_code,
 unit_semantics,currency,collected_at,safe_snapshot,checksum,previous_snapshot_id,
 list_cost_delta,actual_cost_delta,observed_multiplier,derivation_status
) VALUES($1,$2,$3,110,52.2,'USD','fiat_currency','USD',$4,'{}',$3,$5,10,2.2,0.22,'applied')
RETURNING id`, accountID, scopeKey, scopeKey+"-current", periodEnd, previousSnapshotID).Scan(&currentSnapshotID))

	var intervalID int64
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id=$1`, recordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_cost_settlement_allocations WHERE settlement_interval_id=$1`, intervalID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_cost_settlement_intervals WHERE id=$1`, intervalID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_counter_snapshots WHERE id=$1`, currentSnapshotID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_counter_snapshots WHERE id=$1`, previousSnapshotID)
	})

	currency := "USD"
	listCostDelta := decimal.RequireFromString("10")
	observedMultiplier := decimal.RequireFromString("0.22")
	repo := NewFinanceSettlementRepository(integrationDB)
	interval, created, err := repo.CreateOrGetSettlementInterval(ctx, service.FinanceSettlementIntervalInput{
		AccountID: accountID, ScopeKey: scopeKey,
		PreviousSnapshotID: previousSnapshotID, CurrentSnapshotID: currentSnapshotID,
		PeriodStart: periodStart, PeriodEnd: periodEnd,
		UnitSemantics: service.AccountFinanceUnitFiatCurrency, Currency: &currency,
		ListCostDelta:      &listCostDelta,
		ActualCostDelta:    decimal.RequireFromString("2.2"),
		ObservedMultiplier: &observedMultiplier,
	})
	require.NoError(t, err)
	require.True(t, created)
	intervalID = interval.ID

	segments, err := repo.ListSettlementSegments(ctx, interval)
	require.NoError(t, err)
	require.Len(t, segments, 1)
	require.True(t, segments[0].StandardCost.Equal(decimal.RequireFromString("10")))
	result, err := service.AllocateFinanceSettlement(interval.ActualCostDelta, segments)
	require.NoError(t, err)
	require.NoError(t, repo.ApplySettlement(ctx, interval, result, "", nil))

	var segmentCost, segmentMultiplier, parentCost, parentMultiplier, difference string
	var segmentStatus, segmentSource, parentStatus, parentSource, intervalStatus string
	var currentRevision int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT cost_amount::text,upstream_cost_multiplier_snapshot::text,cost_status,pricing_source
FROM usage_finance_cost_segments WHERE usage_finance_record_id=$1 AND attempt_no=1`, recordID).
		Scan(&segmentCost, &segmentMultiplier, &segmentStatus, &segmentSource))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT upstream_cost::text,upstream_cost_multiplier_snapshot::text,cost_status,pricing_source,current_revision
FROM usage_finance_records WHERE id=$1`, recordID).
		Scan(&parentCost, &parentMultiplier, &parentStatus, &parentSource, &currentRevision))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT status,difference_amount::text FROM upstream_cost_settlement_intervals WHERE id=$1`, intervalID).
		Scan(&intervalStatus, &difference))

	require.Equal(t, "2.2000000000", segmentCost)
	require.Equal(t, "0.5000", segmentMultiplier)
	require.Equal(t, string(service.FinanceCostStatusExact), segmentStatus)
	require.Equal(t, service.FinancePricingSourceUpstreamExact, segmentSource)
	require.Equal(t, "2.2000000000", parentCost)
	require.Equal(t, "0.5000", parentMultiplier)
	require.Equal(t, string(service.FinanceCostStatusExact), parentStatus)
	require.Equal(t, service.FinancePricingSourceUpstreamExact, parentSource)
	require.Equal(t, 2, currentRevision)
	require.Equal(t, service.FinanceSettlementSettled, intervalStatus)
	require.Equal(t, "0.0000000000", difference)

	var allocationTotal string
	var revisionCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(allocated_cost),0)::text FROM usage_cost_settlement_allocations WHERE settlement_interval_id=$1`, intervalID).
		Scan(&allocationTotal))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id=$1 AND revision=2`, recordID).
		Scan(&revisionCount))
	require.Equal(t, "2.2000000000", allocationTotal)
	require.Equal(t, 1, revisionCount)

	accountFilter := accountID
	intervals, total, err := repo.ListSettlementIntervals(ctx, service.FinanceSettlementListFilter{
		Status: service.FinanceSettlementSettled, AccountID: &accountFilter, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(1))
	require.NotEmpty(t, intervals)
	allocations, err := repo.ListSettlementAllocations(ctx, intervalID)
	require.NoError(t, err)
	require.Len(t, allocations, 1)
	require.Nil(t, allocations[0].InvalidatedAt)

	var operatorID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT user_id FROM usage_logs WHERE id=$1`, usageID).Scan(&operatorID))
	reallocated, err := repo.ReallocateSettlement(ctx, intervalID, 1, "修正区间标准成本权重", operatorID)
	require.NoError(t, err)
	require.Equal(t, 2, reallocated.CurrentRevision)
	allocations, err = repo.ListSettlementAllocations(ctx, intervalID)
	require.NoError(t, err)
	require.Len(t, allocations, 2)
	require.Nil(t, allocations[0].InvalidatedAt)
	require.NotNil(t, allocations[1].InvalidatedAt)
	require.Equal(t, 2, allocations[0].Revision)
	require.Equal(t, 1, allocations[1].Revision)

	var auditedOperatorID int64
	var auditedReason string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT operator_id,reason FROM finance_calculation_revisions
WHERE entity_type='usage_finance_record' AND entity_id=$1 AND revision=3`, recordID).
		Scan(&auditedOperatorID, &auditedReason))
	require.Equal(t, operatorID, auditedOperatorID)
	require.Contains(t, auditedReason, "修正区间标准成本权重")
	_, err = repo.ReallocateSettlement(ctx, intervalID, 1, "重复提交应发生版本冲突", operatorID)
	require.True(t, service.IsFinanceSettlementError(err, "SETTLEMENT_STATE_CONFLICT"))
}

func TestFinanceSettlementRejectsSecondActiveAllocationForSameAttempt(t *testing.T) {
	ctx := context.Background()
	periodStart := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Second)
	usageID := insertFinanceFixture(t, string(service.FinanceCostStatusEstimated), "10", "5", periodStart.Add(30*time.Minute))
	var accountID, recordID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT account_id FROM usage_logs WHERE id=$1`, usageID).Scan(&accountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `UPDATE usage_finance_records SET upstream_cost_multiplier_snapshot=0.5 WHERE usage_log_id=$1 RETURNING id`, usageID).Scan(&recordID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_cost_segments(usage_finance_record_id,attempt_no,account_id,upstream_model,usage_detail,upstream_cost_multiplier_snapshot,pricing_source,cost_status,cost_amount,calculation_detail)
VALUES($1,1,$2,'duplicate-model','{}',0.5,'system','estimated',5,'{"items":[{"amount_before_multiplier":"10"}]}'::jsonb)`, recordID, accountID)
	require.NoError(t, err)

	createInterval := func(suffix string, start, end time.Time) (*service.FinanceSettlementInterval, error) {
		scope := fmt.Sprintf("settlement-duplicate-%d-%s", accountID, suffix)
		var previousID, currentID int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(account_id,scope_key,idempotency_key,list_cost_total,actual_cost_total,unit_code,unit_semantics,currency,collected_at,safe_snapshot,checksum,derivation_status)
VALUES($1,$2,$3,100,50,'USD','fiat_currency','USD',$4,'{}',$3,'baseline') RETURNING id`, accountID, scope, scope+"-p", start).Scan(&previousID))
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(account_id,scope_key,idempotency_key,list_cost_total,actual_cost_total,unit_code,unit_semantics,currency,collected_at,safe_snapshot,checksum,previous_snapshot_id,list_cost_delta,actual_cost_delta,observed_multiplier,derivation_status)
VALUES($1,$2,$3,110,52,'USD','fiat_currency','USD',$4,'{}',$3,$5,10,2,0.2,'applied') RETURNING id`, accountID, scope, scope+"-c", end, previousID).Scan(&currentID))
		t.Cleanup(func() {
			_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_counter_snapshots WHERE id=$1`, currentID)
			_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_counter_snapshots WHERE id=$1`, previousID)
		})
		currency := "USD"
		listCostDelta := decimal.NewFromInt(10)
		observedMultiplier := decimal.RequireFromString("0.2")
		interval, _, createErr := NewFinanceSettlementRepository(integrationDB).CreateOrGetSettlementInterval(ctx, service.FinanceSettlementIntervalInput{
			AccountID: accountID, ScopeKey: scope, PreviousSnapshotID: previousID, CurrentSnapshotID: currentID,
			PeriodStart: start, PeriodEnd: end, UnitSemantics: service.AccountFinanceUnitFiatCurrency, Currency: &currency,
			ListCostDelta: &listCostDelta, ActualCostDelta: decimal.NewFromInt(2), ObservedMultiplier: &observedMultiplier,
		})
		return interval, createErr
	}
	repo := NewFinanceSettlementRepository(integrationDB)
	first, err := createInterval("a", periodStart, periodStart.Add(time.Hour))
	require.NoError(t, err)
	second, err := createInterval("b", periodStart.Add(15*time.Minute), periodStart.Add(75*time.Minute))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id=$1`, recordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_cost_settlement_allocations WHERE settlement_interval_id IN ($1,$2)`, first.ID, second.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_cost_settlement_intervals WHERE id IN ($1,$2)`, first.ID, second.ID)
	})
	segments, err := repo.ListSettlementSegments(ctx, first)
	require.NoError(t, err)
	result, err := service.AllocateFinanceSettlement(first.ActualCostDelta, segments)
	require.NoError(t, err)
	require.NoError(t, repo.ApplySettlement(ctx, first, result, "", nil))
	secondResult := service.FinanceSettlementAllocationResult{
		Allocations:       []service.FinanceSettlementAllocation{{UsageLogID: usageID, AttemptNo: 1, StandardCost: decimal.NewFromInt(10), AllocationRate: decimal.NewFromInt(1), AllocatedCost: decimal.NewFromInt(2)}},
		StandardCostTotal: decimal.NewFromInt(10), ActualCostTotal: decimal.NewFromInt(2), AllocatedTotal: decimal.NewFromInt(2), Difference: decimal.Zero,
	}
	err = repo.ApplySettlement(ctx, second, secondResult, "", nil)
	require.Error(t, err)
	var conflict *service.FinanceSettlementError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "SETTLEMENT_STATE_CONFLICT", conflict.Code)
}

func TestFinanceSettlementRestoresFreshBaselineAcrossTwoAllocationLifecycles(t *testing.T) {
	ctx := context.Background()
	periodStart := time.Now().UTC().Add(-8 * time.Hour).Truncate(time.Second)
	periodEnd := periodStart.Add(time.Hour)
	inside := periodStart.Add(20 * time.Minute)
	outside := periodEnd.Add(time.Hour)
	firstUsageID := insertFinanceFixture(t, string(service.FinanceCostStatusEstimated), "12", "5", inside)

	var userID, keyID, accountID, firstRecordID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT user_id,api_key_id,account_id FROM usage_logs WHERE id=$1`, firstUsageID).Scan(&userID, &keyID, &accountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT id FROM usage_finance_records WHERE usage_log_id=$1`, firstUsageID).Scan(&firstRecordID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_cost_segments(usage_finance_record_id,attempt_no,account_id,upstream_model,usage_detail,upstream_cost_multiplier_snapshot,pricing_source,cost_status,cost_amount,calculation_detail)
VALUES($1,1,$2,'lifecycle-model','{}',0.5,'system','estimated',5,'{"items":[{"amount_before_multiplier":"10"}]}'::jsonb)`, firstRecordID, accountID)
	require.NoError(t, err)

	var secondUsageID, secondRecordID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,created_at)
VALUES($1,$2,$3,$4,'gpt-test',$5) RETURNING id`, userID, keyID, accountID, fmt.Sprintf("settlement-lifecycle-%d", time.Now().UnixNano()), outside).Scan(&secondUsageID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO usage_finance_records(usage_log_id,user_id,account_id,usage_created_at,requested_model,upstream_model,billing_type,business_type,usage_list_value,upstream_cost,cost_status,pricing_source,calculation_detail,calculated_at)
VALUES($1,$2,$3,$4,'gpt-test','gpt-test','token','balance',12,5,'estimated','system','{}',$4) RETURNING id`, secondUsageID, userID, accountID, outside).Scan(&secondRecordID))
	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_cost_segments(usage_finance_record_id,attempt_no,account_id,upstream_model,usage_detail,upstream_cost_multiplier_snapshot,pricing_source,cost_status,cost_amount,calculation_detail)
VALUES($1,1,$2,'lifecycle-model','{}',0.5,'system','estimated',5,'{"items":[{"amount_before_multiplier":"10"}]}'::jsonb)`, secondRecordID, accountID)
	require.NoError(t, err)

	scopeKey := fmt.Sprintf("settlement-lifecycle-%d", accountID)
	var previousSnapshotID, currentSnapshotID, intervalID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(account_id,scope_key,idempotency_key,list_cost_total,actual_cost_total,unit_code,unit_semantics,currency,collected_at,safe_snapshot,checksum,derivation_status)
VALUES($1,$2,$3,100,50,'USD','fiat_currency','USD',$4,'{}',$3,'baseline') RETURNING id`, accountID, scopeKey, scopeKey+"-previous", periodStart).Scan(&previousSnapshotID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(account_id,scope_key,idempotency_key,list_cost_total,actual_cost_total,unit_code,unit_semantics,currency,collected_at,safe_snapshot,checksum,previous_snapshot_id,list_cost_delta,actual_cost_delta,observed_multiplier,derivation_status)
VALUES($1,$2,$3,110,52,'USD','fiat_currency','USD',$4,'{}',$3,$5,10,2,0.2,'applied') RETURNING id`, accountID, scopeKey, scopeKey+"-current", periodEnd, previousSnapshotID).Scan(&currentSnapshotID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id IN ($1,$2)`, firstRecordID, secondRecordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_cost_settlement_allocations WHERE settlement_interval_id=$1`, intervalID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_cost_settlement_intervals WHERE id=$1`, intervalID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_counter_snapshots WHERE id IN ($1,$2)`, previousSnapshotID, currentSnapshotID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_finance_cost_segments WHERE usage_finance_record_id=$1`, secondRecordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_finance_records WHERE id=$1`, secondRecordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_logs WHERE id=$1`, secondUsageID)
	})

	listDelta := decimal.NewFromInt(10)
	observedMultiplier := decimal.RequireFromString("0.2")
	currency := "USD"
	repo := NewFinanceSettlementRepository(integrationDB)
	interval, created, err := repo.CreateOrGetSettlementInterval(ctx, service.FinanceSettlementIntervalInput{
		AccountID: accountID, ScopeKey: scopeKey, PreviousSnapshotID: previousSnapshotID, CurrentSnapshotID: currentSnapshotID,
		PeriodStart: periodStart, PeriodEnd: periodEnd, UnitSemantics: service.AccountFinanceUnitFiatCurrency, Currency: &currency,
		ListCostDelta: &listDelta, ActualCostDelta: decimal.NewFromInt(2), ObservedMultiplier: &observedMultiplier,
	})
	require.NoError(t, err)
	require.True(t, created)
	intervalID = interval.ID
	segments, err := repo.ListSettlementSegments(ctx, interval)
	require.NoError(t, err)
	result, err := service.AllocateFinanceSettlement(interval.ActualCostDelta, segments)
	require.NoError(t, err)
	require.NoError(t, repo.ApplySettlement(ctx, interval, result, "", nil))

	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_finance_records SET usage_created_at=CASE id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END WHERE id IN ($1,$2)`, firstRecordID, secondRecordID, outside, inside)
	require.NoError(t, err)
	reallocated, err := repo.ReallocateSettlement(ctx, intervalID, 1, "first switch", userID)
	require.NoError(t, err)
	require.Equal(t, 2, reallocated.CurrentRevision)
	var firstDetail []byte
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT calculation_detail FROM usage_finance_cost_segments WHERE usage_finance_record_id=$1 AND attempt_no=1`, firstRecordID).Scan(&firstDetail))
	require.NotContains(t, string(firstDetail), "pre_settlement_cost_status")

	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_finance_cost_segments SET cost_amount=7,cost_status='estimated',pricing_source='upstream_catalog' WHERE usage_finance_record_id=$1 AND attempt_no=1`, firstRecordID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_finance_records SET usage_created_at=CASE id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END WHERE id IN ($1,$2)`, firstRecordID, secondRecordID, inside, outside)
	require.NoError(t, err)
	reallocated, err = repo.ReallocateSettlement(ctx, intervalID, 2, "second switch", userID)
	require.NoError(t, err)
	require.Equal(t, 3, reallocated.CurrentRevision)

	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_finance_records SET usage_created_at=CASE id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END WHERE id IN ($1,$2)`, firstRecordID, secondRecordID, outside, inside)
	require.NoError(t, err)
	reallocated, err = repo.ReallocateSettlement(ctx, intervalID, 3, "third switch", userID)
	require.NoError(t, err)
	require.Equal(t, 4, reallocated.CurrentRevision)
	var restoredCost, restoredStatus, restoredSource string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT cost_amount::text,cost_status,pricing_source,calculation_detail FROM usage_finance_cost_segments WHERE usage_finance_record_id=$1 AND attempt_no=1`, firstRecordID).Scan(&restoredCost, &restoredStatus, &restoredSource, &firstDetail))
	require.Equal(t, "7.0000000000", restoredCost)
	require.Equal(t, "estimated", restoredStatus)
	require.Equal(t, "upstream_catalog", restoredSource)
	require.NotContains(t, string(firstDetail), "pre_settlement_cost_status")
}

func TestFinanceSettlementPersistsAndAppliesCumulativeActualOnlyInterval(t *testing.T) {
	ctx := context.Background()
	periodStart := time.Now().UTC().Add(-12 * time.Hour).Truncate(time.Second)
	periodEnd := periodStart.Add(time.Hour)
	usageID := insertFinanceFixture(t, "estimated", "10", "5", periodStart.Add(30*time.Minute))
	var accountID, recordID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT account_id FROM usage_logs WHERE id=$1`, usageID).Scan(&accountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT id FROM usage_finance_records WHERE usage_log_id=$1`, usageID).Scan(&recordID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_finance_cost_segments(usage_finance_record_id,attempt_no,account_id,upstream_model,usage_detail,upstream_cost_multiplier_snapshot,pricing_source,cost_status,cost_amount,calculation_detail)
VALUES($1,1,$2,'actual-only-model','{}',0.5,'system','estimated',5,'{"items":[{"amount_before_multiplier":"10"}]}'::jsonb)`, recordID, accountID)
	require.NoError(t, err)
	scopeKey := fmt.Sprintf("actual-only-%d", accountID)
	var previousID, currentID, intervalID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(account_id,scope_key,idempotency_key,actual_cost_total,unit_code,unit_semantics,currency,collected_at,safe_snapshot,checksum,derivation_status)
VALUES($1,$2,$3,50,'USD','fiat_currency','USD',$4,'{}',$3,'baseline') RETURNING id`, accountID, scopeKey, scopeKey+"-previous", periodStart).Scan(&previousID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots(account_id,scope_key,idempotency_key,actual_cost_total,unit_code,unit_semantics,currency,collected_at,safe_snapshot,checksum,previous_snapshot_id,actual_cost_delta,derivation_status)
VALUES($1,$2,$3,52.2,'USD','fiat_currency','USD',$4,'{}',$3,$5,2.2,'settlement_ready') RETURNING id`, accountID, scopeKey, scopeKey+"-current", periodEnd, previousID).Scan(&currentID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_calculation_revisions WHERE entity_type='usage_finance_record' AND entity_id=$1`, recordID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_cost_settlement_allocations WHERE settlement_interval_id=$1`, intervalID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_cost_settlement_intervals WHERE id=$1`, intervalID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_counter_snapshots WHERE id IN ($1,$2)`, previousID, currentID)
	})
	currency := "USD"
	repo := NewFinanceSettlementRepository(integrationDB)
	interval, created, err := repo.CreateOrGetSettlementInterval(ctx, service.FinanceSettlementIntervalInput{
		AccountID: accountID, ScopeKey: scopeKey, PreviousSnapshotID: previousID, CurrentSnapshotID: currentID,
		PeriodStart: periodStart, PeriodEnd: periodEnd, UnitSemantics: service.AccountFinanceUnitFiatCurrency, Currency: &currency,
		ActualCostDelta: decimal.RequireFromString("2.2"),
	})
	require.NoError(t, err)
	require.True(t, created)
	intervalID = interval.ID
	require.Nil(t, interval.ListCostDelta)
	require.Nil(t, interval.ObservedMultiplier)
	segments, err := repo.ListSettlementSegments(ctx, interval)
	require.NoError(t, err)
	result, err := service.AllocateFinanceSettlement(interval.ActualCostDelta, segments)
	require.NoError(t, err)
	require.NoError(t, repo.ApplySettlement(ctx, interval, result, "", nil))
	var settledCost, status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT upstream_cost::text,cost_status FROM usage_finance_records WHERE id=$1`, recordID).Scan(&settledCost, &status))
	require.Equal(t, "2.2000000000", settledCost)
	require.Equal(t, "exact", status)
}
