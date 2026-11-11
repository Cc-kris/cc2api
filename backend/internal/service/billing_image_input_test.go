package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// V03-T14：图片输入 token 独立计费（移植上游 06e03f467、62d57c02d、b6a157c19 输入图片部分）。

func newImageInputChannelResolver(t *testing.T, svc *OpenAIGatewayService, groupID int64, model string, imageInputPrice *float64) {
	t.Helper()
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}] = &ChannelModelPricing{
		BillingMode:     BillingModeToken,
		InputPrice:      f64(5e-6),
		OutputPrice:     f64(3e-5),
		ImageInputPrice: imageInputPrice,
	}
	cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
	cache.groupPlatform[groupID] = ""
	cache.loadedAt = time.Now()
	channelService := &ChannelService{}
	channelService.cache.Store(cache)
	svc.resolver = NewModelPricingResolver(channelService, svc.billingService)
}

func recordImageInputUsage(t *testing.T, imageInputPrice *float64, usage OpenAIUsage) *UsageLog {
	t.Helper()
	groupID := int64(26)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	newImageInputChannelResolver(t, svc, groupID, "gpt-5.5", imageInputPrice)
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_image_input_ac26",
			Model:     "gpt-5.5",
			Usage:     usage,
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1026,
			GroupID: i64p(groupID),
			Group:   &Group{ID: groupID, RateMultiplier: 1, LongContextPricingEnabled: true},
		},
		User:    &User{ID: 2026},
		Account: &Account{ID: 3026, Platform: PlatformOpenAI},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	return usageRepo.lastLog
}

// AC26：input 1500（其中图片 1000）、output 200；文本 5e-6、图片 1e-5、输出 3e-5。
func TestImageInputBilling_AC26(t *testing.T) {
	log := recordImageInputUsage(t, f64(1e-5), OpenAIUsage{InputTokens: 1500, ImageInputTokens: 1000, OutputTokens: 200})
	require.Equal(t, 1500, log.InputTokens, "input_tokens 仍是总输入，图片是其子集")
	require.Equal(t, 1000, log.ImageInputTokens)
	require.InDelta(t, 0.0025, log.InputCost, 1e-12)
	require.InDelta(t, 0.01, log.ImageInputCost, 1e-12)
	require.InDelta(t, 0.006, log.OutputCost, 1e-12)
	require.InDelta(t, 0.0185, log.TotalCost, 1e-12)
	require.InDelta(t, 0.0185, log.ActualCost, 1e-12)
}

// AC26 回退：未配置图片输入价时图片 token 按文本输入价计，总额与旧口径一致 0.0135。
func TestImageInputBilling_AC26_FallbackToInputPrice(t *testing.T) {
	log := recordImageInputUsage(t, nil, OpenAIUsage{InputTokens: 1500, ImageInputTokens: 1000, OutputTokens: 200})
	require.Equal(t, 1000, log.ImageInputTokens)
	require.InDelta(t, 0.0025, log.InputCost, 1e-12)
	require.InDelta(t, 0.005, log.ImageInputCost, 1e-12)
	require.InDelta(t, 0.0135, log.TotalCost, 1e-12)
}

// 没有图片输入时行为与改动前完全一致。
func TestImageInputBilling_TextOnlyUnchanged(t *testing.T) {
	log := recordImageInputUsage(t, f64(1e-5), OpenAIUsage{InputTokens: 1500, OutputTokens: 200})
	require.Zero(t, log.ImageInputTokens)
	require.Zero(t, log.ImageInputCost)
	require.InDelta(t, 0.0075, log.InputCost, 1e-12)
	require.InDelta(t, 0.0135, log.TotalCost, 1e-12)
}

// 图片 token 不会超过扣除缓存后的实际输入：input 1500、cached 1200、image 1000 → 实际输入 300，图片截到 300。
func TestImageInputBilling_ClampedToActualInput(t *testing.T) {
	log := recordImageInputUsage(t, f64(1e-5), OpenAIUsage{InputTokens: 1500, CacheReadInputTokens: 1200, ImageInputTokens: 1000, OutputTokens: 0})
	require.Equal(t, 300, log.InputTokens)
	require.Equal(t, 300, log.ImageInputTokens)
	require.InDelta(t, 0.0, log.InputCost, 1e-12)
	require.InDelta(t, 0.003, log.ImageInputCost, 1e-12)
}

func TestComputeTokenBreakdown_ImageInputSplit(t *testing.T) {
	billing := NewBillingService(nil, nil)
	pricing := &ModelPricing{InputPricePerToken: 5e-6, OutputPricePerToken: 3e-5, ImageInputPricePerToken: 1e-5}
	tokens := UsageTokens{InputTokens: 1500, ImageInputTokens: 1000, OutputTokens: 200}
	cost := billing.computeTokenBreakdown(pricing, tokens, 1, "", false)
	require.InDelta(t, 0.0025, cost.InputCost, 1e-12)
	require.InDelta(t, 0.01, cost.ImageInputCost, 1e-12)
	require.InDelta(t, 0.0185, cost.TotalCost, 1e-12)

	// priority 档整体 2x，图片输入同样乘档位倍率。
	cost = billing.computeTokenBreakdown(pricing, tokens, 1, "priority", false)
	require.InDelta(t, 0.02, cost.ImageInputCost, 1e-12)
	require.InDelta(t, 0.037, cost.TotalCost, 1e-12)
}

func TestOpenAIUsageParsesImageInputTokens(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"usage":{"input_tokens":1500,"output_tokens":200,"input_tokens_details":{"cached_tokens":0,"image_tokens":1000}}}`))
	require.True(t, ok)
	require.Equal(t, 1000, usage.ImageInputTokens)

	usage, ok = extractOpenAIUsageFromJSONBytes([]byte(`{"usage":{"prompt_tokens":800,"completion_tokens":20,"prompt_tokens_details":{"image_tokens":512}}}`))
	require.True(t, ok)
	require.Equal(t, 512, usage.ImageInputTokens, "chat completions 形态")

	usage, ok = extractOpenAIUsageFromJSONBytes([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":1}}}`))
	require.True(t, ok)
	require.Zero(t, usage.ImageInputTokens)
}

// 区间定价不带图片输入价时沿用渠道级 image_input_price（上游 06e03f467 intervalToModelPricing）。
func TestImageInputBilling_IntervalPricingUsesChannelImagePrice(t *testing.T) {
	groupID := int64(27)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: "gpt-5.5"}] = &ChannelModelPricing{
		BillingMode:     BillingModeToken,
		ImageInputPrice: f64(1e-5),
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: nil, InputPrice: f64(5e-6), OutputPrice: f64(3e-5)},
		},
	}
	cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
	cache.groupPlatform[groupID] = ""
	cache.loadedAt = time.Now()
	channelService := &ChannelService{}
	channelService.cache.Store(cache)
	svc.resolver = NewModelPricingResolver(channelService, svc.billingService)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_image_input_interval",
			Model:     "gpt-5.5",
			Usage:     OpenAIUsage{InputTokens: 1500, ImageInputTokens: 1000, OutputTokens: 200},
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 1027, GroupID: i64p(groupID), Group: &Group{ID: groupID, RateMultiplier: 1, LongContextPricingEnabled: true}},
		User:    &User{ID: 2027},
		Account: &Account{ID: 3027, Platform: PlatformOpenAI},
	})
	require.NoError(t, err)
	log := usageRepo.lastLog
	require.NotNil(t, log)
	require.InDelta(t, 0.0025, log.InputCost, 1e-12)
	require.InDelta(t, 0.01, log.ImageInputCost, 1e-12)
	require.InDelta(t, 0.0185, log.TotalCost, 1e-12)
}
