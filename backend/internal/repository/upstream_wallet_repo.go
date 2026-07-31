package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type upstreamWalletRepository struct {
	db *sql.DB
}

func NewUpstreamWalletRepository(db *sql.DB) service.UpstreamWalletRepository {
	return &upstreamWalletRepository{db: db}
}

const upstreamWalletSelect = `
SELECT w.id, w.upstream_id, w.name,
       CASE
         WHEN w.pricing_adapter = 'protocol' THEN 'protocol'
         WHEN w.pricing_adapter = 'newapi' THEN 'newapi'
         WHEN w.quota_adapter = 'legacy_openai' THEN 'legacy_openai_billing'
         ELSE 'manual'
       END AS adapter_type,
       COALESCE(w.base_url, ''),
       w.finance_access_token_encrypted,
	   w.protocol_version_id,
	   w.currency, w.balance_kind, COALESCE(w.balance_scope_key, ''), COALESCE(w.pricing_group, ''), w.enabled,
	   w.last_pricing_sync_at, w.pricing_sync_status, COALESCE(w.pricing_sync_error, ''),
	   w.last_balance_sync_at, w.balance_sync_status, COALESCE(w.balance_sync_error, ''),
	   w.last_quota_sync_at, w.quota_sync_status, COALESCE(w.quota_sync_error, ''),
       COALESCE(assignments.assigned_account_count, 0),
       w.created_at, w.updated_at, w.deleted_at
FROM upstream_wallets w
LEFT JOIN LATERAL (
  SELECT COUNT(*)::int AS assigned_account_count
  FROM upstream_wallet_accounts uwa
  WHERE uwa.wallet_id = w.id AND uwa.effective_to IS NULL
) assignments ON TRUE`

func (r *upstreamWalletRepository) ListWallets(ctx context.Context, upstreamID int64, includeDeleted bool) ([]service.UpstreamWallet, error) {
	query := upstreamWalletSelect + " WHERE w.upstream_id = $1"
	if !includeDeleted {
		query += " AND w.deleted_at IS NULL"
	}
	query += " ORDER BY w.id"
	rows, err := r.db.QueryContext(ctx, query, upstreamID)
	if err != nil {
		return nil, fmt.Errorf("list upstream wallets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UpstreamWallet, 0)
	for rows.Next() {
		item, scanErr := scanUpstreamWallet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *item)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream wallets: %w", err)
	}
	return items, nil
}

func (r *upstreamWalletRepository) GetWallet(ctx context.Context, id int64) (*service.UpstreamWallet, error) {
	item, err := scanUpstreamWallet(r.db.QueryRowContext(ctx, upstreamWalletSelect+" WHERE w.id = $1 AND w.deleted_at IS NULL", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUpstreamWalletNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upstream wallet: %w", err)
	}
	return item, nil
}

func (r *upstreamWalletRepository) CreateWallet(ctx context.Context, wallet *service.UpstreamWallet, pricingAdapter, balanceAdapter, quotaAdapter string) error {
	var baseURL any
	if wallet.BaseURL != "" {
		baseURL = wallet.BaseURL
	}
	var pricingGroup any
	if wallet.PricingGroup != "" {
		pricingGroup = wallet.PricingGroup
	}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO upstream_wallets (
  upstream_id, name, base_url, pricing_adapter, pricing_group, balance_adapter,
  quota_adapter, finance_access_token_encrypted, protocol_version_id, currency, balance_kind, balance_scope_key, enabled
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id, created_at, updated_at`,
		wallet.UpstreamID, wallet.Name, baseURL, pricingAdapter, pricingGroup, balanceAdapter,
		quotaAdapter, nullableBytes(wallet.EncryptedCredential), wallet.ProtocolVersionID, wallet.Currency, wallet.BalanceKind, nullableString(wallet.BalanceScopeKey), wallet.Enabled,
	).Scan(&wallet.ID, &wallet.CreatedAt, &wallet.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create upstream wallet: %w", err)
	}
	return nil
}

func (r *upstreamWalletRepository) UpdateWallet(ctx context.Context, wallet *service.UpstreamWallet, pricingAdapter, balanceAdapter, quotaAdapter string, credentialProvided bool) error {
	query := `
UPDATE upstream_wallets
SET name=$2, base_url=$3, pricing_adapter=$4, pricing_group=$5,
    balance_adapter=$6, quota_adapter=$7, currency=$8, balance_kind=$9,
    balance_scope_key=$10, enabled=$11, protocol_version_id=$12, updated_at=NOW()`
	args := []any{wallet.ID, wallet.Name, nullableString(wallet.BaseURL), pricingAdapter, nullableString(wallet.PricingGroup), balanceAdapter, quotaAdapter, wallet.Currency, wallet.BalanceKind, nullableString(wallet.BalanceScopeKey), wallet.Enabled, wallet.ProtocolVersionID}
	if credentialProvided {
		query += ", finance_access_token_encrypted=$13"
		args = append(args, nullableBytes(wallet.EncryptedCredential))
	}
	query += " WHERE id=$1 AND deleted_at IS NULL RETURNING updated_at"
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&wallet.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return service.ErrUpstreamWalletNotFound
	} else if err != nil {
		return fmt.Errorf("update upstream wallet: %w", err)
	}
	return nil
}

func (r *upstreamWalletRepository) IsBindableProtocolVersion(ctx context.Context, versionID int64) (bool, error) {
	var bindable bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM upstream_finance_protocol_versions v
  JOIN upstream_finance_protocols p ON p.id=v.protocol_id
  WHERE v.id=$1 AND v.validation_status='valid' AND v.published_at IS NOT NULL AND p.status='published'
)`, versionID).Scan(&bindable)
	if err != nil {
		return false, fmt.Errorf("validate upstream finance protocol binding: %w", err)
	}
	return bindable, nil
}

func (r *upstreamWalletRepository) SoftDeleteWallet(ctx context.Context, id int64, deletedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete upstream wallet: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE upstream_wallets
SET deleted_at=$2, enabled=FALSE, updated_at=NOW()
WHERE id=$1 AND deleted_at IS NULL`, id, deletedAt)
	if err != nil {
		return fmt.Errorf("soft delete upstream wallet: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return service.ErrUpstreamWalletNotFound
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE upstream_wallet_accounts
SET effective_to=$2
WHERE wallet_id=$1 AND effective_to IS NULL AND effective_from < $2`, id, deletedAt); err != nil {
		return fmt.Errorf("close upstream wallet assignments: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
DELETE FROM upstream_wallet_accounts
WHERE wallet_id=$1 AND effective_to IS NULL AND effective_from >= $2`, id, deletedAt); err != nil {
		return fmt.Errorf("cancel future upstream wallet assignments: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit delete upstream wallet: %w", err)
	}
	return nil
}

func (r *upstreamWalletRepository) AssignWalletAccounts(ctx context.Context, walletID int64, input service.UpstreamWalletAssignmentInput) error {
	accountIDs := append([]int64(nil), input.AccountIDs...)
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin assign upstream wallet: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var walletUpstreamBaseURL string
	if err = tx.QueryRowContext(ctx, `
SELECT u.normalized_base_url
FROM upstream_wallets w
JOIN upstreams u ON u.id=w.upstream_id AND u.deleted_at IS NULL
WHERE w.id=$1 AND w.deleted_at IS NULL AND w.enabled=TRUE
FOR UPDATE OF w,u`, walletID).Scan(&walletUpstreamBaseURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrUpstreamWalletNotFound
		}
		return fmt.Errorf("lock upstream wallet: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id,normalized_account_base_url(credentials,extra,platform,type)
FROM accounts
WHERE id = ANY($1) AND deleted_at IS NULL
ORDER BY id FOR UPDATE`, pq.Array(accountIDs))
	if err != nil {
		return fmt.Errorf("lock wallet accounts: %w", err)
	}
	locked := 0
	for rows.Next() {
		var accountID int64
		var accountBaseURL string
		if err = rows.Scan(&accountID, &accountBaseURL); err != nil {
			return fmt.Errorf("scan locked wallet account: %w", err)
		}
		locked++
		if strings.TrimSpace(accountBaseURL) == "" || !strings.EqualFold(strings.TrimSpace(accountBaseURL), strings.TrimSpace(walletUpstreamBaseURL)) {
			return service.ErrUpstreamWalletAccountMismatch
		}
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("close locked accounts: %w", closeErr)
	}
	if locked != len(accountIDs) {
		return errors.New("one or more accounts do not exist")
	}
	var latestConfirmed sql.NullTime
	if err = tx.QueryRowContext(ctx, `
SELECT MAX(ul.created_at)
FROM usage_finance_records ufr
JOIN usage_logs ul ON ul.id=ufr.usage_log_id
WHERE ul.account_id = ANY($1) AND ufr.cost_status='exact'`, pq.Array(accountIDs)).Scan(&latestConfirmed); err != nil {
		return fmt.Errorf("read latest confirmed finance record: %w", err)
	}
	if latestConfirmed.Valid && input.EffectiveAt.Before(latestConfirmed.Time) {
		return service.ErrUpstreamWalletAssignmentTooEarly
	}
	var conflicts int
	if err = tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM upstream_wallet_accounts
WHERE account_id = ANY($1)
  AND effective_from >= $2`, pq.Array(accountIDs), input.EffectiveAt).Scan(&conflicts); err != nil {
		return fmt.Errorf("check future wallet assignments: %w", err)
	}
	if conflicts > 0 {
		return service.ErrUpstreamWalletAssignmentConflict
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE upstream_wallet_accounts
SET effective_to=$2
WHERE account_id = ANY($1) AND effective_to IS NULL AND effective_from < $2`, pq.Array(accountIDs), input.EffectiveAt); err != nil {
		return fmt.Errorf("close current wallet assignments: %w", err)
	}
	for _, accountID := range accountIDs {
		if _, err = tx.ExecContext(ctx, `
INSERT INTO upstream_wallet_accounts (wallet_id, account_id, effective_from, reason, operator_id)
VALUES ($1,$2,$3,$4,$5)`, walletID, accountID, input.EffectiveAt, input.Reason, input.OperatorID); err != nil {
			return fmt.Errorf("insert upstream wallet assignment: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit upstream wallet assignments: %w", err)
	}
	return nil
}

func (r *upstreamWalletRepository) ListActiveWalletAccountIDs(ctx context.Context, walletID int64, effectiveAt time.Time) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT uwa.account_id
FROM upstream_wallet_accounts uwa
JOIN upstream_wallets w ON w.id=uwa.wallet_id AND w.deleted_at IS NULL AND w.enabled=TRUE
JOIN accounts a ON a.id=uwa.account_id AND a.deleted_at IS NULL AND a.status='active'
WHERE uwa.wallet_id=$1
  AND uwa.effective_from <= $2
  AND (uwa.effective_to IS NULL OR uwa.effective_to > $2)
ORDER BY uwa.account_id`, walletID, effectiveAt)
	if err != nil {
		return nil, fmt.Errorf("list active wallet accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	accountIDs := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		if err = rows.Scan(&accountID); err != nil {
			return nil, fmt.Errorf("scan active wallet account: %w", err)
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active wallet accounts: %w", err)
	}
	return accountIDs, nil
}

type upstreamWalletScanner interface {
	Scan(dest ...any) error
}

func scanUpstreamWallet(scanner upstreamWalletScanner) (*service.UpstreamWallet, error) {
	item := &service.UpstreamWallet{}
	var credential []byte
	err := scanner.Scan(
		&item.ID, &item.UpstreamID, &item.Name, &item.AdapterType, &item.BaseURL,
		&credential, &item.ProtocolVersionID, &item.Currency, &item.BalanceKind, &item.BalanceScopeKey, &item.PricingGroup, &item.Enabled,
		&item.LastPricingSyncAt, &item.PricingSyncStatus, &item.PricingSyncError,
		&item.LastBalanceSyncAt, &item.BalanceSyncStatus, &item.BalanceSyncError,
		&item.LastQuotaSyncAt, &item.QuotaSyncStatus, &item.QuotaSyncError,
		&item.AssignedAccountCount, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	item.EncryptedCredential = append([]byte(nil), credential...)
	item.CredentialConfigured = len(credential) > 0
	return item, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
