package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type financeSettlementRepository struct{ db *sql.DB }

func (r *financeSettlementRepository) FindFinanceFXRateAt(ctx context.Context, currency string, at time.Time) (*service.FinanceFXRateVersion, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" || currency == "USD" {
		return nil, nil
	}
	var item service.FinanceFXRateVersion
	var effectiveTo sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT id,currency,rate_to_usd::text,source,observed_at,effective_from,effective_to,checksum,created_at
FROM finance_fx_rate_versions
WHERE currency=$1 AND effective_from <= $2 AND (effective_to IS NULL OR effective_to > $2)
ORDER BY effective_from DESC,id DESC LIMIT 1`, currency, at).Scan(&item.ID, &item.Currency, &item.RateToUSD, &item.Source, &item.ObservedAt, &item.EffectiveFrom, &effectiveTo, &item.Checksum, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.EffectiveTo = nullTimePointer(effectiveTo)
	return &item, nil
}

type financeSettlementAllocationKey struct {
	usageLogID int64
	attemptNo  int
}

func NewFinanceSettlementRepository(db *sql.DB) service.FinanceSettlementRepository {
	return &financeSettlementRepository{db: db}
}

const financeSettlementIntervalColumns = `
 id,owner_type,owner_id,account_id,account_finance_profile_id,wallet_id,scope_key,previous_snapshot_id,current_snapshot_id,
period_start,period_end,unit_semantics,currency,fx_rate_version_id,fx_rate_to_usd,fx_source,fx_observed_at,list_cost_delta,actual_cost_delta,observed_multiplier,
status,current_revision,request_count,segment_count,standard_cost_total,allocated_cost_total,
difference_amount,error_summary,settled_at`

func (r *financeSettlementRepository) CreateOrGetSettlementInterval(ctx context.Context, input service.FinanceSettlementIntervalInput) (*service.FinanceSettlementInterval, bool, error) {
	row := r.db.QueryRowContext(ctx, `
INSERT INTO upstream_cost_settlement_intervals(
 owner_type,owner_id,account_id,account_finance_profile_id,wallet_id,scope_key,previous_snapshot_id,current_snapshot_id,
 period_start,period_end,unit_semantics,currency,fx_rate_version_id,fx_rate_to_usd,fx_source,fx_observed_at,list_cost_delta,actual_cost_delta,observed_multiplier,status
) VALUES('account',$1,$1,$2,NULL,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'pending')
ON CONFLICT(scope_key,previous_snapshot_id,current_snapshot_id) DO NOTHING
RETURNING `+financeSettlementIntervalColumns,
		input.AccountID, input.AccountFinanceProfileID, input.ScopeKey, input.PreviousSnapshotID, input.CurrentSnapshotID,
		input.PeriodStart, input.PeriodEnd, input.UnitSemantics, input.Currency,
		input.FXRateVersionID, accountFinanceDecimalArgument(input.FXRateToUSD), nullableString(input.FXSource), input.FXObservedAt,
		accountFinanceDecimalArgument(input.ListCostDelta), input.ActualCostDelta.String(), accountFinanceDecimalArgument(input.ObservedMultiplier))
	interval, err := scanFinanceSettlementInterval(row)
	if err == nil {
		return interval, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	interval, err = scanFinanceSettlementInterval(r.db.QueryRowContext(ctx, `SELECT `+financeSettlementIntervalColumns+`
FROM upstream_cost_settlement_intervals
WHERE scope_key=$1 AND previous_snapshot_id=$2 AND current_snapshot_id=$3`, input.ScopeKey, input.PreviousSnapshotID, input.CurrentSnapshotID))
	return interval, false, err
}

func (r *financeSettlementRepository) ListSettlementSegments(ctx context.Context, interval *service.FinanceSettlementInterval) ([]service.FinanceSettlementSegment, error) {
	return listSettlementSegments(ctx, r.db, interval, false)
}

type settlementRowsQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listSettlementSegments(ctx context.Context, queryer settlementRowsQuerier, interval *service.FinanceSettlementInterval, includeCurrent bool) ([]service.FinanceSettlementSegment, error) {
	if interval == nil || interval.AccountID == nil {
		return nil, service.ErrFinanceSettlementInvalid
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT ufr.usage_log_id,seg.attempt_no,COALESCE(uua.billing_observed_at,uua.completed_at,uua.created_at,ufr.usage_created_at) AS billing_observed_at,
       COALESCE(SUM(NULLIF(item->>'amount_before_multiplier','')::numeric),0)::text AS standard_cost
FROM usage_finance_cost_segments seg
JOIN usage_finance_records ufr ON ufr.id=seg.usage_finance_record_id
LEFT JOIN usage_upstream_attempts uua ON uua.usage_log_id=ufr.usage_log_id AND uua.attempt_no=seg.attempt_no
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(seg.calculation_detail->'items','[]'::jsonb)) item
WHERE seg.account_id=$1
  AND uua.account_finance_profile_id IS NOT DISTINCT FROM $6
  AND COALESCE(uua.billing_observed_at,uua.completed_at,uua.created_at,ufr.usage_created_at) >= $2
  AND COALESCE(uua.billing_observed_at,uua.completed_at,uua.created_at,ufr.usage_created_at) < $3
  AND seg.cost_status IN ('exact','estimated')
  AND COALESCE(seg.pricing_source,'') <> 'manual'
  AND (
    COALESCE(seg.pricing_source,'') <> 'upstream_exact'
    OR ($5::boolean AND EXISTS (
      SELECT 1 FROM usage_cost_settlement_allocations current_allocation
      WHERE current_allocation.settlement_interval_id=$4
        AND current_allocation.usage_log_id=ufr.usage_log_id
        AND current_allocation.attempt_no=seg.attempt_no
        AND current_allocation.invalidated_at IS NULL
    ))
  )
  AND NOT EXISTS (
    SELECT 1 FROM usage_cost_settlement_allocations allocation
    WHERE allocation.usage_log_id=ufr.usage_log_id AND allocation.attempt_no=seg.attempt_no
      AND allocation.invalidated_at IS NULL
      AND ($4::bigint=0 OR allocation.settlement_interval_id<>$4)
  )
GROUP BY ufr.usage_log_id,seg.attempt_no,COALESCE(uua.billing_observed_at,uua.completed_at,uua.created_at,ufr.usage_created_at)
ORDER BY billing_observed_at,ufr.usage_log_id,seg.attempt_no`, *interval.AccountID, interval.PeriodStart, interval.PeriodEnd, interval.ID, includeCurrent, interval.AccountFinanceProfileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.FinanceSettlementSegment, 0)
	for rows.Next() {
		var item service.FinanceSettlementSegment
		var standardCost string
		if err = rows.Scan(&item.UsageLogID, &item.AttemptNo, &item.UsageCreatedAt, &standardCost); err != nil {
			return nil, err
		}
		item.StandardCost, err = decimal.NewFromString(standardCost)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *financeSettlementRepository) MarkSettlementNeedsReview(ctx context.Context, intervalID int64, requestCount, segmentCount int64, standardCost, difference decimal.Decimal, summary string) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE upstream_cost_settlement_intervals
SET status='needs_review',request_count=$2,segment_count=$3,standard_cost_total=$4,difference_amount=$5,
    error_summary=$6,updated_at=NOW()
WHERE id=$1 AND status<>'settled'`, intervalID, requestCount, segmentCount, standardCost.String(), difference.String(), settlementErrorSummary(summary))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}
	return nil
}

func (r *financeSettlementRepository) ListSettlementIntervals(ctx context.Context, filter service.FinanceSettlementListFilter) ([]service.FinanceSettlementInterval, int64, error) {
	var accountID any
	if filter.AccountID != nil {
		accountID = *filter.AccountID
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM upstream_cost_settlement_intervals
WHERE ($1='' OR status=$1) AND ($2::bigint IS NULL OR account_id=$2)`, filter.Status, accountID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT `+financeSettlementIntervalColumns+` FROM upstream_cost_settlement_intervals
WHERE ($1='' OR status=$1) AND ($2::bigint IS NULL OR account_id=$2)
ORDER BY period_end DESC,id DESC LIMIT $3 OFFSET $4`, filter.Status, accountID, filter.PageSize, (filter.Page-1)*filter.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.FinanceSettlementInterval, 0)
	for rows.Next() {
		item, scanErr := scanFinanceSettlementInterval(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *financeSettlementRepository) GetSettlementInterval(ctx context.Context, intervalID int64) (*service.FinanceSettlementInterval, error) {
	item, err := scanFinanceSettlementInterval(r.db.QueryRowContext(ctx, `
SELECT `+financeSettlementIntervalColumns+` FROM upstream_cost_settlement_intervals WHERE id=$1`, intervalID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_NOT_FOUND", Message: "结算区间不存在"}
	}
	return item, err
}

func (r *financeSettlementRepository) ListSettlementAllocations(ctx context.Context, intervalID int64) ([]service.FinanceSettlementAllocationView, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT allocation.id,allocation.settlement_interval_id,allocation.usage_log_id,COALESCE(log.request_id,''),
       allocation.attempt_no,allocation.revision,allocation.standard_cost_weight::text,
       allocation.allocation_rate::text,allocation.allocated_cost::text,
       allocation.invalidated_at,allocation.created_at
FROM usage_cost_settlement_allocations allocation
JOIN usage_logs log ON log.id=allocation.usage_log_id
WHERE allocation.settlement_interval_id=$1
ORDER BY allocation.revision DESC,allocation.usage_log_id,allocation.attempt_no`, intervalID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.FinanceSettlementAllocationView, 0)
	for rows.Next() {
		var item service.FinanceSettlementAllocationView
		var weight, rate, cost string
		var invalidatedAt sql.NullTime
		if err = rows.Scan(&item.ID, &item.SettlementInterval, &item.UsageLogID, &item.RequestID,
			&item.AttemptNo, &item.Revision, &weight, &rate, &cost, &invalidatedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		if item.StandardCostWeight, err = decimal.NewFromString(weight); err != nil {
			return nil, err
		}
		if item.AllocationRate, err = decimal.NewFromString(rate); err != nil {
			return nil, err
		}
		if item.AllocatedCost, err = decimal.NewFromString(cost); err != nil {
			return nil, err
		}
		if invalidatedAt.Valid {
			value := invalidatedAt.Time
			item.InvalidatedAt = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *financeSettlementRepository) ApplySettlement(ctx context.Context, interval *service.FinanceSettlementInterval, result service.FinanceSettlementAllocationResult, auditReason string, operatorID *int64) error {
	if interval == nil || interval.AccountID == nil || len(result.Allocations) == 0 || !result.Difference.IsZero() {
		return service.ErrFinanceSettlementInvalid
	}
	expectedActualCostUSD, _, err := service.FinanceSettlementDeltasUSD(interval)
	if err != nil || !result.ActualCostTotal.Equal(expectedActualCostUSD) || !result.AllocatedTotal.Equal(expectedActualCostUSD) {
		return service.ErrFinanceSettlementInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	var revision int
	if err = tx.QueryRowContext(ctx, `SELECT status,current_revision FROM upstream_cost_settlement_intervals WHERE id=$1 FOR UPDATE`, interval.ID).Scan(&status, &revision); err != nil {
		return err
	}
	if status == service.FinanceSettlementSettled {
		return tx.Commit()
	}
	if err = service.ValidateFinanceSettlementTransition(status, service.FinanceSettlementSettled); err != nil && status != service.FinanceSettlementNeedsReview && status != service.FinanceSettlementFailed {
		return err
	}
	if status != service.FinanceSettlementPending {
		if _, err = tx.ExecContext(ctx, `UPDATE upstream_cost_settlement_intervals SET status='pending',updated_at=NOW() WHERE id=$1`, interval.ID); err != nil {
			return err
		}
	}

	usageIDs := make(map[int64]struct{})
	for _, allocation := range result.Allocations {
		usageIDs[allocation.UsageLogID] = struct{}{}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO usage_cost_settlement_allocations(
 settlement_interval_id,usage_log_id,attempt_no,revision,standard_cost_weight,allocation_rate,allocated_cost
) VALUES($1,$2,$3,$4,$5,$6,$7)`, interval.ID, allocation.UsageLogID, allocation.AttemptNo, revision, allocation.StandardCost.String(), allocation.AllocationRate.String(), allocation.AllocatedCost.String()); err != nil {
			return financeSettlementWriteError(err)
		}
		update, updateErr := tx.ExecContext(ctx, `
UPDATE usage_finance_cost_segments seg
SET cost_amount=$4,cost_status='exact',pricing_source='upstream_exact',
	fx_rate_version_id=$10::bigint,source_currency=$11::text,fx_rate_to_usd=$12::numeric,fx_source=$13::text,fx_observed_at=$14::timestamptz,
    calculation_detail=(CASE
      WHEN COALESCE(seg.calculation_detail,'{}'::jsonb) ? 'pre_settlement_cost_status' THEN COALESCE(seg.calculation_detail,'{}'::jsonb)
      ELSE COALESCE(seg.calculation_detail,'{}'::jsonb) || jsonb_build_object(
        'pre_settlement_cost_amount',seg.cost_amount,'pre_settlement_cost_status',seg.cost_status,
		'pre_settlement_pricing_source',seg.pricing_source,
		'pre_settlement_fx_rate_version_id',seg.fx_rate_version_id,'pre_settlement_source_currency',seg.source_currency,
		'pre_settlement_fx_rate_to_usd',seg.fx_rate_to_usd,'pre_settlement_fx_source',seg.fx_source,
		'pre_settlement_fx_observed_at',seg.fx_observed_at
      ) END) || jsonb_build_object(
      'settlement_interval_id',$5::bigint,'settlement_revision',$6::integer,'settlement_observed_multiplier',$7::numeric,
	  'settlement_standard_cost_weight',$8::numeric,'settlement_allocation_rate',$9::numeric,
	  'settlement_currency',$11::text,'settlement_actual_cost_delta_original',$15::numeric,
	  'settlement_list_cost_delta_original',$16::numeric,'settlement_actual_cost_delta_usd',$17::numeric,
	  'settlement_allocated_cost_usd',$4::numeric,'settlement_fx_rate_version_id',$10::bigint,
	  'settlement_fx_rate_to_usd',$12::numeric,'settlement_fx_source',$13::text,'settlement_fx_observed_at',$14::timestamptz
    )
FROM usage_finance_records ufr
WHERE ufr.id=seg.usage_finance_record_id AND ufr.usage_log_id=$1 AND seg.attempt_no=$2 AND seg.account_id=$3`,
			allocation.UsageLogID, allocation.AttemptNo, *interval.AccountID, allocation.AllocatedCost.String(), interval.ID, revision,
			accountFinanceDecimalArgument(interval.ObservedMultiplier), allocation.StandardCost.String(), allocation.AllocationRate.String(),
			interval.FXRateVersionID, dereferenceRepositoryString(interval.Currency), accountFinanceDecimalArgument(interval.FXRateToUSD),
			nullableString(interval.FXSource), interval.FXObservedAt, interval.ActualCostDelta.String(), accountFinanceDecimalArgument(interval.ListCostDelta), result.ActualCostTotal.String())
		if updateErr != nil {
			return updateErr
		}
		affected, affectedErr := update.RowsAffected()
		if affectedErr != nil {
			return affectedErr
		}
		if affected != 1 {
			return fmt.Errorf("settlement target usage_log_id=%d attempt_no=%d changed concurrently", allocation.UsageLogID, allocation.AttemptNo)
		}
	}

	orderedUsageIDs := make([]int64, 0, len(usageIDs))
	for usageID := range usageIDs {
		orderedUsageIDs = append(orderedUsageIDs, usageID)
	}
	sort.Slice(orderedUsageIDs, func(i, j int) bool { return orderedUsageIDs[i] < orderedUsageIDs[j] })
	for _, usageID := range orderedUsageIDs {
		if err = reviseSettledFinanceRecord(ctx, tx, usageID, interval.ID, revision, auditReason, operatorID); err != nil {
			return err
		}
	}
	settledAt := time.Now().UTC()
	requestCount := int64(len(orderedUsageIDs))
	update, err := tx.ExecContext(ctx, `
UPDATE upstream_cost_settlement_intervals
SET status='settled',request_count=$2,segment_count=$3,standard_cost_total=$4,
    allocated_cost_total=$5,difference_amount=$6,error_summary='',settled_at=$7,updated_at=$7
WHERE id=$1 AND status='pending'`, interval.ID, requestCount, len(result.Allocations), result.StandardCostTotal.String(), result.AllocatedTotal.String(), result.Difference.String(), settledAt)
	if err != nil {
		return err
	}
	affected, err := update.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("settlement interval %d changed concurrently", interval.ID)
	}
	return tx.Commit()
}

func (r *financeSettlementRepository) ReallocateSettlement(ctx context.Context, intervalID int64, expectedRevision int, reason string, operatorID int64) (*service.FinanceSettlementInterval, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	interval, err := scanFinanceSettlementInterval(tx.QueryRowContext(ctx, `
SELECT `+financeSettlementIntervalColumns+` FROM upstream_cost_settlement_intervals WHERE id=$1 FOR UPDATE`, intervalID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_NOT_FOUND", Message: "结算区间不存在"}
	}
	if err != nil {
		return nil, err
	}
	if interval.Status != service.FinanceSettlementSettled || interval.CurrentRevision != expectedRevision {
		return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "结算区间状态或版本已变化，请刷新后重试"}
	}
	if interval.UnitSemantics != service.AccountFinanceUnitFiatCurrency || interval.AccountID == nil {
		return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "该区间不支持成本重新分摊"}
	}
	segments, err := listSettlementSegments(ctx, tx, interval, true)
	if err != nil {
		return nil, err
	}
	actualCostUSD, listCostUSD, err := service.FinanceSettlementDeltasUSD(interval)
	if err != nil {
		return nil, err
	}
	result, err := service.AllocateFinanceSettlement(actualCostUSD, segments)
	if err != nil {
		return nil, err
	}
	if listCostUSD != nil {
		difference := result.StandardCostTotal.Sub(*listCostUSD).Round(10)
		tolerance := listCostUSD.Abs().Mul(decimal.RequireFromString("0.0001"))
		if tolerance.LessThan(decimal.RequireFromString("0.000001")) {
			tolerance = decimal.RequireFromString("0.000001")
		}
		if difference.Abs().GreaterThan(tolerance) {
			return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: fmt.Sprintf("当前标准成本 USD %s 与上游区间原价 USD %s 不一致", result.StandardCostTotal.String(), listCostUSD.String())}
		}
	}
	now := time.Now().UTC()
	nextRevision := expectedRevision + 1
	oldAllocationKeys := make(map[financeSettlementAllocationKey]struct{})
	oldRows, oldRowsErr := tx.QueryContext(ctx, `
SELECT usage_log_id,attempt_no FROM usage_cost_settlement_allocations
WHERE settlement_interval_id=$1 AND invalidated_at IS NULL FOR UPDATE`, intervalID)
	if oldRowsErr != nil {
		return nil, oldRowsErr
	}
	for oldRows.Next() {
		var key financeSettlementAllocationKey
		if err = oldRows.Scan(&key.usageLogID, &key.attemptNo); err != nil {
			_ = oldRows.Close()
			return nil, err
		}
		oldAllocationKeys[key] = struct{}{}
	}
	if err = oldRows.Err(); err != nil {
		_ = oldRows.Close()
		return nil, err
	}
	if err = oldRows.Close(); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE usage_cost_settlement_allocations SET invalidated_at=$2
WHERE settlement_interval_id=$1 AND invalidated_at IS NULL`, intervalID, now); err != nil {
		return nil, err
	}
	usageIDs := make(map[int64]struct{})
	newAllocationKeys := make(map[financeSettlementAllocationKey]struct{}, len(result.Allocations))
	for _, allocation := range result.Allocations {
		usageIDs[allocation.UsageLogID] = struct{}{}
		newAllocationKeys[financeSettlementAllocationKey{usageLogID: allocation.UsageLogID, attemptNo: allocation.AttemptNo}] = struct{}{}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO usage_cost_settlement_allocations(
 settlement_interval_id,usage_log_id,attempt_no,revision,standard_cost_weight,allocation_rate,allocated_cost
) VALUES($1,$2,$3,$4,$5,$6,$7)`, intervalID, allocation.UsageLogID, allocation.AttemptNo, nextRevision,
			allocation.StandardCost.String(), allocation.AllocationRate.String(), allocation.AllocatedCost.String()); err != nil {
			return nil, financeSettlementWriteError(err)
		}
		update, updateErr := tx.ExecContext(ctx, `
UPDATE usage_finance_cost_segments seg
SET cost_amount=$4,cost_status='exact',pricing_source='upstream_exact',
	fx_rate_version_id=$10::bigint,source_currency=$11::text,fx_rate_to_usd=$12::numeric,fx_source=$13::text,fx_observed_at=$14::timestamptz,
    calculation_detail=(CASE
      WHEN COALESCE(seg.calculation_detail,'{}'::jsonb) ? 'pre_settlement_cost_status' THEN COALESCE(seg.calculation_detail,'{}'::jsonb)
      ELSE COALESCE(seg.calculation_detail,'{}'::jsonb) || jsonb_build_object(
        'pre_settlement_cost_amount',seg.cost_amount,'pre_settlement_cost_status',seg.cost_status,
		'pre_settlement_pricing_source',seg.pricing_source,
		'pre_settlement_fx_rate_version_id',seg.fx_rate_version_id,'pre_settlement_source_currency',seg.source_currency,
		'pre_settlement_fx_rate_to_usd',seg.fx_rate_to_usd,'pre_settlement_fx_source',seg.fx_source,
		'pre_settlement_fx_observed_at',seg.fx_observed_at
      ) END) || jsonb_build_object(
      'settlement_interval_id',$5::bigint,'settlement_revision',$6::integer,'settlement_observed_multiplier',$7::numeric,
	  'settlement_standard_cost_weight',$8::numeric,'settlement_allocation_rate',$9::numeric,
	  'settlement_currency',$11::text,'settlement_actual_cost_delta_original',$15::numeric,
	  'settlement_list_cost_delta_original',$16::numeric,'settlement_actual_cost_delta_usd',$17::numeric,
	  'settlement_allocated_cost_usd',$4::numeric,'settlement_fx_rate_version_id',$10::bigint,
	  'settlement_fx_rate_to_usd',$12::numeric,'settlement_fx_source',$13::text,'settlement_fx_observed_at',$14::timestamptz
    )
FROM usage_finance_records ufr
WHERE ufr.id=seg.usage_finance_record_id AND ufr.usage_log_id=$1 AND seg.attempt_no=$2 AND seg.account_id=$3`,
			allocation.UsageLogID, allocation.AttemptNo, *interval.AccountID, allocation.AllocatedCost.String(), interval.ID,
			nextRevision, accountFinanceDecimalArgument(interval.ObservedMultiplier), allocation.StandardCost.String(), allocation.AllocationRate.String(),
			interval.FXRateVersionID, dereferenceRepositoryString(interval.Currency), accountFinanceDecimalArgument(interval.FXRateToUSD),
			nullableString(interval.FXSource), interval.FXObservedAt, interval.ActualCostDelta.String(), accountFinanceDecimalArgument(interval.ListCostDelta), result.ActualCostTotal.String())
		if updateErr != nil {
			return nil, updateErr
		}
		affected, affectedErr := update.RowsAffected()
		if affectedErr != nil {
			return nil, affectedErr
		}
		if affected != 1 {
			return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "分摊目标已变化，请刷新后重试"}
		}
	}
	for key := range oldAllocationKeys {
		if _, retained := newAllocationKeys[key]; retained {
			continue
		}
		restore, restoreErr := tx.ExecContext(ctx, `
UPDATE usage_finance_cost_segments seg
SET cost_amount=NULLIF(seg.calculation_detail->>'pre_settlement_cost_amount','')::numeric,
    cost_status=COALESCE(NULLIF(seg.calculation_detail->>'pre_settlement_cost_status',''),seg.cost_status),
    pricing_source=COALESCE(NULLIF(seg.calculation_detail->>'pre_settlement_pricing_source',''),seg.pricing_source),
	fx_rate_version_id=NULLIF(seg.calculation_detail->>'pre_settlement_fx_rate_version_id','')::bigint,
	source_currency=NULLIF(seg.calculation_detail->>'pre_settlement_source_currency',''),
	fx_rate_to_usd=NULLIF(seg.calculation_detail->>'pre_settlement_fx_rate_to_usd','')::numeric,
	fx_source=NULLIF(seg.calculation_detail->>'pre_settlement_fx_source',''),
	fx_observed_at=NULLIF(seg.calculation_detail->>'pre_settlement_fx_observed_at','')::timestamptz,
    calculation_detail=COALESCE(seg.calculation_detail,'{}'::jsonb)
      - 'settlement_interval_id' - 'settlement_revision' - 'settlement_observed_multiplier'
	  - 'settlement_standard_cost_weight' - 'settlement_allocation_rate' - 'settlement_currency'
	  - 'settlement_actual_cost_delta_original' - 'settlement_list_cost_delta_original'
	  - 'settlement_actual_cost_delta_usd' - 'settlement_allocated_cost_usd'
	  - 'settlement_fx_rate_version_id' - 'settlement_fx_rate_to_usd' - 'settlement_fx_source' - 'settlement_fx_observed_at'
      - 'pre_settlement_cost_amount' - 'pre_settlement_cost_status' - 'pre_settlement_pricing_source'
	  - 'pre_settlement_fx_rate_version_id' - 'pre_settlement_source_currency'
	  - 'pre_settlement_fx_rate_to_usd' - 'pre_settlement_fx_source' - 'pre_settlement_fx_observed_at'
FROM usage_finance_records ufr
WHERE ufr.id=seg.usage_finance_record_id AND ufr.usage_log_id=$1 AND seg.attempt_no=$2
  AND seg.account_id=$3 AND (seg.calculation_detail->>'settlement_interval_id')::bigint=$4`,
			key.usageLogID, key.attemptNo, *interval.AccountID, interval.ID)
		if restoreErr != nil {
			return nil, restoreErr
		}
		restored, rowsErr := restore.RowsAffected()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if restored != 1 {
			return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "旧分摊目标已变化，请刷新后重试"}
		}
		usageIDs[key.usageLogID] = struct{}{}
	}
	var activeAllocatedText string
	var activeCount int
	if err = tx.QueryRowContext(ctx, `
SELECT COALESCE(SUM(allocated_cost),0)::text,COUNT(*)::int
FROM usage_cost_settlement_allocations
WHERE settlement_interval_id=$1 AND invalidated_at IS NULL`, intervalID).Scan(&activeAllocatedText, &activeCount); err != nil {
		return nil, err
	}
	activeAllocated, parseErr := decimal.NewFromString(activeAllocatedText)
	if parseErr != nil {
		return nil, parseErr
	}
	if activeCount != len(result.Allocations) || !activeAllocated.Equal(actualCostUSD) {
		return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "有效分摊未满足金额守恒"}
	}
	orderedUsageIDs := make([]int64, 0, len(usageIDs))
	for usageID := range usageIDs {
		orderedUsageIDs = append(orderedUsageIDs, usageID)
	}
	sort.Slice(orderedUsageIDs, func(i, j int) bool { return orderedUsageIDs[i] < orderedUsageIDs[j] })
	for _, usageID := range orderedUsageIDs {
		if err = reviseSettledFinanceRecord(ctx, tx, usageID, interval.ID, nextRevision, reason, &operatorID); err != nil {
			return nil, err
		}
	}
	update, err := tx.ExecContext(ctx, `
UPDATE upstream_cost_settlement_intervals
SET current_revision=$3,request_count=$4,segment_count=$5,standard_cost_total=$6,
    allocated_cost_total=$7,difference_amount=$8,error_summary='',settled_at=$9,updated_at=$9
WHERE id=$1 AND status='settled' AND current_revision=$2`, intervalID, expectedRevision, nextRevision,
		len(orderedUsageIDs), len(result.Allocations), result.StandardCostTotal.String(), result.AllocatedTotal.String(), result.Difference.String(), now)
	if err != nil {
		return nil, err
	}
	affected, err := update.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "结算区间版本已变化，请刷新后重试"}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetSettlementInterval(ctx, intervalID)
}

func reviseSettledFinanceRecord(ctx context.Context, tx *sql.Tx, usageLogID, settlementIntervalID int64, settlementRevision int, reason string, operatorID *int64) error {
	var recordID int64
	var currentRevision int
	var oldSnapshot []byte
	if err := tx.QueryRowContext(ctx, financeSettlementSnapshotSQL, usageLogID).Scan(&recordID, &currentRevision, &oldSnapshot); err != nil {
		return err
	}
	var nextRevision int
	if err := tx.QueryRowContext(ctx, `
WITH aggregate AS (
 SELECT usage_finance_record_id,
        COUNT(*) FILTER (WHERE cost_amount IS NULL) AS unknown_count,
        COUNT(*) FILTER (WHERE cost_status='estimated') AS estimated_count,
        COUNT(*) FILTER (WHERE cost_status='exact') AS exact_count,
        COALESCE(SUM(cost_amount),0) AS known_cost
 FROM usage_finance_cost_segments WHERE usage_finance_record_id=$1 GROUP BY usage_finance_record_id
)
UPDATE usage_finance_records ufr
SET upstream_cost=CASE WHEN aggregate.unknown_count=0 THEN aggregate.known_cost ELSE NULL END,
    cost_status=CASE
      WHEN aggregate.unknown_count>0 THEN ufr.cost_status
      WHEN aggregate.estimated_count>0 THEN 'estimated'
      WHEN aggregate.exact_count>0 THEN 'exact'
      ELSE ufr.cost_status END,
    pricing_source=CASE WHEN aggregate.unknown_count=0 AND aggregate.estimated_count=0 THEN 'upstream_exact' ELSE ufr.pricing_source END,
    current_revision=ufr.current_revision+1,
    calculation_detail=COALESCE(ufr.calculation_detail,'{}'::jsonb) || jsonb_build_object(
      'settlement_interval_id',$2::bigint,'settlement_revision',$3::integer
    ),
    calculated_at=NOW()
FROM aggregate WHERE ufr.id=aggregate.usage_finance_record_id
RETURNING ufr.current_revision`, recordID, settlementIntervalID, settlementRevision).Scan(&nextRevision); err != nil {
		return err
	}
	if nextRevision != currentRevision+1 {
		return fmt.Errorf("finance record %d revision changed concurrently", recordID)
	}
	var newSnapshot []byte
	if err := tx.QueryRowContext(ctx, financeSettlementSnapshotSQL, usageLogID).Scan(&recordID, &nextRevision, &newSnapshot); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		reason = fmt.Sprintf("cumulative upstream settlement interval_id=%d revision=%d", settlementIntervalID, settlementRevision)
	} else if reason == "manual settlement retry" {
		reason = fmt.Sprintf("manual settlement retry interval_id=%d revision=%d", settlementIntervalID, settlementRevision)
	} else {
		reason = fmt.Sprintf("manual settlement reallocation interval_id=%d revision=%d: %s", settlementIntervalID, settlementRevision, strings.TrimSpace(reason))
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO finance_calculation_revisions(entity_type,entity_id,revision,old_result,new_result,reason,operator_id,created_at)
VALUES('usage_finance_record',$1,$2,$3::jsonb,$4::jsonb,$5,$6,NOW())`, recordID, nextRevision, oldSnapshot, newSnapshot, reason, operatorID)
	return err
}

func financeSettlementWriteError(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr != nil && pqErr.Code == "23505" {
		return &service.FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "请求已属于其他有效结算区间，请刷新后重试"}
	}
	return err
}

const financeSettlementSnapshotSQL = `
SELECT ufr.id,ufr.current_revision,
       jsonb_build_object(
         'usage_finance_record',to_jsonb(ufr),
         'segments',COALESCE((
           SELECT jsonb_agg(to_jsonb(seg) ORDER BY seg.attempt_no)
           FROM usage_finance_cost_segments seg WHERE seg.usage_finance_record_id=ufr.id
         ),'[]'::jsonb)
       )
FROM usage_finance_records ufr WHERE ufr.usage_log_id=$1 FOR UPDATE`

func scanFinanceSettlementInterval(scanner interface{ Scan(...any) error }) (*service.FinanceSettlementInterval, error) {
	item := &service.FinanceSettlementInterval{}
	var accountID, accountFinanceProfileID, walletID sql.NullInt64
	var currency, fxSource sql.NullString
	var fxVersionID sql.NullInt64
	var fxRate, listCost, multiplier sql.NullString
	var fxObservedAt sql.NullTime
	var actualCost string
	var standardCost, allocatedCost, difference sql.NullString
	var settledAt sql.NullTime
	if err := scanner.Scan(
		&item.ID, &item.OwnerType, &item.OwnerID, &accountID, &accountFinanceProfileID, &walletID, &item.ScopeKey,
		&item.PreviousSnapshotID, &item.CurrentSnapshotID, &item.PeriodStart, &item.PeriodEnd,
		&item.UnitSemantics, &currency, &fxVersionID, &fxRate, &fxSource, &fxObservedAt, &listCost, &actualCost, &multiplier, &item.Status,
		&item.CurrentRevision, &item.RequestCount, &item.SegmentCount, &standardCost,
		&allocatedCost, &difference, &item.ErrorSummary, &settledAt,
	); err != nil {
		return nil, err
	}
	item.AccountID = nullableInt64Pointer(accountID)
	item.AccountFinanceProfileID = nullableInt64Pointer(accountFinanceProfileID)
	item.WalletID = nullableInt64Pointer(walletID)
	item.FXRateVersionID = nullableInt64Pointer(fxVersionID)
	if currency.Valid {
		value := currency.String
		item.Currency = &value
	}
	if fxSource.Valid {
		item.FXSource = fxSource.String
	}
	if fxObservedAt.Valid {
		value := fxObservedAt.Time
		item.FXObservedAt = &value
	}
	var err error
	if listCost.Valid {
		value, parseErr := decimal.NewFromString(listCost.String)
		if parseErr != nil {
			return nil, parseErr
		}
		item.ListCostDelta = &value
	}
	if item.ActualCostDelta, err = decimal.NewFromString(actualCost); err != nil {
		return nil, err
	}
	if multiplier.Valid {
		value, parseErr := decimal.NewFromString(multiplier.String)
		if parseErr != nil {
			return nil, parseErr
		}
		item.ObservedMultiplier = &value
	}
	if fxRate.Valid {
		value, parseErr := decimal.NewFromString(fxRate.String)
		if parseErr != nil {
			return nil, parseErr
		}
		item.FXRateToUSD = &value
	}
	if item.StandardCostTotal, err = settlementNullableDecimal(standardCost); err != nil {
		return nil, err
	}
	if item.AllocatedCostTotal, err = settlementNullableDecimal(allocatedCost); err != nil {
		return nil, err
	}
	if item.DifferenceAmount, err = settlementNullableDecimal(difference); err != nil {
		return nil, err
	}
	if settledAt.Valid {
		value := settledAt.Time
		item.SettledAt = &value
	}
	return item, nil
}

func settlementNullableDecimal(value sql.NullString) (*decimal.Decimal, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func settlementErrorSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}
