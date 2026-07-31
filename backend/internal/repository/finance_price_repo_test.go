//go:build unit

package repository

import (
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestSelectHistoricalUpstreamPriceOrder(t *testing.T) {
	now := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	fast := "fast"
	versions := []*dbent.UpstreamModelPriceVersion{
		{ID: 1, ModelPattern: "gpt-*", IsWildcard: true, EffectiveFrom: now.Add(-time.Hour)},
		{ID: 2, ModelPattern: "gpt-5*", IsWildcard: true, EffectiveFrom: now.Add(-2 * time.Hour)},
		{ID: 3, ModelPattern: "gpt-5.5", EffectiveFrom: now.Add(-3 * time.Hour)},
		{ID: 4, ModelPattern: "gpt-5.5", ServiceTier: &fast, EffectiveFrom: now.Add(-4 * time.Hour)},
	}

	require.Equal(t, int64(4), selectHistoricalUpstreamPrice(versions, "GPT-5.5", "fast").ID)
	require.Equal(t, int64(3), selectHistoricalUpstreamPrice(versions, "GPT-5.5", "standard").ID)
	require.Equal(t, int64(2), selectHistoricalUpstreamPrice(versions, "gpt-5.4", "standard").ID)
	require.Nil(t, selectHistoricalUpstreamPrice(versions, "claude-test", "standard"))
}

func TestFinanceExchangeRate(t *testing.T) {
	require.Equal(t, "1", financeExchangeRate("USD").String())
	require.Equal(t, "0.14", financeExchangeRate("CNY", map[string]any{"usd_exchange_rate": "0.14"}).String())
	require.True(t, financeExchangeRate("CNY", map[string]any{"usd_exchange_rate": "bad"}).IsZero())
}
