package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type accountFinanceProfileRepository struct {
	db             *sql.DB
	accountRepo    service.AccountRepository
	schedulerCache service.SchedulerCache
}

func NewAccountFinanceProfileRepository(db *sql.DB, accountRepo service.AccountRepository, schedulerCache service.SchedulerCache) service.AccountFinanceProfileRepository {
	return &accountFinanceProfileRepository{db: db, accountRepo: accountRepo, schedulerCache: schedulerCache}
}

const accountFinanceProfileColumns = `
id,account_id,wallet_id,protocol_version_id,cost_mode,pricing_group,endpoint_source,
endpoint_base_url_snapshot,credential_source,counter_scope,counter_scope_key,balance_unit_semantics,
recharge_owner_type,recharge_owner_id,account_multiplier_change_id,account_multiplier_snapshot,
raw_upstream_multiplier,contract_type,contract_multiplier,contract_multiplier_change_id,
readiness_status,readiness_detail,version,effective_from,effective_to,created_by,reason,created_at`

func (r *accountFinanceProfileRepository) CurrentAccountFinanceProfile(ctx context.Context, accountID int64) (*service.AccountFinanceProfile, error) {
	profile, err := scanAccountFinanceProfile(r.db.QueryRowContext(ctx, `SELECT `+accountFinanceProfileColumns+` FROM account_finance_profiles WHERE account_id=$1 AND effective_to IS NULL`, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAccountFinanceProfileNotFound
	}
	return profile, err
}

func (r *accountFinanceProfileRepository) ReplaceAccountFinanceProfile(ctx context.Context, accountID int64, input service.AccountFinanceProfileInput, profile service.AccountFinanceProfile) (*service.AccountFinanceProfile, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var currentVersion int
	var currentEffectiveFrom time.Time
	err = tx.QueryRowContext(ctx, `SELECT version,effective_from FROM account_finance_profiles WHERE account_id=$1 AND effective_to IS NULL FOR UPDATE`, accountID).Scan(&currentVersion, &currentEffectiveFrom)
	if errors.Is(err, sql.ErrNoRows) {
		if input.ExpectedVersion != 0 {
			return nil, service.ErrAccountFinanceProfileConflict
		}
		currentVersion = 0
	} else if err != nil {
		return nil, err
	} else {
		if input.ExpectedVersion != currentVersion || !profile.EffectiveFrom.After(currentEffectiveFrom) {
			return nil, service.ErrAccountFinanceProfileConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE account_finance_profiles SET effective_to=$2 WHERE account_id=$1 AND effective_to IS NULL AND version=$3`, accountID, profile.EffectiveFrom, currentVersion); err != nil {
			return nil, err
		}
	}
	profile.Version = currentVersion + 1
	readinessDetail, err := json.Marshal(profile.ReadinessDetail)
	if err != nil {
		return nil, err
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO account_finance_profiles(
 account_id,wallet_id,protocol_version_id,cost_mode,pricing_group,endpoint_source,endpoint_base_url_snapshot,
 credential_source,counter_scope,counter_scope_key,balance_unit_semantics,recharge_owner_type,recharge_owner_id,
 account_multiplier_change_id,account_multiplier_snapshot,raw_upstream_multiplier,contract_type,contract_multiplier,
 contract_multiplier_change_id,readiness_status,readiness_detail,version,effective_from,created_by,reason
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21::jsonb,$22,$23,$24,$25)
RETURNING `+accountFinanceProfileColumns,
		accountID, profile.WalletID, profile.ProtocolVersionID, profile.CostMode, profile.PricingGroup, profile.EndpointSource,
		profile.EndpointBaseURLSnapshot, profile.CredentialSource, profile.CounterScope, profile.CounterScopeKey,
		profile.BalanceUnitSemantics, profile.RechargeOwnerType, profile.RechargeOwnerID, profile.AccountMultiplierChangeID,
		accountFinanceDecimalArgument(profile.AccountMultiplierSnapshot), accountFinanceDecimalArgument(profile.RawUpstreamMultiplier),
		profile.ContractType, accountFinanceDecimalArgument(profile.ContractMultiplier), profile.ContractMultiplierChangeID,
		profile.ReadinessStatus, readinessDetail, profile.Version, profile.EffectiveFrom, profile.CreatedBy, profile.Reason)
	created, err := scanAccountFinanceProfile(row)
	if err != nil {
		if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
			return nil, service.ErrAccountFinanceProfileConflict
		}
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE accounts SET current_finance_profile_id=$2 WHERE id=$1`, accountID, created.ID); err != nil {
		return nil, err
	}
	if err = enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if r.accountRepo != nil && r.schedulerCache != nil {
		account, readErr := r.accountRepo.GetByID(ctx, accountID)
		if readErr != nil {
			logger.LegacyPrintf("repository.account_finance_profile", "[Scheduler] refresh finance profile snapshot read failed: account=%d err=%v", accountID, readErr)
		} else if writeErr := r.schedulerCache.SetAccount(ctx, account); writeErr != nil {
			logger.LegacyPrintf("repository.account_finance_profile", "[Scheduler] refresh finance profile snapshot write failed: account=%d err=%v", accountID, writeErr)
		}
	}
	return created, nil
}

func (r *accountFinanceProfileRepository) AccountFinanceReadinessEvidence(ctx context.Context, accountID int64, profile *service.AccountFinanceProfile) (service.AccountFinanceReadinessEvidence, error) {
	var result service.AccountFinanceReadinessEvidence
	var multiplier sql.NullString
	var changeID sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `
SELECT a.upstream_cost_multiplier::text,
       (SELECT id FROM account_upstream_multiplier_changes c WHERE c.account_id=a.id AND c.effective_at<=NOW() ORDER BY c.effective_at DESC,c.id DESC LIMIT 1)
FROM accounts a WHERE a.id=$1`, accountID).Scan(&multiplier, &changeID); err != nil {
		return result, err
	}
	if multiplier.Valid {
		value, err := decimal.NewFromString(multiplier.String)
		if err != nil {
			return result, err
		}
		result.AccountMultiplier = &value
	}
	if changeID.Valid {
		value := changeID.Int64
		result.AccountMultiplierChangeID = &value
	}
	if profile == nil {
		return result, nil
	}
	var walletID any
	if profile.WalletID != nil {
		walletID = *profile.WalletID
	}
	var protocolVersionID any
	if profile.ProtocolVersionID != nil {
		protocolVersionID = *profile.ProtocolVersionID
	}
	if err := r.db.QueryRowContext(ctx, `
SELECT COALESCE((SELECT status IN ('failed','unsupported') FROM upstream_finance_sync_runs WHERE wallet_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1),false),
       EXISTS(SELECT 1 FROM upstream_model_price_versions WHERE wallet_id=$1 AND effective_to IS NULL),
       EXISTS(SELECT 1 FROM upstream_cost_settlement_intervals WHERE account_id=$2 AND account_finance_profile_id=$3 AND status='settled'),
       COALESCE((SELECT v.published_at IS NOT NULL AND v.validation_status='valid' AND p.status='published'
                         AND (v.config->'capabilities') ? 'request_charge'
                         AND v.config->>'cost_mode'=$5
                         AND v.config->>'unit_semantics'=$6
                    FROM upstream_finance_protocol_versions v
                    JOIN upstream_finance_protocols p ON p.id=v.protocol_id
                   WHERE v.id=$4),false)`, walletID, accountID, profile.ID, protocolVersionID, profile.CostMode, profile.BalanceUnitSemantics).
		Scan(&result.LatestSyncFailed, &result.HasActiveCatalogPrice, &result.HasSettledInterval, &result.ProtocolReady); err != nil {
		return result, err
	}
	return result, nil
}

func scanAccountFinanceProfile(scanner interface{ Scan(...any) error }) (*service.AccountFinanceProfile, error) {
	item := &service.AccountFinanceProfile{}
	var walletID, protocolVersionID, rechargeOwnerID, multiplierChangeID, contractChangeID, createdBy sql.NullInt64
	var pricingGroup, counterScopeKey, rechargeOwnerType, contractType sql.NullString
	var multiplierSnapshot, rawMultiplier, contractMultiplier sql.NullString
	var effectiveTo sql.NullTime
	var readinessDetail []byte
	if err := scanner.Scan(
		&item.ID, &item.AccountID, &walletID, &protocolVersionID, &item.CostMode, &pricingGroup, &item.EndpointSource,
		&item.EndpointBaseURLSnapshot, &item.CredentialSource, &item.CounterScope, &counterScopeKey, &item.BalanceUnitSemantics,
		&rechargeOwnerType, &rechargeOwnerID, &multiplierChangeID, &multiplierSnapshot, &rawMultiplier, &contractType,
		&contractMultiplier, &contractChangeID, &item.ReadinessStatus, &readinessDetail, &item.Version, &item.EffectiveFrom,
		&effectiveTo, &createdBy, &item.Reason, &item.CreatedAt,
	); err != nil {
		return nil, err
	}
	item.WalletID = nullableInt64Pointer(walletID)
	item.ProtocolVersionID = nullableInt64Pointer(protocolVersionID)
	item.RechargeOwnerID = nullableInt64Pointer(rechargeOwnerID)
	item.AccountMultiplierChangeID = nullableInt64Pointer(multiplierChangeID)
	item.ContractMultiplierChangeID = nullableInt64Pointer(contractChangeID)
	item.CreatedBy = nullableInt64Pointer(createdBy)
	item.PricingGroup = accountFinanceNullableStringPointer(pricingGroup)
	item.CounterScopeKey = accountFinanceNullableStringPointer(counterScopeKey)
	item.RechargeOwnerType = accountFinanceNullableStringPointer(rechargeOwnerType)
	item.ContractType = accountFinanceNullableStringPointer(contractType)
	var err error
	if item.AccountMultiplierSnapshot, err = settlementNullableDecimal(multiplierSnapshot); err != nil {
		return nil, err
	}
	if item.RawUpstreamMultiplier, err = settlementNullableDecimal(rawMultiplier); err != nil {
		return nil, err
	}
	if item.ContractMultiplier, err = settlementNullableDecimal(contractMultiplier); err != nil {
		return nil, err
	}
	if effectiveTo.Valid {
		value := effectiveTo.Time
		item.EffectiveTo = &value
	}
	if len(readinessDetail) > 0 {
		if err = json.Unmarshal(readinessDetail, &item.ReadinessDetail); err != nil {
			return nil, err
		}
	}
	if item.ReadinessDetail == nil {
		item.ReadinessDetail = map[string]any{}
	}
	return item, nil
}

func accountFinanceNullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
