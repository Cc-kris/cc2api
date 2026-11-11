//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// V03-T14：迁移 225 的 channel_model_pricing.image_input_price 读写往返。
func TestChannelRepository_ImageInputPriceRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelRepository(integrationDB)
	p := func(v float64) *float64 { return &v }

	ch := &service.Channel{
		Name:   fmt.Sprintf("imgin-%d", time.Now().UnixNano()),
		Status: service.StatusActive,
		ModelPricing: []service.ChannelModelPricing{
			{Platform: service.PlatformOpenAI, Models: []string{"gpt-5.5"}, BillingMode: service.BillingModeToken, InputPrice: p(5e-6), ImageInputPrice: p(1e-5)},
			{Platform: service.PlatformOpenAI, Models: []string{"gpt-5.4"}, BillingMode: service.BillingModeToken, InputPrice: p(2.5e-6)},
		},
	}
	require.NoError(t, repo.Create(ctx, ch))
	t.Cleanup(func() { _ = repo.Delete(context.Background(), ch.ID) })

	got, err := repo.GetByID(ctx, ch.ID)
	require.NoError(t, err)
	byModel := map[string]service.ChannelModelPricing{}
	for _, mp := range got.ModelPricing {
		byModel[mp.Models[0]] = mp
	}
	gpt55 := byModel["gpt-5.5"]
	require.NotNil(t, gpt55.ImageInputPrice)
	require.InDelta(t, 1e-5, *gpt55.ImageInputPrice, 1e-15)
	require.Nil(t, byModel["gpt-5.4"].ImageInputPrice, "未配置时保持 NULL")

	gpt55.ImageInputPrice = p(2e-5)
	require.NoError(t, repo.UpdateModelPricing(ctx, &gpt55))
	list, err := repo.ListModelPricing(ctx, ch.ID)
	require.NoError(t, err)
	found := false
	for _, mp := range list {
		if mp.ID == gpt55.ID {
			found = true
			require.NotNil(t, mp.ImageInputPrice)
			require.InDelta(t, 2e-5, *mp.ImageInputPrice, 1e-15)
		}
	}
	require.True(t, found)
}

// V03-T14：迁移 226 的 usage_logs.image_input_tokens / image_input_cost 经批量写入路径往返。
func TestUsageLogRepository_ImageInputTokensRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-imgin-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-imgin-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-imgin-" + uuid.NewString()})

	newLog := func() *service.UsageLog {
		return &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
			RequestID: uuid.NewString(), Model: "gpt-5.5",
			InputTokens: 1500, ImageInputTokens: 1000, OutputTokens: 200,
			InputCost: 0.0025, ImageInputCost: 0.01, OutputCost: 0.006,
			TotalCost: 0.0185, ActualCost: 0.0185, CreatedAt: time.Now().UTC(),
		}
	}

	created := newLog()
	inserted, err := repo.Create(ctx, created)
	require.NoError(t, err)
	require.True(t, inserted)
	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, 1000, got.ImageInputTokens)
	require.InDelta(t, 0.01, got.ImageInputCost, 1e-12)
	require.InDelta(t, 0.0025, got.InputCost, 1e-12)

	bestEffort := newLog()
	require.NoError(t, repo.CreateBestEffort(ctx, bestEffort))
	var tokens int
	var cost float64
	require.Eventually(t, func() bool {
		err := integrationDB.QueryRowContext(ctx,
			"SELECT image_input_tokens, image_input_cost FROM usage_logs WHERE request_id = $1", bestEffort.RequestID,
		).Scan(&tokens, &cost)
		return err == nil
	}, 10*time.Second, 50*time.Millisecond)
	require.Equal(t, 1000, tokens)
	require.InDelta(t, 0.01, cost, 1e-12)
}
