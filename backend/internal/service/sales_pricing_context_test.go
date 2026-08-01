package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestBuildV2SalesPricingContextUsesRequestedModelAndSharedPriceView(t *testing.T) {
	ctx, err := BuildV2SalesPricingContext(" gpt-5.5 ", "catalog-v1", &ResolvedPricing{
		Mode: BillingModeToken, Source: PricingSourceLiteLLM,
		BasePricing: &ModelPricing{InputPricePerToken: 2.5e-6},
	}, decimal.RequireFromString("1.1000"))
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", ctx.RequestedModel)
	require.Equal(t, ctx.RequestedModel, ctx.EffectiveModel)
	require.Equal(t, SalesPricingVersionV2, ctx.Version)
	require.Equal(t, "2.75", ctx.Prices.Input.MultiplierPrice.String())
}

func TestAttachV2ShadowKeepsLegacyAsActualContext(t *testing.T) {
	legacy := &SalesPricingContext{RequestedModel: "alias", EffectiveModel: "legacy-model", Version: SalesPricingVersionLegacy}
	v2 := &SalesPricingContext{EffectiveModel: "alias", Version: SalesPricingVersionV2, PricingSource: PricingSourceLiteLLM, Checksum: "v2"}
	shadow, err := AttachV2Shadow(legacy, v2)
	require.NoError(t, err)
	require.Equal(t, "legacy-model", shadow.EffectiveModel)
	require.Equal(t, SalesPricingVersionShadow, shadow.Version)
	require.Equal(t, "alias", shadow.Shadow.EffectiveModel)
	require.Equal(t, SalesPricingVersionLegacy, legacy.Version)
}

func TestApplySalesPricingContextToUsageLogPersistsExactUsageAndAmounts(t *testing.T) {
	pricing, err := BuildLegacySalesPricingContext(
		"gpt-5.4",
		"gpt-5.4-2026-07-01",
		BillingModelSourceUpstream,
		"",
		&ResolvedPricing{
			Mode:   BillingModeToken,
			Source: PricingSourceChannel,
			BasePricing: &ModelPricing{
				InputPricePerToken:         2e-6,
				OutputPricePerToken:        8e-6,
				CacheReadPricePerToken:     2e-7,
				CacheCreationPricePerToken: 2.5e-6,
			},
		},
		decimal.RequireFromString("1.2500"),
	)
	require.NoError(t, err)
	mode := string(BillingModeToken)
	serviceTier := "priority"
	log := &UsageLog{
		InputTokens:         1000,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     500,
		BillingMode:         &mode,
		ServiceTier:         &serviceTier,
	}
	cost := &CostBreakdown{
		InputCost:         0.002,
		OutputCost:        0.0016,
		CacheCreationCost: 0.000125,
		CacheReadCost:     0.0001,
		TotalCost:         0.003825,
		ActualCost:        0.00478125,
	}

	require.NoError(t, ApplySalesPricingContextToUsageLog(log, pricing, cost))
	require.Equal(t, "gpt-5.4", *log.SalesModel)
	require.Equal(t, "gpt-5.4-2026-07-01", *log.SalesPricingEffectiveModel)
	require.Equal(t, "legacy", *log.SalesPricingVersion)
	require.Equal(t, "channel", *log.SalesPricingSource)
	require.NotEmpty(t, *log.SalesPricingChecksum)
	require.True(t, log.UsageListValue.Equal(decimal.RequireFromString("0.00478125")))
	require.Equal(t, "1.2500", log.SalesPricingSnapshot["multiplier"])
	require.Equal(t, "priority", log.SalesPricingSnapshot["service_tier"])
	usage, ok := log.SalesPricingSnapshot["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1000), usage["input_tokens"])
	amounts, ok := log.SalesPricingSnapshot["amounts"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "0.0038250000", amounts["original_total"])
	require.Equal(t, "0.0047812500", amounts["multiplier_total"])
}

func TestApplyShadowSalesPricingContextKeepsLegacyChargeAndStoresV2Delta(t *testing.T) {
	legacy, err := BuildLegacySalesPricingContext("alias", "legacy-model", BillingModelSourceUpstream, "", &ResolvedPricing{
		Mode: BillingModeToken, Source: PricingSourceLiteLLM,
		BasePricing: &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 2e-6},
	}, decimal.RequireFromString("1.2000"))
	require.NoError(t, err)
	v2, err := BuildV2SalesPricingContext("alias", "", &ResolvedPricing{
		Mode: BillingModeToken, Source: PricingSourceChannel,
		BasePricing: &ModelPricing{InputPricePerToken: 1.5e-6, OutputPricePerToken: 2.5e-6},
	}, decimal.RequireFromString("1.2000"))
	require.NoError(t, err)
	shadow, err := AttachV2Shadow(legacy, v2)
	require.NoError(t, err)
	mode := string(BillingModeToken)
	log := &UsageLog{InputTokens: 1000, OutputTokens: 100, BillingMode: &mode}
	legacyCost := &CostBreakdown{TotalCost: 0.0012, ActualCost: 0.00144}
	v2Cost := &CostBreakdown{TotalCost: 0.00175, ActualCost: 0.0021}

	require.NoError(t, ApplyShadowSalesPricingContextToUsageLog(log, shadow, legacyCost, v2Cost))
	require.Equal(t, "shadow", *log.SalesPricingVersion)
	require.True(t, log.UsageListValue.Equal(decimal.RequireFromString("0.00144")))
	require.True(t, log.SalesPricingShadowDelta.Equal(decimal.RequireFromString("0.00066")))
	require.Equal(t, "v2", log.SalesPricingShadowSnapshot["version"])
	amounts, ok := log.SalesPricingShadowSnapshot["amounts"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "0.0021000000", amounts["multiplier_total"])
}

func TestValidateSalesPricingTransition(t *testing.T) {
	require.NoError(t, ValidateSalesPricingTransition(SalesPricingVersionLegacy, SalesPricingVersionShadow))
	require.NoError(t, ValidateSalesPricingTransition(SalesPricingVersionShadow, SalesPricingVersionV2))
	require.NoError(t, ValidateSalesPricingTransition(SalesPricingVersionV2, SalesPricingVersionShadow))
	require.Error(t, ValidateSalesPricingTransition(SalesPricingVersionLegacy, SalesPricingVersionV2))
	require.Error(t, ValidateSalesPricingTransition(SalesPricingVersionV2, SalesPricingVersionLegacy))
}
