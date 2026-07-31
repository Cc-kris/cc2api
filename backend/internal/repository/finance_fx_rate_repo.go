package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type financeFXRateRepository struct{ db *sql.DB }

func nullableFXOperatorID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func NewFinanceFXRateRepository(db *sql.DB) service.FinanceFXRateRepository {
	return &financeFXRateRepository{db: db}
}

func (r *financeFXRateRepository) ListFinanceFXRates(ctx context.Context, currency string, page, pageSize int) ([]service.FinanceFXRateVersion, int64, error) {
	args := make([]any, 0, 3)
	where := "TRUE"
	if currency != "" {
		args = append(args, strings.ToUpper(currency))
		where = fmt.Sprintf("currency=$%d", len(args))
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id,currency,rate_to_usd::text,source,observed_at,effective_from,effective_to,checksum,operator_id,change_reason,idempotency_key,created_at,COUNT(*) OVER()::bigint
FROM finance_fx_rate_versions WHERE %s
ORDER BY effective_from DESC,id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.FinanceFXRateVersion, 0)
	var total int64
	for rows.Next() {
		var item service.FinanceFXRateVersion
		var effectiveTo sql.NullTime
		var operatorID sql.NullInt64
		var reason, idempotencyKey sql.NullString
		if err = rows.Scan(&item.ID, &item.Currency, &item.RateToUSD, &item.Source, &item.ObservedAt, &item.EffectiveFrom, &effectiveTo, &item.Checksum, &operatorID, &reason, &idempotencyKey, &item.CreatedAt, &total); err != nil {
			return nil, 0, err
		}
		item.EffectiveTo = nullTimePointer(effectiveTo)
		if operatorID.Valid {
			item.OperatorID = &operatorID.Int64
		}
		if reason.Valid {
			item.ChangeReason = reason.String
		}
		if idempotencyKey.Valid {
			item.IdempotencyKey = idempotencyKey.String
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *financeFXRateRepository) CreateFinanceFXRate(ctx context.Context, input service.FinanceFXRateCreateInput, rate decimal.Decimal, checksum string) (*service.FinanceFXRateVersion, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, input.Currency); err != nil {
		return nil, err
	}
	load := func(query string, args ...any) (*service.FinanceFXRateVersion, error) {
		var item service.FinanceFXRateVersion
		var effectiveTo sql.NullTime
		var operatorID sql.NullInt64
		var reason, idempotencyKey sql.NullString
		err := tx.QueryRowContext(ctx, query, args...).Scan(&item.ID, &item.Currency, &item.RateToUSD, &item.Source, &item.ObservedAt, &item.EffectiveFrom, &effectiveTo, &item.Checksum, &operatorID, &reason, &idempotencyKey, &item.CreatedAt)
		if err != nil {
			return nil, err
		}
		item.EffectiveTo = nullTimePointer(effectiveTo)
		if operatorID.Valid {
			item.OperatorID = &operatorID.Int64
		}
		if reason.Valid {
			item.ChangeReason = reason.String
		}
		if idempotencyKey.Valid {
			item.IdempotencyKey = idempotencyKey.String
		}
		return &item, nil
	}
	identityQuery := `SELECT id,currency,rate_to_usd::text,source,observed_at,effective_from,effective_to,checksum,operator_id,change_reason,idempotency_key,created_at FROM finance_fx_rate_versions WHERE currency=$1 AND rate_to_usd=$2 AND source=$3 AND effective_from=$4`
	if input.IdempotencyKey != "" {
		if item, loadErr := load(`SELECT id,currency,rate_to_usd::text,source,observed_at,effective_from,effective_to,checksum,operator_id,change_reason,idempotency_key,created_at FROM finance_fx_rate_versions WHERE idempotency_key=$1`, input.IdempotencyKey); loadErr == nil {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return item, nil
		} else if !errors.Is(loadErr, sql.ErrNoRows) {
			return nil, loadErr
		}
	}
	if item, loadErr := load(identityQuery, input.Currency, rate.String(), input.Source, input.EffectiveFrom); loadErr == nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return item, nil
	} else if !errors.Is(loadErr, sql.ErrNoRows) {
		return nil, loadErr
	}
	var conflictingID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM finance_fx_rate_versions WHERE currency=$1 AND effective_from=$2 LIMIT 1`, input.Currency, input.EffectiveFrom).Scan(&conflictingID); err == nil {
		return nil, fmt.Errorf("finance fx rate effective_from conflicts with version %d", conflictingID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var nextEffective time.Time
	var hasNext bool
	if err := tx.QueryRowContext(ctx, `SELECT effective_from FROM finance_fx_rate_versions WHERE currency=$1 AND effective_from > $2 ORDER BY effective_from ASC,id ASC LIMIT 1`, input.Currency, input.EffectiveFrom).Scan(&nextEffective); err == nil {
		hasNext = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE finance_fx_rate_versions SET effective_to=$2 WHERE currency=$1 AND effective_from < $2 AND (effective_to IS NULL OR effective_to > $2)`, input.Currency, input.EffectiveFrom); err != nil {
		return nil, err
	}
	var effectiveTo any
	if hasNext {
		effectiveTo = nextEffective
	}
	created, err := load(`
INSERT INTO finance_fx_rate_versions(currency,rate_to_usd,source,observed_at,effective_from,effective_to,checksum,operator_id,change_reason,idempotency_key)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''))
RETURNING id,currency,rate_to_usd::text,source,observed_at,effective_from,effective_to,checksum,operator_id,change_reason,idempotency_key,created_at`, input.Currency, rate.String(), input.Source, input.ObservedAt, input.EffectiveFrom, effectiveTo, checksum, nullableFXOperatorID(input.OperatorID), input.ChangeReason, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}
