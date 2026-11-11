package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToService_PreservesMessagesDispatchModelConfig(t *testing.T) {
	group := &dbent.Group{
		ID:                    1,
		Name:                  "openai-dispatch",
		Platform:              service.PlatformOpenAI,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		RateMultiplier:        1,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   "gpt-5.4-nano",
			SonnetMappedModel: "gpt-5.3-codex",
			HaikuMappedModel:  "gpt-5.4-mini",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4.5": "gpt-5.4-nano",
			},
		},
	}

	got := groupEntityToService(group)
	require.NotNil(t, got)
	require.Equal(t, group.MessagesDispatchModelConfig, got.MessagesDispatchModelConfig)
}

func TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "getbykey-auth-dispatch-unit@test.com")

	group, err := client.Group.Create().
		SetName("g-auth-dispatch-unit").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		SetAllowMessagesDispatch(true).
		SetDefaultMappedModel("gpt-5.4").
		SetMessagesDispatchModelConfig(service.OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   "gpt-5.4-nano",
			SonnetMappedModel: "gpt-5.3-codex",
			HaikuMappedModel:  "gpt-5.4-mini",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4.5": "gpt-5.4-nano",
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:  user.ID,
		Key:     "sk-getbykey-auth-dispatch-unit",
		Name:    "Dispatch Key Unit",
		GroupID: &group.ID,
		Status:  service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.Equal(t, key.Name, got.Name)
	require.NotNil(t, got.Group)
	require.Equal(t, group.MessagesDispatchModelConfig, got.Group.MessagesDispatchModelConfig)
}

// V03-T11 回归：鉴权查询必须带出 long_context_pricing_enabled，否则鉴权缓存里的分组恒为
// false，CalculateCostForGroup 会把目录长上下文阶梯全部关掉。
func TestAPIKeyRepository_GetByKeyForAuth_SelectsLongContextPricingEnabled_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "getbykey-auth-longctx-unit@test.com")

	for _, enabled := range []bool{true, false} {
		group, err := client.Group.Create().
			SetName("g-auth-longctx-" + map[bool]string{true: "on", false: "off"}[enabled]).
			SetPlatform(service.PlatformOpenAI).
			SetStatus(service.StatusActive).
			SetSubscriptionType(service.SubscriptionTypeStandard).
			SetRateMultiplier(1).
			SetLongContextPricingEnabled(enabled).
			Save(ctx)
		require.NoError(t, err)

		key := &service.APIKey{
			UserID:  user.ID,
			Key:     "sk-getbykey-auth-longctx-" + map[bool]string{true: "on", false: "off"}[enabled],
			Name:    "LongCtx Key",
			GroupID: &group.ID,
			Status:  service.StatusActive,
		}
		require.NoError(t, repo.Create(ctx, key))

		got, err := repo.GetByKeyForAuth(ctx, key.Key)
		require.NoError(t, err)
		require.NotNil(t, got.Group)
		require.Equal(t, enabled, got.Group.LongContextPricingEnabled)
	}
}

// V03-T15：image_count_billing 经分组写入、鉴权查询读回。
func TestAPIKeyRepository_GetByKeyForAuth_SelectsImageCountBilling_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "getbykey-auth-imgcount-unit@test.com")

	group, err := client.Group.Create().
		SetName("g-auth-imgcount").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		SetAllowImageGeneration(true).
		SetImageCountBilling(true).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{UserID: user.ID, Key: "sk-getbykey-auth-imgcount", Name: "ImgCount Key", GroupID: &group.ID, Status: service.StatusActive}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.NotNil(t, got.Group)
	require.True(t, got.Group.ImageCountBilling)
	require.True(t, groupEntityToService(group).ImageCountBilling)
}
