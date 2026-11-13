package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIModelCooldownSelection(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, loadBatch := range []bool{false, true} {
			for _, sticky := range []string{"none", "session", "previous"} {
				for _, state := range []string{"cooled", "other-model", "expired", "unsupported", "missing-cooldown", "malformed-cooldown"} {
					t.Run(fmt.Sprintf("advanced=%t/load=%t/%s/%s", advanced, loadBatch, sticky, state), func(t *testing.T) {
						resetOpenAIAdvancedSchedulerSettingCacheForTest()
						ctx := context.Background()
						groupID := int64(10107)
						reset := time.Now().Add(time.Hour)
						model := "client-a"
						if state == "expired" {
							reset = time.Now().Add(-time.Second)
						}
						if state == "other-model" {
							model = "client-b"
						}
						if state == "unsupported" {
							model = "missing"
						}
						a := Account{ID: 37001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
							Credentials: map[string]any{"model_mapping": map[string]any{"client-a": "gpt-4o", "client-b": "gpt-4o-mini"}},
							Extra:       map[string]any{"openai_apikey_responses_websockets_v2_enabled": true, "model_rate_limits": map[string]any{"gpt-4o": map[string]any{"rate_limit_reset_at": reset.UTC().Format(time.RFC3339Nano)}}}}
						if state == "missing-cooldown" {
							delete(a.Extra, "model_rate_limits")
						}
						if state == "malformed-cooldown" {
							a.Extra["model_rate_limits"].(map[string]any)["gpt-4o"].(map[string]any)["rate_limit_reset_at"] = "invalid-time"
						}
						cfg := &config.Config{}
						cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
						cfg.Gateway.OpenAIWS.Enabled = true
						cfg.Gateway.OpenAIWS.APIKeyEnabled = true
						cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
						cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 3600
						cache := &schedulerTestGatewayCache{}
						svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{a}}, cache: cache, cfg: cfg, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(fmt.Sprint(advanced)), concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
						session, previous := "", ""
						if sticky == "session" {
							session = "cooldown-session"
							require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, session, a.ID, time.Hour))
						}
						if sticky == "previous" {
							previous = "resp_cooldown"
							require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, previous, a.ID, time.Hour))
						}
						selected, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, previous, session, model, nil, OpenAIUpstreamTransportAny, false)
						if state == "cooled" || state == "unsupported" {
							require.ErrorIs(t, err, ErrNoAvailableAccounts)
							require.Nil(t, selected)
							if state == "cooled" {
								require.Contains(t, err.Error(), "model_rate_limited=1")
							} else {
								require.NotContains(t, err.Error(), "model_rate_limited=")
							}
						} else {
							require.NoError(t, err)
							require.NotNil(t, selected)
							require.Equal(t, a.ID, selected.Account.ID)
							if selected.ReleaseFunc != nil {
								selected.ReleaseFunc()
							}
						}
					})
				}
			}
		}
	}
}

func TestOpenAIModelCooldownCompactKey(t *testing.T) {
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"client": "normal"}, "compact_model_mapping": map[string]any{"normal": "compact"}}, Extra: map[string]any{"model_rate_limits": map[string]any{"compact": map[string]any{"rate_limit_reset_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}}}
	require.False(t, openAIRequestModelRateLimited(a, "client", false))
	require.True(t, openAIRequestModelRateLimited(a, "client", true))
	require.True(t, isOpenAIAccountEligibleForRequest(a, "client", false))
	require.False(t, isOpenAIAccountEligibleForRequest(a, "client", true))
}
