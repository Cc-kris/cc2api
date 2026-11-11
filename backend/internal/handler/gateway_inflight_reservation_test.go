package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// 移植上游 43f88c99a / 52c123e6f 的 gateway_inflight_reservation_test.go，
// 加上 CCAI 的续写（AC36）、x_search / Seedance（AC38）接入用例。

func TestRequestMaxOutputTokens(t *testing.T) {
	require.Equal(t, 1024, requestMaxOutputTokens([]byte(`{"max_tokens":1024}`)))
	require.Equal(t, 2048, requestMaxOutputTokens([]byte(`{"max_completion_tokens":2048}`)))
	require.Equal(t, 4096, requestMaxOutputTokens([]byte(`{"max_output_tokens":4096}`)))
	require.Equal(t, 512, requestMaxOutputTokens([]byte(`{"generationConfig":{"maxOutputTokens":512}}`)))
	require.Equal(t, 0, requestMaxOutputTokens([]byte(`{"max_tokens":"x"}`)))
	require.Equal(t, 0, requestMaxOutputTokens([]byte(`{}`)))
}

func TestInflightEstimateRequestBuilders(t *testing.T) {
	require.True(t, requestHasImageInput([]byte(`{"input":[{"type":"input_image","image_url":"data:..."}]}`)))
	require.True(t, requestHasImageInput([]byte(`{"messages":[{"content":[{"type":"image","source":{}}]}]}`)))
	require.False(t, requestHasImageInput([]byte(`{"messages":[{"content":"hi"}]}`)))

	req := withGeminiImageEstimate(tokenInflightEstimate("gemini-3-pro-image-preview", []byte(`{"generationConfig":{"candidateCount":2}}`), ""),
		[]byte(`{"generationConfig":{"candidateCount":2}}`), "gemini-3-pro-image-preview")
	require.True(t, req.MayGenerateImage)
	require.Equal(t, 2, req.Units)
	req = withGeminiImageEstimate(tokenInflightEstimate("gemini-2.5-pro", nil, ""), nil, "gemini-2.5-pro")
	require.False(t, req.MayGenerateImage)

	require.Equal(t, "fast", service.InflightAnthropicSpeedTier("fast", "my-alias", "claude-opus-5"))
	require.Empty(t, service.InflightAnthropicSpeedTier("fast", "claude-opus-4-7"))
	require.Empty(t, service.InflightAnthropicSpeedTier("", "claude-opus-5"))
	require.Equal(t, "priority", service.InflightOpenAIServiceTier([]byte(`{"service_tier":"fast"}`)))
	require.Equal(t, "flex", service.InflightOpenAIServiceTier([]byte(`{"service_tier":"flex"}`)))
	require.Equal(t, "priority", service.InflightOpenAIMessagesServiceTier("fast-mode-2026-02-01", nil))
}

type countingEstimator struct {
	calls  int
	cost   float64
	priced bool
}

func (e *countingEstimator) EstimateInflightReservation(context.Context, *service.APIKey, service.InflightEstimateRequest) (float64, bool) {
	e.calls++
	return e.cost, e.priced
}

func newInflightTestGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return c
}

func TestReserveInflightBalance_SkipsWhenDisabledOrSubscription(t *testing.T) {
	cfg := &config.Config{}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	est := &countingEstimator{cost: 1, priced: true}
	apiKey := &service.APIKey{User: &service.User{ID: 1}}

	done, err := reserveInflightBalance(newInflightTestGinContext(), billing, est, apiKey, nil, tokenInflightEstimate("m", []byte(`{}`), ""))
	require.NoError(t, err)
	done()
	require.Equal(t, 0, est.calls, "disabled switch must not even estimate")

	cfg.Billing.InflightReservation.Enabled = true
	apiKey.Group = &service.Group{SubscriptionType: service.SubscriptionTypeSubscription}
	done, err = reserveInflightBalance(newInflightTestGinContext(), billing, est, apiKey, &service.UserSubscription{}, tokenInflightEstimate("m", []byte(`{}`), ""))
	require.NoError(t, err)
	done()
	require.Equal(t, 0, est.calls, "subscription mode must be unaffected")
}

// AC34（无定价）：默认放行不预占；fail_closed_on_unpriced=true 时拒绝（余额不足错误）。
func TestReserveInflightBalance_UnpricedFailOpenByDefaultFailClosedOptIn(t *testing.T) {
	_, _, billing := newHandlerRedisBillingCache(t, 10, nil)
	apiKey := &service.APIKey{User: &service.User{ID: 1}}
	est := &countingEstimator{priced: false}

	done, err := reserveInflightBalance(newInflightTestGinContext(), billing, est, apiKey, nil, tokenInflightEstimate("unknown", nil, ""))
	require.NoError(t, err)
	done()

	_, _, billing = newHandlerRedisBillingCache(t, 10, func(c *config.InflightReservationConfig) { c.FailClosedOnUnpriced = true })
	_, err = reserveInflightBalance(newInflightTestGinContext(), billing, est, apiKey, nil, tokenInflightEstimate("unknown", nil, ""))
	require.ErrorIs(t, err, service.ErrInsufficientBalance)
}

func TestReserveInflightBalance_DoesNotReserveTwiceOnSameRequest(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 10, nil)
	apiKey := &service.APIKey{User: &service.User{ID: 4}}
	c := newInflightTestGinContext()
	done1, err := reserveInflightBalance(c, billing, &countingEstimator{cost: 0.5, priced: true}, apiKey, nil, tokenInflightEstimate("m", nil, ""))
	require.NoError(t, err)
	done2, err := reserveInflightBalance(c, billing, &countingEstimator{cost: 0.5, priced: true}, apiKey, nil, tokenInflightEstimate("m", nil, ""))
	require.NoError(t, err)
	require.Equal(t, int32(1), counting.reserves.Load(), "re-admission on the same request reuses the reservation")
	done2()
	done1()
	require.Zero(t, counting.zcard(t, 4))
}

func TestWrapUsageRecordTaskInflight_HandsReservationToBillingTask(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 1, nil)
	apiKey := &service.APIKey{User: &service.User{ID: 5}}

	c := newInflightTestGinContext()
	done, err := reserveInflightBalance(c, billing, &countingEstimator{cost: 0.9, priced: true}, apiKey, nil, tokenInflightEstimate("m", nil, ""))
	require.NoError(t, err)
	require.Equal(t, int64(1), counting.zcard(t, 5))

	ran := false
	task, abandon := wrapUsageRecordTaskInflight(c.Request.Context(), func(context.Context) { ran = true })
	done() // handler returns; billing still pending
	require.Equal(t, int64(1), counting.zcard(t, 5), "reservation held until the billing task finishes")
	task(context.Background())
	require.True(t, ran)
	require.Zero(t, counting.zcard(t, 5))
	abandon() // idempotent with the task's own done

	// Dropped task: the submitter abandons it and the reservation is released.
	c2 := newInflightTestGinContext()
	done2, err := reserveInflightBalance(c2, billing, &countingEstimator{cost: 0.9, priced: true}, apiKey, nil, tokenInflightEstimate("m", nil, ""))
	require.NoError(t, err)
	_, abandon2 := wrapUsageRecordTaskInflight(c2.Request.Context(), func(context.Context) {})
	done2()
	require.Equal(t, int64(1), counting.zcard(t, 5))
	abandon2()
	require.Zero(t, counting.zcard(t, 5))
}

// 池停止 / 溢出丢弃时 submitUsageRecordTask 立即归还引用。
func TestSubmitUsageRecordTask_DroppedTaskReleasesReservation(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 1, nil)
	apiKey := &service.APIKey{User: &service.User{ID: 6}}
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount: 1, QueueSize: 1, TaskTimeout: time.Second, OverflowPolicy: config.UsageRecordOverflowPolicyDrop,
	})
	block := make(chan struct{})
	pool.Submit(func(context.Context) { <-block }) // occupy the worker
	pool.Submit(func(context.Context) {})          // fill the queue
	h := &GatewayHandler{usageRecordWorkerPool: pool}

	c := newInflightTestGinContext()
	done, err := reserveInflightBalance(c, billing, &countingEstimator{cost: 0.5, priced: true}, apiKey, nil, tokenInflightEstimate("m", nil, ""))
	require.NoError(t, err)
	h.submitUsageRecordTask(c.Request.Context(), func(context.Context) {})
	done()
	require.Zero(t, counting.zcard(t, 6), "dropped billing task must not keep the reservation")
	close(block)
	pool.Stop()
}

// ---------------------------------------------------------------------------
// 真实 Redis 语义（miniredis + repository.billingCache Lua）
// ---------------------------------------------------------------------------

type countingInflightCache struct {
	service.BillingCache
	rdb      *redis.Client
	reserves atomic.Int32
	releases atomic.Int32
}

func (c *countingInflightCache) ReserveInflightBalance(ctx context.Context, userID int64, requestID string, amount, balance float64, ttl time.Duration) (bool, float64, error) {
	reserver, isReserver := c.BillingCache.(service.InflightBalanceReservationCache)
	if !isReserver {
		return false, 0, errors.New("billing cache does not implement InflightBalanceReservationCache")
	}
	ok, sum, err := reserver.ReserveInflightBalance(ctx, userID, requestID, amount, balance, ttl)
	if ok && err == nil {
		c.reserves.Add(1)
	}
	return ok, sum, err
}

func (c *countingInflightCache) ReserveInflightBalanceCovered(ctx context.Context, userID int64, requestID string, amount, balance float64, ttl time.Duration) (bool, float64, error) {
	reserver, isReserver := c.BillingCache.(service.InflightBalanceCoveredReserver)
	if !isReserver {
		return false, 0, errors.New("billing cache does not implement InflightBalanceCoveredReserver")
	}
	ok, sum, err := reserver.ReserveInflightBalanceCovered(ctx, userID, requestID, amount, balance, ttl)
	if ok && err == nil {
		c.reserves.Add(1)
	}
	return ok, sum, err
}

func (c *countingInflightCache) ReleaseInflightBalance(ctx context.Context, userID int64, requestID string) error {
	c.releases.Add(1)
	releaser, isReleaser := c.BillingCache.(service.InflightBalanceReservationCache)
	if !isReleaser {
		return errors.New("billing cache does not implement InflightBalanceReservationCache")
	}
	return releaser.ReleaseInflightBalance(ctx, userID, requestID)
}

func (c *countingInflightCache) RenewInflightBalance(ctx context.Context, userID int64, requestID string, ttl time.Duration) (bool, error) {
	renewer, isRenewer := c.BillingCache.(service.InflightBalanceReservationRenewer)
	if !isRenewer {
		return false, errors.New("billing cache does not implement InflightBalanceReservationRenewer")
	}
	return renewer.RenewInflightBalance(ctx, userID, requestID, ttl)
}

// zcard 读 billing:inflight:{uid}。
func (c *countingInflightCache) zcard(t *testing.T, userID int64) int64 {
	t.Helper()
	n, err := c.rdb.ZCard(context.Background(), "billing:inflight:{"+strconv.FormatInt(userID, 10)+"}").Result()
	require.NoError(t, err)
	return n
}

func newHandlerRedisBillingCache(t *testing.T, balance float64, mutate func(*config.InflightReservationConfig)) (*miniredis.Miniredis, *countingInflightCache, *service.BillingCacheService) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &countingInflightCache{BillingCache: repository.NewBillingCache(rdb), rdb: rdb}
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60, DefaultMaxTokens: 8192, MaxOutputTokens: 128000, MaxInputTokens: 200000}
	if mutate != nil {
		mutate(&cfg.Billing.InflightReservation)
	}
	billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	for _, uid := range []int64{1, 4, 5, 6, 38, 9001} {
		require.NoError(t, cache.SetUserBalance(context.Background(), uid, balance))
	}
	return mr, cache, billing
}

// ---------------------------------------------------------------------------
// AC36：续写流程整个请求只有 1 条预占，计费落库后 ZCARD billing:inflight:{uid} = 0
// ---------------------------------------------------------------------------

func TestGatewayHandlerMessages_StreamContinuationHoldsSingleReservation(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 10, nil)
	accountA := newStreamBillingAccount(251, 0, 1)
	accountB := newStreamBillingAccount(252, 0, 2)
	h := newStreamBillingHarnessWithBillingCache(t, true, []*service.Account{accountA, accountB}, map[int64][]streamBillingUpstreamStep{
		251: {{body: continuationInterruptedAttemptSSE(), readErr: io.ErrUnexpectedEOF}},
		252: {{body: continuationCompletedAttemptSSE()}},
	}, billing)
	defer h.cleanup()

	rec := h.serve(t, "claude-sonnet-4-6")
	require.Contains(t, rec.Body.String(), "And B finishes.")
	logs := h.waitBilling(t, 1)
	require.Len(t, logs, 1)
	require.InDelta(t, 0.0114, logs[0].ActualCost, 1e-12)
	require.Equal(t, 2, h.upstream.callCount(251)+h.upstream.callCount(252))

	require.Equal(t, int32(1), counting.reserves.Load(), "续写换号期间不重复预占")
	require.Equal(t, int32(1), counting.releases.Load())
	require.Zero(t, counting.zcard(t, 9001), "ZCARD billing:inflight:{uid} = 0 after billing")
	bal, err := counting.GetUserBalance(context.Background(), 9001)
	require.NoError(t, err)
	require.InDelta(t, 10-0.0114, bal, 1e-9, "余额缓存在释放前同步扣减")
}

// 续写耗尽（W1 T04：按最后观测尝试收费）同样在计费后释放。
func TestGatewayHandlerMessages_StreamContinuationExhaustedReleasesAfterBilling(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 10, nil)
	failBody := `{"type":"error","error":{"type":"api_error","message":"upstream boom"}}`
	h := newStreamBillingHarnessWithBillingCache(t, true, []*service.Account{newStreamBillingAccount(351, 0, 1), newStreamBillingAccount(352, 0, 2)}, map[int64][]streamBillingUpstreamStep{
		351: {{body: continuationInterruptedAttemptSSE(), readErr: io.ErrUnexpectedEOF}},
		352: {{status: 500, body: failBody}},
	}, billing)
	defer h.cleanup()

	_ = h.serve(t, "claude-sonnet-4-6")
	logs := h.waitBilling(t, 1)
	require.Len(t, logs, 1)
	require.InDelta(t, 0.0075, logs[0].ActualCost, 1e-12)
	require.Equal(t, int32(1), counting.reserves.Load())
	require.Zero(t, counting.zcard(t, 9001))
	bal, _ := counting.GetUserBalance(context.Background(), 9001)
	require.InDelta(t, 10-0.0075, bal, 1e-9)
}

// 没有任何计费的失败路径（耗尽且未输出）在 handler 返回时立即释放，不等 TTL。
func TestGatewayHandlerMessages_UnbilledFailureReleasesImmediately(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 10, nil)
	failBody := `{"type":"error","error":{"type":"api_error","message":"upstream boom"}}`
	h := newStreamBillingHarnessWithBillingCache(t, true, []*service.Account{newStreamBillingAccount(451, 0, 1), newStreamBillingAccount(452, 0, 2)}, map[int64][]streamBillingUpstreamStep{
		451: {{status: 500, body: failBody}},
		452: {{status: 500, body: failBody}},
	}, billing)
	defer h.cleanup()

	_ = h.serve(t, "claude-sonnet-4-6")
	require.Empty(t, h.waitBillingNone(t))
	require.Equal(t, int32(1), counting.reserves.Load())
	require.Equal(t, int32(1), counting.releases.Load())
	require.Zero(t, counting.zcard(t, 9001))
}

// 已有在途预占占满余额时，新请求在 Messages 入口被拒绝（余额不足），不残留预占。
func TestGatewayHandlerMessages_RejectsWhenInflightExceedsBalance(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 0.01, nil)
	h := newStreamBillingHarnessWithBillingCache(t, true, []*service.Account{newStreamBillingAccount(551, 0, 1)}, map[int64][]streamBillingUpstreamStep{
		551: {{body: continuationCompletedAttemptSSE()}},
	}, billing)
	defer h.cleanup()

	held, err := billing.ReserveInflight(context.Background(), h.apiKey.User, h.apiKey.Group, nil, 0.009)
	require.NoError(t, err)
	defer held.HandlerDone()

	rec := h.serve(t, "claude-sonnet-4-6")
	require.Contains(t, rec.Body.String(), "insufficient balance")
	require.Zero(t, h.upstream.callCount(551), "rejected before account selection")
	require.Equal(t, int64(1), counting.zcard(t, 9001), "only the held reservation remains")
}

// ---------------------------------------------------------------------------
// AC38：x_search 原来只在认证中间件看缓存余额；现在 Responses 入口 CheckBillingEligibility +
// ReserveFixedCharge（单价 0.025）。
// ---------------------------------------------------------------------------

func newXSearchInflightHandler(t *testing.T, billing *service.BillingCacheService) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{}
	settings := service.NewSettingService(&pageSettingRepoStub{values: map[string]string{
		service.SettingKeyXSearchPricePerRequest: "0.025",
	}}, cfg)
	gw := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, billing, nil, nil, nil, nil, nil, nil, settings, nil, nil)
	return &OpenAIGatewayHandler{
		gatewayService:      gw,
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{
			acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
		}), SSEPingFormatNone, time.Second),
		cfg:          cfg,
		imageLimiter: &imageConcurrencyLimiter{},
	}
}

func serveXSearch(h *OpenAIGatewayHandler, userID int64) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/x_search", bytes.NewBufferString(`{"query":"sub2api"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(38)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID: 38, GroupID: &groupID, User: &service.User{ID: userID, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformGrok, RateMultiplier: 1, Status: service.StatusActive},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID, Concurrency: 10})
	h.XSearch(c)
	return rec
}

func TestXSearch_InflightFixedChargeRejectsSecondConcurrentRequest(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 0.03, nil)
	h := newXSearchInflightHandler(t, billing)

	// 第一个 x_search 请求正在途（0.025）。
	held, err := billing.ReserveFixedCharge(context.Background(), &service.User{ID: 38}, nil, nil, 0.025)
	require.NoError(t, err)
	defer held.HandlerDone()

	rec := serveXSearch(h, 38)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "insufficient balance", "0.03 − 0.025 < 0.025")
	require.Equal(t, int64(1), counting.zcard(t, 38))
}

func TestXSearch_ZeroBalanceRejectedByBillingEligibility(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 0, nil)
	h := newXSearchInflightHandler(t, billing)
	rec := serveXSearch(h, 38)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "insufficient balance")
	require.Zero(t, counting.reserves.Load(), "rejected by CheckBillingEligibility before reservation")
}

// 并发：两个 x_search 同时进入只放行一个（准入层，真实 Lua）。
func TestXSearch_FixedChargeConcurrentAdmission(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 0.03, nil)
	var wg sync.WaitGroup
	var admitted, rejected atomic.Int32
	start := make(chan struct{})
	dones := make(chan func(), 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c := newInflightTestGinContext()
			done, err := reserveInflightFixedCharge(c, billing, &service.APIKey{User: &service.User{ID: 38}}, nil, 0.025)
			if err != nil {
				rejected.Add(1)
				return
			}
			admitted.Add(1)
			dones <- done
		}()
	}
	close(start)
	wg.Wait()
	close(dones)
	require.Equal(t, int32(1), admitted.Load())
	require.Equal(t, int32(1), rejected.Load())
	for done := range dones {
		done()
	}
	require.Zero(t, counting.zcard(t, 38))
}

// Seedance 视频创建：余额 0 在 handler 准入被拒绝（不依赖认证缓存）。
func TestSeedaceVideoCreate_ZeroBalanceRejected(t *testing.T) {
	_, counting, billing := newHandlerRedisBillingCache(t, 0, nil)
	svc := service.NewSeedaceVideoService(nil, nil, nil, nil, nil, nil, nil, nil, nil, billing, nil, nil, nil, nil, &config.Config{})
	h := NewSeedaceVideoHandler(svc)
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(`{"model":"seedance-2.0","duration":4}`))
	groupID := int64(39)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID: 39, GroupID: &groupID, User: &service.User{ID: 38},
		Group: &service.Group{ID: groupID, Platform: service.PlatformSeedace, RateMultiplier: 1},
	})
	h.Create(c)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "insufficient balance")
	require.Zero(t, counting.reserves.Load())
}
