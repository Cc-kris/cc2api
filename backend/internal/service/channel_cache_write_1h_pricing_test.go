package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

// V03-T12：渠道 1h 缓存写入价（上游 4a2b10c94、062af81fb、34b8bf1a6）。

func cacheWrite1hTestResolver(t *testing.T, pricing ChannelModelPricing) (*BillingService, *ModelPricingResolver) {
	t.Helper()
	billing := NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, `{
		"claude-sonnet-4-6": {"litellm_provider": "anthropic", "mode": "chat",
			"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05,
			"cache_creation_input_token_cost": 3.75e-06, "cache_creation_input_token_cost_above_1hr": 6e-06,
			"cache_read_input_token_cost": 3e-07}
	}`))
	pricing.Platform = PlatformAnthropic
	pricing.Models = []string{"claude-sonnet-4-6"}
	if pricing.BillingMode == "" {
		pricing.BillingMode = BillingModeToken
	}
	ch := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{1}, ModelPricing: []ChannelModelPricing{pricing}}
	channels := NewChannelService(&cacheWrite1hChannelRepo{channel: ch}, nil, nil, nil)
	return billing, NewModelPricingResolver(channels, billing)
}

func cacheWrite1hCost(t *testing.T, billing *BillingService, resolver *ModelPricingResolver) float64 {
	t.Helper()
	gid := int64(1)
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-sonnet-4-6", GroupID: &gid})
	require.Equal(t, PricingSourceChannel, resolved.Source)
	cost, err := billing.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "claude-sonnet-4-6", GroupID: &gid,
		Tokens: UsageTokens{
			OutputTokens:          100,
			CacheCreationTokens:   20000,
			CacheCreation5mTokens: 10000,
			CacheCreation1hTokens: 10000,
		},
		RateMultiplier: 1, Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	return cost.ActualCost
}

func f64(v float64) *float64 { return &v }

// cacheWrite1hChannelRepo 只实现渠道缓存装载用到的两个方法，其余方法由嵌入的 nil 接口占位。
type cacheWrite1hChannelRepo struct {
	ChannelRepository
	channel Channel
}

func (r *cacheWrite1hChannelRepo) ListAll(context.Context) ([]Channel, error) {
	return []Channel{r.channel}, nil
}

func (r *cacheWrite1hChannelRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{1: PlatformAnthropic}, nil
}

// AC25：渠道显式 1h 价时 1h 写入按它计；未配置时沿用缓存写入价（与升级前一致）。
func TestChannelCacheWrite1hPrice_AC25(t *testing.T) {
	billing, resolver := cacheWrite1hTestResolver(t, ChannelModelPricing{
		OutputPrice: f64(1.5e-5), CacheWritePrice: f64(3.75e-6), CacheWrite1hPrice: f64(6e-6),
	})
	require.InDelta(t, 0.099, cacheWrite1hCost(t, billing, resolver), 1e-12)

	billing, resolver = cacheWrite1hTestResolver(t, ChannelModelPricing{
		OutputPrice: f64(1.5e-5), CacheWritePrice: f64(3.75e-6),
	})
	require.InDelta(t, 0.0765, cacheWrite1hCost(t, billing, resolver), 1e-12, "1h 价为空时与现在一致")
}

// 只配置 1h 价：5m 写入保持目录价，渠道显式价不被目录价覆盖。
func TestChannelCacheWrite1hPrice_OnlyOneHourConfigured(t *testing.T) {
	billing, resolver := cacheWrite1hTestResolver(t, ChannelModelPricing{
		OutputPrice: f64(1.5e-5), CacheWrite1hPrice: f64(8e-6),
	})
	require.InDelta(t, 10000*3.75e-6+10000*8e-6+100*1.5e-5, cacheWrite1hCost(t, billing, resolver), 1e-12)
}

// 区间价：区间 1h 价生效。
func TestChannelCacheWrite1hPrice_Interval(t *testing.T) {
	billing, resolver := cacheWrite1hTestResolver(t, ChannelModelPricing{
		Intervals: []PricingInterval{{
			MinTokens: 0, OutputPrice: f64(1.5e-5), CacheWritePrice: f64(3.75e-6), CacheWrite1hPrice: f64(6e-6),
		}},
	})
	require.InDelta(t, 0.099, cacheWrite1hCost(t, billing, resolver), 1e-12)
}

// GetModelPricingWithChannel（旧路径）口径一致。
func TestGetModelPricingWithChannel_CacheWrite1h(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	pricing, err := billing.GetModelPricingWithChannel("claude-sonnet-4", &ChannelModelPricing{
		CacheWritePrice: f64(3.75e-6), CacheWrite1hPrice: f64(6e-6),
	})
	require.NoError(t, err)
	require.InDelta(t, 3.75e-6, pricing.CacheCreation5mPrice, 1e-15)
	require.InDelta(t, 6e-6, pricing.CacheCreation1hPrice, 1e-15)
	require.True(t, pricing.SupportsCacheBreakdown)

	pricing, err = billing.GetModelPricingWithChannel("claude-sonnet-4", &ChannelModelPricing{CacheWritePrice: f64(3.75e-6)})
	require.NoError(t, err)
	require.InDelta(t, 3.75e-6, pricing.CacheCreation1hPrice, 1e-15, "未配置 1h 价时沿用缓存写入价")
}

// 账号统计自定义规则按 5m/1h 明细计价。
func TestAccountStatsCacheWrite1hPrice(t *testing.T) {
	tokens := UsageTokens{CacheCreationTokens: 20000, CacheCreation5mTokens: 10000, CacheCreation1hTokens: 10000}
	cost := calculateTokenStatsCost(&ChannelModelPricing{CacheWritePrice: f64(3.75e-6), CacheWrite1hPrice: f64(6e-6)}, tokens)
	require.NotNil(t, cost)
	require.InDelta(t, 0.0975, *cost, 1e-12)

	cost = calculateTokenStatsCost(&ChannelModelPricing{CacheWritePrice: f64(3.75e-6)}, tokens)
	require.NotNil(t, cost)
	require.InDelta(t, 0.075, *cost, 1e-12, "未配置 1h 价时按总数计")

	// 只有总数没有明细时全部按 5m 价计。
	cost = calculateTokenStatsCost(&ChannelModelPricing{CacheWritePrice: f64(3.75e-6), CacheWrite1hPrice: f64(6e-6)}, UsageTokens{CacheCreationTokens: 1000})
	require.NotNil(t, cost)
	require.InDelta(t, 1000*3.75e-6, *cost, 1e-12)
}

// apicompat 已解析 OpenAI cache_write_tokens（CCAI types.go，不重复移植 4a2b10c94）。
// 同一份 usage 里多处出现缓存写入字段时只计一次，不会把 token 计两次。
func TestResponsesUsageCacheWriteTokensNotDoubleCounted(t *testing.T) {
	var usage apicompat.ResponsesUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"input_tokens": 1000, "output_tokens": 10,
		"cache_write_tokens": 300, "cache_creation_tokens": 300,
		"input_tokens_details": {"cached_tokens": 200, "cache_write_tokens": 300}
	}`), &usage))
	require.Equal(t, 300, usage.CacheCreationInputTokens)
	require.Equal(t, 1000, usage.InputTokens)

	chat := apicompat.ChatUsageToResponsesUsage(&apicompat.ChatUsage{
		PromptTokens: 1000, CompletionTokens: 10,
		PromptTokensDetails: &apicompat.ChatTokenDetails{CachedTokens: 200, CacheWriteTokens: 300, CacheCreationTokens: 300},
	})
	require.Equal(t, 300, chat.CacheCreationInputTokens)
}
