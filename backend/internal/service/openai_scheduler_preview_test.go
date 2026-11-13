//go:build unit

package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type schedulerPreviewLoadCache struct {
	schedulerTestConcurrencyCache
	calls    int
	accounts []AccountWithConcurrency
}

func (c *schedulerPreviewLoadCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.calls++
	c.accounts = accounts
	return map[int64]*AccountLoadInfo{}, nil
}

func TestSchedulerPreviewSkipsNonOpenAIAndUnionsGroupLoads(t *testing.T) {
	rl := &RateLimitService{accountRepo: struct{ AccountRepository }{}}
	require.Empty(t, rl.PreviewOpenAISchedulerScores(context.Background(), []Account{{ID: 1, Platform: PlatformGrok}}, nil))
	pool := []Account{{ID: 1, Platform: PlatformOpenAI, GroupIDs: []int64{7, 8}}, {ID: 2, Platform: PlatformOpenAI, GroupIDs: []int64{7}}, {ID: 3, Platform: PlatformAnthropic}}
	rl.accountRepo = schedulerTestOpenAIAccountRepo{accounts: pool}
	cache := &schedulerPreviewLoadCache{}
	scores := rl.PreviewOpenAISchedulerScores(context.Background(), pool, NewConcurrencyService(cache))
	require.Equal(t, 1, cache.calls)
	require.Len(t, cache.accounts, 2)
	require.Len(t, scores[1], 2)
	require.NotContains(t, scores, int64(3))
	require.Equal(t, int64(7), *scores[1][0].GroupID)
	require.Equal(t, int64(8), *scores[1][1].GroupID)
}

func TestSchedulerResetWeightNormalizesPoolAndIsOptIn(t *testing.T) {
	early, late := time.Now().Add(time.Hour), time.Now().Add(4*time.Hour)
	accounts := []*Account{{ID: 1, SessionWindowEnd: &early}, {ID: 2, SessionWindowEnd: &late}, {ID: 3}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Reset = 1
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfg}}
	plan := scheduler.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{}, accounts, nil)
	require.Equal(t, 1.0, plan.candidates[0].score)
	require.Equal(t, 0.0, plan.candidates[1].score)
	require.Equal(t, 0.0, plan.candidates[2].score)
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Reset = 0
	plan = scheduler.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{}, accounts, nil)
	for _, candidate := range plan.candidates {
		require.Zero(t, candidate.score)
	}
}

func TestLegacyStickySpilloverPreservesBinding(t *testing.T) {
	for _, loadFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "batch", true: "batch_error"}[loadFailure], func(t *testing.T) {
			group := int64(7)
			accounts := []Account{
				{ID: 1, GroupIDs: []int64{7}, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
				{ID: 2, GroupIDs: []int64{7}, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
			}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:spill": 1}}
			load := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false, 2: true}, waitCounts: map[int64]int{1: 100}, loadMap: map[int64]*AccountLoadInfo{1: {AccountID: 1, LoadRate: 100}, 2: {AccountID: 2, LoadRate: 0}}}
			if loadFailure {
				load.loadBatchErr = fmt.Errorf("batch unavailable")
			}
			svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cache: cache, concurrencyService: NewConcurrencyService(load)}
			result, err := svc.SelectAccountWithLoadAwareness(context.Background(), &group, "spill", "gpt-5.1", nil)
			require.NoError(t, err)
			require.True(t, result.Acquired)
			require.Equal(t, int64(2), result.Account.ID)
			require.Equal(t, int64(1), cache.sessionBindings["openai:spill"], "spillover must not migrate a healthy sticky conversation")
			result.ReleaseFunc()
		})
	}
}

func TestLegacyPreviousResponseRoutingWithAdvancedDisabled(t *testing.T) {
	ctx := context.Background()
	group := int64(7)
	account := Account{ID: 101, GroupIDs: []int64{7}, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Extra: map[string]any{"openai_apikey_responses_websockets_v2_enabled": true}}
	svc := &OpenAIGatewayService{cfg: newSchedulerTestOpenAIWSV2Config(), accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("false")}
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, group, "resp_legacy", 101, time.Hour))
	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &group, "resp_legacy", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(101), selection.Account.ID)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
}
