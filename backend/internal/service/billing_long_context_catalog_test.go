package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// V03-T11：目录驱动长上下文阶梯，门控只看分组。移植上游 530fb20f2 的解析/计费用例，
// 并按 acceptance AC29、AC37 给出的金额断言。

const openAILadderCatalogJSON = `{
	"gpt-5.4": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1.5e-05,
		"cache_read_input_token_cost": 2.5e-07,
		"input_cost_per_token_above_272k_tokens": 5e-06,
		"output_cost_per_token_above_272k_tokens": 2.25e-05,
		"input_cost_per_token_above_272k_tokens_flex": 2.5e-06},
	"gpt-5.4-2026-03-05": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1.5e-05,
		"input_cost_per_token_above_272k_tokens": 5e-06,
		"output_cost_per_token_above_272k_tokens": 2.25e-05},
	"gpt-5.6-sol": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
		"cache_creation_input_token_cost": 6.25e-06, "cache_read_input_token_cost": 5e-07,
		"input_cost_per_token_above_272k_tokens": 1e-05, "output_cost_per_token_above_272k_tokens": 4.5e-05},
	"gpt-5.6-terra": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1.5e-05,
		"cache_creation_input_token_cost": 3.125e-06, "cache_read_input_token_cost": 2.5e-07,
		"input_cost_per_token_above_272k_tokens": 5e-06, "output_cost_per_token_above_272k_tokens": 2.25e-05},
	"gpt-5.6-luna": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 1e-06, "output_cost_per_token": 6e-06,
		"cache_creation_input_token_cost": 1.25e-06, "cache_read_input_token_cost": 1e-07,
		"input_cost_per_token_above_272k_tokens": 2e-06, "output_cost_per_token_above_272k_tokens": 9e-06}
}`

func newStubPricingServiceFromJSON(t *testing.T, body string) *PricingService {
	t.Helper()
	svc := &PricingService{cfg: &config.Config{}}
	data, err := svc.parsePricingData([]byte(body))
	require.NoError(t, err)
	svc.pricingData = data
	return svc
}

func newTestBillingServiceWithOpenAILadderCatalog(t *testing.T) *BillingService {
	t.Helper()
	return NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, openAILadderCatalogJSON))
}

// 仓库出厂目录（data/model_pricing.json）。
func newShippedCatalogBillingService(t *testing.T) (*BillingService, *PricingService) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "data", "model_pricing.json"))
	require.NoError(t, err)
	pricing := &PricingService{cfg: &config.Config{}}
	data, err := pricing.parsePricingData(body)
	require.NoError(t, err)
	pricing.pricingData = data
	return NewBillingService(&config.Config{}, pricing), pricing
}

// longContextGroupRepo 只实现 Resolve 用到的 GetByIDLite；其余方法由嵌入的 nil 接口占位，
// 被调用会 panic，便于发现意外依赖。
type longContextGroupRepo struct {
	GroupRepository
	groups map[int64]*Group
}

func (r *longContextGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if g, ok := r.groups[id]; ok {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func TestDeriveLongContextFromAboveTierFields(t *testing.T) {
	svc := newStubPricingServiceFromJSON(t, openAILadderCatalogJSON)
	p := svc.pricingData["gpt-5.4"]
	require.Equal(t, 272000, p.LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, p.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, p.LongContextOutputCostMultiplier, 1e-12)
	require.False(t, p.LongContextThresholdInclusive)

	// above 价不高于基础价时不生成阶梯。
	flat := newStubPricingServiceFromJSON(t, `{"m": {"input_cost_per_token": 1e-6, "output_cost_per_token": 2e-6,
		"input_cost_per_token_above_200k_tokens": 1e-6, "output_cost_per_token_above_200k_tokens": 2e-6}}`)
	require.Zero(t, flat.pricingData["m"].LongContextInputTokenThreshold)

	// 多阈值取最小阈值。
	multi := newStubPricingServiceFromJSON(t, `{"m": {"input_cost_per_token": 1e-6, "output_cost_per_token": 2e-6,
		"input_cost_per_token_above_128k_tokens": 2e-6, "input_cost_per_token_above_200k_tokens": 4e-6}}`)
	require.Equal(t, 128000, multi.pricingData["m"].LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, multi.pricingData["m"].LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.0, multi.pricingData["m"].LongContextOutputCostMultiplier, 1e-12)

	// xAI 达到阈值即进高档。
	xai := newStubPricingServiceFromJSON(t, `{"grok-x": {"litellm_provider": "xai", "input_cost_per_token": 1e-6, "output_cost_per_token": 2e-6,
		"input_cost_per_token_above_200k_tokens": 2e-6, "output_cost_per_token_above_200k_tokens": 4e-6}}`)
	require.True(t, xai.pricingData["grok-x"].LongContextThresholdInclusive)
}

// 显式 long_context_* 字段（glm-5）优先于 above 字段。
func TestExplicitLongContextFieldsTakePrecedence(t *testing.T) {
	svc := newStubPricingServiceFromJSON(t, `{"glm-x": {"litellm_provider": "zhipu",
		"input_cost_per_token": 1e-6, "output_cost_per_token": 2e-6, "cache_read_input_token_cost": 1e-7,
		"input_cost_per_token_above_200k_tokens": 9e-6,
		"long_context_input_token_threshold": 32000,
		"long_context_input_cost_multiplier": 1.5,
		"long_context_output_cost_multiplier": 1.2,
		"long_context_cache_read_cost_multiplier": 1.25,
		"long_context_threshold_inclusive": true}}`)
	p := svc.pricingData["glm-x"]
	require.Equal(t, 32000, p.LongContextInputTokenThreshold)
	require.InDelta(t, 1.5, p.LongContextInputCostMultiplier, 1e-12)
	require.True(t, p.LongContextThresholdInclusive)

	billing := NewBillingService(&config.Config{}, svc)
	cost, err := billing.CalculateCost("glm-x", UsageTokens{InputTokens: 22000, CacheReadTokens: 10000, OutputTokens: 1000}, 1)
	require.NoError(t, err)
	// 22000+10000=32000 达到阈值（inclusive）。
	require.InDelta(t, 22000*1e-6*1.5, cost.InputCost, 1e-12)
	require.InDelta(t, 1000*2e-6*1.2, cost.OutputCost, 1e-12)
	require.InDelta(t, 10000*1e-7*1.25, cost.CacheReadCost, 1e-12, "显式缓存读取倍率")

	// 显式 threshold=0 关闭阶梯（override 场景）。
	off := newStubPricingServiceFromJSON(t, `{"m": {"input_cost_per_token": 1e-6, "output_cost_per_token": 2e-6,
		"input_cost_per_token_above_200k_tokens": 2e-6, "long_context_input_token_threshold": 0}}`)
	require.Zero(t, off.pricingData["m"].LongContextInputTokenThreshold)
}

// 倍率 ≤0（显式字段只写一侧）按 1 计，不把分项算成免费。
func TestLongContextMultiplierNonPositiveTreatedAsOne(t *testing.T) {
	svc := newStubPricingServiceFromJSON(t, `{"m": {"input_cost_per_token": 1e-6, "output_cost_per_token": 2e-6,
		"long_context_input_token_threshold": 1000, "long_context_input_cost_multiplier": 2}}`)
	billing := NewBillingService(&config.Config{}, svc)
	cost, err := billing.CalculateCost("m", UsageTokens{InputTokens: 2000, OutputTokens: 100}, 1)
	require.NoError(t, err)
	require.InDelta(t, 2000*1e-6*2, cost.InputCost, 1e-12)
	require.InDelta(t, 100*2e-6, cost.OutputCost, 1e-12)
}

// AC29：gpt-5.4 阶梯受分组开关控制；无分组视为开启；没有账号级开关。
func TestLongContextGateGroupOnly(t *testing.T) {
	billing, _ := newShippedCatalogBillingService(t)
	repo := &longContextGroupRepo{groups: map[int64]*Group{
		1: {ID: 1, LongContextPricingEnabled: true},
		2: {ID: 2, LongContextPricingEnabled: false},
	}}
	resolver := NewModelPricingResolver(nil, billing, repo)

	cost := func(groupID *int64, input int) float64 {
		resolved := resolver.Resolve(context.Background(), PricingInput{Model: "gpt-5.4", GroupID: groupID})
		c, err := billing.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "gpt-5.4", GroupID: groupID,
			Tokens:         UsageTokens{InputTokens: input, OutputTokens: 1000},
			RateMultiplier: 1, Resolver: resolver, Resolved: resolved,
		})
		require.NoError(t, err)
		return c.ActualCost
	}
	on, off := int64(1), int64(2)
	require.InDelta(t, 1.5225, cost(&on, 300000), 1e-9, "开启")
	require.InDelta(t, 0.765, cost(&off, 300000), 1e-9, "关闭")
	require.InDelta(t, 0.515, cost(&on, 200000), 1e-9, "未过阈值")
	require.InDelta(t, 1.5225, cost(nil, 300000), 1e-9, "无分组")

	// 不经 resolver 的路径（CalculateCostForGroup）口径一致。
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000}
	c, err := billing.CalculateCostForGroup("gpt-5.4", tokens, 1, "", repo.groups[1])
	require.NoError(t, err)
	require.InDelta(t, 1.5225, c.ActualCost, 1e-9)
	c, err = billing.CalculateCostForGroup("gpt-5.4", tokens, 1, "", repo.groups[2])
	require.NoError(t, err)
	require.InDelta(t, 0.765, c.ActualCost, 1e-9)
	c, err = billing.CalculateCostForGroup("gpt-5.4", tokens, 1, "", nil)
	require.NoError(t, err)
	require.InDelta(t, 1.5225, c.ActualCost, 1e-9)
}

// AC37：Gemini 2.5 Pro 和 Claude Sonnet 4.5 整请求按目录高档价。
func TestCatalogLongContextTierWholeRequest(t *testing.T) {
	billing, _ := newShippedCatalogBillingService(t)
	enabled := &Group{ID: 1, LongContextPricingEnabled: true}
	disabled := &Group{ID: 2, LongContextPricingEnabled: false}
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000}

	c, err := billing.CalculateCostForGroup("gemini-2.5-pro", tokens, 1, "", enabled)
	require.NoError(t, err)
	require.InDelta(t, 0.765, c.ActualCost, 1e-9, "原生/兼容接口开启（修复前原生 0.51、兼容 0.385）")

	c, err = billing.CalculateCostForGroup("gemini-2.5-pro", tokens, 1, "", disabled)
	require.NoError(t, err)
	require.InDelta(t, 0.385, c.ActualCost, 1e-9, "分组关闭")

	c, err = billing.CalculateCostForGroup("claude-sonnet-4-5", tokens, 1, "", enabled)
	require.NoError(t, err)
	require.InDelta(t, 1.8225, c.ActualCost, 1e-9, "sonnet-4-5（修复前 0.915）")
}

// 渠道配置定价区间时完全按渠道区间，不叠加目录阶梯。
func TestChannelIntervalsDoNotStackCatalogLadder(t *testing.T) {
	billing, _ := newShippedCatalogBillingService(t)
	resolver := NewModelPricingResolver(nil, billing)
	low, high := 1e-6, 2e-6
	out := 1e-5
	resolved := &ResolvedPricing{
		Mode:        BillingModeToken,
		BasePricing: &ModelPricing{InputPricePerToken: 2.5e-6, OutputPricePerToken: 1.5e-5, LongContextInputThreshold: 272000, LongContextInputMultiplier: 2, LongContextOutputMultiplier: 1.5},
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: intPtrForLongContextTest(272000), InputPrice: &low, OutputPrice: &out},
			{MinTokens: 272000, InputPrice: &high, OutputPrice: &out},
		},
		Source: PricingSourceChannel,
	}
	c, err := billing.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-5.4",
		Tokens:         UsageTokens{InputTokens: 300000, OutputTokens: 1000},
		RateMultiplier: 1, Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	require.InDelta(t, 300000*2e-6+1000*1e-5, c.ActualCost, 1e-9)
}

func intPtrForLongContextTest(v int) *int { return &v }

// 目录重载后原有阶梯丢失打 WARN。
func TestWarnDroppedLongContextLadders(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()
	old := map[string]*LiteLLMModelPricing{"gpt-5.4": {LongContextInputTokenThreshold: 272000}}
	next := map[string]*LiteLLMModelPricing{"gpt-5.4": {}}
	warnDroppedLongContextLadders(old, next)
	require.True(t, logSink.ContainsMessageAtLevel("long-context ladder dropped for 1 model(s) after reload: gpt-5.4", "warn"))
}

// override 显式 threshold=0 关闭目录阶梯（上游 593fc9365 用例，依赖 T11 的 above 折算）。
func TestPricingOverride_ExplicitZeroThresholdDisablesCatalogLadder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"gpt-5.4": {"long_context_input_token_threshold": 0}}`), 0644))
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.OverrideFile = path
	data, err := svc.parsePricingData([]byte(openAILadderCatalogJSON))
	require.NoError(t, err)
	require.Zero(t, data["gpt-5.4"].LongContextInputTokenThreshold)
	require.Equal(t, 272000, data["gpt-5.6-sol"].LongContextInputTokenThreshold, "未覆盖的模型保持目录阶梯")

	svc.pricingData = data
	billing := NewBillingService(&config.Config{}, svc)
	cost, err := billing.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 300000, OutputTokens: 1000}, 1)
	require.NoError(t, err)
	require.InDelta(t, 0.765, cost.ActualCost, 1e-9)
}

// 认证缓存快照携带分组长上下文开关，否则缓存命中时门控误判为关闭。
func TestAPIKeyAuthSnapshotCarriesLongContextPricingEnabled(t *testing.T) {
	svc := &APIKeyService{}
	key := &APIKey{ID: 1, UserID: 2, Status: StatusActive, User: &User{ID: 2, Status: StatusActive},
		Group: &Group{ID: 3, Status: StatusActive, LongContextPricingEnabled: true}}
	snapshot := svc.snapshotFromAPIKey(context.Background(), key)
	require.NotNil(t, snapshot.Group)
	require.True(t, snapshot.Group.LongContextPricingEnabled)
	restored := svc.snapshotToAPIKey("k", snapshot)
	require.True(t, restored.Group.LongContextPricingEnabled)
}
