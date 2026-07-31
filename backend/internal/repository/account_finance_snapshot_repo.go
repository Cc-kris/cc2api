package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type accountFinanceSnapshotRepository struct {
	db *sql.DB
}

var _ service.AccountFinanceMultiplierAccountRepository = (*accountRepository)(nil)

func NewAccountFinanceSnapshotRepository(db *sql.DB) service.AccountFinanceSnapshotRepository {
	return &accountFinanceSnapshotRepository{db: db}
}

// BindAccountFinanceMultiplierAccountRepository preserves the existing broad
// AccountRepository provider while exposing its audited multiplier transaction
// to the cumulative snapshot service.
func BindAccountFinanceMultiplierAccountRepository(repo service.AccountRepository) (service.AccountFinanceMultiplierAccountRepository, error) {
	audited, ok := repo.(service.AccountFinanceMultiplierAccountRepository)
	if !ok {
		return nil, service.ErrAccountFinanceSnapshotRepoUnavailable
	}
	return audited, nil
}

func BindUpstreamFinanceSyncAccountRepository(repo service.AccountRepository) service.UpstreamFinanceSyncAccountRepository {
	syncRepo, _ := repo.(service.UpstreamFinanceSyncAccountRepository)
	return syncRepo
}

func (r *accountFinanceSnapshotRepository) WithAccountSyncLock(ctx context.Context, accountID int64, scopeKey string, fn func(context.Context) error) error {
	if r == nil || r.db == nil {
		return service.ErrAccountFinanceSnapshotRepoUnavailable
	}
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	lockKey := fmt.Sprintf("account_finance_multiplier:%d:%s", accountID, scopeKey)
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey)
	}()
	return fn(ctx)
}

func (r *accountFinanceSnapshotRepository) ClaimCounterOwner(ctx context.Context, identityKey string, walletID int64, protocolVersionID *int64, accountID int64, upstreamCounterID, counterPeriod *string) error {
	row := r.db.QueryRowContext(ctx, `
INSERT INTO upstream_finance_counter_owners (
    counter_identity_key, wallet_id, protocol_version_id, owner_account_id,
    upstream_counter_id, counter_period, first_seen_at, last_seen_at
) VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())
ON CONFLICT (counter_identity_key) DO UPDATE
SET last_seen_at=NOW()
WHERE upstream_finance_counter_owners.owner_account_id=EXCLUDED.owner_account_id
RETURNING owner_account_id`, identityKey, walletID, protocolVersionID, accountID, upstreamCounterID, counterPeriod)
	var ownerAccountID int64
	if err := row.Scan(&ownerAccountID); errors.Is(err, sql.ErrNoRows) {
		return service.ErrAccountFinanceCounterOwnerConflict
	} else if err != nil {
		return err
	}
	if ownerAccountID != accountID {
		return service.ErrAccountFinanceCounterOwnerConflict
	}
	return nil
}

func (r *accountFinanceSnapshotRepository) LatestCounterSnapshot(ctx context.Context, accountID int64, scopeKey string) (*service.AccountFinanceCounterSnapshot, error) {
	row := r.db.QueryRowContext(ctx, accountFinanceCounterSnapshotSelect+`
FROM account_finance_counter_snapshots
WHERE account_id = $1 AND scope_key = $2
ORDER BY collected_at DESC, id DESC
LIMIT 1`, accountID, scopeKey)
	snapshot, err := scanAccountFinanceCounterSnapshot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return snapshot, err
}

func (r *accountFinanceSnapshotRepository) CounterSnapshotByID(ctx context.Context, id int64) (*service.AccountFinanceCounterSnapshot, error) {
	snapshot, err := scanAccountFinanceCounterSnapshot(r.db.QueryRowContext(ctx, accountFinanceCounterSnapshotSelect+`
FROM account_finance_counter_snapshots WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAccountFinanceSnapshotInvalid
	}
	return snapshot, err
}

func (r *accountFinanceSnapshotRepository) CreateCounterSnapshot(ctx context.Context, snapshot *service.AccountFinanceCounterSnapshot) (*service.AccountFinanceCounterSnapshot, bool, error) {
	if snapshot == nil {
		return nil, false, service.ErrAccountFinanceSnapshotInvalid
	}
	safeSnapshot, err := json.Marshal(snapshot.SafeSnapshot)
	if err != nil {
		return nil, false, err
	}
	row := r.db.QueryRowContext(ctx, `
INSERT INTO account_finance_counter_snapshots (
    account_id, account_finance_profile_id, scope_key, idempotency_key, upstream_counter_id, counter_period,
    list_cost_total, actual_cost_total, unit_code, unit_semantics, currency,
    upstream_observed_at, collected_at, safe_snapshot, checksum,
    previous_snapshot_id, list_cost_delta, actual_cost_delta, observed_multiplier,
    derivation_status, anomaly_code, multiplier_change_id, multiplier_effective_at
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23
)
ON CONFLICT (account_id, scope_key, idempotency_key) DO NOTHING
RETURNING `+accountFinanceCounterSnapshotReturning, snapshot.AccountID, snapshot.AccountFinanceProfileID, snapshot.ScopeKey, snapshot.IdempotencyKey,
		snapshot.UpstreamCounterID, snapshot.CounterPeriod, accountFinanceDecimalArgument(snapshot.ListCostTotal),
		accountFinanceDecimalArgument(snapshot.ActualCostTotal), snapshot.UnitCode, snapshot.UnitSemantics,
		snapshot.Currency, snapshot.UpstreamObservedAt, snapshot.CollectedAt, safeSnapshot, snapshot.Checksum,
		snapshot.PreviousSnapshotID, accountFinanceDecimalArgument(snapshot.ListCostDelta),
		accountFinanceDecimalArgument(snapshot.ActualCostDelta), accountFinanceDecimalArgument(snapshot.ObservedMultiplier),
		snapshot.DerivationStatus, snapshot.AnomalyCode, snapshot.MultiplierChangeID, snapshot.MultiplierEffectiveAt)
	stored, err := scanAccountFinanceCounterSnapshot(row)
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	stored, err = scanAccountFinanceCounterSnapshot(r.db.QueryRowContext(ctx, accountFinanceCounterSnapshotSelect+`
FROM account_finance_counter_snapshots
WHERE account_id = $1 AND scope_key = $2 AND idempotency_key = $3`, snapshot.AccountID, snapshot.ScopeKey, snapshot.IdempotencyKey))
	return stored, false, err
}

func (r *accountFinanceSnapshotRepository) MarkCounterSnapshotMultiplierResult(ctx context.Context, snapshotID int64, status string, anomalyCode *string, multiplierChangeID *int64, effectiveAt *time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE account_finance_counter_snapshots
SET derivation_status = $2,
    anomaly_code = $3,
    multiplier_change_id = $4,
    multiplier_effective_at = $5
WHERE id = $1 AND derivation_status IN ('candidate','conflict')`, snapshotID, status, anomalyCode, multiplierChangeID, effectiveAt)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return service.ErrAccountUpstreamMultiplierConflict
	}
	return nil
}

func (r *accountFinanceSnapshotRepository) ResolveEffectiveMultiplierVersion(ctx context.Context, accountID int64, effectiveAt time.Time) (*service.AccountFinanceMultiplierVersion, error) {
	var (
		version service.AccountFinanceMultiplierVersion
		oldRaw  sql.NullString
		newRaw  string
	)
	err := r.db.QueryRowContext(ctx, `
SELECT id, account_id, old_multiplier::text, new_multiplier::text, effective_at, reason
FROM account_upstream_multiplier_changes
WHERE account_id = $1 AND effective_at <= $2
ORDER BY effective_at DESC, id DESC
LIMIT 1`, accountID, effectiveAt).Scan(&version.ID, &version.AccountID, &oldRaw, &newRaw, &version.EffectiveAt, &version.Reason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	version.OldMultiplier, err = parseAccountFinanceNullableDecimal(oldRaw)
	if err != nil {
		return nil, err
	}
	version.NewMultiplier, err = decimal.NewFromString(newRaw)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

const accountFinanceCounterSnapshotReturning = `
    id, account_id, account_finance_profile_id, scope_key, idempotency_key, upstream_counter_id, counter_period,
list_cost_total::text, actual_cost_total::text, unit_code, unit_semantics, currency,
upstream_observed_at, collected_at, safe_snapshot, checksum, previous_snapshot_id,
list_cost_delta::text, actual_cost_delta::text, observed_multiplier::text,
derivation_status, anomaly_code, multiplier_change_id, multiplier_effective_at, created_at`

const accountFinanceCounterSnapshotSelect = `SELECT ` + accountFinanceCounterSnapshotReturning

type accountFinanceSnapshotScanner interface {
	Scan(dest ...any) error
}

func scanAccountFinanceCounterSnapshot(scanner accountFinanceSnapshotScanner) (*service.AccountFinanceCounterSnapshot, error) {
	var (
		snapshot                service.AccountFinanceCounterSnapshot
		upstreamCounterID       sql.NullString
		counterPeriod           sql.NullString
		listCostTotal           sql.NullString
		actualCostTotal         sql.NullString
		currency                sql.NullString
		upstreamObservedAt      sql.NullTime
		safeSnapshot            []byte
		previousSnapshotID      sql.NullInt64
		listCostDelta           sql.NullString
		actualCostDelta         sql.NullString
		observedMultiplier      sql.NullString
		anomalyCode             sql.NullString
		multiplierChangeID      sql.NullInt64
		multiplierEffectiveAt   sql.NullTime
		accountFinanceProfileID sql.NullInt64
	)
	err := scanner.Scan(
		&snapshot.ID, &snapshot.AccountID, &accountFinanceProfileID, &snapshot.ScopeKey, &snapshot.IdempotencyKey,
		&upstreamCounterID, &counterPeriod, &listCostTotal, &actualCostTotal,
		&snapshot.UnitCode, &snapshot.UnitSemantics, &currency, &upstreamObservedAt,
		&snapshot.CollectedAt, &safeSnapshot, &snapshot.Checksum, &previousSnapshotID,
		&listCostDelta, &actualCostDelta, &observedMultiplier, &snapshot.DerivationStatus,
		&anomalyCode, &multiplierChangeID, &multiplierEffectiveAt, &snapshot.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	snapshot.UpstreamCounterID = accountFinanceNullableString(upstreamCounterID)
	snapshot.AccountFinanceProfileID = accountFinanceNullableInt64(accountFinanceProfileID)
	snapshot.CounterPeriod = accountFinanceNullableString(counterPeriod)
	snapshot.Currency = accountFinanceNullableString(currency)
	snapshot.UpstreamObservedAt = accountFinanceNullableTime(upstreamObservedAt)
	snapshot.PreviousSnapshotID = accountFinanceNullableInt64(previousSnapshotID)
	snapshot.AnomalyCode = accountFinanceNullableString(anomalyCode)
	snapshot.MultiplierChangeID = accountFinanceNullableInt64(multiplierChangeID)
	snapshot.MultiplierEffectiveAt = accountFinanceNullableTime(multiplierEffectiveAt)
	if len(safeSnapshot) > 0 {
		if err = json.Unmarshal(safeSnapshot, &snapshot.SafeSnapshot); err != nil {
			return nil, err
		}
	}
	if snapshot.SafeSnapshot == nil {
		snapshot.SafeSnapshot = map[string]any{}
	}
	if snapshot.ListCostTotal, err = parseAccountFinanceNullableDecimal(listCostTotal); err != nil {
		return nil, err
	}
	if snapshot.ActualCostTotal, err = parseAccountFinanceNullableDecimal(actualCostTotal); err != nil {
		return nil, err
	}
	if snapshot.ListCostDelta, err = parseAccountFinanceNullableDecimal(listCostDelta); err != nil {
		return nil, err
	}
	if snapshot.ActualCostDelta, err = parseAccountFinanceNullableDecimal(actualCostDelta); err != nil {
		return nil, err
	}
	if snapshot.ObservedMultiplier, err = parseAccountFinanceNullableDecimal(observedMultiplier); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func accountFinanceDecimalArgument(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func parseAccountFinanceNullableDecimal(value sql.NullString) (*decimal.Decimal, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func accountFinanceNullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func accountFinanceNullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func accountFinanceNullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}
