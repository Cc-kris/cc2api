package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// V02-T18 / AC29：OAuth + API Key 混合分组请求 gpt-5.5，每个调度入口都只落到 API Key 账号；
// 粘性绑定在 OAuth 上的会话重新选号。

func gpt55MixedAccounts() []Account {
	return []Account{
		{ID: 501, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
			Extra: map[string]any{"openai_oauth_responses_websockets_v2_enabled": true}},
		{ID: 502, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5,
			Extra: map[string]any{"openai_apikey_responses_websockets_v2_enabled": true}},
	}
}

// 调度器路径 + HTTP 粘性：会话粘在 OAuth 账号上，请求 gpt-5.5 时重新选到 API Key 账号。
func TestGPT55RetirementSchedulerStickyReselects(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	ctx := context.Background()
	groupID := int64(7701)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:sess-gpt55": 501}}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: gpt55MixedAccounts()},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	for i := 0; i < 3; i++ {
		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "sess-gpt55", "gpt-5.5", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, int64(502), selection.Account.ID)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}

	// 对照：gpt-5.4 仍可以用 OAuth 账号（排除 API Key 账号后能选到 OAuth）。
	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.4", map[int64]struct{}{502: {}}, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(501), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

// 旧选号路径（SelectAccountForModelWithExclusions，粘性命中复核经过 isOpenAIAccountEligibleForRequest）。
func TestGPT55RetirementLegacySelectionStickyReselects(t *testing.T) {
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:sess-legacy": 501}}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: gpt55MixedAccounts()}, cache: cache}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "sess-legacy", "gpt-5.5", nil)
	require.NoError(t, err)
	require.Equal(t, int64(502), acc.ID)
	require.Equal(t, int64(502), cache.sessionBindings["openai:sess-legacy"])
}

// WS previous_response_id 粘连到 OAuth 账号：请求 gpt-5.5 时不命中，交回调度器重新选号。
func TestGPT55RetirementPreviousResponseIDMissOnOAuth(t *testing.T) {
	ctx := context.Background()
	groupID := int64(7702)
	accounts := gpt55MixedAccounts()
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                newOpenAIWSV2TestConfig(),
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_oauth_prev", 501, time.Hour))

	selection, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_oauth_prev", "gpt-5.5", nil, false)
	require.NoError(t, err)
	require.Nil(t, selection, "OAuth 账号不承接 gpt-5.5，previous_response_id 粘连不命中")

	selection, err = svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_oauth_prev", "gpt-5.4", nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(501), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
