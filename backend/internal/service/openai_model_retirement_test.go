package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V02-T18：GPT-5.5 订阅退役。

// 拷自上游 8e92c892b openai_model_retirement_test.go。去掉影子账号（ParentAccountID）用例，
// CCAI 没有影子账号。
func TestGPT55RetirementModelSupport(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, passthrough := range []bool{false, true} {
			t.Run(typ+"/passthrough="+map[bool]string{true: "true", false: "false"}[passthrough], func(t *testing.T) {
				account := &Account{Platform: PlatformOpenAI, Type: typ, Extra: map[string]any{"openai_passthrough": passthrough}}
				for _, model := range []string{"gpt-5.5", "openai/gpt-5.5-high", "gpt-5.5-2026-04-23", "GPT_5.5", "gpt-5.5-openai-compact"} {
					require.Equal(t, typ == AccountTypeAPIKey, account.IsModelSupported(model), model)
				}
				require.True(t, account.IsModelSupported("gpt-6.1-sol"))
			})
		}
	}
	for _, mapping := range []map[string]any{
		{"legacy": "gpt-5.5"}, {"legacy*": "gpt-5.5"},
	} {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": mapping}}
		require.False(t, account.IsModelSupported("legacy"))
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-6.1-sol"}}}
	require.True(t, account.IsModelSupported("gpt-5.5"), "explicit administrator replacement remains authoritative")
	account.Type = AccountTypeAPIKey
	require.True(t, account.IsModelSupported("gpt-5.5"))
	require.False(t, isRetiredOpenAISubscriptionModel(&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, "gpt-5.5"))
	require.False(t, isRetiredOpenAISubscriptionModel(account, "gpt-5.50"))
}

// 拷自上游，CCAI 没有 projectAccountModelsBody（投影层），直接测公开名过滤。
func TestGPT55RetirementAccountProjection(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"gpt-5.5"},{"id":"gpt-6.1-sol","unknown":true}],"unknown_top":true}`)
	for _, typ := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		account := Account{Platform: PlatformOpenAI, Type: typ}
		got, _, err := filterRetiredOpenAIPublicModelsBody(body, []Account{account}, false)
		require.NoError(t, err)
		require.Equal(t, typ == AccountTypeAPIKey, strings.Contains(string(got), "gpt-5.5"))
		require.True(t, gjson.GetBytes(got, "unknown_top").Bool())
		require.True(t, json.Valid(got))
	}
}

// AC30：渠道上游模型限制按升级后的 resolve 判定（透传账号不套 model_mapping，OAuth 做规范化）。
func TestResolveOpenAIAccountUpstreamModelForRequestMatchesForward(t *testing.T) {
	mapping := map[string]any{"my-alias": "gpt-5.5", "gpt-5.4": "gpt-5.4"}

	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": mapping}}
	require.Equal(t, "gpt-5.5", resolveOpenAIAccountUpstreamModelForRequest(oauth, "my-alias", false))
	require.Equal(t, normalizeCodexModel("gpt-5.4"), resolveOpenAIAccountUpstreamModelForRequest(oauth, "gpt-5.4", false))

	// 透传账号：forwardOpenAIPassthrough 不套 model_mapping，出站模型就是请求模型。
	passthrough := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": map[string]any{"x": "gpt-5.5", "gpt-5.5": "gpt-6.1-sol"}},
		Extra:       map[string]any{"openai_passthrough": true}}
	require.Equal(t, "x", resolveOpenAIAccountUpstreamModelForRequest(passthrough, "x", false))
	require.Equal(t, "gpt-5.5", resolveOpenAIAccountUpstreamModelForRequest(passthrough, "gpt-5.5", false))
	require.False(t, passthrough.IsModelSupported("gpt-5.5"), "透传 OAuth 账号不承接 gpt-5.5，即使 model_mapping 里写了替换")

	// compact：先账号映射，再按映射后的模型套 compact_model_mapping（和 CCAI Forward 一致）。
	compact := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"model_mapping":         map[string]any{"client": "gpt-5.4"},
		"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.5"},
	}}
	require.Equal(t, "gpt-5.5", resolveOpenAIAccountUpstreamModelForRequest(compact, "client", true))
	require.True(t, supportsOpenAIRequestModel(compact, "client", false))
	require.False(t, supportsOpenAIRequestModel(compact, "client", true), "compact 映射到 gpt-5.5 的 OAuth 账号不能承接 compact")
	require.True(t, compact.IsModelSupported("client"))

	// API Key 账号不规范化，也不退役。
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": mapping}}
	require.Equal(t, "gpt-5.5", resolveOpenAIAccountUpstreamModelForRequest(apiKey, "my-alias", false))
	require.True(t, apiKey.IsModelSupported("my-alias"))
}

// AC30：渠道上游模型限制的回归——透传账号用请求模型判定，不再用 model_mapping 结果。
func TestIsUpstreamModelRestrictedByChannelUsesForwardModel(t *testing.T) {
	passthrough := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": map[string]any{"client": "restricted-upstream"}},
		Extra:       map[string]any{"openai_passthrough": true}}
	require.Equal(t, "client", resolveOpenAIAccountUpstreamModelForRequest(passthrough, "client", false))

	plain := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{"client": "restricted-upstream"}}}
	require.Equal(t, "restricted-upstream", resolveOpenAIAccountUpstreamModelForRequest(plain, "client", false))
}

// AC29：调度器路径（isAccountRequestCompatible）只把 gpt-5.5 调度给 API Key 账号；compact 传 RequireCompact。
func TestGPT55RetirementSchedulerCompatibility(t *testing.T) {
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: &config.Config{}}}
	oauth := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	apiKey := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	ctx := context.Background()

	require.False(t, scheduler.isAccountRequestCompatible(ctx, oauth, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.5"}))
	require.True(t, scheduler.isAccountRequestCompatible(ctx, apiKey, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.5"}))
	require.True(t, scheduler.isAccountRequestCompatible(ctx, oauth, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.4"}))

	compact := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.5"}}}
	require.True(t, scheduler.isAccountRequestCompatible(ctx, compact, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.4"}))
	require.False(t, scheduler.isAccountRequestCompatible(ctx, compact, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.4", RequireCompact: true}))

	// 旧选号路径的资格判断（粘性、DB 复核都经过它）。
	require.False(t, isOpenAIAccountEligibleForRequest(oauth, "gpt-5.5", false))
	require.True(t, isOpenAIAccountEligibleForRequest(apiKey, "gpt-5.5", false))
	require.False(t, isOpenAIAccountEligibleForRequest(compact, "gpt-5.4", true))
}

// AC29 / AC31：模型列表过滤。
func TestFilterRetiredOpenAIModelIDs(t *testing.T) {
	oauth := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	models := []string{"gpt-5.5", "gpt-5.4"}
	require.Equal(t, []string{"gpt-5.4"}, filterRetiredOpenAIModelIDs([]Account{oauth}, models))
	require.Equal(t, models, filterRetiredOpenAIModelIDs([]Account{oauth, apiKey}, models))
	replacement := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-6.1-sol"}}}
	require.Equal(t, models, filterRetiredOpenAIModelIDs([]Account{replacement}, models))
}

// AC28：OAuth-only 分组请求 gpt-5.5，诊断带退役标记（handler 据此给迁移说明）。
func TestOpenAIDiagnoseModelAvailabilityMarksRetired(t *testing.T) {
	oauth := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	diag := newModelAvailabilityService([]Account{oauth}, nil).DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5.5", PlatformOpenAI)
	require.True(t, diag.HasAccountsInPool)
	require.False(t, diag.HasModelSupport)
	require.True(t, diag.HasRetiredSubscriptionModel)

	apiKey := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	diag = newModelAvailabilityService([]Account{oauth, apiKey}, nil).DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5.5", PlatformOpenAI)
	require.True(t, diag.HasModelSupport)
}

// AC31：Codex 清单过滤 slug，未知字段保留，ETag 可重算。
func TestFilterRetiredCodexModelsManifest(t *testing.T) {
	body := []byte(`{"models":[{"slug":"gpt-5.5","x":1},{"slug":"gpt-5.4"}],"etag_hint":"keep"}`)
	oauthOnly := newModelAvailabilityService([]Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}}, nil)
	got, changed := oauthOnly.FilterRetiredCodexModelsManifest(context.Background(), nil, body)
	require.True(t, changed)
	require.False(t, strings.Contains(string(got), "gpt-5.5"))
	require.Equal(t, "keep", gjson.GetBytes(got, "etag_hint").String())
	require.NotEqual(t, CodexModelsManifestETag(body), CodexModelsManifestETag(got))

	mixed := newModelAvailabilityService([]Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
	}, nil)
	got, changed = mixed.FilterRetiredCodexModelsManifest(context.Background(), nil, body)
	require.False(t, changed)
	require.Equal(t, body, got)
}
