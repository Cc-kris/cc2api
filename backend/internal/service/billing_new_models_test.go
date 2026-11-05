package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// V02-T19/T21/T22：新模型计价。生产的远程价格表没有档位 ID 和部分新模型，
// 计费靠 PricingService 的族匹配和 BillingService 的回退价，这里两条路都测。

type expectedTokenPrice struct {
	input     float64
	output    float64
	cacheRead float64
}

func assertTokenPrice(t *testing.T, want expectedTokenPrice, got *ModelPricing) {
	t.Helper()
	require.NotNil(t, got)
	assert.InDelta(t, want.input, got.InputPricePerToken, 1e-15, "input")
	assert.InDelta(t, want.output, got.OutputPricePerToken, 1e-15, "output")
	assert.InDelta(t, want.cacheRead, got.CacheReadPricePerToken, 1e-15, "cache read")
}

var (
	priceFable5   = expectedTokenPrice{10e-6, 50e-6, 1e-6}
	priceFable51  = expectedTokenPrice{10e-6, 50e-6, 0.25e-6}
	priceOpus48   = expectedTokenPrice{5e-6, 25e-6, 0.5e-6}
	priceOpus55   = expectedTokenPrice{4e-6, 20e-6, 0.2e-6}
	priceSonnet55 = expectedTokenPrice{2e-6, 10e-6, 0.2e-6}
	priceSonnet4  = expectedTokenPrice{3e-6, 15e-6, 0.3e-6}
	priceHaiku55  = expectedTokenPrice{0.1e-6, 0.5e-6, 0.01e-6}
)

// 价格服务完全不可用（远程下载失败且回退文件也没有）时，8 个新 Claude 模型和
// 5.5 档位 ID 都必须有非 0 回退价，且不能落到 claude-3-opus 的 $15/$75。
func TestBillingFallbackPricing_NewClaudeModels(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	cases := map[string]expectedTokenPrice{
		"claude-fable-5":                     priceFable5,
		"anthropic.claude-fable-5":           priceFable5,
		"claude-fable-5-1":                   priceFable51,
		"anthropic.claude-fable-5-1":         priceFable51,
		"claude-opus-4-8":                    priceOpus48,
		"us.anthropic.claude-opus-4-8-v1":    priceOpus48,
		"claude-opus-5":                      priceOpus48,
		"us.anthropic.claude-opus-5-v1":      priceOpus48,
		"claude-opus-5-5":                    priceOpus55,
		"claude-opus-5-5-high":               priceOpus55,
		"claude-opus-5-5-medium":             priceOpus55,
		"claude-opus-5-5-low":                priceOpus55,
		"claude-sonnet-5":                    priceSonnet4,
		"claude-sonnet-5-5":                  priceSonnet55,
		"claude-sonnet-5-5-high":             priceSonnet55,
		"claude-sonnet-5-5-low":              priceSonnet55,
		"global.anthropic.claude-sonnet-5-5": priceSonnet55,
		"claude-haiku-5-5":                   priceHaiku55,
		// 相邻型号不能被 "opus-5" / "fable-5" 误匹配
		"claude-opus-4-5-20251101": priceOpus48,
	}
	for model, want := range cases {
		t.Run(model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			assertTokenPrice(t, want, pricing)
		})
	}

	opus5, err := svc.GetModelPricing("claude-opus-5")
	require.NoError(t, err)
	assert.InDelta(t, 10e-6, opus5.InputPricePerTokenPriority, 1e-15, "opus-5 priority input = 2x")
	assert.InDelta(t, 50e-6, opus5.OutputPricePerTokenPriority, 1e-15, "opus-5 priority output = 2x")

	haiku, err := svc.GetModelPricing("claude-haiku-5-5")
	require.NoError(t, err)
	assert.Equal(t, 100000, haiku.LongContextInputThreshold)
	assert.InDelta(t, 5, haiku.LongContextInputMultiplier, 1e-12)
}

// 实收不为 0（V02-AC44 反向：不得记 0 元）。
func TestBillingFallbackPricing_NewClaudeModelsChargeNonZero(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200}
	for _, model := range []string{
		"claude-fable-5", "claude-fable-5-1", "claude-opus-4-8", "claude-opus-5",
		"claude-opus-5-5", "claude-sonnet-5", "claude-sonnet-5-5", "claude-haiku-5-5",
		"claude-opus-5-5-high", "claude-sonnet-5-5-low",
	} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.Greater(t, cost.ActualCost, 0.0)
		})
	}
}

// V02-AC45：远程价格表只有基础模型（没有档位 ID），档位 ID 必须按基础模型计价，
// 不能被族匹配按 opus-4（$15/$75）或 sonnet-4 计价。
func TestPricingService_Claude55TierIDsUseBaseModelPrice(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"claude-opus-5-5":        {InputCostPerToken: 4e-6, OutputCostPerToken: 20e-6, CacheReadInputTokenCost: 0.2e-6},
		"claude-sonnet-5-5":      {InputCostPerToken: 2e-6, OutputCostPerToken: 10e-6, CacheReadInputTokenCost: 0.2e-6},
		"claude-opus-5":          {InputCostPerToken: 5e-6, OutputCostPerToken: 25e-6, CacheReadInputTokenCost: 0.5e-6},
		"claude-opus-4-1":        {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		"claude-opus-4-20250514": {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		"claude-sonnet-4-6":      {InputCostPerToken: 3e-6, OutputCostPerToken: 15e-6, CacheReadInputTokenCost: 0.3e-6},
	}}
	svc := NewBillingService(&config.Config{}, pricing)

	cases := map[string]expectedTokenPrice{
		"claude-opus-5-5-high":     priceOpus55,
		"claude-opus-5-5-medium":   priceOpus55,
		"claude-opus-5-5-low":      priceOpus55,
		"claude-sonnet-5-5-high":   priceSonnet55,
		"claude-sonnet-5-5-medium": priceSonnet55,
		"claude-sonnet-5-5-low":    priceSonnet55,
		"claude-opus-5":            priceOpus48,
	}
	for model, want := range cases {
		t.Run(model, func(t *testing.T) {
			got, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			assertTokenPrice(t, want, got)
		})
	}
}

// 远程价格表连基础 5.5 条目都没有时，族匹配用内置兜底价，不能落到 opus-4 / sonnet-4 / haiku-3。
func TestPricingService_Claude55FamilyFallbackWithoutCatalogEntry(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"claude-opus-4-1":          {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		"claude-3-opus-20240229":   {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		"claude-sonnet-4-20250514": {InputCostPerToken: 3e-6, OutputCostPerToken: 15e-6},
		"claude-3-haiku-20240307":  {InputCostPerToken: 0.25e-6, OutputCostPerToken: 1.25e-6},
	}}
	cases := map[string]float64{
		"claude-opus-5-5":                    4e-6,
		"claude-opus-5-5-high":               4e-6,
		"anthropic/claude-opus-5.5":          4e-6,
		"claude-sonnet-5-5-low":              2e-6,
		"global.anthropic.claude-sonnet-5-5": 2e-6,
		"claude-haiku-5-5":                   0.1e-6,
	}
	for model, wantInput := range cases {
		t.Run(model, func(t *testing.T) {
			got := pricing.GetModelPricing(model)
			require.NotNil(t, got)
			assert.InDelta(t, wantInput, got.InputCostPerToken, 1e-15)
		})
	}
}

// 价格表没有 claude-opus-5 时回退到同价的 4.8，不能按 opus-4 计（6c9b84cc7）；
// 也不能被 claude-opus-5-5 条目抢走。
func TestPricingService_Opus5FamilyFallbackSkipsOpus55(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"claude-opus-4-8":        {InputCostPerToken: 5e-6, OutputCostPerToken: 25e-6},
		"claude-opus-5-5":        {InputCostPerToken: 4e-6, OutputCostPerToken: 20e-6},
		"claude-opus-4-1":        {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		"claude-3-opus-20240229": {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
	}}
	for _, model := range []string{"claude-opus-5", "us.anthropic.claude-opus-5-v1"} {
		t.Run(model, func(t *testing.T) {
			// map 遍历顺序随机，多跑几次保证不会偶然命中 5.5
			for i := 0; i < 20; i++ {
				got := pricing.GetModelPricing(model)
				require.NotNil(t, got)
				require.InDelta(t, 5e-6, got.InputCostPerToken, 1e-15)
			}
		})
	}
}

// V02-AC49：Gemini 档位 ID 用基础模型价；价格表有档位专属条目时优先用条目。
func TestPricingService_GeminiThinkingTiersUseBasePricing(t *testing.T) {
	basePricing := &LiteLLMModelPricing{InputCostPerToken: 0.75e-6, OutputCostPerToken: 3.75e-6, CacheReadInputTokenCost: 0.075e-6}
	for _, baseModel := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash", "gemini-4-argon"} {
		svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{baseModel: basePricing}}
		for _, tier := range []string{"", "-high", "-low", "-medium", "-tiered"} {
			model := baseModel + tier
			t.Run(model, func(t *testing.T) {
				require.Same(t, basePricing, svc.GetModelPricing(model))
				require.Same(t, basePricing, svc.GetModelPricing("models/"+model))
			})
		}
	}
}

func TestPricingService_GeminiTierSpecificPricingTakesPrecedence(t *testing.T) {
	basePricing := &LiteLLMModelPricing{InputCostPerToken: 1.5e-6}
	tierPricing := &LiteLLMModelPricing{InputCostPerToken: 2e-6}
	for _, baseModel := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
		t.Run(baseModel, func(t *testing.T) {
			svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
				baseModel:          basePricing,
				baseModel + "-low": tierPricing,
			}}
			require.Same(t, tierPricing, svc.GetModelPricing("models/"+baseModel+"-low"))
			require.Same(t, basePricing, svc.GetModelPricing(baseModel+"-high"))
		})
	}
}

// 价格表里的 cyber 条目不能被 argon 抢走（反过来也一样）。
func TestPricingService_Gemini4ArgonCyberUsesOwnEntry(t *testing.T) {
	basePricing := &LiteLLMModelPricing{InputCostPerToken: 2e-6}
	cyberPricing := &LiteLLMModelPricing{InputCostPerToken: 4e-6}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gemini-4-argon":                  basePricing,
		"gemini-4-argon-cyber-permissive": cyberPricing,
	}}
	for _, model := range []string{"gemini-4-argon", "gemini-4-argon-high", "models/gemini-4-argon-low", "projects/p/locations/global/publishers/google/models/gemini-4-argon"} {
		require.Same(t, basePricing, svc.GetModelPricing(model), model)
	}
	require.Same(t, cyberPricing, svc.GetModelPricing("gemini-4-argon-cyber-permissive"))
}

// 生产远程价格表没有 argon / cyber：必须走 BillingService 回退价，不能报价格不可用或记 0 元。
// 3.6/3.7/3.8 在远程价格表里有，但远程下载失败时也要能走回退价。
func TestBillingFallbackPricing_NewGeminiModelsAreBillable(t *testing.T) {
	// 模拟生产：价格表有别的模型，但没有这几个
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gemini-2.5-pro": {InputCostPerToken: 1.25e-6, OutputCostPerToken: 10e-6},
	}}
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}
	cases := []struct {
		model               string
		input, output, read float64
	}{
		{"gemini-3.6-flash", 0.75, 3.75, 0.075},
		{"gemini-3.6-flash-high", 0.75, 3.75, 0.075},
		{"gemini-3.6-flash-tiered", 0.75, 3.75, 0.075},
		{"gemini-3.7-flash-low", 0.75, 3.75, 0.075},
		{"gemini-3.8-flash-medium", 0.75, 3.75, 0.075},
		{"gemini-4-argon", 2, 10, 0.1},
		{"gemini-4-argon-high", 2, 10, 0.1},
		{"gemini-4-argon-low", 2, 10, 0.1},
		{"gemini-4-argon-medium", 2, 10, 0.1},
		{"models/gemini-4-argon", 2, 10, 0.1},
		{"gemini-4-argon-cyber-permissive", 4, 20, 0.2},
		{"gemini-4-argon-cyber", 4, 20, 0.2},
		{"publishers/google/models/gemini-4-argon-cyber-permissive", 4, 20, 0.2},
	}
	for _, svc := range []*BillingService{NewBillingService(&config.Config{}, nil), NewBillingService(&config.Config{}, pricing)} {
		for _, tc := range cases {
			t.Run(tc.model, func(t *testing.T) {
				cost, err := svc.CalculateCost(tc.model, tokens, 1)
				require.NoError(t, err)
				require.InDelta(t, tc.input, cost.InputCost, 1e-9)
				require.InDelta(t, tc.output, cost.OutputCost, 1e-9)
				require.InDelta(t, tc.read, cost.CacheReadCost, 1e-9)
				require.Greater(t, cost.ActualCost, 0.0)
			})
		}
	}
}
