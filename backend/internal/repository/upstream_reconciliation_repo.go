package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

const (
	financeReconciliationAlertAmountThreshold = "1"
	financeReconciliationAlertRateThreshold   = "0.01"
)

type financeReconciliationRepository struct{ db *sql.DB }

func NewFinanceReconciliationRepository(db *sql.DB) service.FinanceReconciliationRepository {
	return &financeReconciliationRepository{db: db}
}

func (r *financeReconciliationRepository) ImportFinanceReconciliation(
	ctx context.Context,
	input service.FinanceReconciliationImportInput,
	now time.Time,
) (*service.FinanceReconciliationImportResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var walletExists bool
	if err = tx.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM upstream_wallets WHERE id=$1 AND deleted_at IS NULL)`, input.WalletID).Scan(&walletExists); err != nil {
		return nil, fmt.Errorf("check upstream wallet: %w", err)
	}
	if !walletExists {
		return nil, service.ErrUpstreamWalletNotFound
	}

	var systemCostRaw string
	if err = tx.QueryRowContext(ctx, `
SELECT COALESCE(SUM(upstream_cost),0)::text
FROM usage_finance_records
WHERE wallet_id=$1 AND usage_created_at >= $2 AND usage_created_at < $3
  AND cost_status='exact'`, input.WalletID, input.PeriodStart, input.PeriodEnd).Scan(&systemCostRaw); err != nil {
		return nil, fmt.Errorf("summarize exact upstream cost: %w", err)
	}
	systemCost, err := decimal.NewFromString(systemCostRaw)
	if err != nil {
		return nil, fmt.Errorf("parse exact upstream cost: %w", err)
	}
	difference := input.UpstreamBillAmount.Sub(systemCost)
	var differenceRate *decimal.Decimal
	if !input.UpstreamBillAmount.IsZero() {
		value := difference.Abs().Div(input.UpstreamBillAmount.Abs())
		differenceRate = &value
	}
	status := service.FinanceReconciliationMatched
	if !difference.IsZero() {
		status = service.FinanceReconciliationDifference
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO upstream_bill_reconciliations (
 wallet_id,period_start,period_end,upstream_bill_amount,system_cost_amount,difference_amount,
 difference_rate,currency,source_reference,source_file_name,source_file_checksum,status,created_at,updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$13)
ON CONFLICT (wallet_id,period_start,period_end,source_file_checksum) DO NOTHING
RETURNING id`,
		input.WalletID, input.PeriodStart, input.PeriodEnd, input.UpstreamBillAmount.String(), systemCost.String(), difference.String(),
		decimalPointerValue(differenceRate), input.Currency, input.SourceReference, input.SourceFileName, input.SourceFileChecksum, status, now,
	).Scan(&id)
	duplicate := false
	if errors.Is(err, sql.ErrNoRows) {
		duplicate = true
		if err = tx.QueryRowContext(ctx, `
SELECT id FROM upstream_bill_reconciliations
WHERE wallet_id=$1 AND period_start=$2 AND period_end=$3 AND source_file_checksum=$4`,
			input.WalletID, input.PeriodStart, input.PeriodEnd, input.SourceFileChecksum).Scan(&id); err != nil {
			return nil, fmt.Errorf("load duplicate reconciliation: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("insert upstream reconciliation: %w", err)
	}

	if !duplicate && status == service.FinanceReconciliationDifference && financeReconciliationNeedsAlert(difference, differenceRate) {
		if err = upsertFinanceReconciliationAlert(ctx, tx, input.WalletID, difference, now); err != nil {
			return nil, err
		}
	}

	item, err := getFinanceReconciliation(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	jobID, err := ensureFinanceReconciliationJob(ctx, tx, id, input, now)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &service.FinanceReconciliationImportResult{
		Reconciliation: *item,
		JobID:          jobID,
		JobStatus:      "completed",
		Duplicate:      duplicate,
	}, nil
}

func (r *financeReconciliationRepository) ListFinanceReconciliations(
	ctx context.Context,
	request service.FinanceReconciliationListRequest,
) ([]service.FinanceReconciliation, int64, error) {
	args := make([]any, 0, 8)
	where := []string{"1=1"}
	if request.StartAt != nil {
		args = append(args, *request.StartAt)
		where = append(where, fmt.Sprintf("ubr.period_start >= $%d", len(args)))
	}
	if request.EndBefore != nil {
		args = append(args, *request.EndBefore)
		where = append(where, fmt.Sprintf("ubr.period_start < $%d", len(args)))
	}
	if request.UpstreamID != nil {
		args = append(args, *request.UpstreamID)
		where = append(where, fmt.Sprintf("uw.upstream_id = $%d", len(args)))
	}
	if request.WalletID != nil {
		args = append(args, *request.WalletID)
		where = append(where, fmt.Sprintf("ubr.wallet_id = $%d", len(args)))
	}
	if request.Status != "" {
		args = append(args, request.Status)
		where = append(where, fmt.Sprintf("ubr.status = $%d", len(args)))
	}
	args = append(args, request.PageSize, (request.Page-1)*request.PageSize)
	query := financeReconciliationSelect + fmt.Sprintf(`
WHERE %s
ORDER BY ubr.period_start DESC, ubr.id DESC
LIMIT $%d OFFSET $%d`, strings.Join(where, " AND "), len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list finance reconciliations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.FinanceReconciliation, 0)
	var total int64
	for rows.Next() {
		item, scanErr := scanFinanceReconciliation(rows, &total)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *financeReconciliationRepository) UpdateFinanceReconciliationStatus(
	ctx context.Context,
	id int64,
	status, note string,
	actorID int64,
	now time.Time,
) (*service.FinanceReconciliation, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var oldStatus string
	var oldHandledBy sql.NullInt64
	var oldHandledNote string
	var oldHandledAt sql.NullTime
	if err = tx.QueryRowContext(ctx, `
SELECT status,handled_by,COALESCE(handled_note,''),handled_at
FROM upstream_bill_reconciliations
WHERE id=$1
FOR UPDATE`, id).Scan(&oldStatus, &oldHandledBy, &oldHandledNote, &oldHandledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrFinanceReconciliationNotFound
		}
		return nil, fmt.Errorf("lock finance reconciliation: %w", err)
	}
	var handledBy any
	if actorID > 0 {
		handledBy = actorID
	}
	result, err := tx.ExecContext(ctx, `
UPDATE upstream_bill_reconciliations
SET status=$2,handled_by=$3,handled_note=$4,handled_at=$5,updated_at=$5
WHERE id=$1`, id, status, handledBy, note, now)
	if err != nil {
		return nil, fmt.Errorf("update finance reconciliation status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, service.ErrFinanceReconciliationNotFound
	}
	oldResult := map[string]any{
		"status":       oldStatus,
		"handled_note": oldHandledNote,
	}
	if oldHandledBy.Valid {
		oldResult["handled_by"] = oldHandledBy.Int64
	}
	if oldHandledAt.Valid {
		oldResult["handled_at"] = oldHandledAt.Time.UTC()
	}
	newResult := map[string]any{
		"status":       status,
		"handled_note": note,
		"handled_at":   now.UTC(),
	}
	if actorID > 0 {
		newResult["handled_by"] = actorID
	}
	oldJSON, err := json.Marshal(oldResult)
	if err != nil {
		return nil, err
	}
	newJSON, err := json.Marshal(newResult)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO finance_calculation_revisions(
 entity_type,entity_id,revision,old_result,new_result,reason,operator_id,created_at
)
SELECT 'upstream_bill_reconciliation',$1,
       COALESCE(MAX(revision),0)+1,$2::jsonb,$3::jsonb,$4,$5,$6
FROM finance_calculation_revisions
WHERE entity_type='upstream_bill_reconciliation' AND entity_id=$1`,
		id, string(oldJSON), string(newJSON), "reconciliation status update: "+status, handledBy, now)
	if err != nil {
		return nil, fmt.Errorf("append finance reconciliation audit: %w", err)
	}
	item, err := getFinanceReconciliation(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

const financeReconciliationSelect = `
SELECT ubr.id,ubr.wallet_id,uw.name,uw.upstream_id,u.name,
       ubr.period_start,ubr.period_end,ubr.upstream_bill_amount::text,ubr.system_cost_amount::text,
       ubr.difference_amount::text,ubr.difference_rate::text,ubr.currency,
       COALESCE(ubr.source_reference,''),COALESCE(ubr.source_file_name,''),ubr.source_file_checksum,
       ubr.status,ubr.handled_by,COALESCE(ubr.handled_note,''),ubr.handled_at,ubr.created_at,ubr.updated_at,
       COUNT(*) OVER()::bigint
FROM upstream_bill_reconciliations ubr
JOIN upstream_wallets uw ON uw.id=ubr.wallet_id
JOIN upstreams u ON u.id=uw.upstream_id
`

type financeReconciliationQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getFinanceReconciliation(ctx context.Context, queryer financeReconciliationQueryer, id int64) (*service.FinanceReconciliation, error) {
	item, err := scanFinanceReconciliation(queryer.QueryRowContext(ctx, financeReconciliationSelect+" WHERE ubr.id=$1", id), nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrFinanceReconciliationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get finance reconciliation: %w", err)
	}
	return &item, nil
}

type financeReconciliationScanner interface{ Scan(...any) error }

func scanFinanceReconciliation(scanner financeReconciliationScanner, total *int64) (service.FinanceReconciliation, error) {
	var item service.FinanceReconciliation
	var differenceRate sql.NullString
	var handledBy sql.NullInt64
	var handledAt sql.NullTime
	var rowTotal int64
	if err := scanner.Scan(
		&item.ID, &item.WalletID, &item.WalletName, &item.UpstreamID, &item.UpstreamName,
		&item.PeriodStart, &item.PeriodEnd, &item.UpstreamBillAmount, &item.SystemCostAmount,
		&item.DifferenceAmount, &differenceRate, &item.Currency, &item.SourceReference, &item.SourceFileName,
		&item.SourceFileChecksum, &item.Status, &handledBy, &item.HandledNote, &handledAt,
		&item.CreatedAt, &item.UpdatedAt, &rowTotal,
	); err != nil {
		return item, err
	}
	item.DifferenceRate = nullableStringPointer(differenceRate)
	item.HandledBy = nullableInt64Pointer(handledBy)
	if handledAt.Valid {
		value := handledAt.Time
		item.HandledAt = &value
	}
	if total != nil {
		*total = rowTotal
	}
	return item, nil
}

func financeReconciliationNeedsAlert(difference decimal.Decimal, rate *decimal.Decimal) bool {
	amountThreshold := decimal.RequireFromString(financeReconciliationAlertAmountThreshold)
	rateThreshold := decimal.RequireFromString(financeReconciliationAlertRateThreshold)
	return difference.Abs().GreaterThan(amountThreshold) || (rate != nil && rate.GreaterThan(rateThreshold))
}

func upsertFinanceReconciliationAlert(ctx context.Context, tx *sql.Tx, walletID int64, difference decimal.Decimal, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO finance_alerts (
 alert_type,severity,aggregation_key,title,description,dimension_type,dimension_id,impact_amount,request_count,
 occurrence_count,status,first_occurred_at,last_occurred_at,created_at,updated_at
) VALUES ('reconciliation_difference','warning',$1,'上游账单存在未处理差额','上游账单与系统确认成本存在差异',
          'wallet',$2,$3,1,1,'open',$4,$4,$4,$4)
ON CONFLICT (aggregation_key) WHERE status IN ('open','acknowledged') DO UPDATE SET
 impact_amount=COALESCE(finance_alerts.impact_amount,0)+EXCLUDED.impact_amount,
 request_count=finance_alerts.request_count+1,
 occurrence_count=finance_alerts.occurrence_count+1,
 last_occurred_at=EXCLUDED.last_occurred_at,updated_at=EXCLUDED.updated_at`,
		fmt.Sprintf("reconciliation_difference:%d", walletID), walletID, difference.Abs().String(), now)
	if err != nil {
		return fmt.Errorf("upsert finance reconciliation alert: %w", err)
	}
	return nil
}

func ensureFinanceReconciliationJob(ctx context.Context, tx *sql.Tx, reconciliationID int64, input service.FinanceReconciliationImportInput, now time.Time) (int64, error) {
	var jobID int64
	err := tx.QueryRowContext(ctx, `
SELECT id
FROM finance_async_jobs
WHERE job_type='upstream_bill_reconciliation'
  AND request_checksum=$1
  AND parameters->>'reconciliation_id'=$2
ORDER BY id
LIMIT 1`, input.SourceFileChecksum, fmt.Sprintf("%d", reconciliationID)).Scan(&jobID)
	if err == nil {
		return jobID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find reconciliation import job: %w", err)
	}
	var operatorID any
	if input.OperatorID > 0 {
		operatorID = input.OperatorID
	}
	idempotencyKey := fmt.Sprintf("%d:%s:%s:%s", input.WalletID, input.PeriodStart.UTC().Format(time.RFC3339), input.PeriodEnd.UTC().Format(time.RFC3339), input.SourceFileChecksum)
	err = tx.QueryRowContext(ctx, `
INSERT INTO finance_async_jobs (
 job_type,status,idempotency_key,request_checksum,parameters,progress,
 processed_count,success_count,operator_id,created_at,started_at,finished_at,updated_at
) VALUES (
 'upstream_bill_reconciliation','completed',$1,$2,
 jsonb_build_object('reconciliation_id',$3::bigint,'wallet_id',$4::bigint,'period_start',$5::text,'period_end',$6::text),
 1,1,1,$7,$8,$8,$8,$8
) RETURNING id`, idempotencyKey, input.SourceFileChecksum, reconciliationID, input.WalletID,
		input.PeriodStart.UTC().Format(time.RFC3339), input.PeriodEnd.UTC().Format(time.RFC3339), operatorID, now).Scan(&jobID)
	if err != nil {
		return 0, fmt.Errorf("create reconciliation import job: %w", err)
	}
	return jobID, nil
}

func decimalPointerValue(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return value.String()
}
