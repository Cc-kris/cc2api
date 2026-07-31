package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type upstreamFinanceSyncRepository struct{ db *sql.DB }

func NewUpstreamFinanceSyncRepository(db *sql.DB) service.UpstreamFinanceSyncRepository {
	return &upstreamFinanceSyncRepository{db: db}
}

func (r *upstreamFinanceSyncRepository) CreateOrGetActiveSyncJob(ctx context.Context, walletID int64, syncType string, operatorID *int64) (*service.UpstreamFinanceSyncJob, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin finance sync enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, fmt.Sprintf("finance-sync:%d:%s", walletID, syncType)); err != nil {
		return nil, false, fmt.Errorf("lock finance sync enqueue: %w", err)
	}
	existing, err := queryFinanceSyncJob(ctx, tx, `
SELECT j.id, r.wallet_id, r.sync_type, j.status, j.progress::text,
       j.processed_count, j.success_count, j.failed_count, COALESCE(j.error_summary,''),
       j.created_at, j.started_at, j.finished_at
FROM upstream_finance_sync_runs r
JOIN finance_async_jobs j ON j.id=r.async_job_id
WHERE r.wallet_id=$1 AND r.sync_type=$2 AND r.status IN ('queued','running')
ORDER BY r.id DESC LIMIT 1`, walletID, syncType)
	if err == nil {
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, false, commitErr
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	parameters, _ := json.Marshal(map[string]any{"wallet_id": walletID, "sync_type": syncType})
	job := &service.UpstreamFinanceSyncJob{WalletID: walletID, SyncType: syncType, Status: "queued", Progress: "0"}
	err = tx.QueryRowContext(ctx, `
INSERT INTO finance_async_jobs (job_type,status,parameters,operator_id)
VALUES ('upstream_finance_sync','queued',$1,$2)
RETURNING id, created_at`, parameters, operatorID).Scan(&job.ID, &job.CreatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("create finance sync job: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO upstream_finance_sync_runs (async_job_id,wallet_id,sync_type,status,started_at)
VALUES ($1,$2,$3,'queued',NOW())`, job.ID, walletID, syncType); err != nil {
		return nil, false, fmt.Errorf("create upstream finance sync run: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit finance sync enqueue: %w", err)
	}
	return job, true, nil
}

func (r *upstreamFinanceSyncRepository) ClaimNextSyncJob(ctx context.Context, leaseOwner string, leaseUntil time.Time) (*service.UpstreamFinanceSyncJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := queryFinanceSyncJob(ctx, tx, `
SELECT j.id, r.wallet_id, r.sync_type, j.status, j.progress::text,
       j.processed_count, j.success_count, j.failed_count, COALESCE(j.error_summary,''),
       j.created_at, j.started_at, j.finished_at
FROM finance_async_jobs j
JOIN upstream_finance_sync_runs r ON r.async_job_id=j.id
WHERE j.job_type='upstream_finance_sync'
  AND (j.status='queued' OR (j.status='running' AND (j.lease_expires_at IS NULL OR j.lease_expires_at < NOW())))
ORDER BY j.created_at, j.id
FOR UPDATE OF j SKIP LOCKED
LIMIT 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status='running', lease_owner=$2, lease_expires_at=$3,
    started_at=COALESCE(started_at,$4), updated_at=$4
WHERE id=$1`, job.ID, leaseOwner, leaseUntil, now); err != nil {
		return nil, fmt.Errorf("claim finance sync job: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE upstream_finance_sync_runs SET status='running', started_at=$2
WHERE async_job_id=$1`, job.ID, now); err != nil {
		return nil, fmt.Errorf("mark finance sync run running: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	job.Status = "running"
	job.StartedAt = &now
	return job, nil
}

func (r *upstreamFinanceSyncRepository) RenewSyncJobLease(ctx context.Context, jobID int64, leaseOwner string, leaseUntil time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE finance_async_jobs SET lease_expires_at=$3,updated_at=NOW()
WHERE id=$1 AND lease_owner=$2 AND job_type='upstream_finance_sync' AND status='running'`, jobID, leaseOwner, leaseUntil)
	return requireFinanceSyncLeaseResult(result, err)
}

func (r *upstreamFinanceSyncRepository) CompletePricingSync(ctx context.Context, job *service.UpstreamFinanceSyncJob, leaseOwner string, prices []service.UpstreamFinancePrice, finishedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	created, skipped := int64(0), int64(0)
	for _, price := range prices {
		inserted, saveErr := saveUpstreamPriceVersion(ctx, tx, job.WalletID, price, finishedAt)
		if saveErr != nil {
			return saveErr
		}
		if inserted {
			created++
		} else {
			skipped++
		}
	}
	if err = closeRemovedUpstreamPriceVersions(ctx, tx, job.WalletID, prices, finishedAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE upstream_wallets
SET last_pricing_sync_at=$2, pricing_sync_status='success', pricing_sync_error=NULL, updated_at=NOW()
WHERE id=$1`, job.WalletID, finishedAt); err != nil {
		return fmt.Errorf("update wallet pricing sync status: %w", err)
	}
	if err = completeFinanceSyncTx(ctx, tx, job, leaseOwner, "success", created, skipped, "", finishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *upstreamFinanceSyncRepository) CompleteBalanceSync(ctx context.Context, job *service.UpstreamFinanceSyncJob, leaseOwner string, balance *service.UpstreamFinanceBalance, finishedAt time.Time) error {
	if balance == nil {
		return errors.New("upstream balance result is nil")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	dedupeKey, err := financeChecksum(map[string]any{
		"wallet_id": job.WalletID, "kind": balance.BalanceKind, "balance": decimalString(balance.BalanceAmount),
		"total": decimalString(balance.TotalQuota), "used": decimalString(balance.UsedQuota),
		"currency": balance.Currency, "collected_at": balance.CollectedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	safeSnapshot, _ := json.Marshal(balance.SafeSnapshot)
	result, err := tx.ExecContext(ctx, `
INSERT INTO upstream_balance_snapshots (
  wallet_id,dedupe_key,balance_kind,balance_amount,total_quota,used_quota,currency,source,collected_at,sync_status,safe_snapshot
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'success',$10)
ON CONFLICT (wallet_id,dedupe_key) DO NOTHING`,
		job.WalletID, dedupeKey, balance.BalanceKind, balance.BalanceAmount, balance.TotalQuota, balance.UsedQuota,
		balance.Currency, balance.Source, balance.CollectedAt, safeSnapshot)
	if err != nil {
		return fmt.Errorf("insert upstream balance snapshot: %w", err)
	}
	created, _ := result.RowsAffected()
	skipped := int64(0)
	if created == 0 {
		skipped = 1
	}
	if job.SyncType == service.UpstreamFinanceSyncBalance {
		_, err = tx.ExecContext(ctx, `UPDATE upstream_wallets SET last_balance_sync_at=$2,balance_sync_status='success',balance_sync_error=NULL,updated_at=NOW() WHERE id=$1`, job.WalletID, finishedAt)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE upstream_wallets SET last_quota_sync_at=$2,quota_sync_status='success',quota_sync_error=NULL,updated_at=NOW() WHERE id=$1`, job.WalletID, finishedAt)
	}
	if err != nil {
		return fmt.Errorf("update wallet balance sync status: %w", err)
	}
	if err = completeFinanceSyncTx(ctx, tx, job, leaseOwner, "success", created, skipped, "", finishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *upstreamFinanceSyncRepository) CompleteFundingSync(ctx context.Context, job *service.UpstreamFinanceSyncJob, leaseOwner string, collected, skipped int64, finishedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = completeFinanceSyncTx(ctx, tx, job, leaseOwner, "success", collected, skipped, "", finishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *upstreamFinanceSyncRepository) CompleteAccountUsageSync(ctx context.Context, job *service.UpstreamFinanceSyncJob, leaseOwner string, collected, skipped int64, finishedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = completeFinanceSyncTx(ctx, tx, job, leaseOwner, "success", collected, skipped, "", finishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *upstreamFinanceSyncRepository) FailSyncJob(ctx context.Context, job *service.UpstreamFinanceSyncJob, leaseOwner, summary string, finishedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	status := "failed"
	if summary == "capability unsupported" {
		status = "unsupported"
	}
	if job.SyncType == service.UpstreamFinanceSyncPricing {
		_, err = tx.ExecContext(ctx, `UPDATE upstream_wallets SET pricing_sync_status=$2,pricing_sync_error=$3,updated_at=NOW() WHERE id=$1`, job.WalletID, status, summary)
	} else if job.SyncType == service.UpstreamFinanceSyncBalance {
		_, err = tx.ExecContext(ctx, `UPDATE upstream_wallets SET balance_sync_status=$2,balance_sync_error=$3,updated_at=NOW() WHERE id=$1`, job.WalletID, status, summary)
	} else if job.SyncType == service.UpstreamFinanceSyncQuota {
		_, err = tx.ExecContext(ctx, `UPDATE upstream_wallets SET quota_sync_status=$2,quota_sync_error=$3,updated_at=NOW() WHERE id=$1`, job.WalletID, status, summary)
	}
	if err != nil {
		return err
	}
	if err = completeFinanceSyncTx(ctx, tx, job, leaseOwner, status, 0, 0, summary, finishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *upstreamFinanceSyncRepository) RecordProbe(ctx context.Context, walletID int64, probe service.UpstreamFinanceProbe) error {
	status := "success"
	if !probe.Reachable {
		status = "failed"
	} else if probe.Capabilities.Pricing != service.FinanceCapabilitySupported && probe.Capabilities.WalletBalance != service.FinanceCapabilitySupported && probe.Capabilities.TokenQuota != service.FinanceCapabilitySupported {
		status = "partial"
	}
	finishedAt := probe.ProbedAt.Add(time.Duration(probe.LatencyMS) * time.Millisecond)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO upstream_finance_sync_runs (wallet_id,sync_type,status,duration_ms,error_summary,started_at,finished_at)
VALUES ($1,'probe',$2,$3,$4,$5,$6)`, walletID, status, probe.LatencyMS, nullableString(probe.ErrorSummary), probe.ProbedAt, finishedAt)
	if err != nil {
		return fmt.Errorf("record upstream finance probe: %w", err)
	}
	return nil
}

func (r *upstreamFinanceSyncRepository) ListPriceVersions(ctx context.Context, walletID int64, filter service.UpstreamFinancePriceListFilter) ([]service.UpstreamFinancePriceVersion, int64, error) {
	where := "wallet_id=$1"
	args := []any{walletID}
	if strings.TrimSpace(filter.Model) != "" {
		args = append(args, "%"+strings.TrimSpace(filter.Model)+"%")
		where += fmt.Sprintf(" AND model_pattern ILIKE $%d", len(args))
	}
	if filter.EffectiveAt != nil {
		args = append(args, *filter.EffectiveAt)
		where += fmt.Sprintf(" AND effective_from <= $%d AND (effective_to IS NULL OR effective_to > $%d)", len(args), len(args))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM upstream_model_price_versions WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	query := fmt.Sprintf(`
SELECT id,wallet_id,model_pattern,is_wildcard,billing_mode,COALESCE(service_tier,''),price_detail,
       currency,source,checksum,effective_from,effective_to,created_at
FROM upstream_model_price_versions WHERE %s
ORDER BY effective_from DESC,id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UpstreamFinancePriceVersion, 0)
	for rows.Next() {
		var item service.UpstreamFinancePriceVersion
		var detail []byte
		if err = rows.Scan(&item.ID, &item.WalletID, &item.ModelPattern, &item.IsWildcard, &item.BillingMode, &item.ServiceTier, &detail, &item.Currency, &item.Source, &item.Checksum, &item.EffectiveFrom, &item.EffectiveTo, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal(detail, &item.PriceDetail); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *upstreamFinanceSyncRepository) ImportPriceVersions(ctx context.Context, walletID int64, prices []service.UpstreamFinancePrice, effectiveAt time.Time) (int64, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	prices = append([]service.UpstreamFinancePrice(nil), prices...)
	sort.SliceStable(prices, func(i, j int) bool { return prices[i].EffectiveAt.Before(prices[j].EffectiveAt) })
	created, skipped := int64(0), int64(0)
	for _, price := range prices {
		rowEffectiveAt := price.EffectiveAt
		if rowEffectiveAt.IsZero() {
			rowEffectiveAt = effectiveAt
		}
		inserted, saveErr := saveUpstreamPriceVersion(ctx, tx, walletID, price, rowEffectiveAt)
		if saveErr != nil {
			return 0, 0, saveErr
		}
		if inserted {
			created++
		} else {
			skipped++
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return created, skipped, nil
}

func (r *upstreamFinanceSyncRepository) ListSyncHistory(ctx context.Context, walletID int64, filter service.UpstreamFinanceSyncHistoryFilter) ([]service.UpstreamFinanceSyncHistory, int64, error) {
	where := "wallet_id=$1"
	args := []any{walletID}
	if filter.SyncType != "" {
		args = append(args, filter.SyncType)
		where += fmt.Sprintf(" AND sync_type=$%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM upstream_finance_sync_runs WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	query := fmt.Sprintf(`
SELECT id,async_job_id,wallet_id,sync_type,status,collected_count,skipped_count,upstream_status,duration_ms,
       COALESCE(error_summary,''),started_at,finished_at,created_at
FROM upstream_finance_sync_runs WHERE %s ORDER BY created_at DESC,id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UpstreamFinanceSyncHistory, 0)
	for rows.Next() {
		var item service.UpstreamFinanceSyncHistory
		if err = rows.Scan(&item.ID, &item.AsyncJobID, &item.WalletID, &item.SyncType, &item.Status, &item.CollectedCount, &item.SkippedCount, &item.UpstreamStatus, &item.DurationMS, &item.ErrorSummary, &item.StartedAt, &item.FinishedAt, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *upstreamFinanceSyncRepository) ListDueSyncRequests(ctx context.Context, now time.Time) ([]service.UpstreamFinanceSyncRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT wallet_id,sync_type FROM (
  SELECT id AS wallet_id,'pricing'::text AS sync_type
  FROM upstream_wallets
  WHERE deleted_at IS NULL AND enabled=TRUE AND pricing_adapter NOT IN ('manual','none')
    AND (last_pricing_sync_at IS NULL OR last_pricing_sync_at < $1::timestamptz - INTERVAL '6 hours')
  UNION ALL
  SELECT id,'balance'::text
  FROM upstream_wallets
  WHERE deleted_at IS NULL AND enabled=TRUE AND balance_adapter NOT IN ('manual','none')
    AND (last_balance_sync_at IS NULL OR last_balance_sync_at < $1::timestamptz - INTERVAL '10 minutes')
  UNION ALL
  SELECT id,'quota'::text
  FROM upstream_wallets
  WHERE deleted_at IS NULL AND enabled=TRUE AND quota_adapter <> 'none'
    AND (last_quota_sync_at IS NULL OR last_quota_sync_at < $1::timestamptz - INTERVAL '10 minutes')
  UNION ALL
  SELECT w.id,'account_usage'::text
  FROM upstream_wallets w
  JOIN upstream_finance_protocol_versions v ON v.id=w.protocol_version_id
  JOIN upstream_finance_protocols p ON p.id=v.protocol_id
  WHERE w.deleted_at IS NULL AND w.enabled=TRUE AND w.pricing_adapter='protocol'
    AND p.status='published' AND v.published_at IS NOT NULL AND v.validation_status='valid'
    AND v.config->'capabilities' ? 'account_usage'
    AND v.config->'operations' ? 'account_usage'
    AND EXISTS (
      SELECT 1 FROM upstream_wallet_accounts uwa
      JOIN accounts a ON a.id=uwa.account_id AND a.deleted_at IS NULL AND a.status='active'
      WHERE uwa.wallet_id=w.id AND uwa.effective_from <= $1
        AND (uwa.effective_to IS NULL OR uwa.effective_to > $1)
    )
    AND NOT EXISTS (
      SELECT 1 FROM upstream_finance_sync_runs r
      WHERE r.wallet_id=w.id AND r.sync_type='account_usage' AND r.status='success'
        AND r.finished_at >= $1::timestamptz - INTERVAL '5 minutes'
    )
) due ORDER BY wallet_id,sync_type`, now)
	if err != nil {
		return nil, fmt.Errorf("list due upstream finance syncs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UpstreamFinanceSyncRequest, 0)
	for rows.Next() {
		var item service.UpstreamFinanceSyncRequest
		if err = rows.Scan(&item.WalletID, &item.SyncType); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type financeSyncQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func queryFinanceSyncJob(ctx context.Context, q financeSyncQueryer, query string, args ...any) (*service.UpstreamFinanceSyncJob, error) {
	job := &service.UpstreamFinanceSyncJob{}
	err := q.QueryRowContext(ctx, query, args...).Scan(
		&job.ID, &job.WalletID, &job.SyncType, &job.Status, &job.Progress,
		&job.ProcessedCount, &job.SuccessCount, &job.FailedCount, &job.ErrorSummary,
		&job.CreatedAt, &job.StartedAt, &job.FinishedAt,
	)
	return job, err
}

func saveUpstreamPriceVersion(ctx context.Context, tx *sql.Tx, walletID int64, price service.UpstreamFinancePrice, effectiveAt time.Time) (bool, error) {
	checksum, err := financeChecksum(map[string]any{
		"model_pattern": price.ModelPattern, "is_wildcard": price.IsWildcard, "billing_mode": price.BillingMode,
		"service_tier": price.ServiceTier, "price_detail": price.PriceDetail, "currency": price.Currency, "source": price.Source,
	})
	if err != nil {
		return false, err
	}
	var activeID int64
	var activeChecksum string
	var activeFrom time.Time
	var activeTo sql.NullTime
	err = tx.QueryRowContext(ctx, `
SELECT id,checksum,effective_from,effective_to FROM upstream_model_price_versions
WHERE wallet_id=$1 AND model_pattern=$2 AND billing_mode=$3 AND COALESCE(service_tier,'')=$4
  AND effective_from <= $5 AND (effective_to IS NULL OR effective_to > $5)
ORDER BY effective_from DESC,id DESC LIMIT 1
FOR UPDATE`, walletID, price.ModelPattern, price.BillingMode, price.ServiceTier, effectiveAt).Scan(&activeID, &activeChecksum, &activeFrom, &activeTo)
	if err == nil && activeChecksum == checksum {
		return false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read current upstream price version: %w", err)
	}
	if activeID > 0 {
		if activeFrom.Equal(effectiveAt) {
			return false, errors.New("a different price version already starts at the same effective_at")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE upstream_model_price_versions SET effective_to=$2 WHERE id=$1`, activeID, effectiveAt); err != nil {
			return false, fmt.Errorf("close upstream price version: %w", err)
		}
	}
	var nextEffective sql.NullTime
	if err = tx.QueryRowContext(ctx, `
SELECT MIN(effective_from) FROM upstream_model_price_versions
WHERE wallet_id=$1 AND model_pattern=$2 AND billing_mode=$3 AND COALESCE(service_tier,'')=$4 AND effective_from > $5`,
		walletID, price.ModelPattern, price.BillingMode, price.ServiceTier, effectiveAt).Scan(&nextEffective); err != nil {
		return false, fmt.Errorf("find next upstream price version: %w", err)
	}
	detail, _ := json.Marshal(price.PriceDetail)
	snapshot, _ := json.Marshal(price.SourceSnapshot)
	_, err = tx.ExecContext(ctx, `
INSERT INTO upstream_model_price_versions (
 wallet_id,model_pattern,is_wildcard,billing_mode,service_tier,price_detail,currency,source,source_snapshot,checksum,effective_from,effective_to
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		walletID, price.ModelPattern, price.IsWildcard, price.BillingMode, nullableString(price.ServiceTier), detail,
		price.Currency, price.Source, snapshot, checksum, effectiveAt, nullableTime(nextEffective))
	if err != nil {
		return false, fmt.Errorf("insert upstream price version: %w", err)
	}
	return true, nil
}

func closeRemovedUpstreamPriceVersions(ctx context.Context, tx *sql.Tx, walletID int64, prices []service.UpstreamFinancePrice, effectiveAt time.Time) error {
	type priceKey struct {
		modelPattern string
		billingMode  string
		serviceTier  string
	}
	current := make(map[priceKey]struct{}, len(prices))
	for _, price := range prices {
		current[priceKey{modelPattern: price.ModelPattern, billingMode: price.BillingMode, serviceTier: price.ServiceTier}] = struct{}{}
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id,model_pattern,billing_mode,COALESCE(service_tier,'')
FROM upstream_model_price_versions
WHERE wallet_id=$1 AND effective_from <= $2 AND (effective_to IS NULL OR effective_to > $2)
FOR UPDATE`, walletID, effectiveAt)
	if err != nil {
		return fmt.Errorf("list active upstream price versions: %w", err)
	}
	defer rows.Close()
	removedIDs := make([]int64, 0)
	for rows.Next() {
		var id int64
		var key priceKey
		if err = rows.Scan(&id, &key.modelPattern, &key.billingMode, &key.serviceTier); err != nil {
			return fmt.Errorf("scan active upstream price version: %w", err)
		}
		if _, exists := current[key]; !exists {
			removedIDs = append(removedIDs, id)
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate active upstream price versions: %w", err)
	}
	for _, id := range removedIDs {
		if _, err = tx.ExecContext(ctx, `UPDATE upstream_model_price_versions SET effective_to=$2 WHERE id=$1`, id, effectiveAt); err != nil {
			return fmt.Errorf("close removed upstream price version: %w", err)
		}
	}
	return nil
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func completeFinanceSyncTx(ctx context.Context, tx *sql.Tx, job *service.UpstreamFinanceSyncJob, leaseOwner, runStatus string, collected, skipped int64, summary string, finishedAt time.Time) error {
	jobStatus := "completed"
	failedCount := int64(0)
	progress := "1"
	if runStatus == "failed" || runStatus == "unsupported" {
		jobStatus = "failed"
		failedCount = 1
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE upstream_finance_sync_runs
SET status=$2,collected_count=$3,skipped_count=$4,error_summary=$5,
    duration_ms=GREATEST(0,EXTRACT(EPOCH FROM ($6-started_at))*1000)::bigint,finished_at=$6
WHERE async_job_id=$1`, job.ID, runStatus, collected, skipped, nullableString(summary), finishedAt); err != nil {
		return fmt.Errorf("complete upstream finance sync run: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status=$2,progress=$3,processed_count=$4,success_count=$5,failed_count=$6,
    error_summary=$7,finished_at=$8,lease_owner=NULL,lease_expires_at=NULL,updated_at=$8
WHERE id=$1 AND lease_owner=$9 AND job_type='upstream_finance_sync' AND status='running'`,
		job.ID, jobStatus, progress, collected+skipped, collected, failedCount, nullableString(summary), finishedAt, leaseOwner)
	if err = requireFinanceSyncLeaseResult(result, err); err != nil {
		return err
	}
	return nil
}

func requireFinanceSyncLeaseResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrUpstreamFinanceSyncLeaseLost
	}
	return nil
}

func financeChecksum(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal finance checksum: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func decimalString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}
