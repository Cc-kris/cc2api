package repository

import (
	"context"
	"net/http"
	"strings"
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

func TestGatewayCacheListLocalResponseCacheClearAudits(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	cache := &gatewayCache{db: db}
	ctx := context.Background()
	operatorID := int64(9)
	start := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	createdAt := start.Add(30 * time.Minute)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_cache_clear_audits WHERE created_at >= \$1 AND created_at <= \$2 AND operator_user_id = \$3 AND clear_type = \$4 AND status = \$5`).
		WithArgs(start, end, operatorID, service.LocalResponseCacheClearTypeByModel, service.LocalResponseCacheClearStatusSuccess).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT id, operator_user_id, clear_type, scope::text, matched_keys, deleted_keys, status, error_message, created_at.*FROM ops_cache_clear_audits WHERE created_at >= \$1 AND created_at <= \$2 AND operator_user_id = \$3 AND clear_type = \$4 AND status = \$5.*LIMIT \$6 OFFSET \$7`).
		WithArgs(start, end, operatorID, service.LocalResponseCacheClearTypeByModel, service.LocalResponseCacheClearStatusSuccess, 10, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "operator_user_id", "clear_type", "scope", "matched_keys", "deleted_keys", "status", "error_message", "created_at"}).
			AddRow(7, operatorID, service.LocalResponseCacheClearTypeByModel, `{"models":["gpt-5.5"]}`, 3, 3, service.LocalResponseCacheClearStatusSuccess, nil, createdAt))

	page, err := cache.ListLocalResponseCacheClearAudits(ctx, service.LocalResponseCacheClearAuditFilter{
		Page:           2,
		PageSize:       10,
		StartTime:      &start,
		EndTime:        &end,
		OperatorUserID: &operatorID,
		ClearType:      service.LocalResponseCacheClearTypeByModel,
		Status:         service.LocalResponseCacheClearStatusSuccess,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	require.Equal(t, int64(7), page.Items[0].ID)
	require.Equal(t, operatorID, *page.Items[0].OperatorUserID)
	require.Equal(t, []string{"gpt-5.5"}, page.Items[0].Scope.Models)
	require.Equal(t, createdAt, page.Items[0].CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayCacheEvictLocalResponseCacheLRUDeletesOldestUntilUnderCapacity(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()
	base := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)

	recent := &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"id":"recent","payload":"` + strings.Repeat("r", 128) + `"}`), CreatedAt: base, LastAccessedAt: base.Add(3 * time.Minute)}
	oldest := &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"id":"oldest","payload":"` + strings.Repeat("o", 128) + `"}`), CreatedAt: base, LastAccessedAt: base.Add(time.Minute)}
	middle := &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"id":"middle","payload":"` + strings.Repeat("m", 128) + `"}`), CreatedAt: base, LastAccessedAt: base.Add(2 * time.Minute)}
	require.NoError(t, cache.SetLocalResponse(ctx, "recent", recent, time.Minute))
	require.NoError(t, cache.SetLocalResponse(ctx, "oldest", oldest, time.Minute))
	require.NoError(t, cache.SetLocalResponse(ctx, "middle", middle, time.Minute))

	statsBefore, err := cache.GetLocalResponseCacheStats(ctx)
	require.NoError(t, err)
	capacity := statsBefore.Bytes - 1

	res, err := cache.EvictLocalResponseCache(ctx, service.LocalResponseCacheEvictionRequest{CapacityBytes: capacity, Policy: "LRU"})

	require.NoError(t, err)
	require.GreaterOrEqual(t, res.DeletedKeys, int64(1))
	require.False(t, mr.Exists(buildLocalResponseCacheKey("oldest")))
	require.True(t, mr.Exists(buildLocalResponseCacheKey("recent")))
	statsAfter, err := cache.GetLocalResponseCacheStats(ctx)
	require.NoError(t, err)
	require.LessOrEqual(t, statsAfter.Bytes, capacity)
	require.Greater(t, statsAfter.Counters["eviction_deleted_keys"], int64(0))
}

func TestGatewayCacheEvictLocalResponseCacheLFUDeletesLowestHitCountFirst(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()
	base := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)

	hot := &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"id":"hot","payload":"` + strings.Repeat("h", 128) + `"}`), CreatedAt: base, LastAccessedAt: base.Add(time.Minute), HitCount: 10}
	cold := &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"id":"cold","payload":"` + strings.Repeat("c", 128) + `"}`), CreatedAt: base, LastAccessedAt: base.Add(2 * time.Minute), HitCount: 0}
	require.NoError(t, cache.SetLocalResponse(ctx, "hot", hot, time.Minute))
	require.NoError(t, cache.SetLocalResponse(ctx, "cold", cold, time.Minute))

	statsBefore, err := cache.GetLocalResponseCacheStats(ctx)
	require.NoError(t, err)
	capacity := statsBefore.Bytes - 1

	_, err = cache.EvictLocalResponseCache(ctx, service.LocalResponseCacheEvictionRequest{CapacityBytes: capacity, Policy: "LFU"})

	require.NoError(t, err)
	require.False(t, mr.Exists(buildLocalResponseCacheKey("cold")))
	require.True(t, mr.Exists(buildLocalResponseCacheKey("hot")))
}

func TestGatewayCacheGetLocalResponseUpdatesAccessMetadata(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()
	createdAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	require.NoError(t, cache.SetLocalResponse(ctx, "hit", &service.LocalResponseCacheEntry{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"ok":true}`), CreatedAt: createdAt}, time.Minute))

	got, err := cache.GetLocalResponse(ctx, "hit")
	require.NoError(t, err)
	require.Equal(t, int64(1), got.HitCount)
	require.False(t, got.LastAccessedAt.IsZero())

	stored, err := cache.GetLocalResponse(ctx, "hit")
	require.NoError(t, err)
	require.Equal(t, int64(2), stored.HitCount)
}

func TestGatewayCacheLocalResponseHotspotsRanksAndFilters(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()
	groupID := int64(3)
	apiKeyID := int64(12)
	hitAt := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		require.NoError(t, cache.RecordLocalResponseCacheHotspot(ctx, service.LocalResponseCacheHotspotEvent{CacheKey: "hot", Platform: service.PlatformOpenAI, Model: "gpt-5.5", GroupID: &groupID, APIKeyID: &apiKeyID, HitTokens: 10, HitAt: hitAt, Window: time.Hour}))
	}
	require.NoError(t, cache.RecordLocalResponseCacheHotspot(ctx, service.LocalResponseCacheHotspotEvent{CacheKey: "cold", Platform: service.PlatformOpenAI, Model: "gpt-4o", GroupID: &groupID, APIKeyID: &apiKeyID, HitTokens: 5, HitAt: hitAt.Add(time.Minute), Window: time.Hour}))

	items, err := cache.ListLocalResponseCacheHotspots(ctx, service.LocalResponseCacheHotspotFilter{Window: time.Hour, Limit: 10})

	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "hot", items[0].CacheKey)
	require.Equal(t, int64(3), items[0].HitCount)
	require.Equal(t, int64(30), items[0].HitTokens)
	require.Equal(t, service.PlatformOpenAI, items[0].Platform)
	require.Equal(t, "gpt-5.5", items[0].Model)
	require.Equal(t, groupID, *items[0].GroupID)
	require.Equal(t, apiKeyID, *items[0].APIKeyID)
	require.Equal(t, hitAt, items[0].LastHitAt)

	filtered, err := cache.ListLocalResponseCacheHotspots(ctx, service.LocalResponseCacheHotspotFilter{Window: time.Hour, Limit: 10, Model: "gpt-4o"})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "cold", filtered[0].CacheKey)
}

func TestGatewayCacheLocalResponseHotspotsLimitAndNoSensitivePayload(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()

	require.NoError(t, cache.RecordLocalResponseCacheHotspot(ctx, service.LocalResponseCacheHotspotEvent{CacheKey: "cache-key-only", Platform: service.PlatformOpenAI, Model: "gpt-5.5", HitTokens: 1, HitAt: time.Now().UTC(), Window: time.Hour}))

	items, err := cache.ListLocalResponseCacheHotspots(ctx, service.LocalResponseCacheHotspotFilter{Window: time.Hour, Limit: 1})

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "cache-key-only", items[0].CacheKey)
	require.Empty(t, strings.Contains(items[0].CacheKey, "{"))
}
