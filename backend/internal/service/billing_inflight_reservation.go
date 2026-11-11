package service

import (
	"context"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// V03-R12 余额模式在途预占（移植上游 43f88c99a、52c123e6f、652a7d74f）。
//
// 与上游的差异：
//   - 预估不用上游 estimateOne 的手写公式，改为构造合成用量后走 RecordUsage 同一套
//     计费函数（GatewayService.calculateRecordUsageCost / OpenAIGatewayService.
//     calculateOpenAIRecordUsageCost → BillingService.CalculateCostUnified /
//     CalculateCostForGroup / CalculateImageCost / CalculateVideoCost），销售定价 v2 开启时
//     再走 ResolveV2SalesPricingContextAndCost，见 inflightCostEstimator。
//   - 只保留余额维度；上游 26b31a83e 的订阅 / API Key 维度预占不移植（订阅分组不预占）。
//   - ReserveFixedCharge 在 Lua 里原子地要求余额覆盖本笔金额（require_cover），上游是先读余额再登记。

// InflightBalanceReservationCache 余额在途预占的缓存能力（可选）。
// BillingCache 的 Redis 实现同时实现此接口；未实现时在途预占自动关闭（fail-open）。
type InflightBalanceReservationCache interface {
	// ReserveInflightBalance 原子地：清理过期预占；若用户已有在途预占且
	// balance - sum(在途) < amount 则拒绝；否则登记 requestID 的预占（ttl 后自动失效）。
	// 返回是否放行以及登记前的在途合计。
	ReserveInflightBalance(ctx context.Context, userID int64, requestID string, amount, balance float64, ttl time.Duration) (bool, float64, error)
	// ReleaseInflightBalance 释放 requestID 的预占（幂等）。
	ReleaseInflightBalance(ctx context.Context, userID int64, requestID string) error
}

// InflightBalanceReservationRenewer 可选：续期仍存活的预占（流式长请求期间防止 TTL 过期）。
type InflightBalanceReservationRenewer interface {
	// RenewInflightBalance 若 requestID 仍存在且未过期则把其过期时间推迟到 now+ttl；返回是否仍存在。
	RenewInflightBalance(ctx context.Context, userID int64, requestID string, ttl time.Duration) (bool, error)
}

// InflightBalanceCoveredReserver 可选：确定金额费用的预占，即使没有在途预占也要求
// balance - sum(在途) >= amount（ReserveFixedCharge 使用）。
type InflightBalanceCoveredReserver interface {
	ReserveInflightBalanceCovered(ctx context.Context, userID int64, requestID string, amount, balance float64, ttl time.Duration) (bool, float64, error)
}

const (
	defaultInflightReservationTTL      = 15 * time.Minute
	defaultInflightDefaultMaxTokens    = 8192
	inflightReservationReleaseTimeout  = 2 * time.Second
	inflightReservationReserveTimeout  = 2 * time.Second
	inflightReservationRenewTimeout    = 2 * time.Second
	inflightInputBytesPerTokenEstimate = 4
	inflightUnpricedLogInterval        = time.Minute
)

// InflightReservation 一次请求的在途预占句柄。
//
// 生命周期（引用计数，归零时释放一次）：
//   - 创建时持有 1 个「handler」引用，并启动续期协程（每 ttl/3 续期一次）。
//   - handler 提交计费任务时通过 Acquire 再取一个引用，由计费任务在余额缓存
//     实际扣减之后归还（任务被丢弃时由提交方立即归还）。
//   - handler 返回时调用 HandlerDone：停止续期并归还 handler 引用。
//
// 因此预占会一直保持到「handler 结束 且 所有计费任务已完成扣减」；没有提交任何
// 计费任务的失败路径在 handler 返回时立即释放。handler 结束后不再续期，所以最长
// 持有时间受 TTL 约束（计费任务卡死时预占自动过期）。所有方法对 nil 接收者安全。
type InflightReservation struct {
	cache     InflightBalanceReservationCache
	userID    int64
	requestID string
	amount    float64
	ttl       time.Duration

	refs        atomic.Int64
	releaseOnce sync.Once
	stopOnce    sync.Once
	closeOnce   sync.Once
	stopRenew   chan struct{}
	renewDone   chan struct{}
}

// Amount 预占金额。
func (r *InflightReservation) Amount() float64 {
	if r == nil {
		return 0
	}
	return r.amount
}

// RequestID 预占在 Redis 中的成员名。
func (r *InflightReservation) RequestID() string {
	if r == nil {
		return ""
	}
	return r.requestID
}

// Acquire 为一个异步计费任务增加引用；返回的 done 幂等，必须在任务结束（或被丢弃）时调用。
// 预占已释放时返回 no-op。
func (r *InflightReservation) Acquire() func() {
	if r == nil {
		return noopRelease
	}
	for {
		cur := r.refs.Load()
		if cur <= 0 {
			return noopRelease
		}
		if r.refs.CompareAndSwap(cur, cur+1) {
			break
		}
	}
	var once sync.Once
	return func() { once.Do(r.decRef) }
}

// HandlerDone handler 结束：停止续期并归还 handler 引用（幂等）。
func (r *InflightReservation) HandlerDone() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.stopRenewal()
		r.decRef()
	})
}

// Release 立即释放预占（幂等），不论引用计数。
func (r *InflightReservation) Release() {
	if r == nil {
		return
	}
	r.stopRenewal()
	r.releaseOnce.Do(func() {
		r.refs.Store(0)
		relCtx, relCancel := context.WithTimeout(context.Background(), inflightReservationReleaseTimeout)
		defer relCancel()
		if err := r.cache.ReleaseInflightBalance(relCtx, r.userID, r.requestID); err != nil {
			logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation release failed for user %d (expires by ttl): %v", r.userID, err)
		}
	})
}

func (r *InflightReservation) decRef() {
	if r.refs.Add(-1) <= 0 {
		r.Release()
	}
}

func (r *InflightReservation) stopRenewal() {
	if r.stopRenew == nil {
		return
	}
	r.closeOnce.Do(func() { close(r.stopRenew) })
	<-r.renewDone
}

func (r *InflightReservation) startRenewal(renewer InflightBalanceReservationRenewer) {
	interval := r.ttl / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	r.stopRenew = make(chan struct{})
	r.renewDone = make(chan struct{})
	go func() {
		defer close(r.renewDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-r.stopRenew:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), inflightReservationRenewTimeout)
				alive, err := renewer.RenewInflightBalance(ctx, r.userID, r.requestID, r.ttl)
				cancel()
				if err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation renew failed for user %d: %v", r.userID, err)
					continue
				}
				if !alive {
					return
				}
			}
		}
	}()
}

type inflightReservationCtxKey struct{}

// WithInflightReservation 把预占句柄挂到 context 上，供计费任务提交时交接。
func WithInflightReservation(ctx context.Context, r *InflightReservation) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, inflightReservationCtxKey{}, r)
}

// InflightReservationFromContext 读取 context 中的预占句柄（可能为 nil）。
func InflightReservationFromContext(ctx context.Context) *InflightReservation {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(inflightReservationCtxKey{}).(*InflightReservation)
	return r
}

func noopRelease() {}

func (s *BillingCacheService) inflightReservationConfig() (config.InflightReservationConfig, bool) {
	if s == nil || s.cfg == nil {
		return config.InflightReservationConfig{}, false
	}
	cfg := s.cfg.Billing.InflightReservation
	if !cfg.Enabled || s.cfg.RunMode == config.RunModeSimple {
		return cfg, false
	}
	return cfg, true
}

// InflightReservationEnabled 是否启用余额在途预占。
func (s *BillingCacheService) InflightReservationEnabled() bool {
	_, ok := s.inflightReservationConfig()
	return ok
}

// InflightReservationFailClosedOnUnpriced 无法估算费用时是否拒绝请求（默认 false = fail-open）。
func (s *BillingCacheService) InflightReservationFailClosedOnUnpriced() bool {
	cfg, ok := s.inflightReservationConfig()
	return ok && cfg.FailClosedOnUnpriced
}

func (s *BillingCacheService) inflightReservationTTL(cfg config.InflightReservationConfig) time.Duration {
	if cfg.TTLSeconds > 0 {
		return time.Duration(cfg.TTLSeconds) * time.Second
	}
	return defaultInflightReservationTTL
}

// ReserveInflightBalance 简化封装：返回释放函数，不续期、不做计费交接（预占最长存活 TTL）。
func (s *BillingCacheService) ReserveInflightBalance(ctx context.Context, user *User, group *Group, subscription *UserSubscription, estimate float64) (func(), error) {
	r, err := s.reserveInflight(ctx, user, group, subscription, estimate, false, true, false)
	if err != nil {
		return noopRelease, err
	}
	if r == nil {
		return noopRelease, nil
	}
	return r.Release, nil
}

// ReserveInflight 在余额模式下为本次请求登记在途预占。
//
// 必须在 CheckBillingEligibility 通过后调用。返回的句柄可能为 nil（未预占）；
// 调用方须在 handler 结束时调用 HandlerDone（nil 安全）。
//
// 以下情况直接放行且不登记预占（fail-open，保持旧行为）：
// 开关关闭 / 简易模式 / 订阅模式 / estimate <= 0 / 缓存不支持 / 余额读取失败 / Redis 执行失败。
// 仅当 Redis 明确判定 缓存余额 - 在途合计 < estimate（且已有在途请求）时返回 ErrInsufficientBalance。
func (s *BillingCacheService) ReserveInflight(ctx context.Context, user *User, group *Group, subscription *UserSubscription, estimate float64) (*InflightReservation, error) {
	return s.reserveInflight(ctx, user, group, subscription, estimate, true, true, false)
}

// ReserveFixedCharge 为扣费前即可确定金额的费用（倍率后 USD，如 x_search 单次价）做准入并登记在途预占。
//
// 与 ReserveInflight 的区别：金额是确定值，不按 max_reservation_usd 封顶；即使用户没有其他在途请求，
// 也要求「缓存余额 − 其他在途预占合计 ≥ amount」（Lua 内原子判断），否则返回 ErrInsufficientBalance。
// 订阅分组、简易模式、开关关闭时不预占（保持旧行为，由 CheckBillingEligibility 决定准入）；
// 余额读取或 Redis 故障时放行不预占（fail-open）。须在 CheckBillingEligibility 通过后调用，
// 返回的句柄可能为 nil；调用方须在 handler 结束时调用 HandlerDone，预占在计费落库后释放。
func (s *BillingCacheService) ReserveFixedCharge(ctx context.Context, user *User, group *Group, subscription *UserSubscription, amount float64) (*InflightReservation, error) {
	return s.reserveInflight(ctx, user, group, subscription, QuantizeUsageBillingAmount(amount), true, false, true)
}

func (s *BillingCacheService) reserveInflight(ctx context.Context, user *User, group *Group, subscription *UserSubscription, estimate float64, renew, capped, requireCover bool) (*InflightReservation, error) {
	cfg, ok := s.inflightReservationConfig()
	if !ok || user == nil {
		return nil, nil
	}
	if group != nil && group.IsSubscriptionType() && subscription != nil {
		return nil, nil
	}
	if capped && cfg.MaxReservationUSD > 0 && estimate > cfg.MaxReservationUSD {
		estimate = cfg.MaxReservationUSD
	}
	if estimate <= 0 || math.IsNaN(estimate) || math.IsInf(estimate, 0) {
		return nil, nil
	}
	rc, ok := s.cache.(InflightBalanceReservationCache)
	if !ok || rc == nil {
		return nil, nil
	}

	balance, err := s.GetUserBalance(ctx, user.ID)
	if err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation balance read failed for user %d (fail-open): %v", user.ID, err)
		return nil, nil
	}

	ttl := s.inflightReservationTTL(cfg)
	requestID := uuid.NewString()
	reserveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inflightReservationReserveTimeout)
	var allowed bool
	var inflight float64
	if requireCover {
		if covered, ok := s.cache.(InflightBalanceCoveredReserver); ok && covered != nil {
			allowed, inflight, err = covered.ReserveInflightBalanceCovered(reserveCtx, user.ID, requestID, estimate, balance, ttl)
		} else if balance < estimate {
			cancel()
			logger.LegacyPrintf("service.billing_cache", "inflight reservation rejected: user=%d balance=%.8f inflight=0 estimate=%.8f (fixed charge)", user.ID, balance, estimate)
			return nil, ErrInsufficientBalance
		} else {
			allowed, inflight, err = rc.ReserveInflightBalance(reserveCtx, user.ID, requestID, estimate, balance, ttl)
		}
	} else {
		allowed, inflight, err = rc.ReserveInflightBalance(reserveCtx, user.ID, requestID, estimate, balance, ttl)
	}
	cancel()
	if err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation failed for user %d (fail-open): %v", user.ID, err)
		return nil, nil
	}
	if !allowed {
		logger.LegacyPrintf("service.billing_cache", "inflight reservation rejected: user=%d balance=%.8f inflight=%.8f estimate=%.8f", user.ID, balance, inflight, estimate)
		return nil, ErrInsufficientBalance
	}

	r := &InflightReservation{cache: rc, userID: user.ID, requestID: requestID, amount: estimate, ttl: ttl}
	r.refs.Store(1)
	if renewer, ok := s.cache.(InflightBalanceReservationRenewer); renew && ok && renewer != nil {
		r.startRenewal(renewer)
	}
	return r, nil
}

// ============================================
// 费用估算：与 RecordUsage 同一套计费函数
// ============================================

// InflightEstimateKind 估算口径。
type InflightEstimateKind int

const (
	// InflightEstimateToken 文本 / 对话类：输入估算 + 输出上限。
	InflightEstimateToken InflightEstimateKind = iota
	// InflightEstimateImage 图片生成端点：按张计（取 1K/2K/4K 最高价 × 张数），同时估算 token 计价渠道。
	InflightEstimateImage
	// InflightEstimateVideo 视频生成：按秒 × 条数。
	InflightEstimateVideo
)

// InflightEstimateRequest 单请求估算输入。
type InflightEstimateRequest struct {
	Model     string
	BodyBytes int
	MaxTokens int
	Kind      InflightEstimateKind
	// ServiceTier 请求声明的档位（OpenAI service_tier；Anthropic speed=fast 归一为 "fast"）。
	// 结算只降不升（ResolveBillingServiceTier），所以请求档位是上界。
	ServiceTier string
	// Units 图片张数 / 视频条数（<=0 视为 1）。
	Units int
	// ImageEndpoint 来自 /v1/images/*（分组按张计费只对它生效）。
	ImageEndpoint bool
	// MayGenerateImage 文本端点可能产出图片（Responses image_generation 工具、Gemini 图片模型），
	// 同时估算按张口径并取较高者。
	MayGenerateImage bool
	// HasImageInput 请求带图片输入：额外按「全部输入都是图片 token」估一次（图片输入价可能高于文本价）。
	HasImageInput bool
	// 视频：分辨率与时长（秒）。
	VideoResolution      string
	VideoDurationSeconds int
}

// inflightSyntheticUsage 是喂给 RecordUsage 计费函数的合成用量。
type inflightSyntheticUsage struct {
	Tokens               UsageTokens
	ServiceTier          string
	ImageCount           int
	ImageSize            string
	ImageEndpoint        bool
	VideoCount           int
	VideoResolution      string
	VideoDurationSeconds int
}

// inflightRates 与 RecordUsage 同源的倍率（用户专属倍率优先，分组兜底，系统默认最后）。
type inflightRates struct {
	text, image, video float64
}

// inflightLegacyCostFunc 按 RecordUsage 的 legacy 计费路径（销售定价 v2 之前）计算一个计费模型的费用。
// 返回 error 表示该模型无法定价。
type inflightLegacyCostFunc func(ctx context.Context, apiKey *APIKey, model string, rates inflightRates, usage inflightSyntheticUsage) (*CostBreakdown, error)

// inflightCostEstimator 两种网关共用的估算器。只负责「挑计费模型 + 构造合成用量 + 取最高」，
// 金额本身全部来自 legacyCost（RecordUsage 的计费函数）和 ResolveV2SalesPricingContextAndCost。
type inflightCostEstimator struct {
	cfg            *config.Config
	billing        *BillingService
	resolver       *ModelPricingResolver
	settings       *SettingService
	channels       *ChannelService
	resolveMapping func(ctx context.Context, groupID int64, model string) ChannelMappingResult
	userGroupRate  func(ctx context.Context, userID, groupID int64, groupDefault float64) float64
	// accountMappedModels 返回调度器快照里可选账号对 model 的账号级映射结果（去重，不含 model 本身）。
	// 仅在请求模型 / 渠道映射模型都无法定价时调用（同上游 b886fb8ed）。
	accountMappedModels func(ctx context.Context, apiKey *APIKey, model string) []string
	legacyCost          inflightLegacyCostFunc
}

func inflightReservationCfg(cfg *config.Config) config.InflightReservationConfig {
	if cfg == nil {
		return config.InflightReservationConfig{}
	}
	return cfg.Billing.InflightReservation
}

func inflightTokenCounts(cfg config.InflightReservationConfig, bodyBytes, maxTokens int) (int, int) {
	inputTokens := 0
	if bodyBytes > 0 {
		inputTokens = bodyBytes / inflightInputBytesPerTokenEstimate
	}
	if cfg.MaxInputTokens > 0 && inputTokens > cfg.MaxInputTokens {
		inputTokens = cfg.MaxInputTokens
	}
	outputTokens := maxTokens
	if outputTokens <= 0 {
		outputTokens = cfg.DefaultMaxTokens
		if outputTokens <= 0 {
			outputTokens = defaultInflightDefaultMaxTokens
		}
	}
	if cfg.MaxOutputTokens > 0 && outputTokens > cfg.MaxOutputTokens {
		outputTokens = cfg.MaxOutputTokens
	}
	return inputTokens, outputTokens
}

func validInflightCost(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

var inflightUnpricedLastLog atomic.Int64

func logInflightUnpriced(model string, groupID *int64) {
	now := time.Now().UnixNano()
	last := inflightUnpricedLastLog.Load()
	if now-last < int64(inflightUnpricedLogInterval) || !inflightUnpricedLastLog.CompareAndSwap(last, now) {
		return
	}
	var gid int64
	if groupID != nil {
		gid = *groupID
	}
	logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation cannot price model=%q group=%d; request admitted without reservation (fail-open, throttled log)", model, gid)
}

// rates 与 RecordUsage 相同：cfg.Default.RateMultiplier → 分组倍率 → 用户专属倍率；图片 / 视频独立倍率。
func (e inflightCostEstimator) rates(ctx context.Context, apiKey *APIKey) inflightRates {
	text := 1.0
	if e.cfg != nil {
		text = e.cfg.Default.RateMultiplier
	}
	if apiKey != nil && apiKey.GroupID != nil && apiKey.Group != nil {
		text = apiKey.Group.RateMultiplier
		if e.userGroupRate != nil && apiKey.User != nil {
			text = e.userGroupRate(ctx, apiKey.User.ID, *apiKey.GroupID, text)
		}
	}
	return inflightRates{
		text:  text,
		image: resolveImageRateMultiplier(apiKey, text),
		video: resolveVideoRateMultiplier(apiKey, text),
	}
}

// billingModelCandidates 与 RecordUsage 的计费模型选择同口径：
//   - channel_mapped / upstream / 未配置：计费模型可能是请求模型或映射模型（upstream 在准入时未知），
//     两者都是首选，取最高；
//   - requested：计费模型是请求模型；OpenAI 计费在它无价时按候选顺序回退到映射模型，所以映射模型作为回退。
func (e inflightCostEstimator) billingModelCandidates(ctx context.Context, apiKey *APIKey, model string) (primary, fallbacks []string, upstreamInput string) {
	upstreamInput = model
	primary = []string{model}
	if apiKey == nil || apiKey.GroupID == nil || e.resolveMapping == nil {
		return primary, nil, upstreamInput
	}
	m := e.resolveMapping(ctx, *apiKey.GroupID, model)
	mapped := strings.TrimSpace(m.MappedModel)
	if mapped == "" || mapped == model {
		return primary, nil, upstreamInput
	}
	upstreamInput = mapped
	if m.BillingModelSource == BillingModelSourceRequested {
		return primary, []string{mapped}, upstreamInput
	}
	return []string{model, mapped}, nil, upstreamInput
}

// syntheticUsages 按请求口径构造合成用量；每个合成用量独立计价后取最高。
func (e inflightCostEstimator) syntheticUsages(req InflightEstimateRequest) []inflightSyntheticUsage {
	cfg := inflightReservationCfg(e.cfg)
	units := req.Units
	if units <= 0 {
		units = 1
	}
	inputTokens, outputTokens := inflightTokenCounts(cfg, req.BodyBytes, req.MaxTokens)
	sizes := []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K}
	var out []inflightSyntheticUsage
	switch req.Kind {
	case InflightEstimateVideo:
		out = append(out, inflightSyntheticUsage{
			Tokens:               UsageTokens{InputTokens: inputTokens},
			VideoCount:           units,
			VideoResolution:      req.VideoResolution,
			VideoDurationSeconds: req.VideoDurationSeconds,
		})
	case InflightEstimateImage:
		// token 计价渠道：每张图按一份输出上限估算图片输出 token。
		imageTokens := UsageTokens{InputTokens: inputTokens, OutputTokens: outputTokens * units, ImageOutputTokens: outputTokens * units}
		for _, size := range sizes {
			out = append(out, inflightSyntheticUsage{
				Tokens:        imageTokens,
				ServiceTier:   req.ServiceTier,
				ImageCount:    units,
				ImageSize:     size,
				ImageEndpoint: req.ImageEndpoint,
			})
		}
	default:
		base := inflightSyntheticUsage{Tokens: UsageTokens{InputTokens: inputTokens, OutputTokens: outputTokens}, ServiceTier: req.ServiceTier}
		out = append(out, base)
		if req.HasImageInput && inputTokens > 0 {
			withImage := base
			withImage.Tokens.ImageInputTokens = inputTokens
			out = append(out, withImage)
		}
		if req.MayGenerateImage {
			for _, size := range sizes {
				img := base
				img.ImageCount = units
				img.ImageSize = size
				out = append(out, img)
			}
		}
	}
	return out
}

// salesPricingV2Active 与 ApplyConfiguredSalesPricing 同口径：解析器开启且版本为 v2 时，
// 用户扣费改由 ResolveV2SalesPricingContextAndCost 决定。
func (e inflightCostEstimator) salesPricingV2Active(ctx context.Context) bool {
	if e.settings == nil {
		return false
	}
	return e.settings.IsSalesPricingResolverEnabled(ctx) && e.settings.GetSalesPricingVersion(ctx) == SalesPricingVersionV2
}

// v2Cost 构造合成 UsageLog，调用 RecordUsage 在销售定价 v2 下使用的同一个函数。
func (e inflightCostEstimator) v2Cost(ctx context.Context, apiKey *APIKey, model string, rates inflightRates, usage inflightSyntheticUsage) (float64, bool) {
	if e.resolver == nil || e.billing == nil || apiKey == nil {
		return 0, false
	}
	log := &UsageLog{
		InputTokens:           usage.Tokens.InputTokens,
		ImageInputTokens:      usage.Tokens.ImageInputTokens,
		OutputTokens:          usage.Tokens.OutputTokens,
		CacheCreationTokens:   usage.Tokens.CacheCreationTokens,
		CacheReadTokens:       usage.Tokens.CacheReadTokens,
		CacheCreation5mTokens: usage.Tokens.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.Tokens.CacheCreation1hTokens,
		ImageOutputTokens:     usage.Tokens.ImageOutputTokens,
		ImageCount:            usage.ImageCount,
		VideoCount:            usage.VideoCount,
		CreatedAt:             time.Now(),
	}
	if usage.ImageSize != "" {
		size := usage.ImageSize
		log.ImageSize = &size
	}
	if usage.VideoCount > 0 {
		duration := NormalizeVideoBillingDurationSecondsOrDefault(usage.VideoDurationSeconds)
		log.VideoDurationSeconds = &duration
	}
	if tier := strings.TrimSpace(usage.ServiceTier); tier != "" {
		log.ServiceTier = &tier
	}
	_, cost, err := ResolveV2SalesPricingContextAndCost(ctx, e.resolver, e.channels, e.billing, apiKey.GroupID, model, log, decimal.NewFromFloat(rates.text))
	if err != nil || cost == nil {
		return 0, false
	}
	return cost.ActualCost, true
}

// costOf 一个计费模型、一份合成用量的费用：legacy 计费路径；销售定价 v2 开启时再取 v2 结果的较高者。
// 第二个返回值表示是否成功定价（免费渠道可以是 0 且已定价）。
func (e inflightCostEstimator) costOf(ctx context.Context, apiKey *APIKey, model string, rates inflightRates, usage inflightSyntheticUsage, v2 bool) (float64, bool) {
	best, priced := 0.0, false
	if e.legacyCost != nil {
		if cost, err := e.legacyCost(ctx, apiKey, model, rates, usage); err == nil && cost != nil {
			best, priced = cost.ActualCost, true
		}
	}
	if v2 {
		if c, ok := e.v2Cost(ctx, apiKey, model, rates, usage); ok {
			if !priced || c > best {
				best = c
			}
			priced = true
		}
	}
	if math.IsNaN(best) || math.IsInf(best, 0) || best < 0 {
		return 0, false
	}
	return best, priced
}

// estimate 返回保守的单请求费用（USD，已乘倍率、已量化到 8 位）；无法定价返回 (0,false)。
func (e inflightCostEstimator) estimate(ctx context.Context, apiKey *APIKey, req InflightEstimateRequest) (float64, bool) {
	if apiKey == nil || apiKey.User == nil {
		return 0, false
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		// 非计量请求（媒体状态查询等）：无需预占，也不算「无法定价」。
		return 0, true
	}
	rates := e.rates(ctx, apiKey)
	if rates.text <= 0 && rates.image <= 0 && rates.video <= 0 {
		// 免费分组：不计费，也无需预占。
		return 0, true
	}
	usages := e.syntheticUsages(req)
	v2 := e.salesPricingV2Active(ctx)
	bestOf := func(models []string) (float64, bool) {
		best, anyPriced := 0.0, false
		for _, m := range models {
			for _, usage := range usages {
				if c, ok := e.costOf(ctx, apiKey, m, rates, usage, v2); ok {
					anyPriced = true
					if c > best {
						best = c
					}
				}
			}
		}
		return best, anyPriced
	}
	primary, fallbacks, upstreamInput := e.billingModelCandidates(ctx, apiKey, req.Model)
	best, priced := bestOf(primary)
	if !priced && len(fallbacks) > 0 {
		best, priced = bestOf(fallbacks)
	}
	if !priced && e.accountMappedModels != nil {
		// 与计费侧同口径：请求 / 渠道映射模型都无价时，计费模型是账号映射后的实际转发模型。
		best, priced = bestOf(e.accountMappedModels(ctx, apiKey, upstreamInput))
	}
	if !priced {
		logInflightUnpriced(req.Model, apiKey.GroupID)
		return 0, false
	}
	if !validInflightCost(best) {
		return 0, true
	}
	return QuantizeUsageBillingAmount(best), true
}

func accountMappedModelsFrom(accounts []Account, model string) []string {
	var seen map[string]struct{}
	var out []string
	for i := range accounts {
		mapped, matched := accounts[i].ResolveMappedModel(model)
		mapped = strings.TrimSpace(mapped)
		if !matched || mapped == "" || mapped == model {
			continue
		}
		if seen == nil {
			seen = map[string]struct{}{}
		}
		if _, dup := seen[mapped]; dup {
			continue
		}
		seen[mapped] = struct{}{}
		out = append(out, mapped)
	}
	return out
}

// --- GatewayService（Anthropic / Gemini / Antigravity 及其兼容端点） ---

// inflightLegacyCost 构造合成 ForwardResult，调用 RecordUsage 使用的 calculateRecordUsageCost 同一分派：
// ImageCount>0 走 calculateImageCost（渠道按次/图片价 → 分组图片价 → 目录默认价），否则走
// calculateTokenCost（渠道定价 → CalculateCostUnified，否则 CalculateCostForGroup）。
func (s *GatewayService) inflightLegacyCost(ctx context.Context, apiKey *APIKey, model string, rates inflightRates, usage inflightSyntheticUsage) (*CostBreakdown, error) {
	result := &ForwardResult{
		Model: model,
		Usage: ClaudeUsage{
			InputTokens:              usage.Tokens.InputTokens,
			OutputTokens:             usage.Tokens.OutputTokens,
			CacheCreationInputTokens: usage.Tokens.CacheCreationTokens,
			CacheReadInputTokens:     usage.Tokens.CacheReadTokens,
			CacheCreation5mTokens:    usage.Tokens.CacheCreation5mTokens,
			CacheCreation1hTokens:    usage.Tokens.CacheCreation1hTokens,
			ImageOutputTokens:        usage.Tokens.ImageOutputTokens,
		},
		ImageCount: usage.ImageCount,
		ImageSize:  usage.ImageSize,
	}
	if tier := strings.TrimSpace(usage.ServiceTier); tier != "" {
		result.ServiceTier = &tier
	}
	if result.ImageCount > 0 {
		return s.calculateImageCost(ctx, result, apiKey, model, rates.image), nil
	}
	return s.calculateTokenCostE(ctx, result, apiKey, model, rates.text)
}

// inflightAccountMappedModels 从调度器快照（与选号同源）读取账号级映射候选；没有快照时不回源查库。
func (s *GatewayService) inflightAccountMappedModels(ctx context.Context, apiKey *APIKey, model string) []string {
	if s == nil || s.schedulerSnapshot == nil || apiKey == nil {
		return nil
	}
	platform, forced, err := s.resolvePlatform(ctx, apiKey.GroupID, apiKey.Group)
	if err != nil || platform == "" {
		return nil
	}
	accounts, _, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, apiKey.GroupID, platform, forced)
	if err != nil {
		return nil
	}
	return accountMappedModelsFrom(accounts, model)
}

func (s *GatewayService) inflightEstimator() inflightCostEstimator {
	e := inflightCostEstimator{
		cfg:                 s.cfg,
		billing:             s.billingService,
		resolver:            s.resolver,
		settings:            s.settingService,
		channels:            s.channelService,
		userGroupRate:       s.getUserGroupRateMultiplier,
		accountMappedModels: s.inflightAccountMappedModels,
		legacyCost:          s.inflightLegacyCost,
	}
	if s.channelService != nil {
		e.resolveMapping = s.channelService.ResolveChannelMapping
	}
	return e
}

// EstimateInflightReservation 用 RecordUsage 的计费函数估算在途预占金额。
// 第二个返回值为 false 表示无法定价（调用方 fail-open 或按配置 fail-closed）。
func (s *GatewayService) EstimateInflightReservation(ctx context.Context, apiKey *APIKey, req InflightEstimateRequest) (float64, bool) {
	if s == nil {
		return 0, false
	}
	return s.inflightEstimator().estimate(ctx, apiKey, req)
}

// --- OpenAIGatewayService（OpenAI / Grok 及其兼容端点） ---

// inflightLegacyCost 构造合成 OpenAIForwardResult，调用 RecordUsage 使用的 calculateOpenAIRecordUsageCost：
// 视频 → calculateOpenAIVideoCost；图片 → 按张开关 / 渠道价 / calculateOpenAIImageCost；
// 其他 → calculateOpenAIRecordUsageTokenCost（CalculateCostUnified 或 CalculateCostForGroup）。
// 候选模型由估算器逐个传入（单元素 billingModels），回退顺序由估算器决定。
func (s *OpenAIGatewayService) inflightLegacyCost(ctx context.Context, apiKey *APIKey, model string, rates inflightRates, usage inflightSyntheticUsage) (*CostBreakdown, error) {
	result := &OpenAIForwardResult{
		Model: model,
		Usage: OpenAIUsage{
			InputTokens:              usage.Tokens.InputTokens,
			ImageInputTokens:         usage.Tokens.ImageInputTokens,
			OutputTokens:             usage.Tokens.OutputTokens,
			CacheCreationInputTokens: usage.Tokens.CacheCreationTokens,
			CacheReadInputTokens:     usage.Tokens.CacheReadTokens,
			ImageOutputTokens:        usage.Tokens.ImageOutputTokens,
		},
		ImageCount:           usage.ImageCount,
		ImageEndpoint:        usage.ImageEndpoint,
		ImageSize:            usage.ImageSize,
		VideoCount:           usage.VideoCount,
		VideoResolution:      usage.VideoResolution,
		VideoDurationSeconds: usage.VideoDurationSeconds,
	}
	tier := strings.TrimSpace(usage.ServiceTier)
	if tier != "" {
		result.ServiceTier = &tier
	}
	return s.calculateOpenAIRecordUsageCost(ctx, result, apiKey, []string{model}, rates.text, rates.image, rates.video, usageTokensFromOpenAIUsage(result.Usage), tier)
}

// inflightAccountMappedModels 从调度器快照读取账号级映射候选；没有快照时不回源查库。
func (s *OpenAIGatewayService) inflightAccountMappedModels(ctx context.Context, apiKey *APIKey, model string) []string {
	if s == nil || s.schedulerSnapshot == nil || apiKey == nil {
		return nil
	}
	platform := PlatformOpenAI
	if apiKey.Group != nil && strings.TrimSpace(apiKey.Group.Platform) != "" {
		platform = apiKey.Group.Platform
	}
	accounts, _, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, apiKey.GroupID, platform, false)
	if err != nil {
		return nil
	}
	return accountMappedModelsFrom(accounts, model)
}

func (s *OpenAIGatewayService) inflightUserGroupRate(ctx context.Context, userID, groupID int64, groupDefault float64) float64 {
	resolver := s.userGroupRateResolver
	if resolver == nil {
		resolver = newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.openai_gateway")
	}
	return resolver.Resolve(ctx, userID, groupID, groupDefault)
}

func (s *OpenAIGatewayService) inflightEstimator() inflightCostEstimator {
	e := inflightCostEstimator{
		cfg:                 s.cfg,
		billing:             s.billingService,
		resolver:            s.resolver,
		settings:            s.settingService,
		channels:            s.channelService,
		userGroupRate:       s.inflightUserGroupRate,
		accountMappedModels: s.inflightAccountMappedModels,
		legacyCost:          s.inflightLegacyCost,
	}
	if s.channelService != nil {
		e.resolveMapping = s.channelService.ResolveChannelMapping
	}
	return e
}

// EstimateInflightReservation 同 GatewayService.EstimateInflightReservation（OpenAI 网关计费口径）。
func (s *OpenAIGatewayService) EstimateInflightReservation(ctx context.Context, apiKey *APIKey, req InflightEstimateRequest) (float64, bool) {
	if s == nil {
		return 0, false
	}
	return s.inflightEstimator().estimate(ctx, apiKey, req)
}
