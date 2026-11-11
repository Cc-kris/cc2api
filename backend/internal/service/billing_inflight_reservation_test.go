//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// 移植上游 43f88c99a / 52c123e6f / 652a7d74f 的 billing_inflight_reservation_test.go。
// 改动：上游 EstimateInflightReservationCost（手写公式）不移植，估算一律走 RecordUsage 的计费函数；
// 上游的 search / audio / composite 用例在 CCAI 没有对应能力，不移植。

func TestReserveInflightBalance_NoReservationCacheFailsOpen(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	svc := NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)
	release, err := svc.ReserveInflightBalance(context.Background(), &User{ID: 1}, nil, nil, 100)
	require.NoError(t, err)
	release()
}

func TestReserveInflightBalance_SimpleModeDisabled(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Billing.InflightReservation.Enabled = true
	svc := NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)
	require.False(t, svc.InflightReservationEnabled())
}

func TestInflightTokenCounts(t *testing.T) {
	cfg := config.InflightReservationConfig{DefaultMaxTokens: 8192, MaxOutputTokens: 64000, MaxInputTokens: 1000}
	in, out := inflightTokenCounts(cfg, 400, 1000)
	require.Equal(t, 100, in)
	require.Equal(t, 1000, out)
	in, out = inflightTokenCounts(cfg, 0, 0)
	require.Equal(t, 0, in)
	require.Equal(t, 8192, out, "missing max_tokens → default")
	in, out = inflightTokenCounts(cfg, 1_000_000, 1_000_000)
	require.Equal(t, 1000, in, "input capped")
	require.Equal(t, 64000, out, "output capped")
	_, out = inflightTokenCounts(config.InflightReservationConfig{}, 0, 0)
	require.Equal(t, defaultInflightDefaultMaxTokens, out)
}

// ---------------------------------------------------------------------------
// estimate: same model / resolver as billing
// ---------------------------------------------------------------------------

func newInflightEstimateGateway(t *testing.T, channelService *ChannelService, groups GroupRepository) *GatewayService {
	t.Helper()
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, DefaultMaxTokens: 1000, MaxInputTokens: 200000, MaxOutputTokens: 128000}
	billing, _ := newShippedCatalogBillingService(t)
	return &GatewayService{
		cfg:            cfg,
		billingService: billing,
		resolver:       NewModelPricingResolver(channelService, billing, groups),
		channelService: channelService,
	}
}

func TestInflightEstimate_ChannelAliasUsesMappedModel(t *testing.T) {
	groupID := int64(10)
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{groupID},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"my-alias": "claude-sonnet-4-5"},
		},
	}
	cs := newTestChannelService(makeStandardRepo(ch, map[int64]string{groupID: "anthropic"}))
	svc := newInflightEstimateGateway(t, cs, nil)
	apiKey := &APIKey{User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 1}}

	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "my-alias", BodyBytes: 4000, MaxTokens: 1000})
	require.True(t, priced)
	require.Greater(t, est, 0.0)

	direct, _ := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-sonnet-4-5", BodyBytes: 4000, MaxTokens: 1000})
	require.InDelta(t, direct, est, 1e-12, "alias must be estimated as the channel-mapped billing model")
	require.InDelta(t, 1000*3e-6+1000*1.5e-5, est, 1e-12)
}

func TestInflightEstimate_ChannelPerRequestPricing(t *testing.T) {
	groupID := int64(20)
	price := 0.5
	ch := Channel{ID: 2, Status: StatusActive, GroupIDs: []int64{groupID}, ModelPricing: []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"custom-per-request"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price,
	}}}
	cs := newTestChannelService(makeStandardRepo(ch, map[int64]string{groupID: "anthropic"}))
	svc := newInflightEstimateGateway(t, cs, nil)
	apiKey := &APIKey{User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 2}}

	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "custom-per-request", BodyBytes: 100})
	require.True(t, priced)
	require.InDelta(t, 1.0, est, 1e-12, "per_request 0.5 × group rate 2")
}

func TestInflightEstimate_UserGroupRateOverridesGroupRate(t *testing.T) {
	groupID := int64(21)
	svc := newInflightEstimateGateway(t, nil, nil)
	svc.userGroupRateResolver = newUserGroupRateResolver(&inflightUserGroupRateRepo{rate: 3}, nil, time.Minute, nil, "test")
	apiKey := &APIKey{User: &User{ID: 5}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 1}}
	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-sonnet-4-6", BodyBytes: 4000, MaxTokens: 1000})
	require.True(t, priced)
	require.InDelta(t, (1000*3e-6+1000*1.5e-5)*3, est, 1e-12)
}

func TestInflightEstimate_UnpricedIsReportedAndNonMeteredIsNot(t *testing.T) {
	svc := newInflightEstimateGateway(t, nil, nil)
	apiKey := &APIKey{User: &User{ID: 1}}
	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "totally-unknown-model-xyz"})
	require.False(t, priced)
	require.Zero(t, est)

	est, priced = svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{})
	require.True(t, priced, "non-metered requests (e.g. media status lookups) are not 'unpriced'")
	require.Zero(t, est)

	// 免费分组：不计费也不算无法定价。
	gid := int64(3)
	free := &APIKey{User: &User{ID: 1}, GroupID: &gid, Group: &Group{ID: gid, RateMultiplier: 0}}
	est, priced = svc.EstimateInflightReservation(context.Background(), free, InflightEstimateRequest{Model: "claude-sonnet-4-6", BodyBytes: 4000})
	require.True(t, priced)
	require.Zero(t, est)
}

func TestInflightEstimate_ImageKindUsesHighestTier(t *testing.T) {
	groupID := int64(30)
	p4k := 0.3
	group := &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1, ImagePrice4K: &p4k}
	svc := newInflightEstimateGateway(t, nil, nil)
	apiKey := &APIKey{User: &User{ID: 1}, GroupID: &groupID, Group: group}

	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "gemini-3-pro-image-preview", Kind: InflightEstimateImage, Units: 2})
	require.True(t, priced)
	require.InDelta(t, 0.6, est, 1e-12, "image estimate uses the highest size tier × n")
}

// 销售定价 v2 开启：估算取 v2 价（与 ApplyConfiguredSalesPricing 同一函数）。
func TestInflightEstimate_SalesPricingV2UsesV2Cost(t *testing.T) {
	groupID := int64(40)
	ch := Channel{ID: 4, Status: StatusActive, GroupIDs: []int64{groupID}, ModelPricing: []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4-6"}, BillingMode: BillingModeToken,
		InputPrice: f64(6e-6), OutputPrice: f64(3e-5), CacheWritePrice: f64(7.5e-6), CacheReadPrice: f64(6e-7),
	}}}
	cs := newTestChannelService(makeStandardRepo(ch, map[int64]string{groupID: "anthropic"}))
	svc := newInflightEstimateGateway(t, cs, nil)
	svc.settingService = NewSettingService(&inflightSettingRepo{values: map[string]string{
		SettingKeySalesPricingVersion: string(SalesPricingVersionV2),
	}}, svc.cfg)
	apiKey := &APIKey{User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 1}}
	est, priced := svc.EstimateInflightReservation(context.Background(), apiKey, InflightEstimateRequest{Model: "claude-sonnet-4-6", BodyBytes: 4000, MaxTokens: 1000})
	require.True(t, priced)
	require.InDelta(t, 1000*6e-6+1000*3e-5, est, 1e-12)
}

type inflightUserGroupRateRepo struct {
	UserGroupRateRepository
	rate float64
}

func (r *inflightUserGroupRateRepo) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	v := r.rate
	return &v, nil
}

type inflightSettingRepo struct {
	SettingRepository
	values map[string]string
}

func (r *inflightSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

// 账号级映射：请求模型和渠道映射都无价时，按调度器快照里候选账号映射模型的最高估算。
func TestInflightEstimate_AccountLevelMappingFallsBackLikeBilling(t *testing.T) {
	groupID := int64(31)
	svc := newInflightEstimateGateway(t, nil, nil)
	svc.schedulerSnapshot = &SchedulerSnapshotService{cache: &inflightSnapshotCache{accounts: []*Account{
		{ID: 1, Platform: PlatformAnthropic, Credentials: map[string]any{"model_mapping": map[string]any{"acct-alias-31": "claude-sonnet-4-5"}}},
		{ID: 2, Platform: PlatformAnthropic, Credentials: map[string]any{"model_mapping": map[string]any{"acct-alias-31": "claude-opus-4-1"}}},
		{ID: 3, Platform: PlatformAnthropic},
	}}}
	apiKey := &APIKey{User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 1}}
	ctx := context.Background()

	est, priced := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: "acct-alias-31", BodyBytes: 4000, MaxTokens: 1000})
	require.True(t, priced)
	opus, _ := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: "claude-opus-4-1", BodyBytes: 4000, MaxTokens: 1000})
	sonnet, _ := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: "claude-sonnet-4-5", BodyBytes: 4000, MaxTokens: 1000})
	require.Greater(t, opus, sonnet)
	require.InDelta(t, opus, est, 1e-12, "estimate = max over candidate account-mapped billing models")
}

type inflightSnapshotCache struct {
	SchedulerCache
	accounts []*Account
}

func (c *inflightSnapshotCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	return c.accounts, true, nil
}

// ---------------------------------------------------------------------------
// reservation handle: hand-off to billing task, renewal
// ---------------------------------------------------------------------------

type memInflightCache struct {
	BillingCache
	mu        sync.Mutex
	balance   float64
	res       map[string]float64
	exp       map[string]time.Time
	renews    atomic.Int32
	deducts   atomic.Int32
	releaseCt atomic.Int32
	failRes   bool
}

func newMemInflightCache(balance float64) *memInflightCache {
	return &memInflightCache{balance: balance, res: map[string]float64{}, exp: map[string]time.Time{}}
}

func (m *memInflightCache) GetUserBalance(context.Context, int64) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.balance, nil
}

func (m *memInflightCache) DeductUserBalance(_ context.Context, _ int64, amount float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balance -= amount
	m.deducts.Add(1)
	return nil
}

func (m *memInflightCache) gc() {
	now := time.Now()
	for id, e := range m.exp {
		if !e.After(now) {
			delete(m.exp, id)
			delete(m.res, id)
		}
	}
}

func (m *memInflightCache) reserve(id string, amount, balance float64, ttl time.Duration, cover bool) (bool, float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRes {
		return false, 0, errors.New("redis down")
	}
	m.gc()
	sum := 0.0
	for _, v := range m.res {
		sum += v
	}
	if (len(m.res) > 0 || cover) && balance-sum < amount {
		return false, sum, nil
	}
	m.res[id] = amount
	m.exp[id] = time.Now().Add(ttl)
	return true, sum, nil
}

func (m *memInflightCache) ReserveInflightBalance(_ context.Context, _ int64, id string, amount, balance float64, ttl time.Duration) (bool, float64, error) {
	return m.reserve(id, amount, balance, ttl, false)
}

func (m *memInflightCache) ReserveInflightBalanceCovered(_ context.Context, _ int64, id string, amount, balance float64, ttl time.Duration) (bool, float64, error) {
	return m.reserve(id, amount, balance, ttl, true)
}

func (m *memInflightCache) ReleaseInflightBalance(_ context.Context, _ int64, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.res, id)
	delete(m.exp, id)
	m.releaseCt.Add(1)
	return nil
}

func (m *memInflightCache) RenewInflightBalance(_ context.Context, _ int64, id string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renews.Add(1)
	if _, ok := m.res[id]; !ok {
		return false, nil
	}
	m.exp[id] = time.Now().Add(ttl)
	return true, nil
}

func (m *memInflightCache) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gc()
	return len(m.res)
}

func newInflightSvc(t *testing.T, cache BillingCache, ttlSeconds int) *BillingCacheService {
	t.Helper()
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: ttlSeconds}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)
	return svc
}

func TestInflightReservation_HeldUntilBillingTaskDone(t *testing.T) {
	cache := newMemInflightCache(1)
	svc := newInflightSvc(t, cache, 60)
	user := &User{ID: 1}

	res, err := svc.ReserveInflight(context.Background(), user, nil, nil, 0.9)
	require.NoError(t, err)
	require.NotNil(t, res)
	taskDone := res.Acquire() // billing task submitted
	res.HandlerDone()         // handler returned
	require.Equal(t, 1, cache.count(), "reservation must survive handler return while billing is pending")

	_, err = svc.ReserveInflight(context.Background(), user, nil, nil, 0.9)
	require.ErrorIs(t, err, ErrInsufficientBalance)

	taskDone()
	taskDone() // idempotent
	require.Equal(t, 0, cache.count())
	require.Equal(t, int32(1), cache.releaseCt.Load(), "released exactly once")

	// No billing task → HandlerDone releases immediately; Acquire after release is a no-op.
	res2, err := svc.ReserveInflight(context.Background(), user, nil, nil, 0.1)
	require.NoError(t, err)
	res2.HandlerDone()
	res2.HandlerDone()
	require.Equal(t, 0, cache.count())
	res2.Acquire()()
	require.Equal(t, int32(2), cache.releaseCt.Load())

	// Nil handle is safe.
	var nilRes *InflightReservation
	nilRes.Acquire()()
	nilRes.HandlerDone()
	nilRes.Release()
}

func TestInflightReservation_RenewedWhileHandlerActive(t *testing.T) {
	cache := newMemInflightCache(1)
	svc := newInflightSvc(t, cache, 1)
	user := &User{ID: 2}

	res, err := svc.ReserveInflight(context.Background(), user, nil, nil, 0.9)
	require.NoError(t, err)
	time.Sleep(2500 * time.Millisecond) // > 2 × TTL
	require.Equal(t, 1, cache.count(), "renewal keeps the reservation alive past its TTL")
	require.Greater(t, cache.renews.Load(), int32(3))
	_, err = svc.ReserveInflight(context.Background(), user, nil, nil, 0.9)
	require.ErrorIs(t, err, ErrInsufficientBalance)

	done := res.Acquire()
	res.HandlerDone()
	renewsAtStop := cache.renews.Load()
	time.Sleep(1200 * time.Millisecond)
	require.Equal(t, renewsAtStop, cache.renews.Load(), "no renewal after handler end")
	require.Equal(t, 0, cache.count(), "post-handler hold is bounded by TTL")
	done()
}

func TestSyncBalanceCacheAfterDeduction_SynchronousWhenInflightEnabled(t *testing.T) {
	cache := newMemInflightCache(1)
	svc := newInflightSvc(t, cache, 60)
	p := &postUsageBillingParams{Cost: &CostBreakdown{ActualCost: 0.25}, User: &User{ID: 3}}
	syncBalanceCacheAfterDeduction(context.Background(), p, &billingDeps{billingCacheService: svc})
	require.Equal(t, int32(1), cache.deducts.Load(), "cache deduction must land before the billing task returns")
	bal, _ := cache.GetUserBalance(context.Background(), 3)
	require.InDelta(t, 0.75, bal, 1e-12)
}

// AC34：余额 0.01 的单个请求放行（无在途）；订阅分组不预占；Redis 故障放行不预占；
// estimate=0 不预占。
func TestInflightReservation_AC34(t *testing.T) {
	ctx := context.Background()
	cache := newMemInflightCache(0.01)
	svc := newInflightSvc(t, cache, 60)
	user := &User{ID: 34}

	res, err := svc.ReserveInflight(ctx, user, nil, nil, 0.15288)
	require.NoError(t, err, "单个请求无在途：放行（与现在一致）")
	require.NotNil(t, res)
	res.HandlerDone()

	group := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription}
	res, err = svc.ReserveInflight(ctx, user, group, &UserSubscription{ID: 1}, 10)
	require.NoError(t, err)
	require.Nil(t, res, "订阅分组不预占")
	require.Zero(t, cache.count())

	cache.failRes = true
	for i := 0; i < 3; i++ {
		res, err = svc.ReserveInflight(ctx, user, nil, nil, 10)
		require.NoError(t, err, "Redis 故障放行")
		require.Nil(t, res, "Redis 故障不预占")
	}
	cache.failRes = false

	res, err = svc.ReserveInflight(ctx, user, nil, nil, 0)
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestInflightReservation_MaxReservationCapsEstimateNotFixedCharge(t *testing.T) {
	ctx := context.Background()
	cache := newMemInflightCache(10)
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60, MaxReservationUSD: 0.5}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	res, err := svc.ReserveInflight(ctx, &User{ID: 1}, nil, nil, 3)
	require.NoError(t, err)
	require.InDelta(t, 0.5, res.Amount(), 1e-12, "estimate capped by max_reservation_usd")
	res.HandlerDone()

	res, err = svc.ReserveFixedCharge(ctx, &User{ID: 1}, nil, nil, 3)
	require.NoError(t, err)
	require.InDelta(t, 3, res.Amount(), 1e-12, "fixed charge is exact and not capped")
	res.HandlerDone()
}

// ---------------------------------------------------------------------------
// AC33 端到端：估算 → 预占 → RecordUsage（真实计费路径）→ 计费后释放
// ---------------------------------------------------------------------------

type inflightBalanceBillingRepo struct {
	mu      sync.Mutex
	balance float64
	applied int
}

func (r *inflightBalanceBillingRepo) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.balance -= cmd.BalanceCost
	r.applied++
	return &UsageBillingApplyResult{Applied: true}, nil
}

func runAC33(t *testing.T, enabled bool) (admitted, rejected int32, dbBalance float64, cache *memInflightCache) {
	t.Helper()
	cache = newMemInflightCache(1)
	bcCfg := &config.Config{}
	bcCfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: enabled, TTLSeconds: 60, DefaultMaxTokens: 8192, MaxOutputTokens: 128000, MaxInputTokens: 200000}
	bcs := NewBillingCacheService(cache, nil, nil, nil, nil, nil, bcCfg, nil)
	t.Cleanup(bcs.Stop)

	billingRepo := &inflightBalanceBillingRepo{balance: 1}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(&openAIRecordUsageLogRepoStub{inserted: true}, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	billing, _ := newShippedCatalogBillingService(t)
	svc.billingService = billing
	svc.billingCacheService = bcs
	svc.cfg.Default.RateMultiplier = 1
	svc.cfg.Billing.InflightReservation = bcCfg.Billing.InflightReservation

	groupID := int64(33)
	user := &User{ID: 33}
	apiKey := &APIKey{ID: 33, User: user, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 1}}
	account := &Account{ID: 33, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	const n = 10
	var wg sync.WaitGroup
	start := make(chan struct{})
	var admittedWG sync.WaitGroup
	admittedWG.Add(n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ctx := context.Background()
			est, priced := svc.EstimateInflightReservation(ctx, apiKey, InflightEstimateRequest{Model: "claude-sonnet-4-6", BodyBytes: 40000, MaxTokens: 8192})
			require.True(t, priced)
			require.InDelta(t, 0.15288, est, 1e-12)
			res, err := bcs.ReserveInflight(ctx, user, apiKey.Group, nil, est)
			admittedWG.Done()
			if err != nil {
				require.ErrorIs(t, err, ErrInsufficientBalance)
				atomic.AddInt32(&rejected, 1)
				return
			}
			atomic.AddInt32(&admitted, 1)
			admittedWG.Wait() // 全部在任何一个结束前完成准入
			taskDone := res.Acquire()
			res.HandlerDone()
			defer taskDone()
			ctx = WithInflightReservation(ctx, res)
			require.NoError(t, svc.RecordUsage(ctx, &RecordUsageInput{
				Result: &ForwardResult{
					RequestID: "ac33-" + time.Now().Format("150405.000000") + "-" + string(rune('a'+i)),
					Model:     "claude-sonnet-4-6",
					Usage:     ClaudeUsage{InputTokens: 10000, OutputTokens: 8192},
					Duration:  time.Second,
				},
				APIKey: apiKey, User: user, Account: account,
			}))
		}(i)
	}
	close(start)
	wg.Wait()
	billingRepo.mu.Lock()
	dbBalance = billingRepo.balance
	billingRepo.mu.Unlock()
	return admitted, rejected, dbBalance, cache
}

// AC33：余额 1、单个预估 0.15288、10 个并发 → 放行 6 个、拒绝 4 个，最终余额 0.08272000；
// 关闭开关时 10 个全放行，最终余额 −0.52880000。
func TestInflightReservation_AC33_EndToEnd(t *testing.T) {
	admitted, rejected, balance, cache := runAC33(t, true)
	require.Equal(t, int32(6), admitted)
	require.Equal(t, int32(4), rejected)
	require.InDelta(t, 0.08272, balance, 1e-9)
	require.Equal(t, "0.08272000", decimal.NewFromFloat(balance).Round(8).StringFixed(8))
	require.Zero(t, cache.count(), "all reservations released after billing landed")
	cacheBalance, _ := cache.GetUserBalance(context.Background(), 33)
	require.InDelta(t, 0.08272, cacheBalance, 1e-9, "balance cache deducted synchronously before release")

	admitted, rejected, balance, _ = runAC33(t, false)
	require.Equal(t, int32(10), admitted)
	require.Zero(t, rejected)
	require.InDelta(t, -0.5288, balance, 1e-9)
}
