//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// V03-T19 / AC38：Seedance 视频创建（含用户视频生成页，前端同样调 POST /v1/video/generations）
// 原来只靠 api_key_auth 看认证缓存余额 > 0。现在 AdmitCreate 先做 CheckBillingEligibility，
// 余额模式再按本次确定费用（秒价 × 时长 × 分组倍率）预占，只放行余额能覆盖的请求。

func newSeedaceInflightService(t *testing.T, cache BillingCache, enabled bool) (*SeedaceVideoService, *APIKey) {
	t.Helper()
	account := seedaceAccountForTest(303, "https://seedace.example/v1", "seedace-key")
	groupID := int64(7)
	perSecondPrice := 0.028
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: enabled, TTLSeconds: 60}
	bcs := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(bcs.Stop)
	svc := &SeedaceVideoService{
		accountRepo:      &seedaceAccountRepoStub{list: []Account{account}},
		usageLogRepo:     &seedaceUsageLogRepoStub{},
		usageBillingRepo: &seedaceBillingRepoStub{},
		channelService: NewChannelService(&seedaceChannelRepoStub{
			groupPlatforms: map[int64]string{groupID: PlatformSeedace},
			channels: []Channel{{
				ID: 9, Status: StatusActive, GroupIDs: []int64{groupID},
				ModelPricing: []ChannelModelPricing{{
					Platform: PlatformSeedace, Models: []string{"seedance-2.0"},
					BillingMode: BillingModePerSecond, PerRequestPrice: &perSecondPrice,
				}},
			}},
		}, nil, nil, nil),
		billingCacheService: bcs,
		deferredService:     NewDeferredService(nil, nil, time.Second),
		httpUpstream:        &seedaceUpstreamStub{},
		cfg:                 cfg,
	}
	apiKey := &APIKey{ID: 22, User: &User{ID: 33}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformSeedace, RateMultiplier: 1}}
	return svc, apiKey
}

const seedaceInflightBody = `{"model":"seedance-2.0","duration":4}`

// 估算与 recordCreateUsage 同一计算：0.028 × 4 = 0.112。
func TestSeedaceVideoEstimateCreateChargeMatchesBilling(t *testing.T) {
	svc, apiKey := newSeedaceInflightService(t, newMemInflightCache(1), true)
	amount, ok := svc.EstimateCreateCharge(context.Background(), apiKey, []byte(seedaceInflightBody))
	require.True(t, ok)
	require.InDelta(t, 0.112, amount, 1e-12)

	_, ok = svc.EstimateCreateCharge(context.Background(), apiKey, []byte(`not json`))
	require.False(t, ok)
}

// AC38：余额 0.2，单次 0.112，10 个并发只放行 1 个（0.2 − 0.112 < 0.112）。
func TestSeedaceVideoAdmitCreate_ConcurrentOnlyAdmitsCoveredRequests(t *testing.T) {
	cache := newMemInflightCache(0.2)
	svc, apiKey := newSeedaceInflightService(t, cache, true)

	const n = 10
	var admitted, rejected atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	holds := make(chan *InflightReservation, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := svc.AdmitCreate(context.Background(), apiKey, nil, []byte(seedaceInflightBody))
			if err != nil {
				require.ErrorIs(t, err, ErrInsufficientBalance)
				rejected.Add(1)
				return
			}
			admitted.Add(1)
			holds <- res
		}()
	}
	close(start)
	wg.Wait()
	close(holds)
	require.Equal(t, int32(1), admitted.Load())
	require.Equal(t, int32(n-1), rejected.Load())
	for res := range holds {
		res.HandlerDone()
	}
	require.Zero(t, cache.count())
}

// AC38：余额 0 的用户直接在准入被 CheckBillingEligibility 拒绝（不依赖认证缓存）；
// 余额不足以覆盖单次费用（0.05 < 0.112）也拒绝。
func TestSeedaceVideoAdmitCreate_RejectsZeroAndUncoveredBalance(t *testing.T) {
	svc, apiKey := newSeedaceInflightService(t, newMemInflightCache(0), true)
	_, err := svc.AdmitCreate(context.Background(), apiKey, nil, []byte(seedaceInflightBody))
	require.ErrorIs(t, err, ErrInsufficientBalance)

	svc, apiKey = newSeedaceInflightService(t, newMemInflightCache(0.05), true)
	_, err = svc.AdmitCreate(context.Background(), apiKey, nil, []byte(seedaceInflightBody))
	require.ErrorIs(t, err, ErrInsufficientBalance)

	// 开关关闭：只做 CheckBillingEligibility（余额 > 0 即放行），不预占。
	svc, apiKey = newSeedaceInflightService(t, newMemInflightCache(0.05), false)
	res, err := svc.AdmitCreate(context.Background(), apiKey, nil, []byte(seedaceInflightBody))
	require.NoError(t, err)
	require.Nil(t, res)
}

// 预占持续到 Create 内同步计费完成，计费后释放；余额缓存同步扣减。
func TestSeedaceVideoAdmitCreate_ReleasedAfterSynchronousBilling(t *testing.T) {
	cache := newMemInflightCache(1)
	svc, apiKey := newSeedaceInflightService(t, cache, true)
	res, err := svc.AdmitCreate(context.Background(), apiKey, nil, []byte(seedaceInflightBody))
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 1, cache.count())
	_, err = svc.Create(context.Background(), SeedaceVideoCreateInput{APIKey: apiKey, Body: []byte(seedaceInflightBody)})
	require.NoError(t, err)
	res.HandlerDone()
	require.Zero(t, cache.count())
	bal, _ := cache.GetUserBalance(context.Background(), 33)
	require.InDelta(t, 1-0.112, bal, 1e-9)
}
