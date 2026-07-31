package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
)

const (
	AccountFinanceUnitFiatCurrency              = "fiat_currency"
	AccountFinanceUnitPlatformCredit            = "platform_credit"
	AccountFinanceMultiplierSourceUpstreamUsage = "upstream_usage"

	AccountFinanceDerivationBaseline          = "baseline"
	AccountFinanceDerivationRawOnly           = "raw_only"
	AccountFinanceDerivationMissingValues     = "missing_values"
	AccountFinanceDerivationBoundaryChanged   = "boundary_changed"
	AccountFinanceDerivationTimeReversed      = "time_reversed"
	AccountFinanceDerivationCounterReset      = "counter_reset"
	AccountFinanceDerivationNoActivity        = "no_activity"
	AccountFinanceDerivationInvalidList       = "invalid_list_delta"
	AccountFinanceDerivationCandidate         = "candidate"
	AccountFinanceDerivationSettlementReady   = "settlement_ready"
	AccountFinanceDerivationApplied           = "applied"
	AccountFinanceDerivationUnchanged         = "unchanged"
	AccountFinanceDerivationConflict          = "conflict"
	AccountFinanceDerivationInactiveAccount   = "inactive_account"
	AccountFinanceDerivationInvalidMultiplier = "invalid_multiplier"

	AccountFinanceAnomalyMultiplierJump = "multiplier_jump"
)

var (
	ErrAccountFinanceSnapshotInvalid         = errors.New("account finance counter snapshot is invalid")
	ErrAccountFinanceSnapshotRepoUnavailable = errors.New("account finance counter snapshot repository is unavailable")
	ErrAccountFinanceCounterOwnerConflict    = errors.New("upstream cumulative counter is already owned by another account")
)

// AccountFinanceCounterSnapshot is the immutable cumulative upstream cost fact.
// Derived fields only describe how this fact was classified locally; recharge
// ratios and wallet balances are deliberately absent from this model.
type AccountFinanceCounterSnapshot struct {
	ID                      int64
	AccountID               int64
	AccountFinanceProfileID *int64
	ScopeKey                string
	IdempotencyKey          string
	UpstreamCounterID       *string
	CounterPeriod           *string
	ListCostTotal           *decimal.Decimal
	ActualCostTotal         *decimal.Decimal
	UnitCode                string
	UnitSemantics           string
	Currency                *string
	UpstreamObservedAt      *time.Time
	CollectedAt             time.Time
	SafeSnapshot            map[string]any
	Checksum                string
	PreviousSnapshotID      *int64
	ListCostDelta           *decimal.Decimal
	ActualCostDelta         *decimal.Decimal
	ObservedMultiplier      *decimal.Decimal
	DerivationStatus        string
	AnomalyCode             *string
	MultiplierChangeID      *int64
	MultiplierEffectiveAt   *time.Time
	CreatedAt               time.Time
}

type AccountFinanceCounterObservation struct {
	AccountID               int64
	AccountFinanceProfileID *int64
	WalletID                int64
	ProtocolVersionID       *int64
	CounterIdentityKey      string
	ScopeKey                string
	IdempotencyKey          string
	UpstreamCounterID       *string
	CounterPeriod           *string
	ListCostTotal           *decimal.Decimal
	ActualCostTotal         *decimal.Decimal
	UnitCode                string
	UnitSemantics           string
	Currency                *string
	UpstreamObservedAt      *time.Time
	CollectedAt             time.Time
	SafeSnapshot            map[string]any
}

type AccountFinanceMultiplierVersion struct {
	ID            int64
	AccountID     int64
	OldMultiplier *decimal.Decimal
	NewMultiplier decimal.Decimal
	EffectiveAt   time.Time
	Reason        string
}

type AccountFinanceSnapshotRepository interface {
	WithAccountSyncLock(ctx context.Context, accountID int64, scopeKey string, fn func(context.Context) error) error
	ClaimCounterOwner(ctx context.Context, identityKey string, walletID int64, protocolVersionID *int64, accountID int64, upstreamCounterID, counterPeriod *string) error
	LatestCounterSnapshot(ctx context.Context, accountID int64, scopeKey string) (*AccountFinanceCounterSnapshot, error)
	CounterSnapshotByID(ctx context.Context, id int64) (*AccountFinanceCounterSnapshot, error)
	CreateCounterSnapshot(ctx context.Context, snapshot *AccountFinanceCounterSnapshot) (*AccountFinanceCounterSnapshot, bool, error)
	MarkCounterSnapshotMultiplierResult(ctx context.Context, snapshotID int64, status string, anomalyCode *string, multiplierChangeID *int64, effectiveAt *time.Time) error
	ResolveEffectiveMultiplierVersion(ctx context.Context, accountID int64, effectiveAt time.Time) (*AccountFinanceMultiplierVersion, error)
}

// AccountFinanceMultiplierAccountRepository is intentionally narrow. The
// production account repository already implements these methods, including the
// atomic account update + immutable multiplier audit transaction.
type AccountFinanceMultiplierAccountRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	GetFinanceProfileByID(ctx context.Context, id int64) (*AccountFinanceProfile, error)
	UpdateObservedUpstreamMultiplierWithAudit(ctx context.Context, accountID int64, expectedOld *decimal.Decimal, newMultiplier decimal.Decimal, effectiveAt time.Time, reason string) (int64, error)
}

type AccountFinanceSnapshotService struct {
	snapshots     AccountFinanceSnapshotRepository
	accounts      AccountFinanceMultiplierAccountRepository
	settlements   AccountFinanceSettlementProcessor
	now           func() time.Time
	jumpThreshold decimal.Decimal
}

func NewAccountFinanceSnapshotService(snapshots AccountFinanceSnapshotRepository, accounts AccountFinanceMultiplierAccountRepository, settlements AccountFinanceSettlementProcessor) *AccountFinanceSnapshotService {
	return &AccountFinanceSnapshotService{
		snapshots:     snapshots,
		accounts:      accounts,
		settlements:   settlements,
		now:           time.Now,
		jumpThreshold: decimal.RequireFromString("0.25"),
	}
}

// ObserveCounter records one upstream cumulative observation. Two consecutive
// observations with the same counter contract can prove and audit the account's
// effective upstream multiplier for subsequent usage records.
func (s *AccountFinanceSnapshotService) ObserveCounter(ctx context.Context, input AccountFinanceCounterObservation) (*AccountFinanceCounterSnapshot, error) {
	if s == nil || s.snapshots == nil || s.accounts == nil {
		return nil, ErrAccountFinanceSnapshotRepoUnavailable
	}
	normalized, err := normalizeAccountFinanceObservation(input, s.now)
	if err != nil {
		return nil, err
	}

	var result *AccountFinanceCounterSnapshot
	err = s.snapshots.WithAccountSyncLock(ctx, normalized.AccountID, normalized.ScopeKey, func(lockCtx context.Context) error {
		if err := s.snapshots.ClaimCounterOwner(lockCtx, normalized.CounterIdentityKey, normalized.WalletID, normalized.ProtocolVersionID, normalized.AccountID, normalized.UpstreamCounterID, normalized.CounterPeriod); err != nil {
			return err
		}
		account, err := s.accounts.GetByID(lockCtx, normalized.AccountID)
		if err != nil {
			return err
		}
		previous, err := s.snapshots.LatestCounterSnapshot(lockCtx, normalized.AccountID, normalized.ScopeKey)
		if err != nil {
			return err
		}
		// A sync job passes the profile selected at job start. Preserve that
		// immutable identity even if the account is edited while the upstream
		// request is in flight. Only legacy callers without a snapshot use the
		// account's current profile as a fallback.
		if normalized.AccountFinanceProfileID == nil {
			normalized.AccountFinanceProfileID = cloneInt64Pointer(account.CurrentFinanceProfileID)
		}
		candidate := newAccountFinanceCounterSnapshot(normalized)
		profileContinuous := equalFinanceInt64(candidate.AccountFinanceProfileID, previousFinanceProfileID(previous))
		if previous != nil && !profileContinuous && candidate.AccountFinanceProfileID != nil && previous.AccountFinanceProfileID != nil {
			previousProfile, profileErr := s.accounts.GetFinanceProfileByID(lockCtx, *previous.AccountFinanceProfileID)
			if profileErr != nil {
				return profileErr
			}
			currentProfile, profileErr := s.accounts.GetFinanceProfileByID(lockCtx, *candidate.AccountFinanceProfileID)
			if profileErr != nil {
				return profileErr
			}
			profileContinuous = accountFinanceCounterProfilesContinuous(previousProfile, currentProfile)
		}
		deriveAccountFinanceMultiplier(candidate, previous, profileContinuous)
		if !account.IsActive() {
			candidate.DerivationStatus = AccountFinanceDerivationInactiveAccount
			candidate.ObservedMultiplier = nil
		}

		stored, created, err := s.snapshots.CreateCounterSnapshot(lockCtx, candidate)
		if err != nil {
			return err
		}
		result = stored
		if !created && stored.DerivationStatus != AccountFinanceDerivationCandidate && stored.DerivationStatus != AccountFinanceDerivationConflict {
			if s.settlements != nil && stored.PreviousSnapshotID != nil && accountFinanceSnapshotReadyForSettlement(stored) {
				prior, priorErr := s.snapshots.CounterSnapshotByID(lockCtx, *stored.PreviousSnapshotID)
				if priorErr != nil {
					return priorErr
				}
				return s.settlements.ProcessSnapshotInterval(lockCtx, prior, stored)
			}
			return nil
		}
		if stored.DerivationStatus == AccountFinanceDerivationSettlementReady {
			if s.settlements != nil {
				return s.settlements.ProcessSnapshotInterval(lockCtx, previous, stored)
			}
			return nil
		}
		if stored.DerivationStatus != AccountFinanceDerivationCandidate && stored.DerivationStatus != AccountFinanceDerivationConflict {
			return nil
		}
		if err := s.applyObservedMultiplier(lockCtx, account, stored); err != nil {
			return err
		}
		if s.settlements != nil && accountFinanceSnapshotReadyForSettlement(stored) {
			return s.settlements.ProcessSnapshotInterval(lockCtx, previous, stored)
		}
		return nil
	})
	return result, err
}

func accountFinanceSnapshotReadyForSettlement(snapshot *AccountFinanceCounterSnapshot) bool {
	if snapshot == nil {
		return false
	}
	return snapshot.DerivationStatus == AccountFinanceDerivationApplied || snapshot.DerivationStatus == AccountFinanceDerivationUnchanged || snapshot.DerivationStatus == AccountFinanceDerivationSettlementReady
}

// ResolveMultiplierVersionAt resolves the unique audit version visible when an
// account is selected. It reads multiplier history only; recharge and wallet
// records are not inputs to this resolver.
func (s *AccountFinanceSnapshotService) ResolveMultiplierVersionAt(ctx context.Context, accountID int64, selectedAt time.Time) (*AccountFinanceMultiplierVersion, error) {
	if s == nil || s.snapshots == nil {
		return nil, ErrAccountFinanceSnapshotRepoUnavailable
	}
	return s.snapshots.ResolveEffectiveMultiplierVersion(ctx, accountID, selectedAt)
}

func (s *AccountFinanceSnapshotService) applyObservedMultiplier(ctx context.Context, account *Account, snapshot *AccountFinanceCounterSnapshot) error {
	if snapshot.ObservedMultiplier == nil {
		return nil
	}
	newMultiplier := snapshot.ObservedMultiplier.Round(4)
	anomaly := multiplierJumpAnomaly(account.UpstreamCostMultiplier, newMultiplier, s.jumpThreshold)
	if err := ValidateUpstreamCostMultiplier(newMultiplier); err != nil {
		if markErr := s.snapshots.MarkCounterSnapshotMultiplierResult(ctx, snapshot.ID, AccountFinanceDerivationInvalidMultiplier, anomaly, nil, nil); markErr != nil {
			return markErr
		}
		snapshot.DerivationStatus = AccountFinanceDerivationInvalidMultiplier
		snapshot.AnomalyCode = anomaly
		return err
	}

	effectiveAt := snapshot.CollectedAt.UTC()
	if snapshot.UpstreamObservedAt != nil {
		effectiveAt = snapshot.UpstreamObservedAt.UTC()
	}
	if account.UpstreamCostMultiplier != nil && account.UpstreamCostMultiplier.Equal(newMultiplier) {
		if err := s.snapshots.MarkCounterSnapshotMultiplierResult(ctx, snapshot.ID, AccountFinanceDerivationUnchanged, anomaly, account.UpstreamCostMultiplierChangeID, account.UpstreamCostMultiplierUpdatedAt); err != nil {
			return err
		}
		snapshot.DerivationStatus = AccountFinanceDerivationUnchanged
		snapshot.AnomalyCode = anomaly
		snapshot.MultiplierChangeID = cloneInt64Pointer(account.UpstreamCostMultiplierChangeID)
		snapshot.MultiplierEffectiveAt = cloneFinanceTime(account.UpstreamCostMultiplierUpdatedAt)
		return nil
	}

	changeID, err := s.accounts.UpdateObservedUpstreamMultiplierWithAudit(
		ctx,
		account.ID,
		cloneFinanceDecimal(account.UpstreamCostMultiplier),
		newMultiplier,
		effectiveAt,
		"上游累计账单自动识别倍率",
	)
	if err != nil {
		if errors.Is(err, ErrAccountUpstreamMultiplierConflict) {
			_ = s.snapshots.MarkCounterSnapshotMultiplierResult(ctx, snapshot.ID, AccountFinanceDerivationConflict, anomaly, nil, nil)
			snapshot.DerivationStatus = AccountFinanceDerivationConflict
			snapshot.AnomalyCode = anomaly
		}
		return err
	}
	if err := s.snapshots.MarkCounterSnapshotMultiplierResult(ctx, snapshot.ID, AccountFinanceDerivationApplied, anomaly, &changeID, &effectiveAt); err != nil {
		return err
	}
	snapshot.DerivationStatus = AccountFinanceDerivationApplied
	snapshot.AnomalyCode = anomaly
	snapshot.MultiplierChangeID = &changeID
	snapshot.MultiplierEffectiveAt = &effectiveAt
	return nil
}

func normalizeAccountFinanceObservation(input AccountFinanceCounterObservation, now func() time.Time) (AccountFinanceCounterObservation, error) {
	input.ScopeKey = strings.TrimSpace(input.ScopeKey)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.CounterIdentityKey = strings.TrimSpace(input.CounterIdentityKey)
	input.UnitCode = strings.ToUpper(strings.TrimSpace(input.UnitCode))
	input.UnitSemantics = strings.ToLower(strings.TrimSpace(input.UnitSemantics))
	if input.AccountID <= 0 || input.WalletID <= 0 || input.CounterIdentityKey == "" || len(input.CounterIdentityKey) > 200 || input.ScopeKey == "" || len(input.ScopeKey) > 200 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 || input.UnitCode == "" || len(input.UnitCode) > 30 {
		return input, ErrAccountFinanceSnapshotInvalid
	}
	if input.UnitSemantics != AccountFinanceUnitFiatCurrency && input.UnitSemantics != AccountFinanceUnitPlatformCredit {
		return input, ErrAccountFinanceSnapshotInvalid
	}
	if input.ListCostTotal == nil && input.ActualCostTotal == nil {
		return input, ErrAccountFinanceSnapshotInvalid
	}
	if (input.ListCostTotal != nil && input.ListCostTotal.IsNegative()) || (input.ActualCostTotal != nil && input.ActualCostTotal.IsNegative()) {
		return input, ErrAccountFinanceSnapshotInvalid
	}
	if input.UnitSemantics == AccountFinanceUnitFiatCurrency {
		if input.Currency == nil {
			return input, ErrAccountFinanceSnapshotInvalid
		}
		currency := strings.ToUpper(strings.TrimSpace(*input.Currency))
		if len(currency) != 3 {
			return input, ErrAccountFinanceSnapshotInvalid
		}
		input.Currency = &currency
	} else {
		input.Currency = nil
	}
	input.UpstreamCounterID = normalizeOptionalString(input.UpstreamCounterID)
	input.CounterPeriod = normalizeOptionalString(input.CounterPeriod)
	if input.CollectedAt.IsZero() {
		input.CollectedAt = now().UTC()
	} else {
		input.CollectedAt = input.CollectedAt.UTC()
	}
	if input.UpstreamObservedAt != nil {
		observed := input.UpstreamObservedAt.UTC()
		input.UpstreamObservedAt = &observed
	}
	input.SafeSnapshot = sanitizeAccountFinanceSnapshot(input.SafeSnapshot)
	return input, nil
}

func newAccountFinanceCounterSnapshot(input AccountFinanceCounterObservation) *AccountFinanceCounterSnapshot {
	snapshot := &AccountFinanceCounterSnapshot{
		AccountID:               input.AccountID,
		AccountFinanceProfileID: cloneInt64Pointer(input.AccountFinanceProfileID),
		ScopeKey:                input.ScopeKey,
		IdempotencyKey:          input.IdempotencyKey,
		UpstreamCounterID:       input.UpstreamCounterID,
		CounterPeriod:           input.CounterPeriod,
		ListCostTotal:           cloneFinanceDecimal(input.ListCostTotal),
		ActualCostTotal:         cloneFinanceDecimal(input.ActualCostTotal),
		UnitCode:                input.UnitCode,
		UnitSemantics:           input.UnitSemantics,
		Currency:                cloneFinanceString(input.Currency),
		UpstreamObservedAt:      cloneFinanceTime(input.UpstreamObservedAt),
		CollectedAt:             input.CollectedAt,
		SafeSnapshot:            input.SafeSnapshot,
		DerivationStatus:        AccountFinanceDerivationBaseline,
	}
	snapshot.Checksum = accountFinanceSnapshotChecksum(snapshot)
	return snapshot
}

func deriveAccountFinanceMultiplier(current, previous *AccountFinanceCounterSnapshot, profileContinuous bool) {
	if previous == nil {
		current.DerivationStatus = AccountFinanceDerivationBaseline
		return
	}
	current.PreviousSnapshotID = &previous.ID
	if previous.DerivationStatus == AccountFinanceDerivationInactiveAccount {
		current.DerivationStatus = AccountFinanceDerivationBaseline
		return
	}
	if previous.UnitSemantics != current.UnitSemantics {
		current.DerivationStatus = AccountFinanceDerivationBoundaryChanged
		return
	}
	if !equalFinanceInt64(current.AccountFinanceProfileID, previous.AccountFinanceProfileID) && !profileContinuous {
		current.DerivationStatus = AccountFinanceDerivationBoundaryChanged
		return
	}
	if !current.CollectedAt.After(previous.CollectedAt) || (current.UpstreamObservedAt != nil && previous.UpstreamObservedAt != nil && !current.UpstreamObservedAt.After(*previous.UpstreamObservedAt)) {
		current.DerivationStatus = AccountFinanceDerivationTimeReversed
		return
	}
	if current.UnitCode != previous.UnitCode || !equalFinanceString(current.Currency, previous.Currency) || !equalFinanceString(current.CounterPeriod, previous.CounterPeriod) || !equalFinanceString(current.UpstreamCounterID, previous.UpstreamCounterID) {
		current.DerivationStatus = AccountFinanceDerivationBoundaryChanged
		return
	}
	if current.ActualCostTotal == nil || previous.ActualCostTotal == nil {
		current.DerivationStatus = AccountFinanceDerivationMissingValues
		return
	}
	actualDelta := current.ActualCostTotal.Sub(*previous.ActualCostTotal)
	if actualDelta.IsNegative() {
		current.DerivationStatus = AccountFinanceDerivationCounterReset
		return
	}
	current.ActualCostDelta = &actualDelta
	if current.ListCostTotal == nil && previous.ListCostTotal == nil {
		if actualDelta.IsZero() {
			current.DerivationStatus = AccountFinanceDerivationNoActivity
		} else {
			current.DerivationStatus = AccountFinanceDerivationSettlementReady
		}
		return
	}
	if current.ListCostTotal == nil || previous.ListCostTotal == nil {
		current.DerivationStatus = AccountFinanceDerivationBoundaryChanged
		return
	}
	listDelta := current.ListCostTotal.Sub(*previous.ListCostTotal)
	if listDelta.IsNegative() {
		current.DerivationStatus = AccountFinanceDerivationCounterReset
		return
	}
	current.ListCostDelta = &listDelta
	if listDelta.IsZero() {
		if actualDelta.IsZero() {
			current.DerivationStatus = AccountFinanceDerivationNoActivity
		} else {
			current.DerivationStatus = AccountFinanceDerivationInvalidList
		}
		return
	}
	multiplier := actualDelta.DivRound(listDelta, 10)
	current.ObservedMultiplier = &multiplier
	current.DerivationStatus = AccountFinanceDerivationCandidate
}

func previousFinanceProfileID(snapshot *AccountFinanceCounterSnapshot) *int64 {
	if snapshot == nil {
		return nil
	}
	return snapshot.AccountFinanceProfileID
}

func accountFinanceCounterProfilesContinuous(previous, current *AccountFinanceProfile) bool {
	if previous == nil || current == nil || previous.AccountID != current.AccountID || previous.CostMode != current.CostMode {
		return false
	}
	if current.CostMode != FinanceCostModeCumulativeListAndActual && current.CostMode != FinanceCostModeCumulativeActual {
		return false
	}
	return equalFinanceInt64(previous.WalletID, current.WalletID) &&
		equalFinanceInt64(previous.ProtocolVersionID, current.ProtocolVersionID) &&
		strings.TrimSpace(previous.EndpointSource) == strings.TrimSpace(current.EndpointSource) &&
		strings.TrimSpace(previous.EndpointBaseURLSnapshot) == strings.TrimSpace(current.EndpointBaseURLSnapshot) &&
		strings.TrimSpace(previous.CredentialSource) == strings.TrimSpace(current.CredentialSource) &&
		strings.TrimSpace(previous.CounterScope) == strings.TrimSpace(current.CounterScope) &&
		equalFinanceString(previous.CounterScopeKey, current.CounterScopeKey) &&
		strings.TrimSpace(previous.BalanceUnitSemantics) == strings.TrimSpace(current.BalanceUnitSemantics)
}

func multiplierJumpAnomaly(oldMultiplier *decimal.Decimal, newMultiplier, threshold decimal.Decimal) *string {
	if oldMultiplier == nil || oldMultiplier.Equal(newMultiplier) {
		return nil
	}
	jump := false
	if oldMultiplier.IsZero() {
		jump = !newMultiplier.IsZero()
	} else {
		relative := newMultiplier.Sub(*oldMultiplier).Abs().Div(oldMultiplier.Abs())
		jump = relative.GreaterThan(threshold)
	}
	if !jump {
		return nil
	}
	value := AccountFinanceAnomalyMultiplierJump
	return &value
}

func accountFinanceSnapshotChecksum(snapshot *AccountFinanceCounterSnapshot) string {
	payload := struct {
		AccountID               int64          `json:"account_id"`
		AccountFinanceProfileID *int64         `json:"account_finance_profile_id"`
		ScopeKey                string         `json:"scope_key"`
		UpstreamCounterID       *string        `json:"upstream_counter_id"`
		CounterPeriod           *string        `json:"counter_period"`
		ListCostTotal           string         `json:"list_cost_total"`
		ActualCostTotal         string         `json:"actual_cost_total"`
		UnitCode                string         `json:"unit_code"`
		UnitSemantics           string         `json:"unit_semantics"`
		Currency                *string        `json:"currency"`
		UpstreamObservedAt      *time.Time     `json:"upstream_observed_at"`
		CollectedAt             time.Time      `json:"collected_at"`
		SafeSnapshot            map[string]any `json:"safe_snapshot"`
	}{
		AccountID: snapshot.AccountID, AccountFinanceProfileID: snapshot.AccountFinanceProfileID, ScopeKey: snapshot.ScopeKey, UpstreamCounterID: snapshot.UpstreamCounterID,
		CounterPeriod: snapshot.CounterPeriod, ListCostTotal: accountFinanceDecimalString(snapshot.ListCostTotal),
		ActualCostTotal: accountFinanceDecimalString(snapshot.ActualCostTotal), UnitCode: snapshot.UnitCode,
		UnitSemantics: snapshot.UnitSemantics, Currency: snapshot.Currency, UpstreamObservedAt: snapshot.UpstreamObservedAt,
		CollectedAt: snapshot.CollectedAt, SafeSnapshot: snapshot.SafeSnapshot,
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func sanitizeAccountFinanceSnapshot(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		if accountFinanceSensitiveKey(key) {
			result[key] = "[REDACTED]"
			continue
		}
		result[key] = sanitizeAccountFinanceValue(item)
	}
	return result
}

func sanitizeAccountFinanceValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeAccountFinanceSnapshot(typed)
	case []any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = sanitizeAccountFinanceValue(typed[index])
		}
		return result
	default:
		return value
	}
}

func accountFinanceSensitiveKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, marker := range []string{"authorization", "credential", "password", "secret", "token", "apikey", "cookie"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func equalFinanceString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalFinanceInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func cloneFinanceDecimal(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFinanceString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFinanceTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func accountFinanceDecimalString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.StringFixed(10)
}
