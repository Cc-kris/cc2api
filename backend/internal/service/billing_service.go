package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// APIKeyRateLimitCacheData holds rate limit usage data cached in Redis.
type APIKeyRateLimitCacheData struct {
	Usage5h  float64 `json:"usage_5h"`
	Usage1d  float64 `json:"usage_1d"`
	Usage7d  float64 `json:"usage_7d"`
	Window5h int64   `json:"window_5h"` // unix timestamp, 0 = not started
	Window1d int64   `json:"window_1d"`
	Window7d int64   `json:"window_7d"`
}

// UserPlatformQuotaCacheEntry Redis hash 反序列化结果。
//
// SchemaVersion 用于向后兼容：
//   - 0（旧 entry，无 SchemaVersion 字段）→ 视为 cache MISS，强制 refresh
//   - 1（当前版本）→ 包含 limits 和 window_start，可免 DB 查询
//
// limit 字段为 nil 表示"无限额"（DB 中对应列为 NULL）。
const UserPlatformQuotaCacheSchemaV1 = int64(1)

type UserPlatformQuotaCacheEntry struct {
	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64
	Version         int64
	SchemaVersion   int64

	// 以下字段仅在 SchemaVersion >= 1 时有效
	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time
}

// BillingCache defines cache operations for billing service
type BillingCache interface {
	// Balance operations
	GetUserBalance(ctx context.Context, userID int64) (float64, error)
	SetUserBalance(ctx context.Context, userID int64, balance float64) error
	DeductUserBalance(ctx context.Context, userID int64, amount float64) error
	InvalidateUserBalance(ctx context.Context, userID int64) error

	// Subscription operations
	GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error)
	SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error
	UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error
	InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error

	// API Key rate limit operations
	GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error)
	SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error
	UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error
	InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error

	// user × platform quota 缓存
	GetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) (*UserPlatformQuotaCacheEntry, bool, error)
	SetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string, entry *UserPlatformQuotaCacheEntry, ttl time.Duration) error
	DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error
	// IncrUserPlatformQuotaUsageCache 在缓存命中时累加用量；缓存未命中（key 不存在）静默返回 nil。
	IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration) error
}

// ModelPricing 模型价格配置（per-token价格，与LiteLLM格式一致）
type ModelPricing struct {
	InputPricePerToken             float64 // 每token输入价格 (USD)
	InputPricePerTokenPriority     float64 // priority service tier 下每token输入价格 (USD)
	ImageInputPricePerToken        float64 // 图片输入 token 价格 (USD)；0 时回退到文本输入价（V03-T14）
	OutputPricePerToken            float64 // 每token输出价格 (USD)
	OutputPricePerTokenPriority    float64 // priority service tier 下每token输出价格 (USD)
	CacheCreationPricePerToken     float64 // 缓存创建每token价格 (USD)
	CacheReadPricePerToken         float64 // 缓存读取每token价格 (USD)
	CacheReadPricePerTokenPriority float64 // priority service tier 下缓存读取每token价格 (USD)
	CacheCreation5mPrice           float64 // 5分钟缓存创建每token价格 (USD)
	CacheCreation1hPrice           float64 // 1小时缓存创建每token价格 (USD)
	SupportsCacheBreakdown         bool    // 是否支持详细的缓存分类
	LongContextInputThreshold      int     // 超过阈值后按整次会话提升输入价格
	LongContextInputMultiplier     float64 // 长上下文整次会话输入倍率
	LongContextOutputMultiplier    float64 // 长上下文整次会话输出倍率
	// LongContextCacheReadMultiplier 长上下文缓存读取倍率；≤0 时跟随输入倍率。
	LongContextCacheReadMultiplier float64
	// LongContextThresholdInclusive 为 true 时达到阈值即应用（xAI、显式声明的条目）；默认严格大于。
	LongContextThresholdInclusive bool
	// FastCostMultiplier 目录 provider_specific_entry.fast 给出的 Fast 档倍率；0 表示用默认 2x。
	FastCostMultiplier       float64
	ImageOutputPricePerToken float64 // 图片输出 token 价格 (USD)
	// Presence preserves the distinction between an absent price and an explicit
	// zero price. Nil keeps legacy callers compatible by inferring presence from
	// non-zero values.
	Presence *ModelPricingPresence
}

func normalizeBillingServiceTier(serviceTier string) string {
	return strings.ToLower(strings.TrimSpace(serviceTier))
}

// isFastServiceTier 报告档位是否属于 Fast：OpenAI priority 与 Anthropic fast 同义（V03-T13）。
func isFastServiceTier(serviceTier string) bool {
	switch normalizeBillingServiceTier(serviceTier) {
	case "priority", "fast":
		return true
	default:
		return false
	}
}

func usePriorityServiceTierPricing(serviceTier string, pricing *ModelPricing) bool {
	if pricing == nil || !isFastServiceTier(serviceTier) {
		return false
	}
	return pricing.InputPricePerTokenPriority > 0 || pricing.OutputPricePerTokenPriority > 0 || pricing.CacheReadPricePerTokenPriority > 0
}

// pricingWithPriorityMultiplier 复制一份价格，priority 档按标准价乘以 multiplier（上游 billing_service.go:175）。
// CCAI 的 ModelPricing 没有 cache creation priority 字段，该项不设。
func pricingWithPriorityMultiplier(base *ModelPricing, multiplier float64) *ModelPricing {
	if base == nil {
		return nil
	}
	cloned := *base
	cloned.InputPricePerTokenPriority = cloned.InputPricePerToken * multiplier
	cloned.OutputPricePerTokenPriority = cloned.OutputPricePerToken * multiplier
	cloned.CacheReadPricePerTokenPriority = cloned.CacheReadPricePerToken * multiplier
	return &cloned
}

// isClaudeFable51Model 识别 Fable 5.1 的各种写法，后面不能紧跟数字（避免 fable-5-10 之类误判）。
func isClaudeFable51Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, marker := range []string{"fable-5-1", "fable-5.1", "fable5.1", "fable51"} {
		if at := strings.Index(model, marker); at >= 0 {
			after := at + len(marker)
			if after == len(model) || model[after] < '0' || model[after] > '9' {
				return true
			}
		}
	}
	return false
}

func serviceTierCostMultiplier(serviceTier string) float64 {
	switch normalizeBillingServiceTier(serviceTier) {
	case "priority", "fast":
		return 2.0
	case "flex":
		return 0.5
	default:
		return 1.0
	}
}

// UsageTokens 使用的token数量
type UsageTokens struct {
	InputTokens           int
	ImageInputTokens      int // InputTokens 中图片部分的数量（子集，不是额外 token）；0 时按原口径计费
	OutputTokens          int
	CacheCreationTokens   int
	CacheReadTokens       int
	CacheCreation5mTokens int
	CacheCreation1hTokens int
	ImageOutputTokens     int
}

// CostBreakdown 费用明细
type CostBreakdown struct {
	InputCost         float64 // 文本输入费用（不含图片输入，图片输入单独记入 ImageInputCost）
	ImageInputCost    float64 // 图片输入 token 费用（V03-T14）
	OutputCost        float64
	ImageOutputCost   float64
	CacheCreationCost float64
	CacheReadCost     float64
	TotalCost         float64
	ActualCost        float64 // 应用倍率后的实际费用
	BillingMode       string  // 计费模式（"token"/"per_request"/"image"/"per_second"），由 CalculateCostUnified 填充
}

// ErrModelPricingUnavailable indicates that none of the configured pricing
// sources can price the requested model.
var ErrModelPricingUnavailable = errors.New("pricing not found")

// BillingService 计费服务
type BillingService struct {
	cfg            *config.Config
	pricingService *PricingService
	fallbackPrices map[string]*ModelPricing // 硬编码回退价格
}

// NewBillingService 创建计费服务实例
func NewBillingService(cfg *config.Config, pricingService *PricingService) *BillingService {
	s := &BillingService{
		cfg:            cfg,
		pricingService: pricingService,
		fallbackPrices: make(map[string]*ModelPricing),
	}

	// 初始化硬编码回退价格（当动态价格不可用时使用）
	s.initFallbackPricing()

	return s
}

// initFallbackPricing 初始化硬编码回退价格（当动态价格不可用时使用）
// 价格单位：USD per token（与LiteLLM格式一致）
func (s *BillingService) initFallbackPricing() {
	// Claude 4.5 Opus
	s.fallbackPrices["claude-opus-4.5"] = &ModelPricing{
		InputPricePerToken:         5e-6,    // $5 per MTok
		OutputPricePerToken:        25e-6,   // $25 per MTok
		CacheCreationPricePerToken: 6.25e-6, // $6.25 per MTok
		CacheReadPricePerToken:     0.5e-6,  // $0.50 per MTok
		SupportsCacheBreakdown:     false,
	}

	// Claude 4 Sonnet
	s.fallbackPrices["claude-sonnet-4"] = &ModelPricing{
		InputPricePerToken:         3e-6,    // $3 per MTok
		OutputPricePerToken:        15e-6,   // $15 per MTok
		CacheCreationPricePerToken: 3.75e-6, // $3.75 per MTok
		CacheReadPricePerToken:     0.3e-6,  // $0.30 per MTok
		SupportsCacheBreakdown:     false,
	}

	// Claude 3.5 Sonnet
	s.fallbackPrices["claude-3-5-sonnet"] = &ModelPricing{
		InputPricePerToken:         3e-6,    // $3 per MTok
		OutputPricePerToken:        15e-6,   // $15 per MTok
		CacheCreationPricePerToken: 3.75e-6, // $3.75 per MTok
		CacheReadPricePerToken:     0.3e-6,  // $0.30 per MTok
		SupportsCacheBreakdown:     false,
	}

	// Claude 3.5 Haiku
	s.fallbackPrices["claude-3-5-haiku"] = &ModelPricing{
		InputPricePerToken:         1e-6,    // $1 per MTok
		OutputPricePerToken:        5e-6,    // $5 per MTok
		CacheCreationPricePerToken: 1.25e-6, // $1.25 per MTok
		CacheReadPricePerToken:     0.1e-6,  // $0.10 per MTok
		SupportsCacheBreakdown:     false,
	}

	// Claude 3 Opus
	s.fallbackPrices["claude-3-opus"] = &ModelPricing{
		InputPricePerToken:         15e-6,    // $15 per MTok
		OutputPricePerToken:        75e-6,    // $75 per MTok
		CacheCreationPricePerToken: 18.75e-6, // $18.75 per MTok
		CacheReadPricePerToken:     1.5e-6,   // $1.50 per MTok
		SupportsCacheBreakdown:     false,
	}

	// Claude 3 Haiku
	s.fallbackPrices["claude-3-haiku"] = &ModelPricing{
		InputPricePerToken:         0.25e-6, // $0.25 per MTok
		OutputPricePerToken:        1.25e-6, // $1.25 per MTok
		CacheCreationPricePerToken: 0.3e-6,  // $0.30 per MTok
		CacheReadPricePerToken:     0.03e-6, // $0.03 per MTok
		SupportsCacheBreakdown:     false,
	}

	// Claude 4.6 Opus (与4.5同价)
	s.fallbackPrices["claude-opus-4.6"] = s.fallbackPrices["claude-opus-4.5"]

	// Claude 4.7 Opus (暂与4.6同价，待官方定价更新)
	s.fallbackPrices["claude-opus-4.7"] = s.fallbackPrices["claude-opus-4.6"]

	// Claude 4.8 Opus / Claude Opus 5（标准 $5/$25，Fast $10/$50 per MTok）。
	// 缺少这两条时 getFallbackPricing 会掉到 claude-3-opus（$15/$75），造成 3 倍超收。
	s.fallbackPrices["claude-opus-4.8"] = pricingWithPriorityMultiplier(s.fallbackPrices["claude-opus-4.7"], 2)
	s.fallbackPrices["claude-opus-5"] = pricingWithPriorityMultiplier(s.fallbackPrices["claude-opus-4.8"], 2)

	s.fallbackPrices["claude-opus-5-5"] = &ModelPricing{
		InputPricePerToken:         4e-6,
		OutputPricePerToken:        20e-6,
		CacheCreationPricePerToken: 5e-6,
		CacheReadPricePerToken:     0.2e-6,
		CacheCreation5mPrice:       5e-6,
		CacheCreation1hPrice:       8e-6,
		SupportsCacheBreakdown:     true,
	}
	s.fallbackPrices["claude-sonnet-5-5"] = &ModelPricing{
		InputPricePerToken:         2e-6,
		OutputPricePerToken:        10e-6,
		CacheCreationPricePerToken: 2.5e-6,
		CacheReadPricePerToken:     0.2e-6,
		CacheCreation5mPrice:       2.5e-6,
		CacheCreation1hPrice:       4e-6,
		SupportsCacheBreakdown:     true,
	}
	// Claude Haiku 5.5（2026-10-07，$0.10/$0.50，prompt 超 100k tokens 整单 5x）。
	s.fallbackPrices["claude-haiku-5-5"] = &ModelPricing{
		InputPricePerToken:          0.1e-6,
		OutputPricePerToken:         0.5e-6,
		CacheCreationPricePerToken:  0.125e-6,
		CacheReadPricePerToken:      0.01e-6,
		CacheCreation5mPrice:        0.125e-6,
		CacheCreation1hPrice:        0.2e-6,
		SupportsCacheBreakdown:      true,
		LongContextInputThreshold:   100000,
		LongContextInputMultiplier:  5,
		LongContextOutputMultiplier: 5,
	}
	// Claude Fable 5.x uses the same input/output and cache-write prices, while
	// Fable 5.1 reduces cache reads from $1 to $0.25 per MTok.
	s.fallbackPrices["claude-fable-5"] = &ModelPricing{
		InputPricePerToken:         10e-6,
		OutputPricePerToken:        50e-6,
		CacheCreationPricePerToken: 12.5e-6,
		CacheCreation5mPrice:       12.5e-6,
		CacheCreation1hPrice:       20e-6,
		CacheReadPricePerToken:     1e-6,
		SupportsCacheBreakdown:     true,
	}
	s.fallbackPrices["claude-fable-5-1"] = &ModelPricing{
		InputPricePerToken:         10e-6,
		OutputPricePerToken:        50e-6,
		CacheCreationPricePerToken: 12.5e-6,
		CacheCreation5mPrice:       12.5e-6,
		CacheCreation1hPrice:       20e-6,
		CacheReadPricePerToken:     0.25e-6,
		SupportsCacheBreakdown:     true,
	}

	// Gemini 3.1 Pro
	s.fallbackPrices["gemini-3.1-pro"] = &ModelPricing{
		InputPricePerToken:         2e-6,   // $2 per MTok
		OutputPricePerToken:        12e-6,  // $12 per MTok
		CacheCreationPricePerToken: 2e-6,   // $2 per MTok
		CacheReadPricePerToken:     0.2e-6, // $0.20 per MTok
		SupportsCacheBreakdown:     false,
	}

	// Gemini 3.6 / 3.7 / 3.8 Flash（Google AI 优惠价：$0.75 输入 / $3.75 输出 /
	// $0.075 缓存读 per MTok，至 2026-12-31；2027-01-01 起官方价翻倍为 $1.50/$7.50/$0.15）。
	// Antigravity 的 -high/-low/-medium/-tiered 档位 ID 在 getFallbackPricing 里按子串归到这里，
	// 远程价格表不可用时也不会记 0 元（上游 4426593e9、8ed57b000、6254aba5e）。
	s.fallbackPrices["gemini-3.6-flash"] = &ModelPricing{
		InputPricePerToken:     0.75e-6,
		OutputPricePerToken:    3.75e-6,
		CacheReadPricePerToken: 0.075e-6,
		SupportsCacheBreakdown: false,
	}
	s.fallbackPrices["gemini-3.7-flash"] = &ModelPricing{
		InputPricePerToken:     0.75e-6,
		OutputPricePerToken:    3.75e-6,
		CacheReadPricePerToken: 0.075e-6,
		SupportsCacheBreakdown: false,
	}
	s.fallbackPrices["gemini-3.8-flash"] = &ModelPricing{
		InputPricePerToken:     0.75e-6,
		OutputPricePerToken:    3.75e-6,
		CacheReadPricePerToken: 0.075e-6,
		SupportsCacheBreakdown: false,
	}

	// Gemini 4 Argon（Google AI 优惠价：$2 输入 / $10 输出 / $0.10 缓存读 per MTok，
	// 至 2026-11-30；2026-12-01 起官方价翻倍为 $4/$20/$0.20）。不分 prompt 长度。
	// 远程价格表没有这个模型，生产计费走这里（上游 c6c9935ae）。
	s.fallbackPrices["gemini-4-argon"] = &ModelPricing{
		InputPricePerToken:     2e-6,
		OutputPricePerToken:    10e-6,
		CacheReadPricePerToken: 0.1e-6,
		SupportsCacheBreakdown: false,
	}

	// Gemini 4 Argon Cyber Permissive（Vertex AI：$4 输入 / $20 输出 / $0.20 缓存读 per MTok）。
	// 不享受 Gemini 4 Argon 的优惠价。
	s.fallbackPrices["gemini-4-argon-cyber-permissive"] = &ModelPricing{
		InputPricePerToken:     4e-6,
		OutputPricePerToken:    20e-6,
		CacheReadPricePerToken: 0.2e-6,
		SupportsCacheBreakdown: false,
	}

	// xAI Grok 4.7：200k 以内 $2 输入 / $0.50 缓存读 / $6 输出 per MTok；超过 200k 整次会话
	// 输入、输出都按 2 倍（$4 / $12）。远程价格表没有 grok-4.7，生产计费走这里（上游 935db6851）。
	// CCAI 的长上下文判断是严格大于阈值（shouldApplySessionLongContextPricing），
	// 正好 200000 token 不涨价；上游 LongContextThresholdInclusive 不移植（V02-T22）。
	s.fallbackPrices["grok-4.7"] = &ModelPricing{
		InputPricePerToken:          2e-6,
		OutputPricePerToken:         6e-6,
		CacheReadPricePerToken:      0.5e-6,
		SupportsCacheBreakdown:      false,
		LongContextInputThreshold:   200000,
		LongContextInputMultiplier:  2,
		LongContextOutputMultiplier: 2,
	}

	// xAI Grok 4.7 Fast（仅 Cursor 和 Grok Build）：同一模型的高速版，200k 以内
	// $4 输入 / $1 缓存读 / $12 输出 per MTok；超过 200k 按 1.5 倍（上游 1cdecf4a8）。
	s.fallbackPrices["grok-4.7-fast"] = &ModelPricing{
		InputPricePerToken:          4e-6,
		OutputPricePerToken:         12e-6,
		CacheReadPricePerToken:      1e-6,
		SupportsCacheBreakdown:      false,
		LongContextInputThreshold:   200000,
		LongContextInputMultiplier:  1.5,
		LongContextOutputMultiplier: 1.5,
	}

	// OpenAI GPT-5.4（业务指定价格）
	s.fallbackPrices["gpt-5.4"] = &ModelPricing{
		InputPricePerToken:             2.5e-6,  // $2.5 per MTok
		InputPricePerTokenPriority:     5e-6,    // $5 per MTok
		OutputPricePerToken:            15e-6,   // $15 per MTok
		OutputPricePerTokenPriority:    30e-6,   // $30 per MTok
		CacheCreationPricePerToken:     2.5e-6,  // $2.5 per MTok
		CacheReadPricePerToken:         0.25e-6, // $0.25 per MTok
		CacheReadPricePerTokenPriority: 0.5e-6,  // $0.5 per MTok
		SupportsCacheBreakdown:         false,
		// 基线 272K 长上下文阶梯：目录未提供阶梯时保持（目录/override 优先）。
		LongContextInputThreshold:   openAIGPT54BaselineLongContextThreshold,
		LongContextInputMultiplier:  openAIGPT54BaselineLongContextInputMultiplier,
		LongContextOutputMultiplier: openAIGPT54BaselineLongContextOutputMultiplier,
	}
	// GPT-5.5 与 GPT-5.4 同价，但官方无 cache write 额外费用。
	s.fallbackPrices["gpt-5.5"] = &ModelPricing{
		InputPricePerToken:             2.5e-6,
		InputPricePerTokenPriority:     5e-6,
		OutputPricePerToken:            15e-6,
		OutputPricePerTokenPriority:    30e-6,
		CacheReadPricePerToken:         0.25e-6,
		CacheReadPricePerTokenPriority: 0.5e-6,
		SupportsCacheBreakdown:         false,
		// 基线 272K 长上下文阶梯，同 gpt-5.4。
		LongContextInputThreshold:   openAIGPT54BaselineLongContextThreshold,
		LongContextInputMultiplier:  openAIGPT54BaselineLongContextInputMultiplier,
		LongContextOutputMultiplier: openAIGPT54BaselineLongContextOutputMultiplier,
	}

	// OpenAI GPT-5.6 三档模型。基础 gpt-5.6 按 sol 档兜底。
	// 静态兜底价不携带长上下文阶梯：阶梯一律由目录数据驱动（V03-T11）。
	s.fallbackPrices["gpt-5.6-sol"] = &ModelPricing{
		InputPricePerToken:         5e-6,
		OutputPricePerToken:        30e-6,
		CacheCreationPricePerToken: 6.25e-6,
		CacheReadPricePerToken:     0.5e-6,
		SupportsCacheBreakdown:     false,
	}
	s.fallbackPrices["gpt-5.6"] = s.fallbackPrices["gpt-5.6-sol"]
	s.fallbackPrices["gpt-5.6-terra"] = &ModelPricing{
		InputPricePerToken:         2.5e-6,
		OutputPricePerToken:        15e-6,
		CacheCreationPricePerToken: 3.125e-6,
		CacheReadPricePerToken:     0.25e-6,
		SupportsCacheBreakdown:     false,
	}
	s.fallbackPrices["gpt-5.6-luna"] = &ModelPricing{
		InputPricePerToken:         1e-6,
		OutputPricePerToken:        6e-6,
		CacheCreationPricePerToken: 1.25e-6,
		CacheReadPricePerToken:     0.1e-6,
		SupportsCacheBreakdown:     false,
	}

	s.fallbackPrices["gpt-5.4-mini"] = &ModelPricing{
		InputPricePerToken:     7.5e-7,
		OutputPricePerToken:    4.5e-6,
		CacheReadPricePerToken: 7.5e-8,
		SupportsCacheBreakdown: false,
	}
	s.fallbackPrices["gpt-5.4-nano"] = &ModelPricing{
		InputPricePerToken:     2e-7,
		OutputPricePerToken:    1.25e-6,
		CacheReadPricePerToken: 2e-8,
		SupportsCacheBreakdown: false,
	}
	// OpenAI GPT-5.2（本地兜底）
	s.fallbackPrices["gpt-5.2"] = &ModelPricing{
		InputPricePerToken:             1.75e-6,
		InputPricePerTokenPriority:     3.5e-6,
		OutputPricePerToken:            14e-6,
		OutputPricePerTokenPriority:    28e-6,
		CacheCreationPricePerToken:     1.75e-6,
		CacheReadPricePerToken:         0.175e-6,
		CacheReadPricePerTokenPriority: 0.35e-6,
		SupportsCacheBreakdown:         false,
	}
	// Codex 族兜底统一按 GPT-5.3 Codex 价格计费
	s.fallbackPrices["gpt-5.3-codex"] = &ModelPricing{
		InputPricePerToken:             1.5e-6, // $1.5 per MTok
		InputPricePerTokenPriority:     3e-6,   // $3 per MTok
		OutputPricePerToken:            12e-6,  // $12 per MTok
		OutputPricePerTokenPriority:    24e-6,  // $24 per MTok
		CacheCreationPricePerToken:     1.5e-6, // $1.5 per MTok
		CacheReadPricePerToken:         0.15e-6,
		CacheReadPricePerTokenPriority: 0.3e-6,
		SupportsCacheBreakdown:         false,
	}
}

// getFallbackPricing 根据模型系列获取回退价格
func (s *BillingService) getFallbackPricing(model string) *ModelPricing {
	modelLower := normalizeAntigravityClaudeTierAlias(strings.ToLower(model))

	// 按模型系列匹配
	if isClaudeFable51Model(modelLower) {
		return s.fallbackPrices["claude-fable-5-1"]
	}
	if strings.Contains(modelLower, "fable-5") || strings.Contains(modelLower, "fable5") {
		return s.fallbackPrices["claude-fable-5"]
	}
	if claude.IsOpus55(modelLower) {
		return s.fallbackPrices["claude-opus-5-5"]
	}
	if claude.IsSonnet55(modelLower) {
		return s.fallbackPrices["claude-sonnet-5-5"]
	}
	if claude.IsHaiku55(modelLower) {
		return s.fallbackPrices["claude-haiku-5-5"]
	}
	if strings.Contains(modelLower, "opus") {
		// "opus-5" 必须先判：不能用裸 "5" 匹配，否则 claude-opus-4-5 会被误判。
		if strings.Contains(modelLower, "opus-5") || strings.Contains(modelLower, "opus5") {
			return s.fallbackPrices["claude-opus-5"]
		}
		if strings.Contains(modelLower, "4.8") || strings.Contains(modelLower, "4-8") {
			return s.fallbackPrices["claude-opus-4.8"]
		}
		if strings.Contains(modelLower, "4.7") || strings.Contains(modelLower, "4-7") {
			return s.fallbackPrices["claude-opus-4.7"]
		}
		if strings.Contains(modelLower, "4.6") || strings.Contains(modelLower, "4-6") {
			return s.fallbackPrices["claude-opus-4.6"]
		}
		if strings.Contains(modelLower, "4.5") || strings.Contains(modelLower, "4-5") {
			return s.fallbackPrices["claude-opus-4.5"]
		}
		return s.fallbackPrices["claude-3-opus"]
	}
	if strings.Contains(modelLower, "sonnet") {
		if strings.Contains(modelLower, "4") && !strings.Contains(modelLower, "3") {
			return s.fallbackPrices["claude-sonnet-4"]
		}
		return s.fallbackPrices["claude-3-5-sonnet"]
	}
	if strings.Contains(modelLower, "haiku") {
		if strings.Contains(modelLower, "3-5") || strings.Contains(modelLower, "3.5") {
			return s.fallbackPrices["claude-3-5-haiku"]
		}
		return s.fallbackPrices["claude-3-haiku"]
	}
	// Claude 未知型号统一回退到 Sonnet，避免计费中断。
	if strings.Contains(modelLower, "claude") {
		return s.fallbackPrices["claude-sonnet-4"]
	}
	if strings.Contains(modelLower, "gemini-3.1-pro") || strings.Contains(modelLower, "gemini-3-1-pro") {
		return s.fallbackPrices["gemini-3.1-pro"]
	}
	if strings.Contains(modelLower, "gemini-3.6-flash") || strings.Contains(modelLower, "gemini-3-6-flash") {
		return s.fallbackPrices["gemini-3.6-flash"]
	}
	if strings.Contains(modelLower, "gemini-3.7-flash") || strings.Contains(modelLower, "gemini-3-7-flash") {
		return s.fallbackPrices["gemini-3.7-flash"]
	}
	if strings.Contains(modelLower, "gemini-3.8-flash") || strings.Contains(modelLower, "gemini-3-8-flash") {
		return s.fallbackPrices["gemini-3.8-flash"]
	}
	// Cyber 变体不享受 Gemini 4 Argon 的优惠价，必须先于 "gemini-4-argon" 判。
	if strings.Contains(modelLower, "gemini-4-argon-cyber") {
		return s.fallbackPrices["gemini-4-argon-cyber-permissive"]
	}
	if strings.Contains(modelLower, "gemini-4-argon") {
		return s.fallbackPrices["gemini-4-argon"]
	}

	// OpenAI 仅匹配已知 GPT-5/Codex 族，避免未知 OpenAI 型号误计价。
	if normalized := normalizeKnownOpenAICodexModel(modelLower); normalized != "" {
		switch normalized {
		case "gpt-5.6", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra":
			return s.fallbackPrices[normalized]
		case "gpt-5.5":
			return s.fallbackPrices["gpt-5.5"]
		case "gpt-5.4-mini":
			return s.fallbackPrices["gpt-5.4-mini"]
		case "gpt-5.4-nano":
			return s.fallbackPrices["gpt-5.4-nano"]
		case "gpt-5.4":
			return s.fallbackPrices["gpt-5.4"]
		case "gpt-5.2":
			return s.fallbackPrices["gpt-5.2"]
		case "gpt-5.3-codex", "gpt-5.3-codex-spark":
			return s.fallbackPrices["gpt-5.3-codex"]
		}
	}

	switch modelLower {
	case "grok-4.7", "grok-4.7-latest":
		return s.fallbackPrices["grok-4.7"]
	// Grok 4.7 Fast IDs (grok-4.7-fast, and the Grok Build ID
	// grok-4.7-build-fast). Without these cases both fall into the standard
	// 4.7 card or have no price at all. The upstream catalog has no
	// grok-4.7-build-fast entry, so this case is the only way it gets priced.
	case "grok-4.7-fast", "grok-4.7-build-fast":
		return s.fallbackPrices["grok-4.7-fast"]
	}

	return nil
}

// GetModelPricing 获取模型价格配置
func (s *BillingService) GetModelPricing(model string) (*ModelPricing, error) {
	// 标准化模型名称（转小写）
	model = strings.ToLower(model)

	// 1. 优先从动态价格服务获取
	if s.pricingService != nil {
		litellmPricing := s.pricingService.GetModelPricing(model)
		if litellmPricing != nil {
			// 启用 5m/1h 分类计费的条件：
			// 1. 存在 1h 价格
			// 2. 1h 价格 > 5m 价格（防止 LiteLLM 数据错误导致少收费）
			price5m := litellmPricing.CacheCreationInputTokenCost
			price1h := litellmPricing.CacheCreationInputTokenCostAbove1hr
			enableBreakdown := price1h > 0 && price1h > price5m
			return s.applyModelSpecificPricingPolicy(model, &ModelPricing{
				InputPricePerToken:             litellmPricing.InputCostPerToken,
				InputPricePerTokenPriority:     litellmPricing.InputCostPerTokenPriority,
				OutputPricePerToken:            litellmPricing.OutputCostPerToken,
				OutputPricePerTokenPriority:    litellmPricing.OutputCostPerTokenPriority,
				CacheCreationPricePerToken:     litellmPricing.CacheCreationInputTokenCost,
				CacheReadPricePerToken:         litellmPricing.CacheReadInputTokenCost,
				CacheReadPricePerTokenPriority: litellmPricing.CacheReadInputTokenCostPriority,
				CacheCreation5mPrice:           price5m,
				CacheCreation1hPrice:           price1h,
				SupportsCacheBreakdown:         enableBreakdown,
				LongContextInputThreshold:      litellmPricing.LongContextInputTokenThreshold,
				LongContextInputMultiplier:     litellmPricing.LongContextInputCostMultiplier,
				LongContextOutputMultiplier:    litellmPricing.LongContextOutputCostMultiplier,
				LongContextCacheReadMultiplier: litellmPricing.LongContextCacheReadCostMultiplier,
				LongContextThresholdInclusive:  litellmPricing.LongContextThresholdInclusive,
				FastCostMultiplier:             litellmPricing.FastCostMultiplier,
				ImageOutputPricePerToken:       litellmPricing.OutputCostPerImageToken,
			}), nil
		}
	}

	// 2. 使用硬编码回退价格
	fallback := s.getFallbackPricing(model)
	if fallback != nil {
		log.Printf("[Billing] Using fallback pricing for model: %s", model)
		return s.applyModelSpecificPricingPolicy(model, fallback), nil
	}

	return nil, fmt.Errorf("%w for model: %s", ErrModelPricingUnavailable, model)
}

// GetModelPricingWithChannel 获取模型定价，渠道配置的价格覆盖默认值
// 仅覆盖渠道中非 nil 的价格字段，nil 字段使用默认定价
func (s *BillingService) GetModelPricingWithChannel(model string, channelPricing *ChannelModelPricing) (*ModelPricing, error) {
	pricing, err := s.GetModelPricing(model)
	if err != nil {
		return nil, err
	}
	if channelPricing == nil {
		return pricing, nil
	}
	if channelPricing.InputPrice != nil {
		pricing.InputPricePerToken = *channelPricing.InputPrice
		pricing.InputPricePerTokenPriority = *channelPricing.InputPrice
	}
	if channelPricing.ImageInputPrice != nil {
		pricing.ImageInputPricePerToken = *channelPricing.ImageInputPrice
	}
	if channelPricing.OutputPrice != nil {
		pricing.OutputPricePerToken = *channelPricing.OutputPrice
		pricing.OutputPricePerTokenPriority = *channelPricing.OutputPrice
	}
	applyChannelCacheWritePrices(pricing, channelPricing.CacheWritePrice, channelPricing.CacheWrite1hPrice)
	if channelPricing.CacheReadPrice != nil {
		pricing.CacheReadPricePerToken = *channelPricing.CacheReadPrice
		pricing.CacheReadPricePerTokenPriority = *channelPricing.CacheReadPrice
	}
	if channelPricing.ImageOutputPrice != nil {
		pricing.ImageOutputPricePerToken = *channelPricing.ImageOutputPrice
	}
	return pricing, nil
}

// applyChannelCacheWritePrices 应用渠道显式缓存写入价（上游 34b8bf1a6 / 062af81fb 口径）：
// cacheWrite 同时覆盖标准、5m 价；1h 价单独配置时只覆盖 1h 并启用 5m/1h 分档，
// 未配置时 1h 沿用 cacheWrite（与升级前一致）。渠道显式价不会被目录价覆盖。
func applyChannelCacheWritePrices(pricing *ModelPricing, cacheWrite, cacheWrite1h *float64) {
	if pricing == nil {
		return
	}
	if cacheWrite != nil {
		pricing.CacheCreationPricePerToken = *cacheWrite
		pricing.CacheCreation5mPrice = *cacheWrite
		if cacheWrite1h == nil {
			pricing.CacheCreation1hPrice = *cacheWrite
		}
		if pricing.Presence != nil {
			pricing.Presence.CacheWrite = true
			pricing.Presence.CacheWrite5m = true
			pricing.Presence.CacheWrite1h = true
		}
	}
	if cacheWrite1h != nil {
		pricing.CacheCreation1hPrice = *cacheWrite1h
		pricing.SupportsCacheBreakdown = true
		if pricing.Presence != nil {
			pricing.Presence.CacheWrite1h = true
		}
	}
}

// --- 统一计费入口 ---

// CostInput 统一计费输入
type CostInput struct {
	Ctx            context.Context
	Model          string
	GroupID        *int64 // 用于渠道定价查找
	Tokens         UsageTokens
	RequestCount   int    // 按次计费时使用
	SizeTier       string // 按次/图片模式的层级标签（"1K","2K","4K","HD" 等）
	RateMultiplier float64
	ServiceTier    string                // "priority","flex","" 等
	Resolver       *ModelPricingResolver // 定价解析器
	Resolved       *ResolvedPricing      // 可选：预解析的定价结果（避免重复 Resolve 调用）
}

// CalculateCostUnified 统一计费入口，支持三种计费模式。
// 使用 ModelPricingResolver 解析定价，然后根据 BillingMode 分发计算。
func (s *BillingService) CalculateCostUnified(input CostInput) (*CostBreakdown, error) {
	if input.Resolver == nil {
		// 无 Resolver，回退到旧路径
		return s.calculateCostInternal(input.Model, input.Tokens, input.RateMultiplier, input.ServiceTier, nil)
	}

	// 优先使用预解析结果，避免重复 Resolve 调用
	resolved := input.Resolved
	if resolved == nil {
		resolved = input.Resolver.Resolve(input.Ctx, PricingInput{
			Model:   input.Model,
			GroupID: input.GroupID,
		})
	}

	// 保存时强制 > 0；若仍有负数泄漏（缓存/迁移残留），按 0 处理避免按 1x 误扣。
	if input.RateMultiplier < 0 {
		input.RateMultiplier = 0
	}

	var breakdown *CostBreakdown
	var err error
	switch resolved.Mode {
	case BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond:
		breakdown, err = s.calculatePerRequestCost(resolved, input)
	default: // BillingModeToken
		breakdown, err = s.calculateTokenCost(resolved, input)
	}
	if err == nil && breakdown != nil {
		breakdown.BillingMode = string(resolved.Mode)
		if breakdown.BillingMode == "" {
			breakdown.BillingMode = string(BillingModeToken)
		}
	}
	return breakdown, err
}

// calculateTokenCost 按 token 区间计费
func (s *BillingService) calculateTokenCost(resolved *ResolvedPricing, input CostInput) (*CostBreakdown, error) {
	totalContext := input.Tokens.InputTokens + input.Tokens.CacheReadTokens

	pricing := input.Resolver.GetIntervalPricing(resolved, totalContext)
	if pricing == nil {
		return nil, fmt.Errorf("no pricing available for model: %s: %w", input.Model, ErrModelPricingUnavailable)
	}

	pricing = s.applyModelSpecificPricingPolicy(input.Model, pricing)

	// 目录长上下文阶梯只在无区间定价时应用（区间定价已包含上下文分层），并受分组
	// 「长上下文定价」开关控制：无分组视为开启（Resolve 中计算，见 longContextPricingEnabled）。
	applyLongCtx := len(resolved.Intervals) == 0 && resolved.LongContextPricingEnabled()

	return s.computeTokenBreakdown(pricing, input.Tokens, input.RateMultiplier, input.ServiceTier, applyLongCtx), nil
}

// computeTokenBreakdown 是 token 计费的核心逻辑，由 calculateTokenCost 和 calculateCostInternal 共用。
// applyLongCtx 控制是否检查长上下文定价（区间定价已自含上下文分层，不需要额外应用）。
func (s *BillingService) computeTokenBreakdown(
	pricing *ModelPricing, tokens UsageTokens,
	rateMultiplier float64, serviceTier string,
	applyLongCtx bool,
) *CostBreakdown {
	// 保存时强制 > 0；若仍有负数泄漏，按 0 处理避免按 1x 误扣。
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}

	inputPrice := pricing.InputPricePerToken
	outputPrice := pricing.OutputPricePerToken
	cacheReadPrice := pricing.CacheReadPricePerToken
	tierMultiplier := 1.0

	if usePriorityServiceTierPricing(serviceTier, pricing) {
		if pricing.InputPricePerTokenPriority > 0 {
			inputPrice = pricing.InputPricePerTokenPriority
		}
		if pricing.OutputPricePerTokenPriority > 0 {
			outputPrice = pricing.OutputPricePerTokenPriority
		}
		if pricing.CacheReadPricePerTokenPriority > 0 {
			cacheReadPrice = pricing.CacheReadPricePerTokenPriority
		}
	} else {
		tierMultiplier = serviceTierCostMultiplier(serviceTier)
		// 目录 provider_specific_entry.fast（如 claude-opus-5=2.0）给出模型级 Fast 倍率。
		if isFastServiceTier(serviceTier) && pricing.FastCostMultiplier > 0 {
			tierMultiplier = pricing.FastCostMultiplier
		}
	}

	appliesLongContext := applyLongCtx && s.shouldApplySessionLongContextPricing(tokens, pricing)
	longCtxInputMultiplier, longCtxCacheReadMultiplier := 1.0, 1.0
	if appliesLongContext {
		// 倍率 ≤0 表示该项未配置（目录/覆写条目可能只写了 input 或 output 一侧），
		// 按 1 计而不是乘 0：乘 0 会把超阈值请求的对应分项算成免费。
		longCtxInputMultiplier = longContextMultiplierOrOne(pricing.LongContextInputMultiplier)
		longCtxCacheReadMultiplier = longCtxInputMultiplier
		if pricing.LongContextCacheReadMultiplier > 0 {
			longCtxCacheReadMultiplier = pricing.LongContextCacheReadMultiplier
		}
		inputPrice *= longCtxInputMultiplier
		outputPrice *= longContextMultiplierOrOne(pricing.LongContextOutputMultiplier)
	}

	bd := &CostBreakdown{}
	// 分离图片输入 token 与文本输入 token（上游 262fe1230 / 62d57c02d）。InputCost 仅计文本输入，
	// 图片输入费用单独记入 ImageInputCost；ImageInputTokens 为 0 时（绝大多数流量）走原单价路径。
	if tokens.ImageInputTokens > 0 {
		imageInputTokens := tokens.ImageInputTokens
		textInputTokens := tokens.InputTokens - imageInputTokens
		if textInputTokens < 0 {
			textInputTokens = 0
			imageInputTokens = tokens.InputTokens
		}
		imageInputPrice := pricing.ImageInputPricePerToken
		if imageInputPrice == 0 {
			// 未配置图片输入价时回退到文本输入价（已含 priority / 长上下文调整），总额与旧口径一致。
			imageInputPrice = inputPrice
		} else if appliesLongContext {
			imageInputPrice *= longCtxInputMultiplier
		}
		bd.InputCost = float64(textInputTokens) * inputPrice
		bd.ImageInputCost = float64(imageInputTokens) * imageInputPrice
	} else {
		bd.InputCost = float64(tokens.InputTokens) * inputPrice
	}

	// 分离图片输出 token 与文本输出 token
	textOutputTokens := tokens.OutputTokens - tokens.ImageOutputTokens
	if textOutputTokens < 0 {
		textOutputTokens = 0
	}
	bd.OutputCost = float64(textOutputTokens) * outputPrice

	// 图片输出 token 费用（独立费率）
	if tokens.ImageOutputTokens > 0 {
		imgPrice := pricing.ImageOutputPricePerToken
		if imgPrice == 0 {
			imgPrice = outputPrice // 回退到常规输出价格
		}
		bd.ImageOutputCost = float64(tokens.ImageOutputTokens) * imgPrice
	}

	// 缓存创建费用
	bd.CacheCreationCost = s.computeCacheCreationCost(pricing, tokens)

	bd.CacheReadCost = float64(tokens.CacheReadTokens) * cacheReadPrice
	if appliesLongContext {
		bd.CacheCreationCost *= longCtxInputMultiplier
		bd.CacheReadCost *= longCtxCacheReadMultiplier
	}

	if tierMultiplier != 1.0 {
		bd.InputCost *= tierMultiplier
		bd.ImageInputCost *= tierMultiplier
		bd.OutputCost *= tierMultiplier
		bd.ImageOutputCost *= tierMultiplier
		bd.CacheCreationCost *= tierMultiplier
		bd.CacheReadCost *= tierMultiplier
	}

	bd.TotalCost = bd.InputCost + bd.ImageInputCost + bd.OutputCost + bd.ImageOutputCost +
		bd.CacheCreationCost + bd.CacheReadCost
	bd.ActualCost = bd.TotalCost * rateMultiplier

	return bd
}

// computeCacheCreationCost 计算缓存创建费用（支持 5m/1h 分类或标准计费）。
func (s *BillingService) computeCacheCreationCost(pricing *ModelPricing, tokens UsageTokens) float64 {
	if pricing.SupportsCacheBreakdown && (pricing.CacheCreation5mPrice > 0 || pricing.CacheCreation1hPrice > 0) {
		if tokens.CacheCreation5mTokens == 0 && tokens.CacheCreation1hTokens == 0 && tokens.CacheCreationTokens > 0 {
			// API 未返回 ephemeral 明细，回退到全部按 5m 单价计费
			return float64(tokens.CacheCreationTokens) * pricing.CacheCreation5mPrice
		}
		return float64(tokens.CacheCreation5mTokens)*pricing.CacheCreation5mPrice +
			float64(tokens.CacheCreation1hTokens)*pricing.CacheCreation1hPrice
	}
	return float64(tokens.CacheCreationTokens) * pricing.CacheCreationPricePerToken
}

// calculatePerRequestCost 按次/图片计费
func (s *BillingService) calculatePerRequestCost(resolved *ResolvedPricing, input CostInput) (*CostBreakdown, error) {
	count := input.RequestCount
	if count <= 0 {
		count = 1
	}

	var unitPrice float64

	if input.SizeTier != "" {
		unitPrice = input.Resolver.GetRequestTierPrice(resolved, input.SizeTier)
	}

	if unitPrice == 0 {
		totalContext := input.Tokens.InputTokens + input.Tokens.CacheReadTokens
		unitPrice = input.Resolver.GetRequestTierPriceByContext(resolved, totalContext)
	}

	// 回退到默认按次价格
	if unitPrice == 0 {
		unitPrice = resolved.DefaultPerRequestPrice
	}

	totalCost := unitPrice * float64(count)
	actualCost := totalCost * input.RateMultiplier

	return &CostBreakdown{
		TotalCost:  totalCost,
		ActualCost: actualCost,
	}, nil
}

// CalculateCost 计算使用费用
func (s *BillingService) CalculateCost(model string, tokens UsageTokens, rateMultiplier float64) (*CostBreakdown, error) {
	return s.calculateCostInternal(model, tokens, rateMultiplier, "", nil)
}

func (s *BillingService) CalculateCostWithServiceTier(model string, tokens UsageTokens, rateMultiplier float64, serviceTier string) (*CostBreakdown, error) {
	return s.calculateCostInternal(model, tokens, rateMultiplier, serviceTier, nil)
}

// LongContextPricingEnabledForGroup 是售价侧长上下文门控：分组「长上下文定价」开关启用即生效，
// 无分组（无分组 Key）视为启用。不引入账号级开关（V03-R10）。
func LongContextPricingEnabledForGroup(group *Group) bool {
	return group == nil || group.LongContextPricingEnabled
}

// CalculateCostForGroup 与 CalculateCostWithServiceTier 相同，但目录长上下文阶梯受分组开关控制。
// 用于不经过 ModelPricingResolver 的计费路径。
func (s *BillingService) CalculateCostForGroup(model string, tokens UsageTokens, rateMultiplier float64, serviceTier string, group *Group) (*CostBreakdown, error) {
	return s.calculateCostInternalWithPolicy(model, tokens, rateMultiplier, serviceTier, nil, LongContextPricingEnabledForGroup(group))
}

func (s *BillingService) calculateCostInternal(model string, tokens UsageTokens, rateMultiplier float64, serviceTier string, channelPricing *ChannelModelPricing) (*CostBreakdown, error) {
	return s.calculateCostInternalWithPolicy(model, tokens, rateMultiplier, serviceTier, channelPricing, true)
}

func (s *BillingService) calculateCostInternalWithPolicy(model string, tokens UsageTokens, rateMultiplier float64, serviceTier string, channelPricing *ChannelModelPricing, longContextEnabled bool) (*CostBreakdown, error) {
	var pricing *ModelPricing
	var err error
	if channelPricing != nil {
		pricing, err = s.GetModelPricingWithChannel(model, channelPricing)
	} else {
		pricing, err = s.GetModelPricing(model)
	}
	if err != nil {
		return nil, err
	}

	// 旧路径无区间定价概念，目录阶梯只受分组开关控制
	return s.computeTokenBreakdown(pricing, tokens, rateMultiplier, serviceTier, longContextEnabled), nil
}

// grokLongContextCard 返回 Grok 4.7 族的回退价卡（带 200k 长上下文阶梯），其他模型返回 nil。
func (s *BillingService) grokLongContextCard(model string) *ModelPricing {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "grok-4.7", "grok-4.7-latest":
		return s.fallbackPrices["grok-4.7"]
	case "grok-4.7-fast", "grok-4.7-build-fast":
		return s.fallbackPrices["grok-4.7-fast"]
	}
	return nil
}

func (s *BillingService) applyModelSpecificPricingPolicy(model string, pricing *ModelPricing) *ModelPricing {
	if pricing == nil {
		return nil
	}
	// 保留 v0.3.12 的 Grok 4.7 回退价卡：目录缺少阶梯时补齐，目录已提供的
	// 阶梯（包括 override）始终优先。GPT-5.4/5.5 的基线 272K 阶梯由静态回退价与目录解析阶段（applyOpenAIGPT54BaselineLongContext）提供。
	if card := s.grokLongContextCard(model); card != nil {
		if pricing.LongContextInputThreshold > 0 {
			return pricing
		}
		cloned := *pricing
		cloned.LongContextInputThreshold = card.LongContextInputThreshold
		cloned.LongContextInputMultiplier = card.LongContextInputMultiplier
		cloned.LongContextOutputMultiplier = card.LongContextOutputMultiplier
		return &cloned
	}
	return pricing
}

// longContextMultiplierOrOne 把未配置（≤0）的长上下文倍率归一为 1。
func longContextMultiplierOrOne(m float64) float64 {
	if m <= 0 {
		return 1
	}
	return m
}

func (s *BillingService) shouldApplySessionLongContextPricing(tokens UsageTokens, pricing *ModelPricing) bool {
	if pricing == nil || pricing.LongContextInputThreshold <= 0 {
		return false
	}
	if pricing.LongContextInputMultiplier <= 1 && pricing.LongContextOutputMultiplier <= 1 {
		return false
	}
	totalInputTokens := tokens.InputTokens + tokens.CacheReadTokens
	if pricing.LongContextThresholdInclusive {
		return totalInputTokens >= pricing.LongContextInputThreshold
	}
	return totalInputTokens > pricing.LongContextInputThreshold
}

// CalculateCostWithConfig 使用配置中的默认倍率计算费用
func (s *BillingService) CalculateCostWithConfig(model string, tokens UsageTokens) (*CostBreakdown, error) {
	multiplier := s.cfg.Default.RateMultiplier
	if multiplier <= 0 {
		multiplier = 1.0
	}
	return s.CalculateCost(model, tokens, multiplier)
}

// ListSupportedModels 列出所有支持的模型（现在总是返回true，因为有模糊匹配）
func (s *BillingService) ListSupportedModels() []string {
	models := make([]string, 0)
	// 返回回退价格支持的模型系列
	for model := range s.fallbackPrices {
		models = append(models, model)
	}
	return models
}

// IsModelSupported 检查模型是否支持（现在总是返回true，因为有模糊匹配回退）
func (s *BillingService) IsModelSupported(model string) bool {
	// 所有Claude模型都有回退价格支持
	modelLower := strings.ToLower(model)
	return strings.Contains(modelLower, "claude") ||
		strings.Contains(modelLower, "opus") ||
		strings.Contains(modelLower, "sonnet") ||
		strings.Contains(modelLower, "haiku")
}

// GetEstimatedCost 估算费用（用于前端展示）
func (s *BillingService) GetEstimatedCost(model string, estimatedInputTokens, estimatedOutputTokens int) (float64, error) {
	tokens := UsageTokens{
		InputTokens:  estimatedInputTokens,
		OutputTokens: estimatedOutputTokens,
	}

	breakdown, err := s.CalculateCostWithConfig(model, tokens)
	if err != nil {
		return 0, err
	}

	return breakdown.ActualCost, nil
}

// GetPricingServiceStatus 获取价格服务状态
func (s *BillingService) GetPricingServiceStatus() map[string]any {
	if s.pricingService != nil {
		return s.pricingService.GetStatus()
	}
	return map[string]any{
		"model_count":  len(s.fallbackPrices),
		"last_updated": "using fallback",
		"local_hash":   "N/A",
	}
}

// ForceUpdatePricing 强制更新价格数据
func (s *BillingService) ForceUpdatePricing() error {
	if s.pricingService != nil {
		return s.pricingService.ForceUpdate()
	}
	return fmt.Errorf("pricing service not initialized")
}

// ImagePriceConfig 图片计费配置
type ImagePriceConfig struct {
	Price1K *float64 // 1K 尺寸价格（nil 表示使用默认值）
	Price2K *float64 // 2K 尺寸价格（nil 表示使用默认值）
	Price4K *float64 // 4K 尺寸价格（nil 表示使用默认值）
}

// VideoPriceConfig 视频生成计费配置，价格均为每秒单价（USD/s）。
type VideoPriceConfig struct {
	Price480P  *float64
	Price720P  *float64
	Price1080P *float64
}

const (
	defaultImageGenerationPrice = 0.134

	defaultGrokImagineImagePrice1K        = 0.02
	defaultGrokImagineImagePrice2K        = 0.02
	defaultGrokImagineImageQualityPrice1K = 0.05
	defaultGrokImagineImageQualityPrice2K = 0.07

	defaultGrokImagineVideoPrice480P    = 0.05
	defaultGrokImagineVideoPrice720P    = 0.07
	defaultGrokImagineVideo15Price480P  = 0.08
	defaultGrokImagineVideo15Price720P  = 0.14
	defaultGrokImagineVideo15Price1080P = 0.25
)

// CalculateImageCost 计算图片生成费用
// model: 请求的模型名称（用于获取 LiteLLM 默认价格）
// imageSize: 图片尺寸 "1K", "2K", "4K"
// imageCount: 生成的图片数量
// groupConfig: 分组配置的价格（可能为 nil，表示使用默认值）
// rateMultiplier: 费率倍数
func (s *BillingService) CalculateImageCost(model string, imageSize string, imageCount int, groupConfig *ImagePriceConfig, rateMultiplier float64) *CostBreakdown {
	if imageCount <= 0 {
		return &CostBreakdown{}
	}
	imageSize = NormalizeImageBillingTierOrDefault(imageSize)

	// 获取单价
	unitPrice := s.getImageUnitPrice(model, imageSize, groupConfig)

	// 计算总费用
	totalCost := unitPrice * float64(imageCount)

	// 应用倍率（保存时强制 > 0；负数按 0 处理避免按 1x 误扣）
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	actualCost := totalCost * rateMultiplier

	return &CostBreakdown{
		TotalCost:   totalCost,
		ActualCost:  actualCost,
		BillingMode: string(BillingModeImage),
	}
}

// CalculateVideoCost 计算视频生成费用：每秒单价 × 时长 × 数量 × 倍率。
func (s *BillingService) CalculateVideoCost(model, resolution string, videoCount, durationSeconds int, groupConfig *VideoPriceConfig, rateMultiplier float64) *CostBreakdown {
	if videoCount <= 0 {
		return &CostBreakdown{}
	}
	resolution = NormalizeVideoBillingResolutionOrDefault(resolution)
	durationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
	unitPrice := s.getVideoUnitPrice(model, resolution, groupConfig)
	totalCost := unitPrice * float64(durationSeconds) * float64(videoCount)
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	return &CostBreakdown{
		TotalCost:   totalCost,
		ActualCost:  totalCost * rateMultiplier,
		BillingMode: string(BillingModeVideo),
	}
}

// getImageUnitPrice 获取图片单价
func (s *BillingService) getImageUnitPrice(model string, imageSize string, groupConfig *ImagePriceConfig) float64 {
	// 优先使用分组配置的价格
	if groupConfig != nil {
		switch imageSize {
		case "1K":
			if groupConfig.Price1K != nil {
				return *groupConfig.Price1K
			}
		case "2K":
			if groupConfig.Price2K != nil {
				return *groupConfig.Price2K
			}
		case "4K":
			if groupConfig.Price4K != nil {
				return *groupConfig.Price4K
			}
		}
	}

	// 回退到 LiteLLM 默认价格
	return s.getDefaultImagePrice(model, imageSize)
}

func (s *BillingService) getVideoUnitPrice(model, resolution string, groupConfig *VideoPriceConfig) float64 {
	if groupConfig != nil {
		switch resolution {
		case VideoBillingResolution480P:
			if groupConfig.Price480P != nil {
				return *groupConfig.Price480P
			}
		case VideoBillingResolution720P:
			if groupConfig.Price720P != nil {
				return *groupConfig.Price720P
			}
		case VideoBillingResolution1080P:
			if groupConfig.Price1080P != nil {
				return *groupConfig.Price1080P
			}
		}
	}
	if price, ok := getDefaultGrokImagineVideoPrice(model, resolution); ok {
		return price
	}
	return s.getDefaultImagePrice(model, ImageBillingSize2K)
}

// getDefaultImagePrice 获取 LiteLLM 默认图片价格
func (s *BillingService) getDefaultImagePrice(model string, imageSize string) float64 {
	if price, ok := getDefaultGrokImagineImagePrice(model, imageSize); ok {
		return price
	}
	basePrice := 0.0

	// 从 PricingService 获取 output_cost_per_image
	if s.pricingService != nil {
		pricing := s.pricingService.GetModelPricing(model)
		if pricing != nil && pricing.OutputCostPerImage > 0 {
			basePrice = pricing.OutputCostPerImage
		}
	}

	// 如果没有找到价格，使用硬编码默认值（$0.134，来自 gemini-3-pro-image-preview）
	if basePrice <= 0 {
		basePrice = defaultImageGenerationPrice
	}

	// 2K 尺寸 1.5 倍，4K 尺寸翻倍
	if imageSize == "2K" {
		return basePrice * 1.5
	}
	if imageSize == "4K" {
		return basePrice * 2
	}

	return basePrice
}

func getDefaultGrokImagineImagePrice(model, imageSize string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "grok-imagine-image-quality":
		return getGrokImagineImageTierPrice(imageSize, defaultGrokImagineImageQualityPrice1K, defaultGrokImagineImageQualityPrice2K), true
	case "grok-imagine", "grok-imagine-image", "grok-imagine-edit":
		return getGrokImagineImageTierPrice(imageSize, defaultGrokImagineImagePrice1K, defaultGrokImagineImagePrice2K), true
	default:
		return 0, false
	}
}

func getGrokImagineImageTierPrice(imageSize string, price1K, price2K float64) float64 {
	if NormalizeImageBillingTierOrDefault(imageSize) == ImageBillingSize1K {
		return price1K
	}
	return price2K
}

func getDefaultGrokImagineVideoPrice(model, resolution string) (float64, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(model, "grok-imagine-video-1.5"):
		switch NormalizeVideoBillingResolutionOrDefault(resolution) {
		case VideoBillingResolution480P:
			return defaultGrokImagineVideo15Price480P, true
		case VideoBillingResolution720P:
			return defaultGrokImagineVideo15Price720P, true
		case VideoBillingResolution1080P:
			return defaultGrokImagineVideo15Price1080P, true
		}
	case strings.HasPrefix(model, "grok-imagine-video"):
		if NormalizeVideoBillingResolutionOrDefault(resolution) == VideoBillingResolution480P {
			return defaultGrokImagineVideoPrice480P, true
		}
		return defaultGrokImagineVideoPrice720P, true
	}
	return 0, false
}
