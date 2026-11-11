package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 移植自上游 593fc9365 pricing_service_override_test.go。依赖 above_XXXk 阶梯折算的
// 用例在 V03-T11 目录驱动阶梯落地后补到 pricing_service_long_context_test.go。

const overrideCatalogJSON = `{
	"gpt-5.5": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 5e-06, "input_cost_per_token_priority": 1.25e-05,
		"output_cost_per_token": 3e-05, "output_cost_per_token_priority": 7.5e-05,
		"cache_read_input_token_cost": 5e-07},
	"gpt-5.4": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1.5e-05,
		"cache_read_input_token_cost": 2.5e-07}
}`

func newPricingServiceWithOverride(t *testing.T, overrideJSON string) *PricingService {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(overrideJSON), 0644))
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.OverrideFile = path
	return svc
}

func TestPricingOverride_FieldLevelMergeKeepsOtherFields(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.4": {"input_cost_per_token": 3e-06}}`)
	data, err := svc.parsePricingData([]byte(overrideCatalogJSON))
	require.NoError(t, err)

	patched := data["gpt-5.4"]
	require.InDelta(t, 3e-6, patched.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.5e-5, patched.OutputCostPerToken, 1e-12, "未覆盖字段保持目录值")
	require.Equal(t, "openai", patched.LiteLLMProvider)
	require.InDelta(t, 5e-6, data["gpt-5.5"].InputCostPerToken, 1e-12, "未覆盖的模型不受影响")
}

func TestPricingOverride_NullFieldValueRemovesField(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.5": {"input_cost_per_token_priority": null, "cache_read_input_token_cost": null}}`)
	data, err := svc.parsePricingData([]byte(overrideCatalogJSON))
	require.NoError(t, err)
	require.Zero(t, data["gpt-5.5"].InputCostPerTokenPriority)
	require.Zero(t, data["gpt-5.5"].CacheReadInputTokenCost)
	require.InDelta(t, 5e-6, data["gpt-5.5"].InputCostPerToken, 1e-12)
	require.InDelta(t, 7.5e-5, data["gpt-5.5"].OutputCostPerTokenPriority, 1e-12)
}

// 完整加载管线：目录里没有的模型作为独立条目并入；目录条目被修补。
func TestPricingOverride_LoadPipelineAddsNewModel(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	require.NoError(t, os.WriteFile(catalogPath, []byte(`{
		"remote-model": {"litellm_provider": "test", "mode": "chat",
			"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06}
	}`), 0644))
	overridePath := filepath.Join(dir, "overrides.json")
	require.NoError(t, os.WriteFile(overridePath, []byte(`{
		"remote-model": {"output_cost_per_token": 3e-06},
		"override-new-model": {"litellm_provider": "test", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 1e-05}
	}`), 0644))

	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.OverrideFile = overridePath
	require.NoError(t, svc.loadPricingData(catalogPath))

	added := svc.pricingData["override-new-model"]
	require.NotNil(t, added)
	require.InDelta(t, 5e-6, added.InputCostPerToken, 1e-12)
	require.InDelta(t, 1e-5, added.OutputCostPerToken, 1e-12)
	require.InDelta(t, 1e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12)
	require.InDelta(t, 3e-6, svc.pricingData["remote-model"].OutputCostPerToken, 1e-12)
}

// 拼错模型名（或纯补丁落在不存在的模型上）会被有效性过滤丢弃，必须有哨兵 WARN。
func TestPricingOverride_IneffectiveEntryWarns(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	require.NoError(t, os.WriteFile(catalogPath, []byte(`{
		"remote-model": {"litellm_provider": "test", "mode": "chat", "input_cost_per_token": 1e-06}
	}`), 0644))
	overridePath := filepath.Join(dir, "overrides.json")
	require.NoError(t, os.WriteFile(overridePath, []byte(`{
		"typo-model": {"mode": "chat"}
	}`), 0644))

	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.OverrideFile = overridePath
	require.NoError(t, svc.loadPricingData(catalogPath))

	require.NotContains(t, svc.pricingData, "typo-model")
	require.True(t, logSink.ContainsMessageAtLevel("override had no effect for 1 model(s): typo-model", "warn"))
}

func TestPricingOverride_NonObjectEntryKeepsCatalogEntry(t *testing.T) {
	svc := newPricingServiceWithOverride(t, `{"gpt-5.5": "oops"}`)
	data, err := svc.parsePricingData([]byte(overrideCatalogJSON))
	require.NoError(t, err)
	require.InDelta(t, 5e-6, data["gpt-5.5"].InputCostPerToken, 1e-12, "非法补丁忽略，目录条目原样保留")
}

func TestPricingOverride_MissingOrInvalidFileIsIgnored(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		svc := &PricingService{cfg: &config.Config{}}
		svc.cfg.Pricing.OverrideFile = filepath.Join(t.TempDir(), "absent.json")
		data, err := svc.parsePricingData([]byte(overrideCatalogJSON))
		require.NoError(t, err)
		require.InDelta(t, 5e-6, data["gpt-5.5"].InputCostPerToken, 1e-12)
	})

	t.Run("invalid json", func(t *testing.T) {
		svc := newPricingServiceWithOverride(t, `{invalid`)
		data, err := svc.parsePricingData([]byte(overrideCatalogJSON))
		require.NoError(t, err)
		require.InDelta(t, 5e-6, data["gpt-5.5"].InputCostPerToken, 1e-12)
	})
}

// 对仓库出厂目录打补丁：只改指定模型的单价，其余模型不变。
func TestPricingOverride_PatchesShippedCatalog(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "data", "model_pricing.json"))
	require.NoError(t, err)

	svc := newPricingServiceWithOverride(t, `{"gpt-5.5": {"input_cost_per_token": 6e-06}}`)
	data, err := svc.parsePricingData(body)
	require.NoError(t, err)
	svc.pricingData = data
	billing := NewBillingService(&config.Config{}, svc)

	cost, err := billing.CalculateCost("gpt-5.5", UsageTokens{InputTokens: 1000, OutputTokens: 100}, 1)
	require.NoError(t, err)
	require.InDelta(t, 0.009, cost.TotalCost, 1e-12)

	pricing, err := billing.GetModelPricing("gpt-5.4")
	require.NoError(t, err)
	require.InDelta(t, 2.5e-6, pricing.InputPricePerToken, 1e-12)
}
