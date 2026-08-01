package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type upstreamFundRepository struct{ db *sql.DB }

func NewUpstreamFundRepository(db *sql.DB) service.UpstreamFundRepository {
	return &upstreamFundRepository{db: db}
}

func (r *upstreamFundRepository) CreateFundEvent(ctx context.Context, event *service.UpstreamFundEvent) (bool, error) {
	if event.FXRateVersionID == nil {
		versionID, err := ensureFinanceFXRateVersionSQL(ctx, r.db, event.Currency, event.FXRateToUSD, event.FXSource, event.FXObservedAt, event.OccurredAt)
		if err != nil {
			return false, fmt.Errorf("freeze upstream fund fx rate: %w", err)
		}
		event.FXRateVersionID = &versionID
	}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO upstream_fund_events (
 wallet_id,event_type,original_amount,currency,fx_rate_to_usd,fx_source,fx_observed_at,fx_rate_version_id,usd_amount,
 base_credit_units,bonus_credit_units,total_credit_units,base_recharge_ratio,effective_recharge_ratio,
 bonus_income_original,bonus_income_usd,bonus_status,reversed_event_id,
 occurred_at,reference_no,note,operator_id,idempotency_key
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
ON CONFLICT (wallet_id,idempotency_key) DO NOTHING
RETURNING id,created_at`, event.WalletID, event.EventType, event.OriginalAmount, event.Currency,
		event.FXRateToUSD, event.FXSource, event.FXObservedAt, event.FXRateVersionID, event.USDAmount,
		event.BaseCreditUnits, event.BonusCreditUnits, event.TotalCreditUnits, event.BaseRechargeRatio, event.EffectiveRechargeRatio,
		event.BonusIncomeOriginal, event.BonusIncomeUSD, event.BonusStatus, event.ReversedEventID,
		event.OccurredAt, nullableString(event.ReferenceNo), event.Note, event.OperatorID, event.IdempotencyKey,
	).Scan(&event.ID, &event.CreatedAt)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Constraint == "upstream_fund_events_wallet_type_reference_unique" {
			return false, service.ErrUpstreamFundDuplicateReference
		}
		return false, fmt.Errorf("create upstream fund event: %w", err)
	}
	err = scanUpstreamFundEvent(r.db.QueryRowContext(ctx, upstreamFundEventSelect+` WHERE wallet_id=$1 AND idempotency_key=$2`, event.WalletID, event.IdempotencyKey), event)
	if err != nil {
		return false, fmt.Errorf("get idempotent upstream fund event: %w", err)
	}
	return false, nil
}

// CreateFundEventWithOpeningBalance keeps the immutable opening event and its
// balance observation in the same transaction. This is used by finance
// initialization so a failed snapshot cannot leave a misleading fund event.
func (r *upstreamFundRepository) CreateFundEventWithOpeningBalance(ctx context.Context, event *service.UpstreamFundEvent) (bool, error) {
	if event.FXRateVersionID == nil {
		versionID, err := ensureFinanceFXRateVersionSQL(ctx, r.db, event.Currency, event.FXRateToUSD, event.FXSource, event.FXObservedAt, event.OccurredAt)
		if err != nil {
			return false, fmt.Errorf("freeze upstream fund fx rate: %w", err)
		}
		event.FXRateVersionID = &versionID
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	err = tx.QueryRowContext(ctx, `
INSERT INTO upstream_fund_events (
 wallet_id,event_type,original_amount,currency,fx_rate_to_usd,fx_source,fx_observed_at,fx_rate_version_id,usd_amount,
 base_credit_units,bonus_credit_units,total_credit_units,base_recharge_ratio,effective_recharge_ratio,
 bonus_income_original,bonus_income_usd,bonus_status,reversed_event_id,
 occurred_at,reference_no,note,operator_id,idempotency_key
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
ON CONFLICT (wallet_id,idempotency_key) DO NOTHING
RETURNING id,created_at`, event.WalletID, event.EventType, event.OriginalAmount, event.Currency,
		event.FXRateToUSD, event.FXSource, event.FXObservedAt, event.FXRateVersionID, event.USDAmount,
		event.BaseCreditUnits, event.BonusCreditUnits, event.TotalCreditUnits, event.BaseRechargeRatio, event.EffectiveRechargeRatio,
		event.BonusIncomeOriginal, event.BonusIncomeUSD, event.BonusStatus, event.ReversedEventID,
		event.OccurredAt, nullableString(event.ReferenceNo), event.Note, event.OperatorID, event.IdempotencyKey,
	).Scan(&event.ID, &event.CreatedAt)
	created := true
	if errors.Is(err, sql.ErrNoRows) {
		created = false
		err = scanUpstreamFundEvent(tx.QueryRowContext(ctx, upstreamFundEventSelect+` WHERE wallet_id=$1 AND idempotency_key=$2`, event.WalletID, event.IdempotencyKey), event)
	} else if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Constraint == "upstream_fund_events_wallet_type_reference_unique" {
			return false, service.ErrUpstreamFundDuplicateReference
		}
		return false, fmt.Errorf("create upstream opening fund event: %w", err)
	}
	if err != nil {
		return false, fmt.Errorf("get idempotent upstream fund event: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO upstream_balance_snapshots (
  wallet_id,dedupe_key,balance_kind,balance_amount,currency,source,collected_at,sync_status,safe_snapshot
)
SELECT id,$2,'wallet_cash',$3,$4,'manual',$5,'success',jsonb_build_object('kind','opening_balance')
FROM upstream_wallets
WHERE id=$1 AND deleted_at IS NULL AND enabled=TRUE AND balance_kind='wallet_cash'
ON CONFLICT (wallet_id,dedupe_key) DO NOTHING`, event.WalletID, "opening-balance-event-"+fmt.Sprint(event.ID), event.OriginalAmount.String(), event.Currency, event.OccurredAt.UTC())
	if err != nil {
		return false, fmt.Errorf("record opening balance snapshot: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return false, fmt.Errorf("inspect opening balance snapshot result: %w", err)
	} else if affected == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_balance_snapshots WHERE wallet_id=$1 AND dedupe_key=$2)`, event.WalletID, "opening-balance-event-"+fmt.Sprint(event.ID)).Scan(&exists); err != nil {
			return false, fmt.Errorf("inspect existing opening balance snapshot: %w", err)
		}
		if !exists {
			return false, errors.New("opening balance snapshot target wallet is unavailable")
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return created, nil
}

func (r *upstreamFundRepository) GetFundEvent(ctx context.Context, walletID, eventID int64) (*service.UpstreamFundEvent, error) {
	event := &service.UpstreamFundEvent{}
	err := scanUpstreamFundEvent(r.db.QueryRowContext(ctx, upstreamFundEventSelect+` WHERE wallet_id=$1 AND id=$2`, walletID, eventID), event)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUpstreamFundEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upstream fund event: %w", err)
	}
	return event, nil
}

func (r *upstreamFundRepository) ListFundEvents(ctx context.Context, walletID int64, page, pageSize int) ([]service.UpstreamFundEvent, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_fund_events WHERE wallet_id=$1`, walletID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count upstream fund events: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, upstreamFundEventSelect+` WHERE wallet_id=$1 ORDER BY occurred_at DESC,id DESC LIMIT $2 OFFSET $3`, walletID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list upstream fund events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UpstreamFundEvent, 0)
	for rows.Next() {
		var event service.UpstreamFundEvent
		if err := scanUpstreamFundEvent(rows, &event); err != nil {
			return nil, 0, fmt.Errorf("scan upstream fund event: %w", err)
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate upstream fund events: %w", err)
	}
	return items, total, nil
}

const upstreamFundEventSelect = `
SELECT id,wallet_id,event_type,original_amount,currency,fx_rate_to_usd,fx_source,fx_observed_at,fx_rate_version_id,usd_amount,
       base_credit_units,bonus_credit_units,total_credit_units,base_recharge_ratio,effective_recharge_ratio,
       bonus_income_original,bonus_income_usd,bonus_status,reversed_event_id,
       occurred_at,COALESCE(reference_no,''),note,operator_id,idempotency_key,created_at
FROM upstream_fund_events`

type upstreamFundEventScanner interface {
	Scan(dest ...any) error
}

func scanUpstreamFundEvent(scanner upstreamFundEventScanner, event *service.UpstreamFundEvent) error {
	var baseCreditUnits, bonusCreditUnits, totalCreditUnits sql.NullString
	var baseRechargeRatio, effectiveRechargeRatio sql.NullString
	var bonusIncomeOriginal, bonusIncomeUSD sql.NullString
	var reversedEventID, operatorID sql.NullInt64
	if err := scanner.Scan(
		&event.ID, &event.WalletID, &event.EventType, &event.OriginalAmount, &event.Currency, &event.FXRateToUSD, &event.FXSource, &event.FXObservedAt, &event.FXRateVersionID, &event.USDAmount,
		&baseCreditUnits, &bonusCreditUnits, &totalCreditUnits, &baseRechargeRatio, &effectiveRechargeRatio,
		&bonusIncomeOriginal, &bonusIncomeUSD, &event.BonusStatus, &reversedEventID,
		&event.OccurredAt, &event.ReferenceNo, &event.Note, &operatorID, &event.IdempotencyKey, &event.CreatedAt,
	); err != nil {
		return err
	}
	var err error
	if event.BaseCreditUnits, err = nullableDecimal(baseCreditUnits); err != nil {
		return err
	}
	if event.BonusCreditUnits, err = nullableDecimal(bonusCreditUnits); err != nil {
		return err
	}
	if event.TotalCreditUnits, err = nullableDecimal(totalCreditUnits); err != nil {
		return err
	}
	if event.BaseRechargeRatio, err = nullableDecimal(baseRechargeRatio); err != nil {
		return err
	}
	if event.EffectiveRechargeRatio, err = nullableDecimal(effectiveRechargeRatio); err != nil {
		return err
	}
	if event.BonusIncomeOriginal, err = nullableDecimal(bonusIncomeOriginal); err != nil {
		return err
	}
	if event.BonusIncomeUSD, err = nullableDecimal(bonusIncomeUSD); err != nil {
		return err
	}
	if reversedEventID.Valid {
		value := reversedEventID.Int64
		event.ReversedEventID = &value
	} else {
		event.ReversedEventID = nil
	}
	if operatorID.Valid {
		value := operatorID.Int64
		event.OperatorID = &value
	} else {
		event.OperatorID = nil
	}
	return nil
}

func nullableDecimal(value sql.NullString) (*decimal.Decimal, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(value.String)
	if err != nil {
		return nil, fmt.Errorf("parse nullable decimal %q: %w", value.String, err)
	}
	return &parsed, nil
}

func ensureFinanceFXRateVersionSQL(ctx context.Context, db *sql.DB, currency string, rate decimal.Decimal, source string, observedAt, effectiveFrom time.Time) (int64, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	source = strings.TrimSpace(source)
	if currency == "" || len(currency) != 3 || rate.LessThanOrEqual(decimal.Zero) || source == "" {
		return 0, errors.New("invalid finance FX evidence")
	}
	if observedAt.IsZero() {
		observedAt = effectiveFrom
	}
	if effectiveFrom.IsZero() {
		effectiveFrom = observedAt
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(currency+"|"+rate.String()+"|"+source+"|"+effectiveFrom.UTC().Format(time.RFC3339Nano))))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, currency); err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM finance_fx_rate_versions WHERE currency=$1 AND rate_to_usd=$2 AND source=$3 AND effective_from=$4`, currency, rate, source, effectiveFrom).Scan(&id); err == nil {
		if err = tx.Commit(); err != nil {
			return 0, err
		}
		return id, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var nextEffective time.Time
	hasNext := false
	if err = tx.QueryRowContext(ctx, `SELECT effective_from FROM finance_fx_rate_versions WHERE currency=$1 AND effective_from>$2 ORDER BY effective_from ASC,id ASC LIMIT 1`, currency, effectiveFrom).Scan(&nextEffective); err == nil {
		hasNext = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE finance_fx_rate_versions SET effective_to=$2 WHERE currency=$1 AND effective_from<$2 AND (effective_to IS NULL OR effective_to>$2)`, currency, effectiveFrom); err != nil {
		return 0, err
	}
	var effectiveTo any
	if hasNext {
		effectiveTo = nextEffective
	}
	if err = tx.QueryRowContext(ctx, `
INSERT INTO finance_fx_rate_versions(currency,rate_to_usd,source,observed_at,effective_from,effective_to,checksum,change_reason)
VALUES($1,$2,$3,$4,$5,$6,$7,'upstream_fund_event')
RETURNING id`, currency, rate, source, observedAt, effectiveFrom, effectiveTo, checksum).Scan(&id); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}
