package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestAllocateFinanceSettlementUsesStandardCostWeightsAndCollectsTail(t *testing.T) {
	base := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	result, err := AllocateFinanceSettlement(decimal.RequireFromString("0.22"), []FinanceSettlementSegment{
		{UsageLogID: 3, AttemptNo: 1, UsageCreatedAt: base.Add(2 * time.Minute), StandardCost: decimal.RequireFromString("0.3333333333")},
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base, StandardCost: decimal.RequireFromString("0.3333333333")},
		{UsageLogID: 2, AttemptNo: 1, UsageCreatedAt: base.Add(time.Minute), StandardCost: decimal.RequireFromString("0.3333333334")},
	})
	require.NoError(t, err)
	require.Equal(t, "1", result.StandardCostTotal.String())
	require.Equal(t, "0.22", result.AllocatedTotal.String())
	require.True(t, result.Difference.IsZero())
	require.Equal(t, int64(1), result.Allocations[0].UsageLogID)
	require.Equal(t, int64(3), result.Allocations[2].UsageLogID)
	sum := decimal.Zero
	for _, allocation := range result.Allocations {
		sum = sum.Add(allocation.AllocatedCost)
	}
	require.True(t, sum.Equal(decimal.RequireFromString("0.22")))
}

func TestAllocateFinanceSettlementRejectsZeroWeightAndDuplicateAttempt(t *testing.T) {
	base := time.Now().UTC()
	_, err := AllocateFinanceSettlement(decimal.NewFromInt(1), []FinanceSettlementSegment{{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base}})
	require.ErrorIs(t, err, ErrFinanceSettlementZeroWeight)

	_, err = AllocateFinanceSettlement(decimal.NewFromInt(1), []FinanceSettlementSegment{
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base, StandardCost: decimal.NewFromInt(1)},
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base.Add(time.Second), StandardCost: decimal.NewFromInt(1)},
	})
	require.ErrorIs(t, err, ErrFinanceSettlementInvalid)
}

func TestValidateFinanceSettlementTransitionKeepsSettledIntervalsImmutable(t *testing.T) {
	require.NoError(t, ValidateFinanceSettlementTransition(FinanceSettlementPending, FinanceSettlementSettled))
	require.NoError(t, ValidateFinanceSettlementTransition(FinanceSettlementNeedsReview, FinanceSettlementPending))
	err := ValidateFinanceSettlementTransition(FinanceSettlementSettled, FinanceSettlementPending)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrFinanceSettlementInvalid))
}

func TestAccountFinanceSettlementServiceAppliesMatchingFiatInterval(t *testing.T) {
	base := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	repo := &financeSettlementRepoStub{segments: []FinanceSettlementSegment{
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base.Add(time.Minute), StandardCost: decimal.NewFromInt(4)},
		{UsageLogID: 2, AttemptNo: 1, UsageCreatedAt: base.Add(2 * time.Minute), StandardCost: decimal.NewFromInt(6)},
	}}
	service := NewAccountFinanceSettlementService(repo)
	currency := "USD"
	previous := &AccountFinanceCounterSnapshot{ID: 10, AccountID: 7, ScopeKey: "scope", CollectedAt: base, UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency}
	listDelta := decimal.NewFromInt(10)
	actualDelta := decimal.RequireFromString("2.2")
	multiplier := decimal.RequireFromString("0.22")
	current := &AccountFinanceCounterSnapshot{ID: 11, AccountID: 7, ScopeKey: "scope", CollectedAt: base.Add(5 * time.Minute), UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency, ListCostDelta: &listDelta, ActualCostDelta: &actualDelta, ObservedMultiplier: &multiplier}

	require.NoError(t, service.ProcessSnapshotInterval(context.Background(), previous, current))
	require.Equal(t, 1, repo.applyCalls)
	require.Zero(t, repo.reviewCalls)
	require.Equal(t, "2.2", repo.applied.AllocatedTotal.String())
}

func TestAccountFinanceSettlementServiceAppliesCumulativeActualOnlyInterval(t *testing.T) {
	base := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	repo := &financeSettlementRepoStub{segments: []FinanceSettlementSegment{
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base.Add(time.Minute), StandardCost: decimal.NewFromInt(4)},
		{UsageLogID: 2, AttemptNo: 1, UsageCreatedAt: base.Add(2 * time.Minute), StandardCost: decimal.NewFromInt(6)},
	}}
	actualDelta := decimal.RequireFromString("2.2")
	currency := "USD"
	previous := &AccountFinanceCounterSnapshot{ID: 10, AccountID: 7, ScopeKey: "scope", CollectedAt: base, UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency}
	current := &AccountFinanceCounterSnapshot{ID: 11, AccountID: 7, ScopeKey: "scope", CollectedAt: base.Add(5 * time.Minute), UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency, ActualCostDelta: &actualDelta, DerivationStatus: AccountFinanceDerivationSettlementReady}

	require.NoError(t, NewAccountFinanceSettlementService(repo).ProcessSnapshotInterval(context.Background(), previous, current))
	require.Equal(t, 1, repo.applyCalls)
	require.Zero(t, repo.reviewCalls)
	require.Nil(t, repo.interval.ListCostDelta)
	require.Nil(t, repo.interval.ObservedMultiplier)
	require.Equal(t, "2.2", repo.applied.AllocatedTotal.String())
}

func TestAccountFinanceSettlementServiceConvertsCNYDeltasWithFrozenFX(t *testing.T) {
	base := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	repo := &financeSettlementRepoStub{segments: []FinanceSettlementSegment{
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base.Add(time.Minute), StandardCost: decimal.RequireFromString("0.56")},
		{UsageLogID: 2, AttemptNo: 1, UsageCreatedAt: base.Add(2 * time.Minute), StandardCost: decimal.RequireFromString("0.84")},
	}}
	currency := "CNY"
	listDelta := decimal.NewFromInt(10)
	actualDelta := decimal.NewFromInt(10)
	multiplier := decimal.NewFromInt(1)
	previous := &AccountFinanceCounterSnapshot{ID: 10, AccountID: 7, ScopeKey: "scope", CollectedAt: base, UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency}
	current := &AccountFinanceCounterSnapshot{
		ID: 11, AccountID: 7, ScopeKey: "scope", CollectedAt: base.Add(5 * time.Minute),
		UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency,
		ListCostDelta: &listDelta, ActualCostDelta: &actualDelta, ObservedMultiplier: &multiplier,
		SafeSnapshot: map[string]any{
			"fx_rate_version_id": int64(88), "fx_rate_to_usd": "0.14",
			"fx_source": "provider_snapshot", "fx_observed_at": base.Add(4 * time.Minute).Format(time.RFC3339),
		},
	}

	settlements := NewAccountFinanceSettlementService(repo)
	require.NoError(t, settlements.ProcessSnapshotInterval(context.Background(), previous, current))
	require.Equal(t, "10", repo.interval.ActualCostDelta.String(), "interval keeps the original CNY delta")
	require.Equal(t, "10", repo.interval.ListCostDelta.String(), "interval keeps the original CNY list delta")
	require.Equal(t, int64(88), *repo.interval.FXRateVersionID)
	require.Equal(t, "0.14", repo.interval.FXRateToUSD.String())
	require.Equal(t, "1.4", repo.applied.ActualCostTotal.String(), "10 CNY * 0.14 = 1.4 USD")
	require.Equal(t, "1.4", repo.applied.AllocatedTotal.String())

	current.SafeSnapshot["fx_rate_version_id"] = int64(99)
	current.SafeSnapshot["fx_rate_to_usd"] = "0.20"
	repo.interval.Status = FinanceSettlementNeedsReview
	repo.applyCalls = 0
	_, err := settlements.Retry(context.Background(), repo.interval.ID, 77)
	require.NoError(t, err)
	require.Equal(t, 1, repo.applyCalls)
	require.Equal(t, int64(88), *repo.interval.FXRateVersionID, "retry must use the interval's historical FX version")
	require.Equal(t, "0.14", repo.interval.FXRateToUSD.String())
	require.Equal(t, "1.4", repo.applied.AllocatedTotal.String(), "current FX changes must not rewrite historical settlement")
}

func TestAccountFinanceSettlementServiceConvertsCumulativeActualOnlyCNYDelta(t *testing.T) {
	base := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	repo := &financeSettlementRepoStub{segments: []FinanceSettlementSegment{
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base.Add(time.Minute), StandardCost: decimal.NewFromInt(1)},
	}}
	currency := "CNY"
	actualDelta := decimal.NewFromInt(10)
	previous := &AccountFinanceCounterSnapshot{ID: 10, AccountID: 7, ScopeKey: "scope", CollectedAt: base, UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency}
	current := &AccountFinanceCounterSnapshot{
		ID: 11, AccountID: 7, ScopeKey: "scope", CollectedAt: base.Add(5 * time.Minute),
		UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency, ActualCostDelta: &actualDelta,
		DerivationStatus: AccountFinanceDerivationSettlementReady,
		SafeSnapshot:     map[string]any{"fx_rate_version_id": int64(88), "fx_rate_to_usd": "0.14"},
	}

	require.NoError(t, NewAccountFinanceSettlementService(repo).ProcessSnapshotInterval(context.Background(), previous, current))
	require.Nil(t, repo.interval.ListCostDelta)
	require.Equal(t, "10", repo.interval.ActualCostDelta.String())
	require.Equal(t, "1.4", repo.applied.AllocatedTotal.String())
}

func TestAccountFinanceSettlementServiceRequiresFrozenFXForNonUSD(t *testing.T) {
	base := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	repo := &financeSettlementRepoStub{segments: []FinanceSettlementSegment{
		{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base.Add(time.Minute), StandardCost: decimal.NewFromInt(1)},
	}}
	currency := "CNY"
	actualDelta := decimal.NewFromInt(10)
	previous := &AccountFinanceCounterSnapshot{ID: 10, AccountID: 7, ScopeKey: "scope", CollectedAt: base, UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency}
	current := &AccountFinanceCounterSnapshot{ID: 11, AccountID: 7, ScopeKey: "scope", CollectedAt: base.Add(5 * time.Minute), UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency, ActualCostDelta: &actualDelta}
	current.SafeSnapshot = map[string]any{"fx_rate_to_usd": "0.14"}

	require.NoError(t, NewAccountFinanceSettlementService(repo).ProcessSnapshotInterval(context.Background(), previous, current))
	require.Zero(t, repo.applyCalls)
	require.Equal(t, 1, repo.reviewCalls)
}

func TestFinanceFXSnapshotEvidenceReadsProtocolPayloadFallback(t *testing.T) {
	currency := "CNY"
	versionID, rate := int64(77), decimal.RequireFromString("0.14")
	observed := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	gotVersion, gotRate, source, gotObserved := financeFXSnapshotEvidence(map[string]any{
		"payload": map[string]any{
			"fx_rate_version_id": versionID,
			"fx_rate_to_usd":     "0.14",
			"fx_source":          "protocol",
			"fx_observed_at":     observed.Format(time.RFC3339),
		},
	}, &currency, observed.Add(time.Hour))
	require.NotNil(t, gotVersion)
	require.Equal(t, versionID, *gotVersion)
	require.Equal(t, rate.String(), gotRate.String())
	require.Equal(t, "protocol", source)
	require.Equal(t, observed, *gotObserved)
}

func TestAccountFinanceSettlementServiceMarksMismatchedAndPlatformIntervalsForReviewOrEvidenceOnly(t *testing.T) {
	base := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	repo := &financeSettlementRepoStub{segments: []FinanceSettlementSegment{{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: base.Add(time.Minute), StandardCost: decimal.NewFromInt(8)}}}
	service := NewAccountFinanceSettlementService(repo)
	listDelta := decimal.NewFromInt(10)
	actualDelta := decimal.RequireFromString("2.2")
	multiplier := decimal.RequireFromString("0.22")
	currency := "USD"
	previous := &AccountFinanceCounterSnapshot{ID: 10, AccountID: 7, ScopeKey: "scope", CollectedAt: base, UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency}
	current := &AccountFinanceCounterSnapshot{ID: 11, AccountID: 7, ScopeKey: "scope", CollectedAt: base.Add(5 * time.Minute), UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency, ListCostDelta: &listDelta, ActualCostDelta: &actualDelta, ObservedMultiplier: &multiplier}

	require.NoError(t, service.ProcessSnapshotInterval(context.Background(), previous, current))
	require.Zero(t, repo.applyCalls)
	require.Equal(t, 1, repo.reviewCalls)

	current.UnitSemantics = AccountFinanceUnitPlatformCredit
	require.NoError(t, service.ProcessSnapshotInterval(context.Background(), previous, current))
	require.Equal(t, 1, repo.reviewCalls)
}

func TestAccountFinanceSettlementServiceAuditsManualRetryOperator(t *testing.T) {
	now := time.Now().UTC()
	accountID := int64(12)
	listCostDelta := decimal.RequireFromString("10")
	repo := &financeSettlementRepoStub{
		interval: &FinanceSettlementInterval{
			ID: 9, AccountID: &accountID, Status: FinanceSettlementNeedsReview,
			UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: financeSnapshotStringPtr("USD"),
			ListCostDelta: &listCostDelta, ActualCostDelta: decimal.RequireFromString("2.2"),
		},
		segments: []FinanceSettlementSegment{{UsageLogID: 1, AttemptNo: 1, UsageCreatedAt: now, StandardCost: decimal.RequireFromString("10")}},
	}
	_, err := NewAccountFinanceSettlementService(repo).Retry(context.Background(), 9, 77)
	require.NoError(t, err)
	require.Equal(t, "manual settlement retry", repo.auditReason)
	require.NotNil(t, repo.operatorID)
	require.Equal(t, int64(77), *repo.operatorID)
}

type financeSettlementRepoStub struct {
	segments    []FinanceSettlementSegment
	applyCalls  int
	reviewCalls int
	applied     FinanceSettlementAllocationResult
	interval    *FinanceSettlementInterval
	auditReason string
	operatorID  *int64
}

func (r *financeSettlementRepoStub) CreateOrGetSettlementInterval(_ context.Context, input FinanceSettlementIntervalInput) (*FinanceSettlementInterval, bool, error) {
	accountID := input.AccountID
	r.interval = &FinanceSettlementInterval{
		ID: 1, OwnerType: "account", OwnerID: input.AccountID, AccountID: &accountID,
		ScopeKey: input.ScopeKey, PreviousSnapshotID: input.PreviousSnapshotID, CurrentSnapshotID: input.CurrentSnapshotID,
		PeriodStart: input.PeriodStart, PeriodEnd: input.PeriodEnd, UnitSemantics: input.UnitSemantics,
		Currency: input.Currency, FXRateVersionID: input.FXRateVersionID, FXRateToUSD: input.FXRateToUSD,
		FXSource: input.FXSource, FXObservedAt: input.FXObservedAt,
		ListCostDelta: input.ListCostDelta, ActualCostDelta: input.ActualCostDelta,
		ObservedMultiplier: input.ObservedMultiplier, Status: FinanceSettlementPending, CurrentRevision: 1,
	}
	return r.interval, true, nil
}
func (r *financeSettlementRepoStub) ListSettlementSegments(context.Context, *FinanceSettlementInterval) ([]FinanceSettlementSegment, error) {
	return append([]FinanceSettlementSegment(nil), r.segments...), nil
}
func (r *financeSettlementRepoStub) MarkSettlementNeedsReview(_ context.Context, _ int64, _, _ int64, _, _ decimal.Decimal, _ string) error {
	r.reviewCalls++
	return nil
}
func (r *financeSettlementRepoStub) ApplySettlement(_ context.Context, _ *FinanceSettlementInterval, result FinanceSettlementAllocationResult, auditReason string, operatorID *int64) error {
	r.applyCalls++
	r.applied = result
	r.auditReason = auditReason
	r.operatorID = operatorID
	return nil
}

func (r *financeSettlementRepoStub) ListSettlementIntervals(context.Context, FinanceSettlementListFilter) ([]FinanceSettlementInterval, int64, error) {
	return nil, 0, nil
}

func (r *financeSettlementRepoStub) GetSettlementInterval(context.Context, int64) (*FinanceSettlementInterval, error) {
	return r.interval, nil
}

func (r *financeSettlementRepoStub) ListSettlementAllocations(context.Context, int64) ([]FinanceSettlementAllocationView, error) {
	return nil, nil
}

func (r *financeSettlementRepoStub) ReallocateSettlement(context.Context, int64, int, string, int64) (*FinanceSettlementInterval, error) {
	return nil, nil
}
