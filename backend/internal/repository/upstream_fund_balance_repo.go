package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type upstreamFundBalanceRepository struct{ db *sql.DB }

func NewUpstreamFundBalanceRepository(db *sql.DB) service.UpstreamFundBalanceRecorder {
	return &upstreamFundBalanceRepository{db: db}
}

func (r *upstreamFundBalanceRepository) RecordOpeningBalance(ctx context.Context, walletID int64, amount decimal.Decimal, currency string, collectedAt time.Time, dedupeKey string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("upstream fund balance repository is unavailable")
	}
	result, err := r.db.ExecContext(ctx, `
INSERT INTO upstream_balance_snapshots (
  wallet_id,dedupe_key,balance_kind,balance_amount,currency,source,collected_at,sync_status,safe_snapshot
)
SELECT id,$2,'wallet_cash',$3,$4,'manual',$5,'success',jsonb_build_object('kind','opening_balance')
FROM upstream_wallets
WHERE id=$1 AND deleted_at IS NULL AND enabled=TRUE AND balance_kind='wallet_cash'
ON CONFLICT (wallet_id,dedupe_key) DO NOTHING`, walletID, dedupeKey, amount.String(), currency, collectedAt.UTC())
	if err != nil {
		return fmt.Errorf("record opening balance snapshot: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("inspect opening balance snapshot result: %w", err)
	} else if affected != 1 {
		var exists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_balance_snapshots WHERE wallet_id=$1 AND dedupe_key=$2)`, walletID, dedupeKey).Scan(&exists); err != nil {
			return fmt.Errorf("inspect existing opening balance snapshot: %w", err)
		}
		if !exists {
			return fmt.Errorf("opening balance snapshot target wallet is unavailable")
		}
	}
	return nil
}
