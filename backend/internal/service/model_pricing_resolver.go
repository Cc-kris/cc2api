package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// PricingSource 定价来源标识
const (
	PricingSourceChannel  = "channel"
	PricingSourceMixed    = "mixed"
	PricingSourceLiteLLM  = "litellm"
	PricingSourceFallback = "fallback"
)

// ResolvedPricing 统一定价解析结果
type ResolvedPricing struct {
	// Mode 计费模式
	Mode BillingMode

	// Token 模式：基础定价（来自 LiteLLM 或 fallback）
	BasePricing *ModelPricing

	// Token 模式：区间定价列表（如有，覆盖 BasePricing 中的对应字段）
	Intervals []PricingInterval

	// 按次/图片模式：分层定价
	RequestTiers []PricingInterval

	// 按次/图片模式：默认价格（未命中层级时使用）
	DefaultPerRequestPrice float64
	// DefaultPerRequestPricePresent distinguishes an explicit free price from an
	// absent default price.
	DefaultPerRequestPricePresent bool

	// 来源标识
	Source string // "channel", "mixed", "litellm", "fallback"

	// 是否支持缓存细分
	SupportsCacheBreakdown bool
	RuleLayer              string
	TimeMultiplier         float64
	// IntervalImageInputPrice 区间定价模式下渠道级图片输入单价（区间本身不带此价）；0 回退文本输入价。
	IntervalImageInputPrice float64
	ServiceTier             string
	ExplicitServiceTier     bool

	// LongContextPricingDisabled 为 true 表示分组关闭了「长上下文定价」，目录长上下文阶梯
	// 不生效。零值即启用：无分组、分组查不到时都按启用处理（V03-R10 门控只看分组）。
	LongContextPricingDisabled bool
}

// LongContextPricingEnabled 报告目录长上下文阶梯是否可以生效。
func (r *ResolvedPricing) LongContextPricingEnabled() bool {
	return r == nil || !r.LongContextPricingDisabled
}

// ModelPricingResolver 统一模型定价解析器。
// 解析链：Channel → LiteLLM → Fallback。
type ModelPricingResolver struct {
	channelService *ChannelService
	billingService *BillingService
	groupRepo      GroupRepository
}

// NewModelPricingResolver 创建定价解析器实例
func NewModelPricingResolver(channelService *ChannelService, billingService *BillingService, groupRepos ...GroupRepository) *ModelPricingResolver {
	var groupRepo GroupRepository
	if len(groupRepos) > 0 {
		groupRepo = groupRepos[0]
	}
	return &ModelPricingResolver{
		channelService: channelService,
		billingService: billingService,
		groupRepo:      groupRepo,
	}
}

// PricingInput 定价解析输入
type PricingInput struct {
	Model            string
	GroupID          *int64 // nil 表示不检查渠道
	RequestStartedAt time.Time
	ServiceTier      string
}

// Resolve 解析模型定价。
// 1. 获取基础定价（LiteLLM → Fallback）
// 2. 如果指定了 GroupID，查找渠道定价并覆盖
func (r *ModelPricingResolver) Resolve(ctx context.Context, input PricingInput) *ResolvedPricing {
	var chPricing *ChannelModelPricing
	if input.GroupID != nil && r.channelService != nil {
		chPricing = r.channelService.GetChannelModelPricing(ctx, *input.GroupID, input.Model)
		if chPricing != nil {
			mode := chPricing.BillingMode
			if mode == "" {
				mode = BillingModeToken
			}
			if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo || mode == BillingModePerSecond {
				resolved := &ResolvedPricing{
					Mode:      mode,
					Source:    PricingSourceChannel,
					RuleLayer: "channel",
				}
				r.applyRequestTierOverrides(chPricing, resolved)
				if input.GroupID != nil && r.groupRepo != nil {
					if group, err := r.groupRepo.GetByIDLite(ctx, *input.GroupID); err == nil && group != nil {
						resolved.LongContextPricingDisabled = !LongContextPricingEnabledForGroup(group)
						r.applyGroupOverrides(input.Model, group, resolved)
					}
				}
				r.applyChannelModifiers(chPricing, resolved, input)
				return resolved
			}
		}
	}

	// 1. 获取基础定价
	basePricing, source := r.resolveBasePricing(input.Model)

	resolved := &ResolvedPricing{
		Mode:                   BillingModeToken,
		BasePricing:            basePricing,
		Source:                 source,
		SupportsCacheBreakdown: basePricing != nil && basePricing.SupportsCacheBreakdown,
	}

	// 2. 如果有 GroupID，尝试渠道覆盖
	if chPricing != nil {
		resolved.Source = PricingSourceChannel
		resolved.RuleLayer = "channel"
		r.applyTokenOverrides(chPricing, resolved)
	} else if input.GroupID != nil && r.channelService != nil {
		r.applyChannelOverrides(ctx, input, resolved)
	}
	if input.GroupID != nil && r.groupRepo != nil {
		if group, err := r.groupRepo.GetByIDLite(ctx, *input.GroupID); err == nil && group != nil {
			resolved.LongContextPricingDisabled = !LongContextPricingEnabledForGroup(group)
			r.applyGroupOverrides(input.Model, group, resolved)
		}
	}
	if chPricing != nil {
		r.applyChannelModifiers(chPricing, resolved, input)
	}
	if resolved.RuleLayer == "" {
		resolved.RuleLayer = "builtin"
	}

	return resolved
}

// ApplyGroupLongContextGate 按分组「长上下文定价」开关设置 resolved 的目录阶梯门控。
// 用于不带 GroupID 调用 Resolve 的路径（如销售定价 v2），保证门控只看分组且与 Resolve 一致。
// 分组查不到时保持启用。
func (r *ModelPricingResolver) ApplyGroupLongContextGate(ctx context.Context, groupID *int64, resolved *ResolvedPricing) {
	if r == nil || resolved == nil || groupID == nil || r.groupRepo == nil {
		return
	}
	if group, err := r.groupRepo.GetByIDLite(ctx, *groupID); err == nil && group != nil {
		resolved.LongContextPricingDisabled = !LongContextPricingEnabledForGroup(group)
	}
}

func (r *ModelPricingResolver) applyChannelModifiers(chPricing *ChannelModelPricing, resolved *ResolvedPricing, input PricingInput) {
	if chPricing == nil || resolved == nil {
		return
	}
	at := input.RequestStartedAt
	if at.IsZero() {
		at = time.Now()
	}
	tier := normalizeBillingServiceTier(input.ServiceTier)
	multiplier := chPricing.MultiplierAt(at, tier)
	resolved.TimeMultiplier = multiplier
	resolved.ServiceTier = tier
	resolved.ExplicitServiceTier = false
	if isFastServiceTier(tier) && chPricing.FastMultiplier != nil {
		resolved.ExplicitServiceTier = true
	}
	if tier == "flex" && chPricing.FlexMultiplier != nil {
		resolved.ExplicitServiceTier = true
	}
	if multiplier == 1 {
		return
	}
	resolved.BasePricing = multiplyModelPricing(resolved.BasePricing, multiplier)
	resolved.Intervals = multiplyPricingIntervals(resolved.Intervals, multiplier)
	resolved.IntervalImageInputPrice *= multiplier
	resolved.RequestTiers = multiplyPricingIntervals(resolved.RequestTiers, multiplier)
	if resolved.DefaultPerRequestPricePresent {
		resolved.DefaultPerRequestPrice *= multiplier
	}
}

func multiplyModelPricing(pricing *ModelPricing, multiplier float64) *ModelPricing {
	if pricing == nil || multiplier == 1 {
		return pricing
	}
	copy := *pricing
	copy.InputPricePerToken *= multiplier
	copy.InputPricePerTokenPriority *= multiplier
	copy.OutputPricePerToken *= multiplier
	copy.OutputPricePerTokenPriority *= multiplier
	copy.CacheCreationPricePerToken *= multiplier
	copy.CacheReadPricePerToken *= multiplier
	copy.CacheReadPricePerTokenPriority *= multiplier
	copy.CacheCreation5mPrice *= multiplier
	copy.CacheCreation1hPrice *= multiplier
	copy.ImageInputPricePerToken *= multiplier
	copy.ImageOutputPricePerToken *= multiplier
	return &copy
}

func multiplyPricingIntervals(intervals []PricingInterval, multiplier float64) []PricingInterval {
	if len(intervals) == 0 || multiplier == 1 {
		return intervals
	}
	copy := append([]PricingInterval(nil), intervals...)
	for i := range copy {
		copy[i].InputPrice = multiplyPrice(copy[i].InputPrice, multiplier)
		copy[i].OutputPrice = multiplyPrice(copy[i].OutputPrice, multiplier)
		copy[i].CacheWritePrice = multiplyPrice(copy[i].CacheWritePrice, multiplier)
		copy[i].CacheWrite1hPrice = multiplyPrice(copy[i].CacheWrite1hPrice, multiplier)
		copy[i].CacheReadPrice = multiplyPrice(copy[i].CacheReadPrice, multiplier)
		copy[i].PerRequestPrice = multiplyPrice(copy[i].PerRequestPrice, multiplier)
	}
	return copy
}

func multiplyPrice(value *float64, multiplier float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value * multiplier
	return &out
}

func (r *ModelPricingResolver) applyGroupOverrides(model string, group *Group, resolved *ResolvedPricing) {
	gp, ok := group.ModelPricing[model]
	if !ok {
		for name, p := range group.ModelPricing {
			if strings.EqualFold(name, model) {
				gp, ok = p, true
				break
			}
		}
	}
	if !ok {
		return
	}
	if resolved.Mode == "" {
		resolved.Mode = BillingModeToken
	}
	if len(gp.Intervals) > 0 && group.LongContextPricingEnabled {
		ivs := make([]PricingInterval, 0, len(gp.Intervals))
		for _, iv := range gp.Intervals {
			out := PricingInterval{MinTokens: iv.MinTokens, MaxTokens: iv.MaxTokens}
			out.InputPrice = parseGroupPrice(iv.Input)
			out.OutputPrice = parseGroupPrice(iv.Output)
			out.CacheReadPrice = parseGroupPrice(iv.CacheRead)
			ivs = append(ivs, out)
		}
		if valid := filterValidIntervals(ivs); len(valid) > 0 {
			resolved.Intervals = valid
		}
	}
	if resolved.BasePricing == nil {
		resolved.BasePricing = &ModelPricing{}
	}
	resolved.BasePricing.ensurePresence()
	if v := parseGroupPrice(gp.Input); v != nil {
		resolved.BasePricing.InputPricePerToken = *v
		resolved.BasePricing.InputPricePerTokenPriority = *v
		resolved.BasePricing.Presence.Input = true
	}
	if v := parseGroupPrice(gp.Output); v != nil {
		resolved.BasePricing.OutputPricePerToken = *v
		resolved.BasePricing.OutputPricePerTokenPriority = *v
		resolved.BasePricing.Presence.Output = true
	}
	if v := parseGroupPrice(gp.CacheRead); v != nil {
		resolved.BasePricing.CacheReadPricePerToken = *v
		resolved.BasePricing.CacheReadPricePerTokenPriority = *v
		resolved.BasePricing.Presence.CacheRead = true
	}
	switch resolved.Source {
	case PricingSourceChannel, PricingSourceLiteLLM, PricingSourceFallback:
		resolved.Source = PricingSourceMixed
	}
	resolved.RuleLayer = "group"
}

func parseGroupPrice(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return nil
	}
	return &v
}

// resolveBasePricing 从 LiteLLM 或 Fallback 获取基础定价
func (r *ModelPricingResolver) resolveBasePricing(model string) (*ModelPricing, string) {
	pricing, err := r.billingService.GetModelPricing(model)
	if err != nil {
		slog.Debug("failed to get model pricing from LiteLLM, using fallback",
			"model", model, "error", err)
		return nil, PricingSourceFallback
	}
	return pricing, PricingSourceLiteLLM
}

// applyChannelOverrides 应用渠道定价覆盖
func (r *ModelPricingResolver) applyChannelOverrides(ctx context.Context, input PricingInput, resolved *ResolvedPricing) {
	if r.channelService == nil || input.GroupID == nil {
		return
	}
	chPricing := r.channelService.GetChannelModelPricing(ctx, *input.GroupID, input.Model)
	if chPricing == nil {
		return
	}

	resolved.Source = PricingSourceChannel
	resolved.Mode = chPricing.BillingMode
	if resolved.Mode == "" {
		resolved.Mode = BillingModeToken
	}

	switch resolved.Mode {
	case BillingModeToken:
		r.applyTokenOverrides(chPricing, resolved)
	case BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond:
		r.applyRequestTierOverrides(chPricing, resolved)
	}
}

// applyTokenOverrides 应用 token 模式的渠道覆盖
func (r *ModelPricingResolver) applyTokenOverrides(chPricing *ChannelModelPricing, resolved *ResolvedPricing) {
	// 过滤掉所有价格字段都为空的无效 interval
	validIntervals := filterValidIntervals(chPricing.Intervals)

	// 如果有有效的区间定价，使用区间
	if len(validIntervals) > 0 {
		resolved.Intervals = validIntervals
		for i := range validIntervals {
			if validIntervals[i].CacheWrite1hPrice != nil {
				resolved.SupportsCacheBreakdown = true
				break
			}
		}
		// 区间不携带图片输入价，沿用渠道级配置（上游 06e03f467），由 GetIntervalPricing 带入命中区间。
		if chPricing.ImageInputPrice != nil {
			resolved.IntervalImageInputPrice = *chPricing.ImageInputPrice
		}
		return
	}

	// 否则用 flat 字段覆盖 BasePricing
	if resolved.BasePricing == nil {
		resolved.BasePricing = &ModelPricing{}
	}
	resolved.BasePricing.ensurePresence()

	if chPricing.InputPrice != nil {
		resolved.BasePricing.InputPricePerToken = *chPricing.InputPrice
		resolved.BasePricing.InputPricePerTokenPriority = *chPricing.InputPrice
		resolved.BasePricing.Presence.Input = true
		resolved.BasePricing.Presence.FastInput = false
	}
	if chPricing.OutputPrice != nil {
		resolved.BasePricing.OutputPricePerToken = *chPricing.OutputPrice
		resolved.BasePricing.OutputPricePerTokenPriority = *chPricing.OutputPrice
		resolved.BasePricing.Presence.Output = true
		resolved.BasePricing.Presence.FastOutput = false
	}
	applyChannelCacheWritePrices(resolved.BasePricing, chPricing.CacheWritePrice, chPricing.CacheWrite1hPrice)
	if chPricing.CacheWrite1hPrice != nil {
		resolved.SupportsCacheBreakdown = true
	}
	if chPricing.CacheReadPrice != nil {
		resolved.BasePricing.CacheReadPricePerToken = *chPricing.CacheReadPrice
		resolved.BasePricing.CacheReadPricePerTokenPriority = *chPricing.CacheReadPrice
		resolved.BasePricing.Presence.CacheRead = true
		resolved.BasePricing.Presence.FastCacheRead = false
	}
	if chPricing.ImageInputPrice != nil {
		resolved.BasePricing.ImageInputPricePerToken = *chPricing.ImageInputPrice
	}
	if chPricing.ImageOutputPrice != nil {
		resolved.BasePricing.ImageOutputPricePerToken = *chPricing.ImageOutputPrice
		resolved.BasePricing.Presence.ImageOutput = true
	}
}

// applyRequestTierOverrides 应用按次/图片模式的渠道覆盖
func (r *ModelPricingResolver) applyRequestTierOverrides(chPricing *ChannelModelPricing, resolved *ResolvedPricing) {
	resolved.RequestTiers = filterValidIntervals(chPricing.Intervals)
	if chPricing.PerRequestPrice != nil {
		resolved.DefaultPerRequestPrice = *chPricing.PerRequestPrice
		resolved.DefaultPerRequestPricePresent = true
	}
}

// filterValidIntervals 过滤掉所有价格字段都为空的无效 interval。
// 前端可能创建了只有 min/max 但无价格的空 interval。
func filterValidIntervals(intervals []PricingInterval) []PricingInterval {
	var valid []PricingInterval
	for _, iv := range intervals {
		if iv.InputPrice != nil || iv.OutputPrice != nil ||
			iv.CacheWritePrice != nil || iv.CacheWrite1hPrice != nil || iv.CacheReadPrice != nil ||
			iv.PerRequestPrice != nil {
			valid = append(valid, iv)
		}
	}
	return valid
}

// GetIntervalPricing 根据 context token 数获取区间定价。
// 如果有区间列表，找到匹配区间并构造 ModelPricing；否则直接返回 BasePricing。
func (r *ModelPricingResolver) GetIntervalPricing(resolved *ResolvedPricing, totalContextTokens int) *ModelPricing {
	if len(resolved.Intervals) == 0 {
		return resolved.BasePricing
	}

	iv := FindMatchingInterval(resolved.Intervals, totalContextTokens)
	if iv == nil {
		return resolved.BasePricing
	}

	pricing := intervalToModelPricing(iv, resolved.SupportsCacheBreakdown)
	pricing.ImageInputPricePerToken = resolved.IntervalImageInputPrice
	return pricing
}

// intervalToModelPricing 将区间定价转换为 ModelPricing
func intervalToModelPricing(iv *PricingInterval, supportsCacheBreakdown bool) *ModelPricing {
	pricing := &ModelPricing{
		SupportsCacheBreakdown: supportsCacheBreakdown,
		Presence:               &ModelPricingPresence{},
	}
	if iv.InputPrice != nil {
		pricing.InputPricePerToken = *iv.InputPrice
		pricing.InputPricePerTokenPriority = *iv.InputPrice
		pricing.Presence.Input = true
	}
	if iv.OutputPrice != nil {
		pricing.OutputPricePerToken = *iv.OutputPrice
		pricing.OutputPricePerTokenPriority = *iv.OutputPrice
		pricing.Presence.Output = true
	}
	applyChannelCacheWritePrices(pricing, iv.CacheWritePrice, iv.CacheWrite1hPrice)
	if iv.CacheReadPrice != nil {
		pricing.CacheReadPricePerToken = *iv.CacheReadPrice
		pricing.CacheReadPricePerTokenPriority = *iv.CacheReadPrice
		pricing.Presence.CacheRead = true
	}
	return pricing
}

// GetRequestTierPrice 根据层级标签获取按次价格
func (r *ModelPricingResolver) GetRequestTierPrice(resolved *ResolvedPricing, tierLabel string) float64 {
	for _, tier := range resolved.RequestTiers {
		if tier.TierLabel == tierLabel && tier.PerRequestPrice != nil {
			return *tier.PerRequestPrice
		}
	}
	return 0
}

// GetRequestTierPriceByContext 根据 context token 数获取按次价格
func (r *ModelPricingResolver) GetRequestTierPriceByContext(resolved *ResolvedPricing, totalContextTokens int) float64 {
	iv := FindMatchingInterval(resolved.RequestTiers, totalContextTokens)
	if iv != nil && iv.PerRequestPrice != nil {
		return *iv.PerRequestPrice
	}
	return 0
}
