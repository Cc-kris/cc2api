package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type financePaymentFeeRepository struct{ db *sql.DB }

func NewFinancePaymentFeeRepository(db *sql.DB) service.FinancePaymentFeeRepository {
	return &financePaymentFeeRepository{db: db}
}

func (r *financePaymentFeeRepository) ImportPaymentFees(ctx context.Context, input service.PaymentFeeImportInput) (*service.PaymentFeeImportResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result := &service.PaymentFeeImportResult{}
	for _, row := range input.Rows {
		var orderID sql.NullInt64
		err = tx.QueryRowContext(ctx, `SELECT id FROM payment_orders WHERE out_trade_no=$1`, row.OrderNo).Scan(&orderID)
		if errors.Is(err, sql.ErrNoRows) {
			orderID = sql.NullInt64{}
			result.UnmatchedCount++
		} else if err != nil {
			return nil, err
		}
		grossUSD := row.GrossAmount.Mul(row.FXRateToUSD)
		feeUSD := row.FeeAmount.Mul(row.FXRateToUSD)
		netUSD := row.NetAmount.Mul(row.FXRateToUSD)
		insert, execErr := tx.ExecContext(ctx, `
INSERT INTO payment_provider_fee_events (
 payment_order_id,provider,bill_event_id,gross_amount,fee_amount,net_amount,currency,fx_rate_to_usd,
 gross_usd_amount,fee_usd_amount,net_usd_amount,fee_status,source,occurred_at,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'confirmed','csv',$12,NOW())
ON CONFLICT (provider,bill_event_id) DO NOTHING`, nullableInt64Value(orderID), input.Provider, row.BillEventID,
			row.GrossAmount.String(), row.FeeAmount.String(), row.NetAmount.String(), input.Currency, row.FXRateToUSD.String(),
			grossUSD.String(), feeUSD.String(), netUSD.String(), row.OccurredAt)
		if execErr != nil {
			return nil, execErr
		}
		affected, execErr := insert.RowsAffected()
		if execErr != nil {
			return nil, execErr
		}
		if affected == 0 {
			result.DuplicateCount++
		} else {
			result.ImportedCount++
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *financePaymentFeeRepository) ListPaymentFees(ctx context.Context, filter service.FinanceReportFilter, request service.FinancePaymentFeeListRequest) ([]service.FinancePaymentFeeItem, int64, error) {
	args := []any{filter.StartAt, filter.EndBefore, service.RoleAdmin}
	where := "occurred_at >= $1 AND occurred_at < $2"
	if request.OrderNo != "" {
		args = append(args, request.OrderNo)
		where += fmt.Sprintf(" AND order_no=$%d", len(args))
	}
	if request.Provider != "" {
		args = append(args, request.Provider)
		where += fmt.Sprintf(" AND provider=$%d", len(args))
	}
	if request.Status != "" {
		args = append(args, request.Status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	args = append(args, request.PageSize, (request.Page-1)*request.PageSize)
	query := fmt.Sprintf(`
WITH fee_rows AS (
 SELECT fee.id,fee.payment_order_id,COALESCE(po.out_trade_no,'') order_no,fee.provider,fee.bill_event_id,
        fee.gross_amount::text,fee.fee_amount::text,fee.net_amount::text,fee.currency,fee.fx_rate_to_usd::text,fee.fee_usd_amount::text,
        fee.fee_status status,fee.occurred_at
 FROM payment_provider_fee_events fee JOIN payment_orders po ON po.id=fee.payment_order_id
 WHERE NOT EXISTS (SELECT 1 FROM users finance_admin WHERE finance_admin.id=po.user_id AND finance_admin.role=$3)
 UNION ALL
 SELECT NULL::bigint,po.id,po.out_trade_no,COALESCE(NULLIF(po.provider_key,''),'unknown'),'uncollected:'||po.id,
        NULL::text,NULL::text,NULL::text,'USD',NULL::text,NULL::text,'uncollected',po.paid_at
 FROM payment_orders po
 WHERE po.status IN ('PAID','COMPLETED')
   AND NOT EXISTS (SELECT 1 FROM users finance_admin WHERE finance_admin.id=po.user_id AND finance_admin.role=$3)
   AND NOT EXISTS (SELECT 1 FROM payment_provider_fee_events fee WHERE fee.payment_order_id=po.id AND fee.fee_status='confirmed')
)
SELECT id,payment_order_id,order_no,provider,bill_event_id,gross_amount,fee_amount,net_amount,currency,fx_rate_to_usd,fee_usd_amount,status,occurred_at,COUNT(*) OVER()::bigint
FROM fee_rows WHERE %s ORDER BY occurred_at DESC,payment_order_id DESC
LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.FinancePaymentFeeItem, 0)
	var total int64
	for rows.Next() {
		var item service.FinancePaymentFeeItem
		var id, orderID sql.NullInt64
		var gross, fee, net, fx, feeUSD sql.NullString
		if err = rows.Scan(&id, &orderID, &item.OrderNo, &item.Provider, &item.BillEventID, &gross, &fee, &net,
			&item.Currency, &fx, &feeUSD, &item.Status, &item.OccurredAt, &total); err != nil {
			return nil, 0, err
		}
		item.ID = nullableInt64Pointer(id)
		item.PaymentOrderID = nullableInt64Pointer(orderID)
		item.GrossAmount = nullableStringPointer(gross)
		item.FeeAmount = nullableStringPointer(fee)
		item.NetAmount = nullableStringPointer(net)
		item.FXRateToUSD = nullableStringPointer(fx)
		item.FeeUSDAmount = nullableStringPointer(feeUSD)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func nullableInt64Value(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
