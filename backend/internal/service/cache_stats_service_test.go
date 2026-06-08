package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cacheStatsRepoStub struct {
	rows []*CacheStatsRawRow
}

func (r *cacheStatsRepoStub) ListCacheStatsRows(context.Context, *CacheStatsFilter) ([]*CacheStatsRawRow, error) {
	return r.rows, nil
}

func TestCacheStatsServiceCalculatesRatesAndReasons(t *testing.T) {
	svc := NewCacheStatsService(&cacheStatsRepoStub{rows: []*CacheStatsRawRow{
		{
			Platform: "openai", Model: "gpt-5.5", TotalRequests: 10, CandidateRequests: 8, HitRequests: 3, BypassRequests: 2,
			StoreSuccess: 3, StoreSkip: 1, InputTokens: 100, OutputTokens: 50, HitTokens: 60, CandidateTokens: 120, AllRequestTokens: 150,
			BypassReasonsJSON: `{"temperature_too_high":2}`, StoreSkipReasonsJSON: `{"response_too_large":1}`, EstimatedSavedAmount: "1.25000000",
		},
		{
			Platform: "anthropic", Model: "claude-sonnet-4-5", TotalRequests: 5, CandidateRequests: 2, HitRequests: 1, BypassRequests: 3,
			StoreSuccess: 1, StoreSkip: 2, InputTokens: 80, OutputTokens: 20, HitTokens: 20, CandidateTokens: 40, AllRequestTokens: 100,
			BypassReasonsJSON: `{"disabled":1,"temperature_too_high":2}`, StoreSkipReasonsJSON: `{"unsafe_content":2}`, EstimatedSavedAmount: "0.50000000",
		},
	}})
	got, err := svc.GetStats(context.Background(), &CacheStatsFilter{StartTime: time.Now().Add(-time.Hour), EndTime: time.Now()})
	require.NoError(t, err)
	require.Equal(t, int64(15), got.Summary.TotalRequests)
	require.Equal(t, int64(10), got.Summary.CandidateRequests)
	require.Equal(t, int64(4), got.Summary.HitRequests)
	require.Equal(t, 40.0, got.Summary.RequestHitRate)
	require.Equal(t, int64(80), got.Summary.HitTokens)
	require.Equal(t, int64(160), got.Summary.CandidateTokens)
	require.Equal(t, 50.0, got.Summary.TokensHitRate)
	require.Equal(t, 32.0, got.Summary.OverallTokensCoverage)
	require.Equal(t, "1.75000000", got.Summary.EstimatedSavedAmount)
	require.Len(t, got.ModelRows, 2)
	require.Equal(t, "openai", got.ModelRows[0].Platform)
	require.Equal(t, int64(5), got.ModelRows[0].MissRequests)
	require.Equal(t, "temperature_too_high", got.ModelRows[0].TopBypassReason)
	require.Equal(t, []CacheStatsReasonRow{
		{Reason: "temperature_too_high", Count: 4, Percent: 80},
		{Reason: "disabled", Count: 1, Percent: 20},
	}, got.BypassReasons)
	require.Equal(t, []CacheStatsReasonRow{
		{Reason: "unsafe_content", Count: 2, Percent: 66.67},
		{Reason: "response_too_large", Count: 1, Percent: 33.33},
	}, got.StoreSkipReasons)
}

func TestCacheStatsServiceZeroDenominators(t *testing.T) {
	svc := NewCacheStatsService(&cacheStatsRepoStub{rows: []*CacheStatsRawRow{{Platform: "openai", Model: "gpt-5.5"}}})
	got, err := svc.GetStats(context.Background(), &CacheStatsFilter{})
	require.NoError(t, err)
	require.Equal(t, 0.0, got.Summary.RequestHitRate)
	require.Equal(t, 0.0, got.Summary.TokensHitRate)
	require.Equal(t, 0.0, got.Summary.OverallTokensCoverage)
	require.NotNil(t, got.ModelRows)
	require.NotNil(t, got.BypassReasons)
	require.NotNil(t, got.StoreSkipReasons)
}
