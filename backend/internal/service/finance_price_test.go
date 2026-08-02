//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinancePriceDetailFromMapSupportsCanonicalAndDisplayShapes(t *testing.T) {
	detail, err := FinancePriceDetailFromMap(map[string]any{
		"input":       map[string]any{"original_price": "4.25"},
		"output":      12.5,
		"fast_prices": map[string]any{"input": map[string]any{"price": "7.5"}},
		"tiers": []any{
			map[string]any{"min_tokens": float64(1000), "max_tokens": "2000", "prices": map[string]any{"input": "3"}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "4.25", detail.Standard.Input.String())
	require.Equal(t, "12.5", detail.Standard.Output.String())
	require.Equal(t, "7.5", detail.Fast.Input.String())
	require.Len(t, detail.Tiers, 1)
	require.Equal(t, int64(1000), detail.Tiers[0].MinQuantity)
	require.Equal(t, int64(2000), *detail.Tiers[0].MaxQuantity)

	roundTrip, err := FinancePriceDetailFromMap(FinancePriceDetailToMap(detail))
	require.NoError(t, err)
	require.Equal(t, detail.Standard.Input.String(), roundTrip.Standard.Input.String())
	require.Equal(t, detail.Fast.Input.String(), roundTrip.Fast.Input.String())
}

func TestFinancePriceSelectorPrefersHistoricalUpstreamThenSystem(t *testing.T) {
	now := time.Date(2026, 7, 25, 1, 2, 3, 0, time.UTC)
	repo := &financePriceLookupStub{
		profile:  &AccountFinanceProfile{ID: 4, AccountID: 10, WalletID: int64Snapshot(5), CostMode: FinanceCostModeManual},
		wallet:   &FinanceWalletAssignment{WalletID: 5, UpstreamID: 9, Currency: "USD"},
		upstream: &FinancePriceQuote{VersionID: 21, Source: FinancePricingSourceUpstreamCatalog},
		system:   &FinancePriceQuote{VersionID: 31, Source: FinancePricingSourceSystem},
	}
	selector := NewFinancePriceSelector(repo)

	profileID := int64(4)
	selected, err := selector.Select(context.Background(), 10, &profileID, "gpt-test", "token", "fast", now)
	require.NoError(t, err)
	require.Equal(t, int64(21), selected.Quote.VersionID)
	require.Equal(t, now, repo.lookupAt)

	repo.upstream = nil
	selected, err = selector.Select(context.Background(), 10, &profileID, "gpt-test", "token", "fast", now)
	require.NoError(t, err)
	require.Equal(t, int64(31), selected.Quote.VersionID)
}

func TestFinancePriceSelectorDoesNotUseSystemPriceWithoutWalletAssignment(t *testing.T) {
	repo := &financePriceLookupStub{system: &FinancePriceQuote{VersionID: 31, Source: FinancePricingSourceSystem}}
	selected, err := NewFinancePriceSelector(repo).Select(context.Background(), 10, nil, "gpt-test", "token", "", time.Now())
	require.NoError(t, err)
	require.True(t, selected.MissingProfile)
	require.Nil(t, selected.Wallet)
	require.Nil(t, selected.Quote)
	require.False(t, repo.systemLookedUp)
}

func TestFinancePriceSelectorSkipsPriceLookupForRequestCharge(t *testing.T) {
	repo := &financePriceLookupStub{system: &FinancePriceQuote{VersionID: 31, Source: FinancePricingSourceSystem}}
	repo.profile = &AccountFinanceProfile{ID: 4, AccountID: 10, CostMode: FinanceCostModeRequestCharge, ProtocolVersionID: int64Snapshot(8)}
	profileID := int64(4)
	selected, err := NewFinancePriceSelector(repo).Select(context.Background(), 10, &profileID, "unknown-upstream-model", "token", "", time.Now())
	require.NoError(t, err)
	require.NotNil(t, selected.Profile)
	require.Nil(t, selected.Quote)
	require.False(t, repo.systemLookedUp)
}

func TestFinancePriceSelectorFallsBackToMultiplierForPlatformCredit(t *testing.T) {
	multiplier := decimal.RequireFromString("0.2200")
	repo := &financePriceLookupStub{
		profile: &AccountFinanceProfile{ID: 4, AccountID: 10, WalletID: int64Snapshot(5), CostMode: FinanceCostModeRequestCharge, BalanceUnitSemantics: FinanceUnitPlatformCredit, AccountMultiplierSnapshot: &multiplier},
		wallet:  &FinanceWalletAssignment{WalletID: 5, UpstreamID: 9, Currency: "USD"},
		system:  &FinancePriceQuote{VersionID: 31, Source: FinancePricingSourceSystem, Currency: "USD", USDExchangeRate: decimal.NewFromInt(1), Detail: FinancePriceDetail{Standard: FinanceRateCard{Input: financeDecimal("1")}}},
	}
	profileID := int64(4)
	selected, err := NewFinancePriceSelector(repo).Select(context.Background(), 10, &profileID, "gpt-test", "token", "", time.Now())
	require.NoError(t, err)
	require.NotNil(t, selected.Quote)
	require.Equal(t, int64(31), selected.Quote.VersionID)
	require.Equal(t, "0.2200", selected.CostMultiplier.StringFixed(4))
}

func TestFinancePriceSelectorResolvesWalletForProfileWithoutWalletBinding(t *testing.T) {
	multiplier := decimal.RequireFromString("0.22")
	repo := &financePriceLookupStub{
		profile:    &AccountFinanceProfile{ID: 4, AccountID: 10, CostMode: FinanceCostModeManual, AccountMultiplierSnapshot: &multiplier},
		assignment: &FinanceWalletAssignment{WalletID: 8, UpstreamID: 9, Currency: "USD"},
		system:     &FinancePriceQuote{VersionID: 31, Source: FinancePricingSourceSystem},
	}
	profileID := int64(4)
	selected, err := NewFinancePriceSelector(repo).Select(context.Background(), 10, &profileID, "gpt-test", "token", "", time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(8), selected.Wallet.WalletID)
	require.Equal(t, int64(9), selected.Wallet.UpstreamID)
}

func TestParseFinanceDecimalRejectsNegativeAndKeepsExactText(t *testing.T) {
	value, err := ParseFinanceDecimal("0.000000000123456789")
	require.NoError(t, err)
	require.Equal(t, "0.000000000123456789", value.String())
	_, err = ParseFinanceDecimal("-1")
	require.Error(t, err)
}

type financePriceLookupStub struct {
	profile        *AccountFinanceProfile
	profiles       map[int64]*AccountFinanceProfile
	wallet         *FinanceWalletAssignment
	assignment     *FinanceWalletAssignment
	upstream       *FinancePriceQuote
	system         *FinancePriceQuote
	lookupAt       time.Time
	systemLookedUp bool
}

func (s *financePriceLookupStub) FindAccountFinanceProfileByID(_ context.Context, id int64) (*AccountFinanceProfile, error) {
	if s.profiles != nil {
		return s.profiles[id], nil
	}
	return s.profile, nil
}

func (s *financePriceLookupStub) FindWalletByID(_ context.Context, _ int64) (*FinanceWalletAssignment, error) {
	return s.wallet, nil
}

func (s *financePriceLookupStub) FindWalletAssignmentAt(_ context.Context, _ int64, _ time.Time) (*FinanceWalletAssignment, error) {
	return s.assignment, nil
}

func (s *financePriceLookupStub) FindUpstreamPriceAt(_ context.Context, _ int64, _, _, _ string, at time.Time) (*FinancePriceQuote, error) {
	s.lookupAt = at
	return s.upstream, nil
}

func (s *financePriceLookupStub) FindSystemPriceAt(_ context.Context, _, _ string, at time.Time) (*FinancePriceQuote, error) {
	s.lookupAt = at
	s.systemLookedUp = true
	return s.system, nil
}
