package service

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestNewDecimalUnitPricePreservesExplicitZeroAndFixedScaleJSON(t *testing.T) {
	price, err := NewDecimalUnitPrice(decimal.Zero, decimal.RequireFromString("1.1000"), PriceUnitPerMillionTokens)
	require.NoError(t, err)

	payload, err := json.Marshal(price)
	require.NoError(t, err)
	require.JSONEq(t, `{"original":"0.00000000","multiplier_price":"0.00000000","unit":"per_1m_tokens"}`, string(payload))
}

func TestBuildModelPriceViewTokenAndFastPrices(t *testing.T) {
	view, err := BuildModelPriceView(&ResolvedPricing{
		Mode: BillingModeToken,
		BasePricing: &ModelPricing{
			InputPricePerToken:             2.5e-6,
			OutputPricePerToken:            15e-6,
			CacheReadPricePerToken:         0.25e-6,
			InputPricePerTokenPriority:     5e-6,
			OutputPricePerTokenPriority:    30e-6,
			CacheReadPricePerTokenPriority: 0.5e-6,
			CacheCreation5mPrice:           3e-6,
			CacheCreation1hPrice:           5e-6,
			SupportsCacheBreakdown:         true,
		},
	}, decimal.RequireFromString("1.1000"))
	require.NoError(t, err)
	require.Equal(t, "2.5", view.Input.Original.String())
	require.Equal(t, "2.75", view.Input.MultiplierPrice.String())
	require.Equal(t, "16.5", view.Output.MultiplierPrice.String())
	require.Equal(t, "0.275", view.CacheRead.MultiplierPrice.String())
	require.NotNil(t, view.Fast)
	require.Equal(t, "5.5", view.Fast.Input.MultiplierPrice.String())
	require.Equal(t, "3.3", view.Fast.CacheWrite5m.MultiplierPrice.String())
	require.Equal(t, "5.5", view.Fast.CacheWrite1h.MultiplierPrice.String())
	require.Equal(t, "3.3", view.CacheWrite5m.MultiplierPrice.String())
	require.Equal(t, "5.5", view.CacheWrite1h.MultiplierPrice.String())
}

func TestBuildModelPriceViewKeepsExplicitFreeChannelPriceAndHidesFast(t *testing.T) {
	pricing := &ModelPricing{
		InputPricePerToken:         0,
		InputPricePerTokenPriority: 0,
		Presence: &ModelPricingPresence{
			Input: true,
		},
	}
	view, err := BuildModelPriceView(&ResolvedPricing{Mode: BillingModeToken, BasePricing: pricing}, decimal.NewFromInt(2))
	require.NoError(t, err)
	require.NotNil(t, view.Input)
	require.True(t, view.Input.Original.IsZero())
	require.Nil(t, view.Fast)
}

func TestBuildModelPriceViewNormalizesTiersAndRejectsOverlap(t *testing.T) {
	input := 2e-6
	output := 8e-6
	max := 1000
	view, err := BuildModelPriceView(&ResolvedPricing{
		Mode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 1000, OutputPrice: &output, SortOrder: 2},
			{MinTokens: 0, MaxTokens: &max, InputPrice: &input, SortOrder: 1},
		},
	}, decimal.NewFromInt(1))
	require.NoError(t, err)
	require.Len(t, view.Tiers, 2)
	require.Equal(t, 0, view.Tiers[0].MinTokens)
	require.Equal(t, 1000, view.Tiers[1].MinTokens)

	overlapMax := 2000
	_, err = BuildModelPriceView(&ResolvedPricing{
		Mode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: &overlapMax, InputPrice: &input},
			{MinTokens: 1000, OutputPrice: &output},
		},
	}, decimal.NewFromInt(1))
	require.ErrorContains(t, err, "overlapping")
}

func TestBuildModelPriceViewPerImageExplicitZero(t *testing.T) {
	view, err := BuildModelPriceView(&ResolvedPricing{
		Mode:                          BillingModeImage,
		DefaultPerRequestPrice:        0,
		DefaultPerRequestPricePresent: true,
	}, decimal.RequireFromString("1.25"))
	require.NoError(t, err)
	require.NotNil(t, view.PerRequest)
	require.Equal(t, PriceUnitPerImage, view.PerRequest.Unit)
}

func TestBuildModelPriceViewRejectsNegativePriceOrMultiplier(t *testing.T) {
	_, err := NewDecimalUnitPrice(decimal.NewFromInt(-1), decimal.NewFromInt(1), PriceUnitPerRequest)
	require.Error(t, err)
	_, err = BuildModelPriceView(&ResolvedPricing{}, decimal.NewFromInt(-1))
	require.Error(t, err)
}
