package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type financeRevenueRecognitionRepository struct{ db *sql.DB }

func NewFinanceRevenueRecognitionRepository(db *sql.DB) service.FinanceRevenueRecognitionRepository {
	return &financeRevenueRecognitionRepository{db: db}
}

func (r *financeRevenueRecognitionRepository) AcquireSubscriptionRevenueDateLock(ctx context.Context, date time.Time, timezone string) (func() error, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	key := timezone + ":" + date.Format("2006-01-02")
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('finance_subscription_revenue'),hashtext($1))`, key); err != nil {
		_ = conn.Close()
		return nil, err
	}
	released := false
	return func() error {
		if released {
			return nil
		}
		released = true
		_, unlockErr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext('finance_subscription_revenue'),hashtext($1))`, key)
		closeErr := conn.Close()
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}

func (r *financeRevenueRecognitionRepository) OldestUnrecognizedSubscriptionDate(ctx context.Context, through time.Time, timezone string) (*time.Time, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `
WITH eligible AS (
 SELECT id,(paid_at AT TIME ZONE $2)::date AS start_date,
        ((paid_at AT TIME ZONE $2)::date + (subscription_days - 1)) AS end_date,
        CASE WHEN refund_at IS NULL THEN NULL ELSE (refund_at AT TIME ZONE $2)::date END AS refund_date
 FROM payment_orders
 WHERE order_type='subscription'
   AND status IN ('PAID','RECHARGING','COMPLETED','REFUND_REQUESTED','REFUNDING','PARTIALLY_REFUNDED','REFUNDED','REFUND_FAILED')
   AND paid_at IS NOT NULL AND subscription_days IS NOT NULL AND subscription_days>0
), candidate_days AS (
 SELECT e.id,day::date AS recognition_date
 FROM eligible e
 CROSS JOIN LATERAL generate_series(e.start_date,LEAST(e.end_date,$1::date),INTERVAL '1 day') day
 WHERE e.start_date <= $1::date
 UNION
 SELECT e.id,e.refund_date FROM eligible e WHERE e.refund_date IS NOT NULL AND e.refund_date <= $1::date
)
SELECT COALESCE(MIN(c.recognition_date)::text,'')
FROM candidate_days c
LEFT JOIN subscription_revenue_recognitions r
  ON r.payment_order_id=c.id AND r.recognition_date=c.recognition_date
WHERE r.id IS NULL`, through.Format("2006-01-02"), timezone).Scan(&value)
	if err != nil {
		return nil, fmt.Errorf("find oldest unrecognized subscription date: %w", err)
	}
	if value == "" {
		return nil, nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load subscription recognition timezone: %w", err)
	}
	date, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return nil, fmt.Errorf("parse oldest unrecognized subscription date: %w", err)
	}
	return &date, nil
}

func (r *financeRevenueRecognitionRepository) ListSubscriptionOrdersForDate(ctx context.Context, date time.Time, timezone string) ([]service.FinanceSubscriptionOrder, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id,user_id,subscription_group_id,amount::text,COALESCE(refund_amount,0)::text,
       paid_at,subscription_days,refund_at
FROM payment_orders
WHERE order_type='subscription'
  AND status IN ('PAID','RECHARGING','COMPLETED','REFUND_REQUESTED','REFUNDING','PARTIALLY_REFUNDED','REFUNDED','REFUND_FAILED')
  AND paid_at IS NOT NULL
  AND subscription_days IS NOT NULL
  AND subscription_days > 0
  AND (
    $1::date BETWEEN (paid_at AT TIME ZONE $2)::date
                 AND ((paid_at AT TIME ZONE $2)::date + (subscription_days - 1))
    OR (refund_at IS NOT NULL AND (refund_at AT TIME ZONE $2)::date = $1::date)
  )
ORDER BY id`, date.Format("2006-01-02"), timezone)
	if err != nil {
		return nil, fmt.Errorf("list subscription orders for recognition: %w", err)
	}
	defer rows.Close()
	orders := make([]service.FinanceSubscriptionOrder, 0)
	for rows.Next() {
		var order service.FinanceSubscriptionOrder
		var groupID sql.NullInt64
		var amount, refund string
		var refundDate sql.NullTime
		if err = rows.Scan(&order.OrderID, &order.UserID, &groupID, &amount, &refund, &order.ServiceStartDate, &order.ServiceDays, &refundDate); err != nil {
			return nil, fmt.Errorf("scan subscription order for recognition: %w", err)
		}
		order.GroupID = nullableInt64Pointer(groupID)
		if order.Amount, err = decimal.NewFromString(amount); err != nil {
			return nil, err
		}
		if order.RefundAmount, err = decimal.NewFromString(refund); err != nil {
			return nil, err
		}
		if refundDate.Valid {
			value := refundDate.Time
			order.RefundDate = &value
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func (r *financeRevenueRecognitionRepository) ListSubscriptionUsageForDate(ctx context.Context, order service.FinanceSubscriptionOrder, date time.Time, timezone string) ([]service.FinanceSubscriptionUsage, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT usage_log_id,COALESCE(usage_list_value,0)::text
FROM usage_finance_records
WHERE user_id=$1
  AND business_type='subscription'
  AND ($2::bigint IS NULL OR group_id=$2)
  AND (usage_created_at AT TIME ZONE $4)::date=$3::date
ORDER BY usage_created_at,id`, order.UserID, order.GroupID, date.Format("2006-01-02"), timezone)
	if err != nil {
		return nil, fmt.Errorf("list subscription usage for recognition: %w", err)
	}
	defer rows.Close()
	items := make([]service.FinanceSubscriptionUsage, 0)
	for rows.Next() {
		var item service.FinanceSubscriptionUsage
		var value string
		if err = rows.Scan(&item.UsageLogID, &value); err != nil {
			return nil, fmt.Errorf("scan subscription usage for recognition: %w", err)
		}
		if item.UsageListValue, err = decimal.NewFromString(value); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *financeRevenueRecognitionRepository) SaveSubscriptionRecognition(ctx context.Context, recognition service.FinanceRevenueRecognition, allocations []service.FinanceRevenueAllocation) error {
	detail, err := json.Marshal(recognition.CalculationDetail)
	if err != nil {
		return fmt.Errorf("marshal subscription recognition detail: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var recognitionID int64
	var revision int
	err = tx.QueryRowContext(ctx, `
INSERT INTO subscription_revenue_recognitions(
 payment_order_id,user_id,group_id,recognition_date,recognized_revenue,refund_reduction,
 allocated_revenue,unallocated_revenue,allocation_status,calculation_detail,current_revision,updated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,NOW())
ON CONFLICT(payment_order_id,recognition_date) DO UPDATE SET
 recognized_revenue=EXCLUDED.recognized_revenue,
 refund_reduction=EXCLUDED.refund_reduction,
 allocated_revenue=EXCLUDED.allocated_revenue,
 unallocated_revenue=EXCLUDED.unallocated_revenue,
 allocation_status=EXCLUDED.allocation_status,
 calculation_detail=EXCLUDED.calculation_detail,
 current_revision=subscription_revenue_recognitions.current_revision+1,
 updated_at=NOW()
WHERE (
 subscription_revenue_recognitions.recognized_revenue,
 subscription_revenue_recognitions.refund_reduction,
 subscription_revenue_recognitions.allocated_revenue,
 subscription_revenue_recognitions.unallocated_revenue,
 subscription_revenue_recognitions.allocation_status,
 subscription_revenue_recognitions.calculation_detail
) IS DISTINCT FROM (
 EXCLUDED.recognized_revenue,
 EXCLUDED.refund_reduction,
 EXCLUDED.allocated_revenue,
 EXCLUDED.unallocated_revenue,
 EXCLUDED.allocation_status,
 EXCLUDED.calculation_detail
)
RETURNING id,current_revision`, recognition.OrderID, recognition.UserID, recognition.GroupID,
		recognition.RecognitionDate.Format("2006-01-02"), recognition.RecognizedRevenue.String(), recognition.RefundReduction.String(),
		recognition.AllocatedRevenue.String(), recognition.UnallocatedRevenue.String(), recognition.AllocationStatus, detail).Scan(&recognitionID, &revision)
	if err == sql.ErrNoRows {
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("save subscription revenue recognition: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE usage_revenue_allocations
SET invalidated_at=NOW()
WHERE source_type='subscription_recognition' AND source_id=$1 AND invalidated_at IS NULL`, recognitionID); err != nil {
		return fmt.Errorf("invalidate subscription revenue allocations: %w", err)
	}
	for _, allocation := range allocations {
		if _, err = tx.ExecContext(ctx, `
INSERT INTO usage_revenue_allocations(
 usage_log_id,source_type,source_id,allocated_amount,allocation_method,recognition_date,revision,audit_detail
) VALUES($1,'subscription_recognition',$2,$3,$4,$5,$6,'{}')`, allocation.UsageLogID, recognitionID,
			allocation.Amount.String(), allocation.Method, recognition.RecognitionDate.Format("2006-01-02"), revision); err != nil {
			return fmt.Errorf("save subscription revenue allocation: %w", err)
		}
	}
	return tx.Commit()
}
