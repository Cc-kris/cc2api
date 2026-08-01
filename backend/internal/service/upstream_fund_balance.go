package service

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// UpstreamFundBalanceRecorder records a balance observation that belongs to a
// fund event. Keeping it separate from recharge bookkeeping makes a manually
// entered opening balance visible in the funds report without turning it into
// a recharge or changing an account multiplier.
type UpstreamFundBalanceRecorder interface {
	RecordOpeningBalance(ctx context.Context, walletID int64, amount decimal.Decimal, currency string, collectedAt time.Time, dedupeKey string) error
}
