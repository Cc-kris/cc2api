//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountSchedulerWeightedAffinityDoesNotBecomeHardClaim(t *testing.T) {
	group := int64(7)
	accounts := []Account{
		{ID: 1, GroupIDs: []int64{7}, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1},
		{ID: 2, GroupIDs: []int64{7}, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1},
	}
	rl := newOpenAIAdvancedSchedulerRateLimitService("true")
	repo := rl.settingService.settingRepo.(*openAIAdvancedSchedulerSettingRepoStub)
	controls := OpenAISchedulerSettings{StickyWeighted: true, LBTopK: "1", Priority: "0", Load: "10", Queue: "0", ErrorRate: "0", TTFT: "0", Reset: "0", SessionSticky: "0"}
	values, err := controls.settingValues()
	require.NoError(t, err)
	for key, value := range values {
		repo.values[key] = value
	}
	cache := &schedulerClaimRaceCache{schedulerTestGatewayCache: schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:weighted": 1}}, firstLookup: true, winningAccount: 1}
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cache: cache, rateLimitService: rl, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{1: {AccountID: 1, LoadRate: 99}, 2: {AccountID: 2, LoadRate: 0}}})}
	result, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), OpenAIAccountScheduleRequest{GroupID: &group, SessionHash: "weighted", StickyAccountID: 1, RequestedModel: "gpt-5.1"})
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, int64(2), result.Account.ID)
	require.Equal(t, int64(2), cache.sessionBindings["openai:weighted"])
	result.ReleaseFunc()
}

func TestOpenAIAccountSchedulerWeightedWrongGroupClearsBinding(t *testing.T) {
	group := int64(7)
	account := Account{ID: 1, GroupIDs: []int64{8}, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	rl := newOpenAIAdvancedSchedulerRateLimitService("true")
	rl.settingService.settingRepo.(*openAIAdvancedSchedulerSettingRepoStub).values["openai_advanced_scheduler_sticky_weighted_enabled"] = "true"
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:wrong": 1}}
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}, cache: cache, rateLimitService: rl}
	result, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), OpenAIAccountScheduleRequest{GroupID: &group, SessionHash: "wrong", StickyAccountID: 1, RequestedModel: "gpt-5.1"})
	require.Error(t, err)
	require.Nil(t, result)
	require.NotContains(t, cache.sessionBindings, "openai:wrong")
	require.Equal(t, 1, cache.deletedSessions["openai:wrong"])
}

func TestOpenAIAccountSchedulerSubscriptionBusyRemainsWaitable(t *testing.T) {
	group := int64(7)
	subscription := Account{ID: 1, GroupIDs: []int64{7}, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"plan_type": "plus"}}
	regular := Account{ID: 2, GroupIDs: []int64{7}, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false}
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{subscription, regular}}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false}})}
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	result, _, _, _, err := scheduler.selectByLoadBalance(context.Background(), OpenAIAccountScheduleRequest{GroupID: &group, RequestedModel: "gpt-5.1", Controls: OpenAISchedulerSettings{SubscriptionPriority: true}})
	require.NoError(t, err)
	require.NotNil(t, result.WaitPlan)
	require.Equal(t, int64(1), result.WaitPlan.AccountID)
}
