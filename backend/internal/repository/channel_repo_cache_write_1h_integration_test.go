//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// V03-T12：迁移 227 的 cache_write_1h_price 在四张定价表上读写往返。
func TestChannelRepository_CacheWrite1hPriceRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelRepository(integrationDB)
	p := func(v float64) *float64 { return &v }

	ch := &service.Channel{
		Name:   fmt.Sprintf("cw1h-%d", time.Now().UnixNano()),
		Status: service.StatusActive,
		ModelPricing: []service.ChannelModelPricing{
			{
				Platform: service.PlatformAnthropic, Models: []string{"claude-sonnet-4-6"}, BillingMode: service.BillingModeToken,
				CacheWritePrice: p(3.75e-6), CacheWrite1hPrice: p(6e-6),
				Intervals: []service.PricingInterval{{MinTokens: 0, CacheWritePrice: p(3.75e-6), CacheWrite1hPrice: p(7e-6)}},
			},
			{Platform: service.PlatformAnthropic, Models: []string{"claude-opus-5"}, BillingMode: service.BillingModeToken, CacheWritePrice: p(6.25e-6)},
		},
		AccountStatsPricingRules: []service.AccountStatsPricingRule{{
			Name: "rule", GroupIDs: []int64{1}, AccountIDs: []int64{},
			Pricing: []service.ChannelModelPricing{{
				Platform: service.PlatformAnthropic, Models: []string{"claude-sonnet-4-6"}, BillingMode: service.BillingModeToken,
				CacheWritePrice: p(3e-6), CacheWrite1hPrice: p(5e-6),
				Intervals: []service.PricingInterval{{MinTokens: 0, CacheWrite1hPrice: p(4e-6)}},
			}},
		}},
	}
	require.NoError(t, repo.Create(ctx, ch))
	t.Cleanup(func() { _ = repo.Delete(context.Background(), ch.ID) })

	got, err := repo.GetByID(ctx, ch.ID)
	require.NoError(t, err)
	require.Len(t, got.ModelPricing, 2)
	byModel := map[string]service.ChannelModelPricing{}
	for _, mp := range got.ModelPricing {
		byModel[mp.Models[0]] = mp
	}
	sonnet := byModel["claude-sonnet-4-6"]
	require.NotNil(t, sonnet.CacheWrite1hPrice)
	require.InDelta(t, 6e-6, *sonnet.CacheWrite1hPrice, 1e-15)
	require.Len(t, sonnet.Intervals, 1)
	require.NotNil(t, sonnet.Intervals[0].CacheWrite1hPrice)
	require.InDelta(t, 7e-6, *sonnet.Intervals[0].CacheWrite1hPrice, 1e-15)
	require.Nil(t, byModel["claude-opus-5"].CacheWrite1hPrice, "未配置时保持 NULL")

	require.Len(t, got.AccountStatsPricingRules, 1)
	rule := got.AccountStatsPricingRules[0]
	require.Len(t, rule.Pricing, 1)
	require.NotNil(t, rule.Pricing[0].CacheWrite1hPrice)
	require.InDelta(t, 5e-6, *rule.Pricing[0].CacheWrite1hPrice, 1e-15)
	require.Len(t, rule.Pricing[0].Intervals, 1)
	require.InDelta(t, 4e-6, *rule.Pricing[0].Intervals[0].CacheWrite1hPrice, 1e-15)

	// UpdateModelPricing 写入并能清空 1h 价。
	sonnet.CacheWrite1hPrice = nil
	require.NoError(t, repo.UpdateModelPricing(ctx, &sonnet))
	list, err := repo.ListModelPricing(ctx, ch.ID)
	require.NoError(t, err)
	for _, mp := range list {
		if mp.ID == sonnet.ID {
			require.Nil(t, mp.CacheWrite1hPrice)
		}
	}
}
