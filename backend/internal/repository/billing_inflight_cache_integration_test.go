//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// V03-T18 / AC33：真实 Redis（Lua 预占）+ 真实 PostgreSQL（usage_billing_repo.Apply 扣余额）的并发用例。
// 10 个请求同时进入，全部在任何一个结束前完成准入；每个请求实际扣 0.15288（与估算相同，
// 估算值本身由 service 包的 billing_inflight_estimate_consistency_test 断言）。

func runInflightAC33Integration(t *testing.T, enabled bool) (admitted, rejected int32, dbBalance float64, zcard int64) {
	t.Helper()
	ctx := context.Background()
	rdb := testRedis(t)
	cache := &billingCache{rdb: rdb}
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: enabled, TTLSeconds: 60}
	svc := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("inflight-ac33-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      1,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-inflight-" + uuid.NewString(), Name: "inflight"})
	account := mustCreateAccount(t, client, &service.Account{Name: "inflight-" + uuid.NewString(), Type: service.AccountTypeAPIKey})
	require.NoError(t, cache.SetUserBalance(ctx, user.ID, 1))

	const n = 10
	const estimate = 0.15288
	var wg sync.WaitGroup
	var admission sync.WaitGroup
	admission.Add(n)
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := svc.ReserveInflight(ctx, &service.User{ID: user.ID}, nil, nil, estimate)
			admission.Done()
			if err != nil {
				if !errors.Is(err, service.ErrInsufficientBalance) {
					errs <- err
				}
				atomic.AddInt32(&rejected, 1)
				return
			}
			atomic.AddInt32(&admitted, 1)
			admission.Wait()
			taskDone := res.Acquire()
			res.HandlerDone()
			defer taskDone()
			result, err := repo.Apply(ctx, &service.UsageBillingCommand{
				RequestID:   "inflight-ac33-" + uuid.NewString(),
				APIKeyID:    apiKey.ID,
				UserID:      user.ID,
				AccountID:   account.ID,
				AccountType: service.AccountTypeAPIKey,
				BalanceCost: estimate,
			})
			if err != nil {
				errs <- err
				return
			}
			if !result.Applied {
				errs <- errors.New("billing not applied")
				return
			}
			// 计费落库后同步扣减余额缓存，再释放预占（与 syncBalanceCacheAfterDeduction 相同顺序）。
			if err := cache.DeductUserBalance(ctx, user.ID, estimate); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&dbBalance))
	zkey, _ := billingInflightKeys(user.ID)
	var err error
	zcard, err = rdb.ZCard(ctx, zkey).Result()
	require.NoError(t, err)
	return admitted, rejected, dbBalance, zcard
}

func TestInflightReservationIntegration_AC33_ConcurrentOverdraftPrevented(t *testing.T) {
	admitted, rejected, balance, zcard := runInflightAC33Integration(t, true)
	require.Equal(t, int32(6), admitted)
	require.Equal(t, int32(4), rejected)
	require.InDelta(t, 0.08272, balance, 1e-9, "1 − 6×0.15288，不为负")
	require.Zero(t, zcard, "ZCARD billing:inflight:{uid} = 0 after billing")
}

func TestInflightReservationIntegration_AC33_DisabledAdmitsAll(t *testing.T) {
	admitted, rejected, balance, zcard := runInflightAC33Integration(t, false)
	require.Equal(t, int32(10), admitted)
	require.Zero(t, rejected)
	require.InDelta(t, -0.5288, balance, 1e-9)
	require.Zero(t, zcard)
}

// AC38（缓存层）：x_search 单价 0.025、余额 0.03，两个并发只放行一个；真实 Redis。
func TestInflightReservationIntegration_FixedChargeConcurrent(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := &billingCache{rdb: rdb}
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
	svc := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)
	userID := int64(990038)
	require.NoError(t, cache.SetUserBalance(ctx, userID, 0.03))

	var admitted, rejected atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	holds := make(chan *service.InflightReservation, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := svc.ReserveFixedCharge(ctx, &service.User{ID: userID}, nil, nil, 0.025)
			if err != nil {
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
	require.Equal(t, int32(1), rejected.Load())
	for res := range holds {
		res.HandlerDone()
	}
	zkey, _ := billingInflightKeys(userID)
	n, err := rdb.ZCard(ctx, zkey).Result()
	require.NoError(t, err)
	require.Zero(t, n)
}
