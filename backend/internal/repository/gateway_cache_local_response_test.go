package repository

import (
	"context"
	"net/http"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheLocalResponseSetGet(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()

	entry := &service.LocalResponseCacheEntry{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        []byte(`{"id":"resp_1"}`),
		Headers:     map[string]string{"Content-Type": "application/json"},
		CreatedAt:   time.Now().UTC(),
	}

	require.NoError(t, cache.SetLocalResponse(ctx, "hash-a", entry, time.Minute))

	got, err := cache.GetLocalResponse(ctx, "hash-a")
	require.NoError(t, err)
	require.Equal(t, entry.StatusCode, got.StatusCode)
	require.Equal(t, entry.ContentType, got.ContentType)
	require.Equal(t, entry.Body, got.Body)
	require.Equal(t, entry.Headers, got.Headers)
	require.True(t, mr.Exists(buildLocalResponseCacheKey("hash-a")))
}

func TestGatewayCacheLocalResponseStats(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()

	require.NoError(t, cache.IncrLocalResponseCacheStats(ctx, "lookup_hit", 2))
	require.NoError(t, cache.IncrLocalResponseCacheStats(ctx, "lookup_bypass:tools_or_functions", 1))
	require.NoError(t, cache.SetLocalResponse(ctx, "hash-a", &service.LocalResponseCacheEntry{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        []byte(`{"ok":true}`),
		Headers:     map[string]string{"Content-Type": "application/json"},
		CreatedAt:   time.Now().UTC(),
	}, time.Minute))

	stats, err := cache.GetLocalResponseCacheStats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Entries)
	require.Equal(t, int64(2), stats.Counters["lookup_hit"])
	require.Equal(t, int64(1), stats.Counters["lookup_bypass:tools_or_functions"])
}

func TestGatewayCacheRecordLocalResponseCacheMinuteStats(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb, db: db}
	groupID := int64(3)
	apiKeyID := int64(10)
	minute := time.Date(2026, 6, 8, 10, 30, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO ops_cache_minute_stats.*ON CONFLICT`).
		WithArgs(
			minute,
			service.PlatformOpenAI,
			"gpt-5.5",
			groupID,
			apiKeyID,
			"exact",
			int64(1),
			int64(1),
			int64(1),
			int64(0),
			int64(0),
			int64(0),
			int64(11),
			int64(7),
			int64(18),
			int64(18),
			int64(18),
			`{}`,
			`{}`,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO ops_cache_minute_stats.*ON CONFLICT`).
		WithArgs(
			minute,
			service.PlatformOpenAI,
			"gpt-5.5",
			groupID,
			apiKeyID,
			"exact",
			int64(1),
			int64(0),
			int64(0),
			int64(1),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			`{"tools_or_functions":1}`,
			`{}`,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = cache.RecordLocalResponseCacheMinuteStats(context.Background(), []*service.LocalResponseCacheMinuteStatEvent{
		{At: minute, Platform: service.PlatformOpenAI, Model: "gpt-5.5", GroupID: &groupID, APIKeyID: &apiKeyID, CacheType: "exact", Candidate: true, Hit: true, InputTokens: 11, OutputTokens: 7, HitTokens: 18},
		{At: minute, Platform: service.PlatformOpenAI, Model: "gpt-5.5", GroupID: &groupID, APIKeyID: &apiKeyID, CacheType: "exact", BypassReason: "tools_or_functions"},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayCacheClearLocalResponseCacheByModelDeletesMatchingAndLegacyOnly(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()
	entry := &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"ok":true}`), CreatedAt: time.Now().UTC()}

	matchingKey := "cache:" + service.LocalResponseCacheRuleVersion + ":openai:12:3:/v1/responses:gpt-5.5:hash-a"
	otherKey := "cache:" + service.LocalResponseCacheRuleVersion + ":openai:12:3:/v1/responses:gpt-4.1:hash-b"
	require.NoError(t, cache.SetLocalResponse(ctx, matchingKey, entry, time.Minute))
	require.NoError(t, cache.SetLocalResponse(ctx, otherKey, entry, time.Minute))
	require.NoError(t, cache.SetLocalResponse(ctx, "legacy-hash", entry, time.Minute))
	require.NoError(t, cache.IncrLocalResponseCacheStats(ctx, "lookup_hit", 1))

	res, err := cache.ClearLocalResponseCache(ctx, service.LocalResponseCacheClearRequest{ClearType: service.LocalResponseCacheClearTypeByModel, Scope: service.LocalResponseCacheClearScope{Models: []string{"gpt-5.5"}}})

	require.NoError(t, err)
	require.Equal(t, int64(2), res.MatchedKeys)
	require.Equal(t, int64(2), res.DeletedKeys)
	require.False(t, mr.Exists(buildLocalResponseCacheKey(matchingKey)))
	require.False(t, mr.Exists(buildLocalResponseCacheKey("legacy-hash")))
	require.True(t, mr.Exists(buildLocalResponseCacheKey(otherKey)))
	require.True(t, mr.Exists(localResponseCacheStatsKey))
}

func TestGatewayCacheClearLocalResponseCacheByTimeUsesEntryCreatedAt(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	insideKey := "cache:" + service.LocalResponseCacheRuleVersion + ":openai:12:3:/v1/responses:gpt-5.5:hash-in"
	outsideKey := "cache:" + service.LocalResponseCacheRuleVersion + ":openai:12:3:/v1/responses:gpt-5.5:hash-out"

	require.NoError(t, cache.SetLocalResponse(ctx, insideKey, &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{}`), CreatedAt: start.Add(10 * time.Minute)}, time.Minute))
	require.NoError(t, cache.SetLocalResponse(ctx, outsideKey, &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{}`), CreatedAt: start.Add(-time.Minute)}, time.Minute))

	res, err := cache.ClearLocalResponseCache(ctx, service.LocalResponseCacheClearRequest{ClearType: service.LocalResponseCacheClearTypeByTime, Scope: service.LocalResponseCacheClearScope{StartTime: &start, EndTime: &end}})

	require.NoError(t, err)
	require.Equal(t, int64(1), res.DeletedKeys)
	require.False(t, mr.Exists(buildLocalResponseCacheKey(insideKey)))
	require.True(t, mr.Exists(buildLocalResponseCacheKey(outsideKey)))
}

func TestGatewayCacheRecordLocalResponseCacheClearAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	cache := &gatewayCache{db: db}
	operatorID := int64(9)

	mock.ExpectExec(`(?s)INSERT INTO ops_cache_clear_audits`).
		WithArgs(&operatorID, service.LocalResponseCacheClearTypeByGroup, `{"group_ids":[3]}`, int64(4), int64(4), service.LocalResponseCacheClearStatusSuccess, "").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = cache.RecordLocalResponseCacheClearAudit(context.Background(), service.LocalResponseCacheClearAudit{OperatorUserID: &operatorID, ClearType: service.LocalResponseCacheClearTypeByGroup, Scope: service.LocalResponseCacheClearScope{GroupIDs: []int64{3}}, MatchedKeys: 4, DeletedKeys: 4, Status: service.LocalResponseCacheClearStatusSuccess})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
