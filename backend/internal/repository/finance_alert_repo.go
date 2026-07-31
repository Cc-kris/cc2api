package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type financeAlertRepository struct{ db *sql.DB }

func NewFinanceAlertRepository(db *sql.DB) service.FinanceAlertRepository {
	return &financeAlertRepository{db: db}
}

func (r *financeAlertRepository) CollectFinanceAlertSignals(ctx context.Context, now time.Time) ([]service.FinanceAlertSignal, error) {
	windowStart := now.Add(-24 * time.Hour)
	rows, err := r.db.QueryContext(ctx, `
WITH revenue AS (
 SELECT ufr.usage_log_id,ufr.cost_status,ufr.upstream_cost,
        COALESCE((SELECT SUM(ura.allocated_amount) FROM usage_revenue_allocations ura WHERE ura.usage_log_id=ufr.usage_log_id AND ura.invalidated_at IS NULL),CASE WHEN ufr.business_type='subscription' THEN 0 ELSE ufr.usage_list_value END,0) AS revenue
 FROM usage_finance_records ufr WHERE ufr.usage_created_at >= $1 AND ufr.usage_created_at < $2
), signals AS (
	SELECT 'negative_profit' alert_type,'critical' severity,'negative_profit:usage:'||usage_log_id aggregation_key,
		'请求发生确定亏损' title,'采购成本高于该请求的已确认客户收入' description,
		'usage_log' dimension_type,usage_log_id dimension_id,
		upstream_cost-revenue impact_amount,1::bigint request_count
	FROM revenue WHERE cost_status='exact' AND upstream_cost > revenue
 UNION ALL
 SELECT cost_status,
        CASE WHEN cost_status='missing_usage' THEN 'critical' ELSE 'warning' END,
        cost_status||':global',
        CASE cost_status WHEN 'missing_price' THEN '存在缺失上游价格的请求' WHEN 'missing_multiplier' THEN '存在缺失上游倍率的请求' ELSE '存在缺失用量的请求' END,
        '成本数据不完整，相关收入不计入确定利润',
        'global',NULL::bigint,SUM(revenue),COUNT(*)::bigint
 FROM revenue WHERE cost_status IN ('missing_price','missing_multiplier','missing_usage') GROUP BY cost_status
)
SELECT alert_type,severity,aggregation_key,title,description,dimension_type,dimension_id,impact_amount::text,request_count
FROM signals WHERE request_count>0`, windowStart, now)
	if err != nil {
		return nil, fmt.Errorf("collect finance record alerts: %w", err)
	}
	signals := make([]service.FinanceAlertSignal, 0)
	for rows.Next() {
		signal, scanErr := scanFinanceAlertSignal(rows, now)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		signals = append(signals, signal)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	rows, err = r.db.QueryContext(ctx, `
WITH wallet_latest AS (
 SELECT DISTINCT ON (s.wallet_id) s.wallet_id,s.balance_amount,s.currency,s.collected_at,
        COALESCE(NULLIF(w.balance_scope_key,''),'wallet:'||s.wallet_id::text) AS balance_scope_key
 FROM upstream_balance_snapshots s JOIN upstream_wallets w ON w.id=s.wallet_id
 WHERE s.balance_kind='wallet_cash' AND s.sync_status='success' AND w.deleted_at IS NULL
 ORDER BY s.wallet_id,s.collected_at DESC,s.id DESC
), latest AS (
 SELECT wallet_latest.*,ROW_NUMBER() OVER(PARTITION BY balance_scope_key ORDER BY collected_at DESC,wallet_id) AS scope_rank
 FROM wallet_latest
), wallet_scopes AS (
 SELECT id AS wallet_id,COALESCE(NULLIF(balance_scope_key,''),'wallet:'||id::text) AS balance_scope_key
 FROM upstream_wallets WHERE deleted_at IS NULL
), costs AS (
 SELECT ws.balance_scope_key,SUM(ufr.upstream_cost)/7 AS daily_cost
 FROM usage_finance_records ufr JOIN wallet_scopes ws ON ws.wallet_id=ufr.wallet_id
 WHERE ufr.usage_created_at >= $1::timestamptz-INTERVAL '7 days' AND ufr.usage_created_at < $1::timestamptz
	AND ufr.cost_status='exact'
 GROUP BY ws.balance_scope_key
)
SELECT 'wallet_low_balance','critical','wallet_low_balance_scope:'||l.balance_scope_key,
       '上游共享余额预计不足 3 天','最近共享现金余额低于该余额范围近 7 日全部账号平均采购成本的 3 天用量',
       'wallet',l.wallet_id,l.balance_amount::text,0::bigint
FROM latest l JOIN costs c ON c.balance_scope_key=l.balance_scope_key
WHERE l.scope_rank=1 AND l.currency='USD' AND l.collected_at >= $1::timestamptz-INTERVAL '20 minutes' AND c.daily_cost>0 AND l.balance_amount/c.daily_cost<3
UNION ALL
SELECT 'wallet_sync_failed','warning','wallet_sync_failed:'||id,'上游钱包余额同步失败',COALESCE(NULLIF(balance_sync_error,''),NULLIF(quota_sync_error,''),'余额或配额同步失败'),'wallet',id,NULL::text,0::bigint
FROM upstream_wallets WHERE deleted_at IS NULL AND (balance_sync_status='failed' OR quota_sync_status='failed')
UNION ALL
SELECT 'pricing_sync_failed','warning','pricing_sync_failed:'||id,'上游价格同步失败',COALESCE(NULLIF(pricing_sync_error,''),'价格同步失败'),'wallet',id,NULL::text,0::bigint
FROM upstream_wallets WHERE deleted_at IS NULL AND pricing_sync_status='failed'
UNION ALL
SELECT 'reconciliation_difference','warning','reconciliation_difference:'||wallet_id,
       '上游账单存在未处理差额','上游账单与系统成本存在差异','wallet',wallet_id,SUM(ABS(difference_amount))::text,COUNT(*)::bigint
FROM upstream_bill_reconciliations WHERE status='difference' GROUP BY wallet_id
UNION ALL
SELECT 'payment_fee_uncollected','warning','payment_fee_uncollected:'||provider,
       '支付通道手续费尚未采集','支付手续费未进入已知净现金流','payment_provider',NULL::bigint,NULL::text,COUNT(*)::bigint
FROM (
 SELECT COALESCE(NULLIF(po.provider_key,''),'unknown') AS provider
 FROM payment_orders po
 WHERE po.paid_at >= $1::timestamptz-INTERVAL '24 hours' AND po.paid_at < $1::timestamptz
   AND NOT EXISTS (SELECT 1 FROM payment_provider_fee_events fee WHERE fee.payment_order_id=po.id AND fee.fee_status='confirmed')
) unpaid_fees GROUP BY provider`, now)
	if err != nil {
		return nil, fmt.Errorf("collect finance operational alerts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		signal, scanErr := scanFinanceAlertSignal(rows, now)
		if scanErr != nil {
			return nil, scanErr
		}
		signals = append(signals, signal)
	}
	return signals, rows.Err()
}

type financeAlertSignalScanner interface{ Scan(...any) error }

func scanFinanceAlertSignal(scanner financeAlertSignalScanner, occurredAt time.Time) (service.FinanceAlertSignal, error) {
	var signal service.FinanceAlertSignal
	var dimensionID sql.NullInt64
	var impact sql.NullString
	if err := scanner.Scan(
		&signal.AlertType, &signal.Severity, &signal.AggregationKey, &signal.Title, &signal.Description,
		&signal.DimensionType, &dimensionID, &impact, &signal.RequestCount,
	); err != nil {
		return signal, err
	}
	signal.DimensionID = nullableInt64Pointer(dimensionID)
	if impact.Valid {
		value, err := decimal.NewFromString(impact.String)
		if err != nil {
			return signal, err
		}
		signal.ImpactAmount = &value
	}
	signal.OccurredAt = occurredAt
	return signal, nil
}

func (r *financeAlertRepository) UpsertFinanceAlertSignals(ctx context.Context, signals []service.FinanceAlertSignal) error {
	if len(signals) == 0 {
		return nil
	}
	sort.SliceStable(signals, func(i, j int) bool {
		return signals[i].AggregationKey < signals[j].AggregationKey
	})
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, signal := range signals {
		if signal.AlertType == "negative_profit" {
			if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, signal.AggregationKey); err != nil {
				return fmt.Errorf("lock finance alert %s: %w", signal.AggregationKey, err)
			}
			var closed bool
			if err = tx.QueryRowContext(ctx, `
SELECT EXISTS(
 SELECT 1 FROM finance_alerts
 WHERE aggregation_key=$1 AND alert_type='negative_profit' AND status IN ('resolved','ignored')
)`, signal.AggregationKey).Scan(&closed); err != nil {
				return fmt.Errorf("check closed finance alert %s: %w", signal.AggregationKey, err)
			}
			if closed {
				continue
			}
		}
		var impact any
		if signal.ImpactAmount != nil {
			impact = signal.ImpactAmount.String()
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO finance_alerts (
 alert_type,severity,aggregation_key,title,description,dimension_type,dimension_id,impact_amount,request_count,
 occurrence_count,status,first_occurred_at,last_occurred_at,created_at,updated_at
) VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,1,'open',$10,$10,$10,$10)
ON CONFLICT (aggregation_key) WHERE status IN ('open','acknowledged') DO UPDATE SET
 severity=EXCLUDED.severity,title=EXCLUDED.title,description=EXCLUDED.description,
 dimension_type=EXCLUDED.dimension_type,dimension_id=EXCLUDED.dimension_id,
 impact_amount=EXCLUDED.impact_amount,request_count=EXCLUDED.request_count,
 occurrence_count=finance_alerts.occurrence_count+1,last_occurred_at=EXCLUDED.last_occurred_at,updated_at=EXCLUDED.updated_at`,
			signal.AlertType, signal.Severity, signal.AggregationKey, signal.Title, signal.Description,
			signal.DimensionType, signal.DimensionID, impact, signal.RequestCount, signal.OccurredAt)
		if err != nil {
			return fmt.Errorf("upsert finance alert %s: %w", signal.AggregationKey, err)
		}
	}
	return tx.Commit()
}

func (r *financeAlertRepository) ListFinanceAlerts(ctx context.Context, filter service.FinanceReportFilter, request service.FinanceAlertListRequest) ([]service.FinanceAlert, int64, error) {
	args := []any{filter.StartAt, filter.EndBefore}
	where := "last_occurred_at >= $1 AND last_occurred_at < $2"
	if request.AlertType != "" {
		args = append(args, request.AlertType)
		where += fmt.Sprintf(" AND alert_type=$%d", len(args))
	}
	if request.Severity != "" {
		args = append(args, request.Severity)
		where += fmt.Sprintf(" AND severity=$%d", len(args))
	}
	if request.Status != "" {
		args = append(args, request.Status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	args = append(args, request.PageSize, (request.Page-1)*request.PageSize)
	query := fmt.Sprintf(`
SELECT id,alert_type,severity,title,description,COALESCE(dimension_type,''),dimension_id,impact_amount::text,
 request_count,occurrence_count,status,first_occurred_at,last_occurred_at,assignee_id,handled_by,COALESCE(handled_note,''),handled_at,COUNT(*) OVER()::bigint
FROM finance_alerts WHERE %s
ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END,last_occurred_at DESC,id DESC
LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.FinanceAlert, 0)
	var total int64
	for rows.Next() {
		item, scanErr := scanFinanceAlert(rows, &total)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *financeAlertRepository) UpdateFinanceAlertStatus(ctx context.Context, id int64, status, note string, actorID int64, now time.Time) (*service.FinanceAlert, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var aggregationKey string
	if err = tx.QueryRowContext(ctx, `SELECT aggregation_key FROM finance_alerts WHERE id=$1`, id).Scan(&aggregationKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrFinanceAlertNotFound
		}
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, aggregationKey); err != nil {
		return nil, err
	}
	var previous string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM finance_alerts WHERE id=$1 FOR UPDATE`, id).Scan(&previous); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrFinanceAlertNotFound
		}
		return nil, err
	}
	if !validFinanceAlertTransition(previous, status) {
		return nil, errors.New("alert status transition is invalid")
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE finance_alerts SET status=$2,handled_by=NULLIF($3,0),handled_note=$4,handled_at=$5,updated_at=$5 WHERE id=$1`, id, status, actorID, note, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO finance_alert_status_audits (alert_id,from_status,to_status,note,operator_id,created_at)
VALUES ($1,$2,$3,$4,NULLIF($5,0),$6)`, id, previous, status, note, actorID, now); err != nil {
		return nil, err
	}
	row := tx.QueryRowContext(ctx, `
SELECT id,alert_type,severity,title,description,COALESCE(dimension_type,''),dimension_id,impact_amount::text,
 request_count,occurrence_count,status,first_occurred_at,last_occurred_at,assignee_id,handled_by,COALESCE(handled_note,''),handled_at,1::bigint
FROM finance_alerts WHERE id=$1`, id)
	item, err := scanFinanceAlert(row, nil)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &item, nil
}

type financeAlertScanner interface{ Scan(...any) error }

func scanFinanceAlert(scanner financeAlertScanner, totalTarget *int64) (service.FinanceAlert, error) {
	var item service.FinanceAlert
	var dimensionID, assigneeID, handledBy sql.NullInt64
	var impact sql.NullString
	var handledAt sql.NullTime
	var total int64
	err := scanner.Scan(
		&item.ID, &item.AlertType, &item.Severity, &item.Title, &item.Description, &item.DimensionType,
		&dimensionID, &impact, &item.RequestCount, &item.OccurrenceCount, &item.Status,
		&item.FirstOccurredAt, &item.LastOccurredAt, &assigneeID, &handledBy, &item.HandledNote, &handledAt, &total,
	)
	if err != nil {
		return item, err
	}
	item.DimensionID = nullableInt64Pointer(dimensionID)
	item.AssigneeID = nullableInt64Pointer(assigneeID)
	item.HandledBy = nullableInt64Pointer(handledBy)
	if impact.Valid {
		value, parseErr := decimal.NewFromString(impact.String)
		if parseErr != nil {
			return item, parseErr
		}
		formatted := value.Round(8).StringFixed(8)
		item.ImpactAmount = &formatted
	}
	if handledAt.Valid {
		item.HandledAt = &handledAt.Time
	}
	if totalTarget != nil {
		*totalTarget = total
	}
	return item, nil
}

func validFinanceAlertTransition(from, to string) bool {
	if from == to {
		return false
	}
	switch from {
	case "open":
		return to == "acknowledged" || to == "resolved" || to == "ignored"
	case "acknowledged":
		return to == "resolved" || to == "ignored" || to == "open"
	case "resolved", "ignored":
		return to == "open"
	default:
		return false
	}
}
