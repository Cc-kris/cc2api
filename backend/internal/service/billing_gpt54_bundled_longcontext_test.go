package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 随包 fallback 目录（resources/model-pricing）没有 above_272k 字段，也没有 gpt-5.5 条目；
// GPT-5.4/5.5 仍须保持基线 272K 阶梯（输入 2x、输出 1.5x），否则超长会话被少收费。
func newBundledFallbackBillingService(t *testing.T, overrideJSON string) (*BillingService, map[string]*LiteLLMModelPricing) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	svc := &PricingService{cfg: &config.Config{}}
	if overrideJSON != "" {
		path := filepath.Join(t.TempDir(), "overrides.json")
		require.NoError(t, os.WriteFile(path, []byte(overrideJSON), 0644))
		svc.cfg.Pricing.OverrideFile = path
	}
	data, err := svc.parsePricingData(body)
	require.NoError(t, err)
	svc.pricingData = data
	return NewBillingService(&config.Config{}, svc), data
}

func TestBundledFallbackCatalog_GPT54And55KeepBaselineLongContext(t *testing.T) {
	billing, data := newBundledFallbackBillingService(t, "")
	_, hasGPT55 := data["gpt-5.5"]
	require.False(t, hasGPT55, "随包目录无 gpt-5.5 条目，依赖静态回退价")

	for _, model := range []string{"gpt-5.4", "gpt-5.4-2026-03-05", "gpt-5.5", "gpt-5.5-2026-04-23", "gpt5.5", "openai/gpt5.4"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := billing.GetModelPricing(model)
			require.NoError(t, err)
			require.InDelta(t, 2.5e-6, pricing.InputPricePerToken, 1e-12)
			require.InDelta(t, 1.5e-5, pricing.OutputPricePerToken, 1e-12)
			require.Equal(t, 272000, pricing.LongContextInputThreshold)
			require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
			require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
			require.False(t, pricing.LongContextThresholdInclusive)
		})
	}
}

func TestBundledFallbackCatalog_GPT54LongContextCost(t *testing.T) {
	billing, _ := newBundledFallbackBillingService(t, "")
	for _, model := range []string{"gpt-5.4", "gpt-5.5"} {
		t.Run(model, func(t *testing.T) {
			// 超过阈值：整次会话输入 2x、输出 1.5x。
			cost, err := billing.CalculateCost(model, UsageTokens{InputTokens: 300000, OutputTokens: 1000}, 1)
			require.NoError(t, err)
			require.InDelta(t, 300000*2.5e-6*2.0, cost.InputCost, 1e-9)
			require.InDelta(t, 1000*1.5e-5*1.5, cost.OutputCost, 1e-9)

			// 恰等于阈值：严格大于才触发，保持基础价。
			cost, err = billing.CalculateCost(model, UsageTokens{InputTokens: 272000, OutputTokens: 1000}, 1)
			require.NoError(t, err)
			require.InDelta(t, 272000*2.5e-6, cost.InputCost, 1e-9)
			require.InDelta(t, 1000*1.5e-5, cost.OutputCost, 1e-9)
		})
	}
}

func TestBundledFallbackCatalog_MiniNanoHaveNoLongContext(t *testing.T) {
	billing, _ := newBundledFallbackBillingService(t, "")
	for _, model := range []string{"gpt-5.4-mini", "gpt-5.4-nano", "codex-auto-review"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := billing.GetModelPricing(model)
			require.NoError(t, err)
			require.Zero(t, pricing.LongContextInputThreshold)
			require.Zero(t, pricing.LongContextInputMultiplier)
			require.Zero(t, pricing.LongContextOutputMultiplier)
		})
	}
}

func TestBundledFallbackCatalog_OverrideStillWinsOverBaseline(t *testing.T) {
	// 显式 threshold=0 关闭阶梯。
	billing, _ := newBundledFallbackBillingService(t, `{"gpt-5.4": {"long_context_input_token_threshold": 0}}`)
	pricing, err := billing.GetModelPricing("gpt-5.4")
	require.NoError(t, err)
	require.Zero(t, pricing.LongContextInputThreshold)

	// override 给出自己的 above 阶梯时以 override 为准（3x / 2x）。
	billing, _ = newBundledFallbackBillingService(t, `{"gpt-5.4": {"input_cost_per_token_above_272k_tokens": 7.5e-06, "output_cost_per_token_above_272k_tokens": 3e-05}}`)
	pricing, err = billing.GetModelPricing("gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
	require.InDelta(t, 3.0, pricing.LongContextInputMultiplier, 1e-9)
	require.InDelta(t, 2.0, pricing.LongContextOutputMultiplier, 1e-9)
}
