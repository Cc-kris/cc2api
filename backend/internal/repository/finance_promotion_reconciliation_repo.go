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
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

const promotionCreditReconciliationMaxTransactionAttempts = 3

type promotionCreditReconciliationRepository struct{ db *sql.DB }

func NewPromotionCreditReconciliationRepository(db *sql.DB) service.PromotionCreditReconciliationRepository {
	return &promotionCreditReconciliationRepository{db: db}
}

func (r *promotionCreditReconciliationRepository) ListPromotionCreditReconciliations(ctx context.Context, request service.PromotionCreditReconciliationListRequest) ([]service.PromotionCreditReconciliation, int64, error) {
	args := make([]any, 0, 3)
	where := []string{"1=1"}
	if request.Status != "" {
		args = append(args, request.Status)
		where = append(where, fmt.Sprintf("r.status=$%d", len(args)))
	}
	args = append(args, request.PageSize, (request.Page-1)*request.PageSize)
	rows, err := r.db.QueryContext(ctx, promotionCreditReconciliationSelect+fmt.Sprintf(`
WHERE %s ORDER BY r.created_at DESC,r.user_id DESC LIMIT $%d OFFSET $%d`, strings.Join(where, " AND "), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list promotion credit reconciliations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.PromotionCreditReconciliation, 0)
	var total int64
	for rows.Next() {
		item, scanErr := scanPromotionCreditReconciliation(rows, &total)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *promotionCreditReconciliationRepository) ResolvePromotionCreditReconciliation(ctx context.Context, userID int64, amount decimal.Decimal, note string, operatorID int64, now time.Time) (*service.PromotionCreditReconciliation, error) {
	var lastErr error
	for attempt := 0; attempt < promotionCreditReconciliationMaxTransactionAttempts; attempt++ {
		item, err := r.resolvePromotionCreditReconciliationOnce(ctx, userID, amount, note, operatorID, now)
		if err == nil {
			return item, nil
		}
		lastErr = err
		if !isPromotionCreditSerializationFailure(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("resolve promotion credit reconciliation after %d attempts: %w", promotionCreditReconciliationMaxTransactionAttempts, lastErr)
}

func (r *promotionCreditReconciliationRepository) resolvePromotionCreditReconciliationOnce(ctx context.Context, userID int64, amount decimal.Decimal, note string, operatorID int64, now time.Time) (*service.PromotionCreditReconciliation, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err = tx.QueryRowContext(ctx, `
SELECT status FROM user_promotion_credit_reconciliations
WHERE user_id=$1 FOR UPDATE`, userID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrPromotionCreditReconciliationNotFound
		}
		return nil, fmt.Errorf("lock promotion credit reconciliation: %w", err)
	}
	if status == service.PromotionCreditReconciliationResolved {
		return nil, service.ErrPromotionCreditReconciliationResolved
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO user_promotion_credit_balances(user_id,remaining_amount,updated_at)
VALUES($1,0,$2) ON CONFLICT(user_id) DO NOTHING`, userID, now); err != nil {
		return nil, fmt.Errorf("ensure promotion credit balance: %w", err)
	}
	var oldBalance string
	if err = tx.QueryRowContext(ctx, `
SELECT remaining_amount::text FROM user_promotion_credit_balances
WHERE user_id=$1 FOR UPDATE`, userID).Scan(&oldBalance); err != nil {
		return nil, fmt.Errorf("lock promotion credit balance: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE user_promotion_credit_balances
SET remaining_amount=$2,updated_at=$3 WHERE user_id=$1`, userID, amount.String(), now); err != nil {
		return nil, fmt.Errorf("set promotion credit balance: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE user_promotion_credit_reconciliations
SET status=$2,confirmed_remaining_amount=$3,resolved_at=$4,resolved_by=$5,notes=$6
WHERE user_id=$1 AND status=$7`, userID, service.PromotionCreditReconciliationResolved, amount.String(), now, operatorID, note, service.PromotionCreditReconciliationRequired)
	if err != nil {
		return nil, fmt.Errorf("resolve promotion credit reconciliation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, service.ErrPromotionCreditReconciliationResolved
	}
	oldJSON, err := json.Marshal(map[string]any{"status": status, "remaining_amount": oldBalance})
	if err != nil {
		return nil, fmt.Errorf("marshal previous promotion reconciliation state: %w", err)
	}
	newJSON, err := json.Marshal(map[string]any{"status": service.PromotionCreditReconciliationResolved, "remaining_amount": amount.String(), "note": note})
	if err != nil {
		return nil, fmt.Errorf("marshal resolved promotion reconciliation state: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO finance_calculation_revisions(entity_type,entity_id,revision,old_result,new_result,reason,operator_id,created_at)
SELECT 'promotion_credit_reconciliation',$1,COALESCE(MAX(revision),0)+1,$2::jsonb,$3::jsonb,$4,$5,$6
FROM finance_calculation_revisions WHERE entity_type='promotion_credit_reconciliation' AND entity_id=$1`,
		userID, string(oldJSON), string(newJSON), "historical promotion credit reconciled", operatorID, now); err != nil {
		return nil, fmt.Errorf("append promotion credit reconciliation audit: %w", err)
	}
	item, err := getPromotionCreditReconciliation(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func isPromotionCreditSerializationFailure(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr != nil && pqErr.Code == "40001"
}

const promotionCreditReconciliationSelect = `
SELECT r.user_id,u.email,COALESCE(u.username,''),r.detected_historical_bonus::text,
       COALESCE(b.remaining_amount,0)::text,r.confirmed_remaining_amount::text,
       r.status,r.cutover_at,r.created_at,r.resolved_at,r.resolved_by,COALESCE(r.notes,''),
       COUNT(*) OVER()::bigint
FROM user_promotion_credit_reconciliations r
JOIN users u ON u.id=r.user_id
LEFT JOIN user_promotion_credit_balances b ON b.user_id=r.user_id
`

type promotionCreditReconciliationScanner interface{ Scan(...any) error }

func scanPromotionCreditReconciliation(scanner promotionCreditReconciliationScanner, total *int64) (service.PromotionCreditReconciliation, error) {
	var item service.PromotionCreditReconciliation
	var confirmed sql.NullString
	var resolvedAt sql.NullTime
	var resolvedBy sql.NullInt64
	if err := scanner.Scan(&item.UserID, &item.UserEmail, &item.Username, &item.DetectedHistoricalBonus, &item.CurrentRemainingAmount, &confirmed, &item.Status, &item.CutoverAt, &item.CreatedAt, &resolvedAt, &resolvedBy, &item.Notes, total); err != nil {
		return item, err
	}
	if confirmed.Valid {
		item.ConfirmedRemainingAmount = &confirmed.String
	}
	if resolvedAt.Valid {
		value := resolvedAt.Time.UTC()
		item.ResolvedAt = &value
	}
	if resolvedBy.Valid {
		value := resolvedBy.Int64
		item.ResolvedBy = &value
	}
	return item, nil
}

func getPromotionCreditReconciliation(ctx context.Context, queryer financeReconciliationQueryer, userID int64) (*service.PromotionCreditReconciliation, error) {
	var total int64
	item, err := scanPromotionCreditReconciliation(queryer.QueryRowContext(ctx, promotionCreditReconciliationSelect+" WHERE r.user_id=$1", userID), &total)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPromotionCreditReconciliationNotFound
	}
	return &item, err
}
