package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// V03-T18 / AC35：在途预占估算与实际计费一致性。
//
// 每个用例先用 RecordUsage（真实计费路径）算出实际费用，再用同一个 service 的
// EstimateInflightReservation 估算；只要实际 token 不超过估算上界（输入 ≤ body/4、
// 输出 ≤ max_tokens、张数 ≤ Units），就必须 estimate ≥ actual。
// 价格取自出厂目录 data/model_pricing.json（与 acceptance 用例约定一致）。

func inflightTestCfg(cfg *config.Config, maxInput int) {
	cfg.Billing.InflightReservation = config.InflightReservationConfig{
		Enabled: true, TTLSeconds: 900, DefaultMaxTokens: 8192, MaxOutputTokens: 128000, MaxInputTokens: maxInput,
	}
}

// bodyOfTokens 构造一个估算输入恰好为 tokens 的请求体长度（字节 / 4）。
func bodyOfTokens(tokens int) int { return tokens * inflightInputBytesPerTokenEstimate }

func requireEstimateCovers(t *testing.T, estimate float64, priced bool, actual float64, msg string) {
	t.Helper()
	require.True(t, priced, msg)
	require.GreaterOrEqual(t, estimate, QuantizeUsageBillingAmount(actual), msg)
}

func newConsistencyGateway(t *testing.T, maxInput int) (*GatewayService, *tierUsageLogRepo) {
	t.Helper()
	repo := &tierUsageLogRepo{}
	svc := newGatewayRecordUsageServiceForFastTest(repo)
	billing, _ := newShippedCatalogBillingService(t)
	svc.billingService = billing
	inflightTestCfg(svc.cfg, maxInput)
	return svc, repo
}

func gatewayActual(t *testing.T, svc *GatewayService, repo *tierUsageLogRepo, apiKey *APIKey, account *Account, result *ForwardResult) float64 {
	t.Helper()
	repo.lastLog = nil
	if result.RequestID == "" {
		result.RequestID = "consistency_" + result.Model
	}
	result.Duration = time.Second
	require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: result, APIKey: apiKey, User: &User{ID: 1}, Account: account,
	}))
	require.NotNil(t, repo.lastLog)
	return repo.lastLog.ActualCost
}

func openAIActual(t *testing.T, svc *OpenAIGatewayService, repo *openAIRecordUsageLogRepoStub, apiKey *APIKey, result *OpenAIForwardResult) float64 {
	t.Helper()
	repo.lastLog = nil
	if result.RequestID == "" {
		result.RequestID = "consistency_" + result.Model
	}
	result.Duration = time.Second
	require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: result, APIKey: apiKey, User: &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI},
	}))
	require.NotNil(t, repo.lastLog)
	return repo.lastLog.ActualCost
}

func anthropicGroupKey(groupID int64, rate float64) *APIKey {
	return &APIKey{ID: 1, User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: rate, LongContextPricingEnabled: true}}
}

// AC33：claude-sonnet-4-6，body 40000 字节（估算输入 10000），max_tokens=8192 → 0.15288000。
func TestInflightEstimateConsistency_AC33(t *testing.T) {
	svc, repo := newConsistencyGateway(t, 200000)
	apiKey := anthropicGroupKey(33, 1)
	direct := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-sonnet-4-6", BodyBytes: 40000, MaxTokens: 8192})
	require.True(t, priced)
	require.InDelta(t, 0.15288, est, 1e-12)

	actual := gatewayActual(t, svc, repo, apiKey, direct, &ForwardResult{Model: "claude-sonnet-4-6", Usage: ClaudeUsage{InputTokens: 10000, OutputTokens: 8192}})
	require.InDelta(t, 0.15288, actual, 1e-12)
	requireEstimateCovers(t, est, priced, actual, "AC33")
}

// AC35：claude-opus-5 Anthropic 直连 speed=fast，body 4000 字节、max_tokens=1000 → 0.06，实际 0.06。
func TestInflightEstimateConsistency_AC35_FastOpus5(t *testing.T) {
	svc, repo := newConsistencyGateway(t, 200000)
	apiKey := anthropicGroupKey(35, 1)
	direct := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-opus-5", BodyBytes: 4000, MaxTokens: 1000, ServiceTier: "fast"})
	require.True(t, priced)
	require.InDelta(t, 0.06, est, 1e-12)

	actual := gatewayActual(t, svc, repo, apiKey, direct, &ForwardResult{
		Model: "claude-opus-5", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 1000},
		ServiceTier: anthropicSpeedServiceTier(direct, "fast", "claude-opus-5"),
	})
	require.InDelta(t, 0.06, actual, 1e-12)
	requireEstimateCovers(t, est, priced, actual, "AC35")
}

// AC23：分组倍率 1.0 → 0.06；1.5 → 0.09。
func TestInflightEstimateConsistency_AC23(t *testing.T) {
	direct := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	for _, tc := range []struct {
		rate float64
		want float64
	}{{1.0, 0.06}, {1.5, 0.09}} {
		svc, repo := newConsistencyGateway(t, 200000)
		apiKey := anthropicGroupKey(23, tc.rate)
		est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-opus-5", BodyBytes: bodyOfTokens(1000), MaxTokens: 1000, ServiceTier: "fast"})
		actual := gatewayActual(t, svc, repo, apiKey, direct, &ForwardResult{
			Model: "claude-opus-5", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 1000},
			ServiceTier: anthropicSpeedServiceTier(direct, "fast", "claude-opus-5"),
		})
		require.InDelta(t, tc.want, actual, 1e-12)
		require.InDelta(t, tc.want, est, 1e-12)
		requireEstimateCovers(t, est, priced, actual, "AC23")
	}
}

// AC24：响应 standard、Bedrock、opus-4-7 实际 0.03；ultrafast 实际 0.06。估算按请求档 fast（上界）。
func TestInflightEstimateConsistency_AC24(t *testing.T) {
	direct := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	bedrock := &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeBedrock}
	svc, repo := newConsistencyGateway(t, 200000)
	apiKey := anthropicGroupKey(24, 1)
	estimate := func(model string) (float64, bool) {
		return svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: model, BodyBytes: bodyOfTokens(1000), MaxTokens: 1000, ServiceTier: "fast"})
	}
	usage := ClaudeUsage{InputTokens: 1000, OutputTokens: 1000}

	est, priced := estimate("claude-opus-5")
	actual := gatewayActual(t, svc, repo, apiKey, direct, &ForwardResult{Model: "claude-opus-5", Usage: usage, ServiceTier: anthropicSpeedServiceTier(direct, "fast", "claude-opus-5"), UpstreamResponseServiceTier: "standard"})
	require.InDelta(t, 0.03, actual, 1e-12)
	requireEstimateCovers(t, est, priced, actual, "AC24 response standard")

	actual = gatewayActual(t, svc, repo, apiKey, bedrock, &ForwardResult{Model: "claude-opus-5", Usage: usage, ServiceTier: anthropicSpeedServiceTier(bedrock, "fast", "claude-opus-5")})
	require.InDelta(t, 0.03, actual, 1e-12)
	requireEstimateCovers(t, est, priced, actual, "AC24 bedrock")

	actual = gatewayActual(t, svc, repo, apiKey, direct, &ForwardResult{Model: "claude-opus-5", Usage: usage, ServiceTier: anthropicSpeedServiceTier(direct, "fast", "claude-opus-5"), UpstreamResponseServiceTier: "ultrafast"})
	require.InDelta(t, 0.06, actual, 1e-12)
	requireEstimateCovers(t, est, priced, actual, "AC24 ultrafast")

	// opus-4-7 不支持 Fast：handler 不会声明 fast 档（anthropicSpeedServiceTier=nil），估算 0.03 = 实际。
	est47, priced47 := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-opus-4-7", BodyBytes: bodyOfTokens(1000), MaxTokens: 1000})
	actual = gatewayActual(t, svc, repo, apiKey, direct, &ForwardResult{Model: "claude-opus-4-7", Usage: usage, ServiceTier: anthropicSpeedServiceTier(direct, "fast", "claude-opus-4-7")})
	require.InDelta(t, 0.03, actual, 1e-12)
	require.InDelta(t, 0.03, est47, 1e-12)
	requireEstimateCovers(t, est47, priced47, actual, "AC24 opus-4-7")
}

// AC25：渠道 1h 缓存写入价。实际 20000 缓存写（5m 10000 + 1h 10000）+ 输出 100 = 0.099。
// 缓存写 token 也在请求体里，估算输入 ≥ 20000；max_tokens 缺省 8192。
func TestInflightEstimateConsistency_AC25_CacheWrite1h(t *testing.T) {
	for _, tc := range []struct {
		name   string
		price  ChannelModelPricing
		actual float64
	}{
		{"1h price", ChannelModelPricing{OutputPrice: f64(1.5e-5), CacheWritePrice: f64(3.75e-6), CacheWrite1hPrice: f64(6e-6)}, 0.099},
		{"1h price empty", ChannelModelPricing{OutputPrice: f64(1.5e-5), CacheWritePrice: f64(3.75e-6)}, 0.0765},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing, resolver := cacheWrite1hTestResolver(t, tc.price)
			repo := &tierUsageLogRepo{}
			svc := newGatewayRecordUsageServiceForFastTest(repo)
			svc.billingService = billing
			svc.resolver = resolver
			inflightTestCfg(svc.cfg, 200000)
			apiKey := anthropicGroupKey(1, 1)
			actual := gatewayActual(t, svc, repo, apiKey, &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}, &ForwardResult{
				Model: "claude-sonnet-4-6",
				Usage: ClaudeUsage{OutputTokens: 100, CacheCreationInputTokens: 20000, CacheCreation5mTokens: 10000, CacheCreation1hTokens: 10000},
			})
			require.InDelta(t, tc.actual, actual, 1e-12)
			est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-sonnet-4-6", BodyBytes: bodyOfTokens(20000)})
			requireEstimateCovers(t, est, priced, actual, "AC25")
		})
	}
}

// AC26：渠道图片输入价 1e-5 > 文本 5e-6。输入 1500（其中图片 1000）、输出 200 → 0.0185。
// 请求带图片时估算额外按「输入全部是图片」计一次。
func TestInflightEstimateConsistency_AC26_ImageInput(t *testing.T) {
	for _, tc := range []struct {
		name       string
		imagePrice *float64
		actual     float64
	}{{"image input price", f64(1e-5), 0.0185}, {"fallback to input price", nil, 0.0135}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(repo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			newImageInputChannelResolver(t, svc, 26, "gpt-5.5", tc.imagePrice)
			inflightTestCfg(svc.cfg, 200000)
			apiKey := &APIKey{ID: 1026, User: &User{ID: 2}, GroupID: i64p(26), Group: &Group{ID: 26, RateMultiplier: 1, LongContextPricingEnabled: true}}
			actual := openAIActual(t, svc, repo, apiKey, &OpenAIForwardResult{Model: "gpt-5.5", Usage: OpenAIUsage{InputTokens: 1500, ImageInputTokens: 1000, OutputTokens: 200}})
			require.InDelta(t, tc.actual, actual, 1e-12)
			est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "gpt-5.5", BodyBytes: bodyOfTokens(1500), MaxTokens: 200, HasImageInput: true})
			requireEstimateCovers(t, est, priced, actual, "AC26")
			if tc.imagePrice != nil {
				textOnly, _ := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "gpt-5.5", BodyBytes: bodyOfTokens(1500), MaxTokens: 200})
				require.Less(t, textOnly, actual, "without HasImageInput the estimate would undershoot (guards the image-input variant)")
			}
		})
	}
}

// AC27：分组 1K 图片价 0.04、按张开关。实际 3 张 0.12；解析不到张数 1 张 0.04；
// /v1/responses 混合请求按 token 0.011；关闭开关按 token 0.011 / 无渠道价按张 0.12。
func TestInflightEstimateConsistency_AC27_ImageCount(t *testing.T) {
	ctx := context.Background()
	t.Run("three images, switch on", func(t *testing.T) {
		svc, repo, groupID := newAC27Service(t, true)
		inflightTestCfg(svc.cfg, 200000)
		apiKey := ac27APIKey(groupID, true)
		apiKey.User = &User{ID: 2927}
		actual := recordAC27(t, svc, repo, apiKey, &OpenAIForwardResult{ImageEndpoint: true, ImageCount: 3, ImageSize: "1K",
			Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 4000, ImageOutputTokens: 4000}}).ActualCost
		require.InDelta(t, 0.12, actual, 1e-12)
		est, priced := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: ac27Model, BodyBytes: bodyOfTokens(1000), Kind: InflightEstimateImage, Units: 3, ImageEndpoint: true})
		requireEstimateCovers(t, est, priced, actual, "AC27 3 images")
	})
	t.Run("unparsable count bills one", func(t *testing.T) {
		svc, repo, groupID := newAC27Service(t, false)
		inflightTestCfg(svc.cfg, 200000)
		apiKey := ac27APIKey(groupID, true)
		apiKey.User = &User{ID: 2927}
		actual := recordAC27(t, svc, repo, apiKey, &OpenAIForwardResult{ImageEndpoint: true, ImageCount: 1, ImageSize: "1K"}).ActualCost
		require.InDelta(t, 0.04, actual, 1e-12)
		est, priced := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: ac27Model, Kind: InflightEstimateImage, Units: 3, ImageEndpoint: true})
		requireEstimateCovers(t, est, priced, actual, "AC27 unparsable")
	})
	t.Run("mixed responses request keeps token billing", func(t *testing.T) {
		svc, repo, groupID := newAC27Service(t, true)
		inflightTestCfg(svc.cfg, 200000)
		apiKey := ac27APIKey(groupID, true)
		apiKey.User = &User{ID: 2927}
		actual := recordAC27(t, svc, repo, apiKey, &OpenAIForwardResult{ImageCount: 1, ImageSize: "1K", Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 200}}).ActualCost
		require.InDelta(t, 0.011, actual, 1e-12)
		est, priced := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: ac27Model, BodyBytes: bodyOfTokens(1000), MaxTokens: 200, MayGenerateImage: true})
		requireEstimateCovers(t, est, priced, actual, "AC27 mixed")
	})
	t.Run("switch off", func(t *testing.T) {
		svc, repo, groupID := newAC27Service(t, true)
		inflightTestCfg(svc.cfg, 200000)
		apiKey := ac27APIKey(groupID, false)
		apiKey.User = &User{ID: 2927}
		actual := recordAC27(t, svc, repo, apiKey, &OpenAIForwardResult{ImageEndpoint: true, ImageCount: 3, ImageSize: "1K", Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 200}}).ActualCost
		require.InDelta(t, 0.011, actual, 1e-12)
		est, priced := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: ac27Model, BodyBytes: bodyOfTokens(1000), MaxTokens: 200, Kind: InflightEstimateImage, Units: 3, ImageEndpoint: true})
		requireEstimateCovers(t, est, priced, actual, "AC27 switch off token")

		svc, repo, groupID = newAC27Service(t, false)
		inflightTestCfg(svc.cfg, 200000)
		apiKey = ac27APIKey(groupID, false)
		apiKey.User = &User{ID: 2927}
		actual = recordAC27(t, svc, repo, apiKey, &OpenAIForwardResult{ImageEndpoint: true, ImageCount: 3, ImageSize: "1K"}).ActualCost
		require.InDelta(t, 0.12, actual, 1e-12)
		est, priced = svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: ac27Model, Kind: InflightEstimateImage, Units: 3, ImageEndpoint: true})
		requireEstimateCovers(t, est, priced, actual, "AC27 switch off per image")
	})
}

// AC28：gemini-3-pro-image-preview，无分组图片价，3 张 → 0.402。估算按 Units 张数上界。
func TestInflightEstimateConsistency_AC28_GeminiImages(t *testing.T) {
	svc, repo := newConsistencyGateway(t, 200000)
	groupID := int64(28)
	apiKey := &APIKey{ID: 1, User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1, AllowImageGeneration: true}}
	actual := gatewayActual(t, svc, repo, apiKey, geminiImageTestAccount(), &ForwardResult{Model: "gemini-3-pro-image-preview", ImageCount: 3, ImageSize: "1K"})
	require.InDelta(t, 0.402, actual, 1e-9)
	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "gemini-3-pro-image-preview", BodyBytes: 200, MayGenerateImage: true, Units: 3})
	requireEstimateCovers(t, est, priced, actual, "AC28")
	// 4K 档 0.268/张 × 3 = 0.804（目录没有分档价时 4K 为基础价 2 倍）。
	require.InDelta(t, 0.804, est, 1e-9)
}

// AC29：gpt-5.4 长上下文阶梯只看分组开关。输入 300000 需要 max_input_tokens ≥ 300000 才在估算上界内。
func TestInflightEstimateConsistency_AC29_LongContextGate(t *testing.T) {
	groups := &longContextGroupRepo{groups: map[int64]*Group{
		1: {ID: 1, LongContextPricingEnabled: true},
		2: {ID: 2, LongContextPricingEnabled: false},
	}}
	for _, tc := range []struct {
		name   string
		key    *APIKey
		input  int
		actual float64
	}{
		{"enabled", &APIKey{ID: 1, User: &User{ID: 2}, GroupID: i64p(1), Group: &Group{ID: 1, RateMultiplier: 1, LongContextPricingEnabled: true}}, 300000, 1.5225},
		{"disabled", &APIKey{ID: 1, User: &User{ID: 2}, GroupID: i64p(2), Group: &Group{ID: 2, RateMultiplier: 1, LongContextPricingEnabled: false}}, 300000, 0.765},
		{"under threshold", &APIKey{ID: 1, User: &User{ID: 2}, GroupID: i64p(1), Group: &Group{ID: 1, RateMultiplier: 1, LongContextPricingEnabled: true}}, 200000, 0.515},
		{"no group", &APIKey{ID: 1, User: &User{ID: 2}}, 300000, 1.5225 * 1.1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(repo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			billing, _ := newShippedCatalogBillingService(t)
			svc.billingService = billing
			svc.resolver = NewModelPricingResolver(nil, billing, groups)
			inflightTestCfg(svc.cfg, 400000)
			actual := openAIActual(t, svc, repo, tc.key, &OpenAIForwardResult{Model: "gpt-5.4", Usage: OpenAIUsage{InputTokens: tc.input, OutputTokens: 1000}})
			require.InDelta(t, tc.actual, actual, 1e-9)
			est, priced := svc.EstimateInflightReservation(context.Background(), tc.key, InflightEstimateRequest{Model: "gpt-5.4", BodyBytes: bodyOfTokens(tc.input), MaxTokens: 1000})
			requireEstimateCovers(t, est, priced, actual, "AC29 "+tc.name)
		})
	}
}

// AC37：Gemini 2.5 Pro / Claude Sonnet 4.5 整请求目录高档价（输入 300000、输出 1000）。
func TestInflightEstimateConsistency_AC37_CatalogLongContext(t *testing.T) {
	for _, tc := range []struct {
		name     string
		model    string
		platform string
		enabled  bool
		actual   float64
	}{
		{"gemini enabled", "gemini-2.5-pro", PlatformGemini, true, 0.765},
		{"gemini disabled", "gemini-2.5-pro", PlatformGemini, false, 0.385},
		{"sonnet-4-5 enabled", "claude-sonnet-4-5", PlatformAnthropic, true, 1.8225},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newConsistencyGateway(t, 400000)
			groupID := int64(37)
			apiKey := &APIKey{ID: 1, User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: tc.platform, RateMultiplier: 1, LongContextPricingEnabled: tc.enabled}}
			actual := gatewayActual(t, svc, repo, apiKey, &Account{ID: 1, Platform: tc.platform, Type: AccountTypeAPIKey}, &ForwardResult{Model: tc.model, Usage: ClaudeUsage{InputTokens: 300000, OutputTokens: 1000}})
			require.InDelta(t, tc.actual, actual, 1e-9)
			est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: tc.model, BodyBytes: bodyOfTokens(300000), MaxTokens: 1000})
			requireEstimateCovers(t, est, priced, actual, "AC37 "+tc.name)
		})
	}
}

// 已知会低估的情况（不在「实际 token 不超过估算上界」前提内，记录现状，防止误以为已覆盖）。
func TestInflightEstimateConsistency_KnownUnderestimates(t *testing.T) {
	ctx := context.Background()

	// 1. 输入超过 max_input_tokens（默认 200000）：估算输入被封顶，进不了 272k 阶梯。
	groups := &longContextGroupRepo{groups: map[int64]*Group{1: {ID: 1, LongContextPricingEnabled: true}}}
	repo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(repo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	billing, _ := newShippedCatalogBillingService(t)
	svc.billingService = billing
	svc.resolver = NewModelPricingResolver(nil, billing, groups)
	inflightTestCfg(svc.cfg, 200000)
	key := &APIKey{ID: 1, User: &User{ID: 2}, GroupID: i64p(1), Group: &Group{ID: 1, RateMultiplier: 1, LongContextPricingEnabled: true}}
	est, _ := svc.EstimateInflightReservation(ctx, key, InflightEstimateRequest{Model: "gpt-5.4", BodyBytes: bodyOfTokens(300000), MaxTokens: 1000})
	require.InDelta(t, 200000*2.5e-6+1000*1.5e-5, est, 1e-9)
	require.Less(t, est, 1.5225, "input capped at 200000 → long-context tier not reflected")

	// 2. 缓存写入溢价：输入按普通输入价估算，max_tokens 很小而缓存写入很多时低于实际。
	billing2, resolver := cacheWrite1hTestResolver(t, ChannelModelPricing{OutputPrice: f64(1.5e-5), CacheWritePrice: f64(3.75e-6), CacheWrite1hPrice: f64(6e-6)})
	gw := newGatewayRecordUsageServiceForFastTest(&tierUsageLogRepo{})
	gw.billingService = billing2
	gw.resolver = resolver
	inflightTestCfg(gw.cfg, 200000)
	est, _ = gw.EstimateInflightReservation(ctx, anthropicGroupKey(1, 1), InflightEstimateRequest{Model: "claude-sonnet-4-6", BodyBytes: bodyOfTokens(20000), MaxTokens: 100})
	require.Less(t, est, 0.099, "cache write 1h premium is not reflected when max_tokens is tight")

	// 3. Gemini 原生图片：一个候选可能返回多张图，准入时只知道 candidateCount。
	gs, _ := newConsistencyGateway(t, 200000)
	gid := int64(28)
	gkey := &APIKey{ID: 1, User: &User{ID: 1}, GroupID: &gid, Group: &Group{ID: gid, Platform: PlatformGemini, RateMultiplier: 1, AllowImageGeneration: true}}
	est, _ = gs.EstimateInflightReservation(ctx, gkey, InflightEstimateRequest{Model: "gemini-3-pro-image-preview", BodyBytes: 200, MayGenerateImage: true, Units: 1})
	require.Less(t, est, 0.402, "one candidate returning three images exceeds a single-image estimate")
}
