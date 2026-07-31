//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPricingServiceSnapshotIsImmutableAndDeterministic(t *testing.T) {
	pricing := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"model-a": {InputCostPerToken: 0.000001, LiteLLMProvider: "openai"},
		},
	}
	first := pricing.Snapshot()
	second := pricing.Snapshot()
	require.Equal(t, first.Checksum, second.Checksum)
	copyModel := first.Models["model-a"]
	copyModel.InputCostPerToken = 99
	first.Models["model-a"] = copyModel
	require.Equal(t, 0.000001, pricing.pricingData["model-a"].InputCostPerToken)
}

func TestFinanceCatalogVersionSyncIsChecksumIdempotent(t *testing.T) {
	updatedAt := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	pricing := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"model-a": {
				InputCostPerToken:                   0.000004,
				OutputCostPerToken:                  0.000012,
				InputCostPerTokenPriority:           0.000008,
				CacheCreationInputTokenCost:         0.000005,
				CacheCreationInputTokenCostAbove1hr: 0.000008,
				LiteLLMProvider:                     "openai",
			},
		},
		localHash:   "catalog-1",
		lastUpdated: updatedAt,
	}
	repository := &financeCatalogVersionRepositoryStub{}
	syncer := NewFinanceCatalogVersionSync(pricing, repository)

	changed, err := syncer.Sync(context.Background())
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "catalog-1", repository.checksum)
	require.Equal(t, updatedAt, repository.effectiveFrom)
	require.Len(t, repository.versions, 1)
	require.Equal(t, "4", repository.versions[0].PriceDetail["input"])
	require.Equal(t, "12", repository.versions[0].PriceDetail["output"])
	fast := repository.versions[0].PriceDetail["fast"].(map[string]any)
	require.Equal(t, "8", fast["input"])

	changed, err = syncer.Sync(context.Background())
	require.NoError(t, err)
	require.False(t, changed)
}

type financeCatalogVersionRepositoryStub struct {
	checksum      string
	effectiveFrom time.Time
	versions      []FinanceSystemPriceVersion
}

func (s *financeCatalogVersionRepositoryStub) SyncSystemPriceVersions(_ context.Context, checksum string, effectiveFrom time.Time, versions []FinanceSystemPriceVersion) (bool, error) {
	if checksum == s.checksum {
		return false, nil
	}
	s.checksum = checksum
	s.effectiveFrom = effectiveFrom
	s.versions = versions
	return true, nil
}
