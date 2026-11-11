package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// V03-T13：Anthropic Fast 识别与只降不升（移植上游 7dae055f2、75faedda9 及 3a9070359/50ad6e2e5
// 的 Codex OAuth 例外）。

func TestResolveBillingServiceTier(t *testing.T) {
	tests := []struct {
		name       string
		requested  string
		observed   string
		billing    string
		downgraded bool
	}{
		{name: "openai priority served as default", requested: "priority", observed: "default", billing: "default", downgraded: true},
		{name: "anthropic fast served as standard", requested: "fast", observed: "standard", billing: "standard", downgraded: true},
		{name: "priority honoured", requested: "priority", observed: "priority", billing: "priority"},
		{name: "no declaration keeps request", requested: "priority", observed: "", billing: "priority"},
		{name: "no request no declaration", requested: "", observed: "", billing: ""},
		{name: "response never raises the tier", requested: "", observed: "priority", billing: ""},
		{name: "flex never raised to default", requested: "flex", observed: "default", billing: "flex"},
		{name: "default echoed for untiered request", requested: "", observed: "default", billing: ""},
		{name: "unknown response tier ignored", requested: "priority", observed: "turbo", billing: "priority"},
		{name: "ultrafast response tier ignored", requested: "fast", observed: "ultrafast", billing: "fast"},
		{name: "case and whitespace normalised", requested: " Priority ", observed: "DEFAULT", billing: "default", downgraded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBillingServiceTier(tt.requested, tt.observed)
			require.Equal(t, tt.billing, got.Billing)
			require.Equal(t, tt.downgraded, got.Downgraded)
		})
	}
}

func TestApplyServiceTierBillingResolutionOnlyRewritesDowngrades(t *testing.T) {
	apiKeyAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	t.Run("openai downgrade rewrites tier", func(t *testing.T) {
		requested := "priority"
		result := &OpenAIForwardResult{ServiceTier: &requested, UpstreamResponseServiceTier: "default"}
		resolution := ApplyOpenAIServiceTierBillingResolution(apiKeyAccount, result)
		require.True(t, resolution.Downgraded)
		require.Equal(t, "default", *result.ServiceTier)
	})
	t.Run("openai honoured tier keeps pointer", func(t *testing.T) {
		requested := "priority"
		result := &OpenAIForwardResult{ServiceTier: &requested, UpstreamResponseServiceTier: "priority"}
		require.False(t, ApplyOpenAIServiceTierBillingResolution(apiKeyAccount, result).Downgraded)
		require.Same(t, &requested, result.ServiceTier)
	})
	t.Run("openai untiered request stays nil", func(t *testing.T) {
		result := &OpenAIForwardResult{UpstreamResponseServiceTier: "priority"}
		require.False(t, ApplyOpenAIServiceTierBillingResolution(apiKeyAccount, result).Downgraded)
		require.Nil(t, result.ServiceTier)
	})
	t.Run("codex oauth default response is not authoritative", func(t *testing.T) {
		requested := "priority"
		result := &OpenAIForwardResult{ServiceTier: &requested, UpstreamResponseServiceTier: "default"}
		oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		require.False(t, ApplyOpenAIServiceTierBillingResolution(oauth, result).Downgraded)
		require.Equal(t, "priority", *result.ServiceTier)
	})
	t.Run("anthropic standard speed rewrites fast", func(t *testing.T) {
		requested := "fast"
		result := &ForwardResult{ServiceTier: &requested, UpstreamResponseServiceTier: "standard"}
		require.True(t, ApplyForwardServiceTierBillingResolution(result).Downgraded)
		require.Equal(t, "standard", *result.ServiceTier)
	})
	t.Run("nil results are ignored", func(t *testing.T) {
		require.False(t, ApplyOpenAIServiceTierBillingResolution(nil, nil).Downgraded)
		require.False(t, ApplyForwardServiceTierBillingResolution(nil).Downgraded)
	})
}

func TestAnthropicSpeedServiceTier(t *testing.T) {
	direct := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	tier := anthropicSpeedServiceTier(direct, "fast", "claude-opus-5")
	require.NotNil(t, tier)
	require.Equal(t, "fast", *tier)
	require.NotNil(t, anthropicSpeedServiceTier(direct, "fast", "claude-opus-4-8-20260101"))
	require.NotNil(t, anthropicSpeedServiceTier(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}, "fast", "claude-opus-5"))
}

func TestAnthropicSpeedServiceTierRejectsUnsupportedTargets(t *testing.T) {
	direct := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	require.Nil(t, anthropicSpeedServiceTier(direct, "", "claude-opus-5"), "未请求 fast")
	require.Nil(t, anthropicSpeedServiceTier(direct, "standard", "claude-opus-5"))
	require.Nil(t, anthropicSpeedServiceTier(direct, "fast", "claude-opus-4-7"), "Opus 4.7 无 fast mode")
	require.Nil(t, anthropicSpeedServiceTier(direct, "fast", "claude-opus-4-5"), "opus-4-5 不能被 5 误判")
	require.Nil(t, anthropicSpeedServiceTier(direct, "fast", "claude-sonnet-4-6"))
	require.Nil(t, anthropicSpeedServiceTier(&Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock}, "fast", "claude-opus-5"), "Bedrock 不支持")
	require.Nil(t, anthropicSpeedServiceTier(&Account{Platform: PlatformOpenAI}, "fast", "claude-opus-5"), "非 Anthropic 平台")
	require.Nil(t, anthropicSpeedServiceTier(nil, "fast", "claude-opus-5"))
}

func TestAnthropicSpeedModelPrefersMappedUpstreamModel(t *testing.T) {
	parsed := &ParsedRequest{Model: "claude-opus-4-7"}
	require.Equal(t, "claude-opus-5", anthropicSpeedModel(parsed, &ForwardResult{UpstreamModel: "claude-opus-5"}))
	require.Equal(t, "claude-opus-4-7", anthropicSpeedModel(parsed, &ForwardResult{}))
}

func TestParseGatewayRequestSpeedAnthropicOnly(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5","speed":" Fast ","messages":[]}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformAnthropic)
	require.NoError(t, err)
	require.Equal(t, "fast", parsed.Speed)

	parsed, err = ParseGatewayRequest(body, "chat_completions")
	require.NoError(t, err)
	require.Empty(t, parsed.Speed, "非 Anthropic 协议不解析 speed")
}

func TestObserveAnthropicUsageSpeed(t *testing.T) {
	usage := parseClaudeUsageFromResponseBody([]byte(`{"usage":{"input_tokens":1,"output_tokens":2,"speed":"standard"}}`))
	require.Equal(t, "standard", usage.Speed)

	svc := &GatewayService{}
	streamUsage := &ClaudeUsage{}
	svc.parseSSEUsage(`{"type":"message_start","message":{"model":"claude-opus-5","usage":{"input_tokens":5,"speed":"fast"}}}`, streamUsage)
	require.Equal(t, "fast", streamUsage.Speed)
	svc.parseSSEUsage(`{"type":"message_delta","usage":{"output_tokens":9}}`, streamUsage)
	require.Equal(t, "fast", streamUsage.Speed, "后续 delta 不清空")

	passthrough := &ClaudeUsage{}
	svc.parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":{"input_tokens":5,"speed":"standard"}}}`, passthrough)
	require.Equal(t, "standard", passthrough.Speed)

	require.Empty(t, parseClaudeUsageFromResponseBody([]byte(`{"usage":{"speed":"ludicrous"}}`)).Speed, "未知档位忽略")
}

func TestObserveOpenAIResponseServiceTier(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"type":"response.completed","response":{"service_tier":"default","usage":{"input_tokens":10,"output_tokens":5}}}`))
	require.True(t, ok)
	require.Equal(t, "default", usage.ResponseServiceTier)

	usage, ok = extractOpenAIUsageFromJSONBytes([]byte(`{"service_tier":"auto","usage":{"input_tokens":10}}`))
	require.True(t, ok)
	require.Empty(t, usage.ResponseServiceTier, "auto 不是处理档位")
}

func TestCatalogFastMultiplierParsedFromProviderSpecificEntry(t *testing.T) {
	billing, _ := newShippedCatalogBillingService(t)
	opus5, err := billing.GetModelPricing("claude-opus-5")
	require.NoError(t, err)
	require.InDelta(t, 2.0, opus5.FastCostMultiplier, 1e-12)
	fable, err := billing.GetModelPricing("claude-fable-5")
	require.NoError(t, err)
	require.InDelta(t, 6.0, fable.FastCostMultiplier, 1e-12)
}

// ---------------- AC23 / AC24 through RecordUsage ----------------

type tierUsageLogRepo struct {
	UsageLogRepository
	lastLog *UsageLog
}

func (r *tierUsageLogRepo) Create(_ context.Context, log *UsageLog) (bool, error) {
	r.lastLog = log
	return true, nil
}

func (r *tierUsageLogRepo) CreateBestEffort(_ context.Context, log *UsageLog) error {
	r.lastLog = log
	return nil
}

func newFastTierGatewayService(t *testing.T, groupRate float64) (*GatewayService, *tierUsageLogRepo, *APIKey) {
	t.Helper()
	repo := &tierUsageLogRepo{}
	svc := newGatewayRecordUsageServiceForFastTest(repo)
	billing, _ := newShippedCatalogBillingService(t)
	svc.billingService = billing
	groupID := int64(7)
	return svc, repo, &APIKey{ID: 1, GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: groupRate, LongContextPricingEnabled: true}}
}

// 简单模式只写用量记录、不扣余额，便于直接断言计费金额。
func newGatewayRecordUsageServiceForFastTest(usageRepo UsageLogRepository) *GatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.RunMode = config.RunModeSimple
	svc := &GatewayService{cfg: cfg, usageLogRepo: usageRepo, deferredService: &DeferredService{}}
	return svc
}

func recordFastUsage(t *testing.T, svc *GatewayService, apiKey *APIKey, account *Account, model string, requested *string, observed string) *UsageLog {
	t.Helper()
	repo, ok := svc.usageLogRepo.(*tierUsageLogRepo)
	require.True(t, ok, "usage log repo should be *tierUsageLogRepo")
	repo.lastLog = nil
	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:                   "fast_" + model + "_" + observed,
			Usage:                       ClaudeUsage{InputTokens: 1000, OutputTokens: 1000},
			Model:                       model,
			Duration:                    time.Second,
			ServiceTier:                 requested,
			UpstreamResponseServiceTier: observed,
		},
		APIKey:  apiKey,
		User:    &User{ID: 1},
		Account: account,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.lastLog)
	return repo.lastLog
}

func requestedTierFor(account *Account, speed, model string) *string {
	return anthropicSpeedServiceTier(account, speed, model)
}

// AC23：claude-opus-5 speed=fast，响应不声明 speed → fast，(1000×5e-6 + 1000×2.5e-5)×2 = 0.06；分组倍率 1.5 → 0.09。
func TestFastSpeedBilling_AC23(t *testing.T) {
	direct := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	svc, _, apiKey := newFastTierGatewayService(t, 1.0)
	log := recordFastUsage(t, svc, apiKey, direct, "claude-opus-5", requestedTierFor(direct, "fast", "claude-opus-5"), "")
	require.NotNil(t, log.ServiceTier)
	require.Equal(t, "fast", *log.ServiceTier)
	require.InDelta(t, 0.06, log.ActualCost, 1e-12)

	svc, _, apiKey = newFastTierGatewayService(t, 1.5)
	log = recordFastUsage(t, svc, apiKey, direct, "claude-opus-5", requestedTierFor(direct, "fast", "claude-opus-5"), "")
	require.InDelta(t, 0.09, log.ActualCost, 1e-12)
}

// AC24：usage.speed=standard、Bedrock、claude-opus-4-7 都按标准价 0.03；声明 ultrafast 仍按请求档 0.06。
func TestFastSpeedBilling_AC24(t *testing.T) {
	direct := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	bedrock := &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeBedrock}
	svc, _, apiKey := newFastTierGatewayService(t, 1.0)

	log := recordFastUsage(t, svc, apiKey, direct, "claude-opus-5", requestedTierFor(direct, "fast", "claude-opus-5"), "standard")
	require.Equal(t, "standard", *log.ServiceTier)
	require.InDelta(t, 0.03, log.ActualCost, 1e-12, "响应声明标准档只降不升")

	log = recordFastUsage(t, svc, apiKey, bedrock, "claude-opus-5", requestedTierFor(bedrock, "fast", "claude-opus-5"), "")
	require.Nil(t, log.ServiceTier)
	require.InDelta(t, 0.03, log.ActualCost, 1e-12, "Bedrock 不加价")

	// claude-opus-4-7 标准价：目录 5e-6/2.5e-5。
	log = recordFastUsage(t, svc, apiKey, direct, "claude-opus-4-7", requestedTierFor(direct, "fast", "claude-opus-4-7"), "")
	require.Nil(t, log.ServiceTier)
	require.InDelta(t, 0.03, log.ActualCost, 1e-12, "不支持的模型不加价")

	log = recordFastUsage(t, svc, apiKey, direct, "claude-opus-5", requestedTierFor(direct, "fast", "claude-opus-5"), "ultrafast")
	require.Equal(t, "fast", *log.ServiceTier)
	require.InDelta(t, 0.06, log.ActualCost, 1e-12, "更贵或未知档位按请求档")
}

// 渠道 FastMultiplier=3 显式配置时优先于目录 2x：0.03×3 = 0.09，不再乘目录 2 倍。
func TestFastSpeedBilling_ChannelFastMultiplierWins(t *testing.T) {
	billing, _ := newShippedCatalogBillingService(t)
	fast := 3.0
	ch := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{7}, ModelPricing: []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"claude-opus-5"}, BillingMode: BillingModeToken,
		InputPrice: f64(5e-6), OutputPrice: f64(2.5e-5), FastMultiplier: &fast,
	}}}
	channels := NewChannelService(&fastTierChannelRepo{channel: ch}, nil, nil, nil)
	resolver := NewModelPricingResolver(channels, billing)
	gid := int64(7)
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-opus-5", GroupID: &gid, ServiceTier: "fast"})
	require.True(t, resolved.ExplicitServiceTier)
	cost, err := billing.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "claude-opus-5", GroupID: &gid,
		Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 1000}, RateMultiplier: 1,
		Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.09, cost.ActualCost, 1e-12)
}

type fastTierChannelRepo struct {
	ChannelRepository
	channel Channel
}

func (r *fastTierChannelRepo) ListAll(context.Context) ([]Channel, error) {
	return []Channel{r.channel}, nil
}

func (r *fastTierChannelRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{7: PlatformAnthropic}, nil
}

// V03-T13：响应档位观测器（上游 upstreamResponseModelObserver 的档位部分，75faedda9）。
func TestOpenAIResponseServiceTierObserver(t *testing.T) {
	t.Run("created echo ignored, terminal wins", func(t *testing.T) {
		var o openAIResponseServiceTierObserver
		o.ObservePayload([]byte(`{"type":"response.created","response":{"model":"gpt-5.4","service_tier":"priority"}}`), "response.created")
		require.Equal(t, "", o.ServiceTier(), "response.created echoes the requested tier")
		o.ObservePayload([]byte(`{"type":"response.completed","response":{"model":"gpt-5.4","service_tier":"default"}}`), "response.completed")
		require.Equal(t, "default", o.ServiceTier())
	})
	t.Run("untyped payloads trusted while they agree", func(t *testing.T) {
		var o openAIResponseServiceTierObserver
		o.ObservePayload([]byte(`{"object":"chat.completion.chunk","model":"gpt-5.4","service_tier":"default"}`), "")
		o.ObservePayload([]byte(`{"object":"chat.completion.chunk","model":"gpt-5.4","service_tier":"default"}`), "")
		require.Equal(t, "default", o.ServiceTier())
	})
	t.Run("conflicting untyped payloads discarded", func(t *testing.T) {
		var o openAIResponseServiceTierObserver
		o.ObservePayload([]byte(`{"model":"gpt-5.4","service_tier":"priority"}`), "")
		o.ObservePayload([]byte(`{"model":"gpt-5.4","service_tier":"default"}`), "")
		require.Equal(t, "", o.ServiceTier())
	})
	t.Run("terminal overrides untyped conflict", func(t *testing.T) {
		var o openAIResponseServiceTierObserver
		o.ObservePayload([]byte(`{"model":"gpt-5.4","service_tier":"priority"}`), "")
		o.ObservePayload([]byte(`{"model":"gpt-5.4","service_tier":"default"}`), "")
		o.ObservePayload([]byte(`{"type":"response.done","response":{"service_tier":"flex"}}`), "response.done")
		require.Equal(t, "flex", o.ServiceTier())
	})
	t.Run("auto and unknown tiers ignored", func(t *testing.T) {
		var o openAIResponseServiceTierObserver
		o.ObservePayload([]byte(`{"model":"gpt-5.4","service_tier":"auto"}`), "")
		o.ObservePayload([]byte(`{"type":"response.completed","response":{"service_tier":"turbo"}}`), "response.completed")
		require.Equal(t, "", o.ServiceTier())
	})
	t.Run("nil observer is safe", func(t *testing.T) {
		var o *openAIResponseServiceTierObserver
		o.ObservePayload([]byte(`{"service_tier":"default"}`), "")
		require.Equal(t, "", o.ServiceTier())
	})
}
