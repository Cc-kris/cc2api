package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

type gatewayCache struct {
	rdb *redis.Client
	db  *sql.DB
}

func NewGatewayCache(rdb *redis.Client, db *sql.DB) service.GatewayCache {
	return &gatewayCache{rdb: rdb, db: db}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

const localResponseCachePrefix = "local_response_cache:v1:"
const localResponseCacheStatsKey = "local_response_cache:stats:v1:counters"
const localResponseCacheHotspotZSetPrefix = "local_response_cache:hotspots:v1:"
const localResponseCacheHotspotMetaPrefix = "local_response_cache:hotspot_meta:v1:"
const localResponseCacheHotspotTokensPrefix = "local_response_cache:hotspot_tokens:v1:"

func buildLocalResponseCacheKey(hash string) string {
	return localResponseCachePrefix + hash
}

func (c *gatewayCache) GetLocalResponse(ctx context.Context, key string) (*service.LocalResponseCacheEntry, error) {
	if c == nil || c.rdb == nil {
		return nil, redis.Nil
	}
	redisKey := buildLocalResponseCacheKey(key)
	payload, err := c.rdb.Get(ctx, redisKey).Bytes()
	if err != nil {
		return nil, err
	}
	var entry service.LocalResponseCacheEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return nil, err
	}
	c.touchLocalResponseCacheEntry(ctx, redisKey, &entry)
	return &entry, nil
}

func (c *gatewayCache) SetLocalResponse(ctx context.Context, key string, entry *service.LocalResponseCacheEntry, ttl time.Duration) error {
	if c == nil || c.rdb == nil || entry == nil {
		return nil
	}
	prepared := *entry
	if prepared.CreatedAt.IsZero() {
		prepared.CreatedAt = time.Now().UTC()
	}
	if prepared.LastAccessedAt.IsZero() {
		prepared.LastAccessedAt = prepared.CreatedAt
	}
	if prepared.HitCount < 0 {
		prepared.HitCount = 0
	}
	payload, err := json.Marshal(&prepared)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, buildLocalResponseCacheKey(key), payload, ttl).Err()
}

func (c *gatewayCache) touchLocalResponseCacheEntry(ctx context.Context, redisKey string, entry *service.LocalResponseCacheEntry) {
	if c == nil || c.rdb == nil || entry == nil {
		return
	}
	entry.LastAccessedAt = time.Now().UTC()
	entry.HitCount++
	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}
	ttl, err := c.rdb.TTL(ctx, redisKey).Result()
	if err != nil {
		return
	}
	switch {
	case ttl > 0:
		_ = c.rdb.Set(ctx, redisKey, payload, ttl).Err()
	case ttl == -1:
		_ = c.rdb.Set(ctx, redisKey, payload, 0).Err()
	}
}

type localResponseCacheEvictionCandidate struct {
	Key            string
	Bytes          int64
	CreatedAt      time.Time
	LastAccessedAt time.Time
	HitCount       int64
}

func (c *gatewayCache) EvictLocalResponseCache(ctx context.Context, req service.LocalResponseCacheEvictionRequest) (*service.LocalResponseCacheEvictionResult, error) {
	result := &service.LocalResponseCacheEvictionResult{}
	if c == nil || c.rdb == nil || req.CapacityBytes <= 0 {
		return result, nil
	}
	candidates, totalBytes, err := c.collectLocalResponseCacheEvictionCandidates(ctx)
	if err != nil {
		return result, err
	}
	result.BytesBefore = totalBytes
	result.BytesAfter = totalBytes
	result.ScannedKeys = int64(len(candidates))
	if totalBytes <= req.CapacityBytes || len(candidates) == 0 {
		return result, nil
	}
	sortLocalResponseCacheEvictionCandidates(candidates, req.Policy)
	keysToDelete := make([]string, 0)
	bytesToDelete := int64(0)
	for _, candidate := range candidates {
		if result.BytesAfter-bytesToDelete <= req.CapacityBytes {
			break
		}
		keysToDelete = append(keysToDelete, candidate.Key)
		bytesToDelete += candidate.Bytes
	}
	if len(keysToDelete) == 0 {
		return result, nil
	}
	deleted, err := c.rdb.Del(ctx, keysToDelete...).Result()
	if err != nil {
		return result, err
	}
	result.DeletedKeys = deleted
	result.DeletedBytes = bytesToDelete
	result.BytesAfter = maxInt64(0, totalBytes-bytesToDelete)
	_ = c.IncrLocalResponseCacheStats(ctx, "eviction_deleted_keys", deleted)
	_ = c.IncrLocalResponseCacheStats(ctx, "eviction_deleted_bytes", bytesToDelete)
	return result, nil
}

func (c *gatewayCache) collectLocalResponseCacheEvictionCandidates(ctx context.Context) ([]localResponseCacheEvictionCandidate, int64, error) {
	candidates := make([]localResponseCacheEvictionCandidate, 0)
	totalBytes := int64(0)
	iter := c.rdb.Scan(ctx, 0, localResponseCachePrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		redisKey := iter.Val()
		candidate, ok, err := c.localResponseCacheEvictionCandidate(ctx, redisKey)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
		totalBytes += candidate.Bytes
	}
	if err := iter.Err(); err != nil {
		return nil, 0, err
	}
	return candidates, totalBytes, nil
}

func (c *gatewayCache) localResponseCacheEvictionCandidate(ctx context.Context, redisKey string) (localResponseCacheEvictionCandidate, bool, error) {
	payload, err := c.rdb.Get(ctx, redisKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return localResponseCacheEvictionCandidate{}, false, nil
		}
		return localResponseCacheEvictionCandidate{}, false, err
	}
	var entry service.LocalResponseCacheEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return localResponseCacheEvictionCandidate{}, false, nil
	}
	bytesUsed, err := c.rdb.MemoryUsage(ctx, redisKey).Result()
	if err != nil || bytesUsed <= 0 {
		bytesUsed = int64(len(payload))
	}
	return localResponseCacheEvictionCandidate{
		Key:            redisKey,
		Bytes:          bytesUsed,
		CreatedAt:      entry.CreatedAt,
		LastAccessedAt: entry.LastAccessedAt,
		HitCount:       entry.HitCount,
	}, true, nil
}

func sortLocalResponseCacheEvictionCandidates(candidates []localResponseCacheEvictionCandidate, policy string) {
	policy = strings.TrimSpace(strings.ToUpper(policy))
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		switch policy {
		case "LFU", "W-TINYLFU":
			if left.HitCount != right.HitCount {
				return left.HitCount < right.HitCount
			}
		}
		leftAt := left.LastAccessedAt
		if leftAt.IsZero() {
			leftAt = left.CreatedAt
		}
		rightAt := right.LastAccessedAt
		if rightAt.IsZero() {
			rightAt = right.CreatedAt
		}
		if !leftAt.Equal(rightAt) {
			return leftAt.Before(rightAt)
		}
		return left.Key < right.Key
	})
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

type localResponseCacheHotspotMeta struct {
	Platform  string `json:"platform"`
	Model     string `json:"model"`
	GroupID   *int64 `json:"group_id,omitempty"`
	APIKeyID  *int64 `json:"api_key_id,omitempty"`
	LastHitAt string `json:"last_hit_at"`
}

func (c *gatewayCache) RecordLocalResponseCacheHotspot(ctx context.Context, event service.LocalResponseCacheHotspotEvent) error {
	if c == nil || c.rdb == nil || strings.TrimSpace(event.CacheKey) == "" {
		return nil
	}
	window := normalizeLocalResponseCacheHotspotWindow(event.Window)
	hitAt := event.HitAt
	if hitAt.IsZero() {
		hitAt = time.Now().UTC()
	}
	member := strings.TrimSpace(event.CacheKey)
	meta := localResponseCacheHotspotMeta{
		Platform:  strings.TrimSpace(event.Platform),
		Model:     strings.TrimSpace(event.Model),
		GroupID:   cloneRepositoryInt64Ptr(event.GroupID),
		APIKeyID:  cloneRepositoryInt64Ptr(event.APIKeyID),
		LastHitAt: hitAt.UTC().Format(time.RFC3339Nano),
	}
	encodedMeta, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	zKey := localResponseCacheHotspotZSetKey(window)
	metaKey := localResponseCacheHotspotMetaKey(window)
	tokensKey := localResponseCacheHotspotTokensKey(window)
	expireAfter := window * 2
	pipe := c.rdb.Pipeline()
	pipe.ZIncrBy(ctx, zKey, 1, member)
	pipe.HSet(ctx, metaKey, member, string(encodedMeta))
	if event.HitTokens > 0 {
		pipe.HIncrBy(ctx, tokensKey, member, event.HitTokens)
	}
	pipe.Expire(ctx, zKey, expireAfter)
	pipe.Expire(ctx, metaKey, expireAfter)
	pipe.Expire(ctx, tokensKey, expireAfter)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *gatewayCache) ListLocalResponseCacheHotspots(ctx context.Context, filter service.LocalResponseCacheHotspotFilter) ([]service.LocalResponseCacheHotspot, error) {
	if c == nil || c.rdb == nil {
		return []service.LocalResponseCacheHotspot{}, nil
	}
	window := normalizeLocalResponseCacheHotspotWindow(filter.Window)
	limit := normalizeLocalResponseCacheHotspotLimit(filter.Limit)
	readLimit := int64(limit * 10)
	if readLimit < 100 {
		readLimit = 100
	}
	if readLimit > 1000 {
		readLimit = 1000
	}
	items, err := c.rdb.ZRevRangeWithScores(ctx, localResponseCacheHotspotZSetKey(window), 0, readLimit-1).Result()
	if err != nil {
		if err == redis.Nil {
			return []service.LocalResponseCacheHotspot{}, nil
		}
		return nil, err
	}
	out := make([]service.LocalResponseCacheHotspot, 0, min(limit, len(items)))
	for _, item := range items {
		member, ok := item.Member.(string)
		if !ok || strings.TrimSpace(member) == "" {
			continue
		}
		meta, err := c.getLocalResponseCacheHotspotMeta(ctx, window, member)
		if err != nil {
			return nil, err
		}
		if !localResponseCacheHotspotMatchesFilter(meta, filter) {
			continue
		}
		tokens, err := c.getLocalResponseCacheHotspotTokens(ctx, window, member)
		if err != nil {
			return nil, err
		}
		lastHitAt, _ := time.Parse(time.RFC3339Nano, meta.LastHitAt)
		out = append(out, service.LocalResponseCacheHotspot{
			Rank:      len(out) + 1,
			CacheKey:  member,
			Platform:  meta.Platform,
			Model:     meta.Model,
			GroupID:   cloneRepositoryInt64Ptr(meta.GroupID),
			APIKeyID:  cloneRepositoryInt64Ptr(meta.APIKeyID),
			HitCount:  int64(item.Score),
			HitTokens: tokens,
			LastHitAt: lastHitAt,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (c *gatewayCache) getLocalResponseCacheHotspotMeta(ctx context.Context, window time.Duration, member string) (localResponseCacheHotspotMeta, error) {
	raw, err := c.rdb.HGet(ctx, localResponseCacheHotspotMetaKey(window), member).Result()
	if err != nil {
		if err == redis.Nil {
			return localResponseCacheHotspotMeta{}, nil
		}
		return localResponseCacheHotspotMeta{}, err
	}
	var meta localResponseCacheHotspotMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return localResponseCacheHotspotMeta{}, nil
	}
	return meta, nil
}

func (c *gatewayCache) getLocalResponseCacheHotspotTokens(ctx context.Context, window time.Duration, member string) (int64, error) {
	raw, err := c.rdb.HGet(ctx, localResponseCacheHotspotTokensKey(window), member).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	var tokens int64
	if _, scanErr := fmt.Sscan(raw, &tokens); scanErr != nil {
		return 0, nil
	}
	return tokens, nil
}

func localResponseCacheHotspotMatchesFilter(meta localResponseCacheHotspotMeta, filter service.LocalResponseCacheHotspotFilter) bool {
	if strings.TrimSpace(filter.Platform) != "" && !strings.EqualFold(strings.TrimSpace(filter.Platform), strings.TrimSpace(meta.Platform)) {
		return false
	}
	if strings.TrimSpace(filter.Model) != "" && !strings.EqualFold(strings.TrimSpace(filter.Model), strings.TrimSpace(meta.Model)) {
		return false
	}
	if filter.GroupID != nil {
		if meta.GroupID == nil || *meta.GroupID != *filter.GroupID {
			return false
		}
	}
	if filter.APIKeyID != nil {
		if meta.APIKeyID == nil || *meta.APIKeyID != *filter.APIKeyID {
			return false
		}
	}
	return true
}

func normalizeLocalResponseCacheHotspotWindow(window time.Duration) time.Duration {
	switch window {
	case 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour:
		return window
	default:
		return time.Hour
	}
}

func normalizeLocalResponseCacheHotspotLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func localResponseCacheHotspotZSetKey(window time.Duration) string {
	return fmt.Sprintf("%s%d", localResponseCacheHotspotZSetPrefix, int64(window.Seconds()))
}

func localResponseCacheHotspotMetaKey(window time.Duration) string {
	return fmt.Sprintf("%s%d", localResponseCacheHotspotMetaPrefix, int64(window.Seconds()))
}

func localResponseCacheHotspotTokensKey(window time.Duration) string {
	return fmt.Sprintf("%s%d", localResponseCacheHotspotTokensPrefix, int64(window.Seconds()))
}

func cloneRepositoryInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (c *gatewayCache) IncrLocalResponseCacheStats(ctx context.Context, field string, delta int64) error {
	if c == nil || c.rdb == nil || field == "" || delta == 0 {
		return nil
	}
	return c.rdb.HIncrBy(ctx, localResponseCacheStatsKey, field, delta).Err()
}

func (c *gatewayCache) GetLocalResponseCacheStats(ctx context.Context) (*service.LocalResponseCacheStats, error) {
	stats := &service.LocalResponseCacheStats{Counters: map[string]int64{}}
	if c == nil || c.rdb == nil {
		return stats, nil
	}
	counters, err := c.rdb.HGetAll(ctx, localResponseCacheStatsKey).Result()
	if err != nil && err != redis.Nil {
		return stats, err
	}
	for field, raw := range counters {
		var value int64
		if _, scanErr := fmt.Sscan(raw, &value); scanErr == nil {
			stats.Counters[field] = value
		}
	}

	iter := c.rdb.Scan(ctx, 0, localResponseCachePrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if key == localResponseCacheStatsKey {
			continue
		}
		stats.Entries++
		bytes, memErr := c.rdb.MemoryUsage(ctx, key).Result()
		if memErr == nil && bytes > 0 {
			stats.Bytes += bytes
		}
	}
	if err := iter.Err(); err != nil {
		return stats, err
	}
	return stats, nil
}

func (c *gatewayCache) ClearLocalResponseCache(ctx context.Context, req service.LocalResponseCacheClearRequest) (*service.LocalResponseCacheClearResult, error) {
	result := &service.LocalResponseCacheClearResult{ClearType: req.ClearType, Scope: req.Scope, Status: service.LocalResponseCacheClearStatusSuccess}
	if c == nil || c.rdb == nil {
		return result, nil
	}

	keysToDelete := make([]string, 0)
	iter := c.rdb.Scan(ctx, 0, localResponseCachePrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		redisKey := iter.Val()
		if redisKey == localResponseCacheStatsKey {
			continue
		}
		matched, err := c.localResponseCacheKeyMatchesClearScope(ctx, redisKey, req)
		if err != nil {
			return result, err
		}
		if !matched {
			continue
		}
		result.MatchedKeys++
		keysToDelete = append(keysToDelete, redisKey)
	}
	if err := iter.Err(); err != nil {
		return result, err
	}
	if len(keysToDelete) == 0 {
		return result, nil
	}
	deleted, err := c.rdb.Del(ctx, keysToDelete...).Result()
	result.DeletedKeys = deleted
	if err != nil {
		result.Status = service.LocalResponseCacheClearStatusPartialSuccess
		result.ErrorMessage = err.Error()
		return result, err
	}
	if deleted < result.MatchedKeys {
		result.Status = service.LocalResponseCacheClearStatusPartialSuccess
		result.ErrorMessage = "some matched cache keys were not deleted"
	}
	return result, nil
}

func (c *gatewayCache) RecordLocalResponseCacheClearAudit(ctx context.Context, audit service.LocalResponseCacheClearAudit) error {
	if c == nil || c.db == nil {
		return nil
	}
	scope, err := json.Marshal(audit.Scope)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
INSERT INTO ops_cache_clear_audits (
  operator_user_id, clear_type, scope, matched_keys, deleted_keys, status, error_message
) VALUES ($1, $2, $3::jsonb, $4, $5, $6, NULLIF($7, ''))`,
		audit.OperatorUserID,
		audit.ClearType,
		string(scope),
		audit.MatchedKeys,
		audit.DeletedKeys,
		audit.Status,
		audit.ErrorMessage,
	)
	return err
}

func (c *gatewayCache) ListLocalResponseCacheClearAudits(ctx context.Context, filter service.LocalResponseCacheClearAuditFilter) (*service.LocalResponseCacheClearAuditPage, error) {
	page := &service.LocalResponseCacheClearAuditPage{Items: []service.LocalResponseCacheClearAuditRecord{}, Page: filter.Page, PageSize: filter.PageSize}
	if c == nil || c.db == nil {
		return page, nil
	}

	where := make([]string, 0, 5)
	args := make([]any, 0, 8)
	if filter.StartTime != nil {
		args = append(args, *filter.StartTime)
		where = append(where, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if filter.EndTime != nil {
		args = append(args, *filter.EndTime)
		where = append(where, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if filter.OperatorUserID != nil {
		args = append(args, *filter.OperatorUserID)
		where = append(where, fmt.Sprintf("operator_user_id = $%d", len(args)))
	}
	if strings.TrimSpace(filter.ClearType) != "" {
		args = append(args, strings.TrimSpace(filter.ClearType))
		where = append(where, fmt.Sprintf("clear_type = $%d", len(args)))
	}
	if strings.TrimSpace(filter.Status) != "" {
		args = append(args, strings.TrimSpace(filter.Status))
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	countArgs := append([]any(nil), args...)
	if err := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_cache_clear_audits"+whereSQL, countArgs...).Scan(&page.Total); err != nil {
		return nil, err
	}

	limit := filter.PageSize
	if limit <= 0 {
		limit = 20
	}
	currentPage := filter.Page
	if currentPage <= 0 {
		currentPage = 1
	}
	offset := (currentPage - 1) * limit
	page.Page = currentPage
	page.PageSize = limit

	queryArgs := append([]any(nil), args...)
	queryArgs = append(queryArgs, limit, offset)
	rows, err := c.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id, operator_user_id, clear_type, scope::text, matched_keys, deleted_keys, status, error_message, created_at
FROM ops_cache_clear_audits%s
ORDER BY created_at DESC, id DESC
LIMIT $%d OFFSET $%d`, whereSQL, len(queryArgs)-1, len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var record service.LocalResponseCacheClearAuditRecord
		var operator sql.NullInt64
		var scopeRaw string
		var errorMessage sql.NullString
		if err := rows.Scan(
			&record.ID,
			&operator,
			&record.ClearType,
			&scopeRaw,
			&record.MatchedKeys,
			&record.DeletedKeys,
			&record.Status,
			&errorMessage,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		if operator.Valid {
			record.OperatorUserID = &operator.Int64
		}
		if errorMessage.Valid {
			record.ErrorMessage = errorMessage.String
		}
		if strings.TrimSpace(scopeRaw) != "" {
			if err := json.Unmarshal([]byte(scopeRaw), &record.Scope); err != nil {
				return nil, err
			}
		}
		page.Items = append(page.Items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return page, nil
}

type localResponseCacheKeyMeta struct {
	Platform string
	APIKeyID int64
	GroupID  int64
	Model    string
}

func (c *gatewayCache) localResponseCacheKeyMatchesClearScope(ctx context.Context, redisKey string, req service.LocalResponseCacheClearRequest) (bool, error) {
	switch req.ClearType {
	case service.LocalResponseCacheClearTypeAll:
		return true, nil
	case service.LocalResponseCacheClearTypeByTime:
		entry, err := c.getLocalResponseByRedisKey(ctx, redisKey)
		if err != nil {
			return false, err
		}
		if entry == nil || req.Scope.StartTime == nil || req.Scope.EndTime == nil {
			return false, nil
		}
		createdAt := entry.CreatedAt
		return !createdAt.Before(*req.Scope.StartTime) && !createdAt.After(*req.Scope.EndTime), nil
	case service.LocalResponseCacheClearTypeExpired:
		entry, err := c.getLocalResponseByRedisKey(ctx, redisKey)
		if err != nil {
			return false, err
		}
		if entry == nil {
			return true, nil
		}
		if ttl, err := c.rdb.TTL(ctx, redisKey).Result(); err == nil && ttl == -1 {
			return true, nil
		}
		return !entry.CreatedAt.IsZero() && entry.CreatedAt.Add(service.DefaultLocalResponseCacheTTL).Before(time.Now()), nil
	}

	meta, ok := parseLocalResponseCacheKeyMeta(redisKey)
	if !ok {
		// Legacy v1 keys do not carry platform/model/group/api-key metadata.
		// Delete them on scoped clears so fallback lookup cannot keep serving stale responses.
		return true, nil
	}
	scope := req.Scope
	switch req.ClearType {
	case service.LocalResponseCacheClearTypeByPlatform:
		return stringInList(meta.Platform, scope.Platforms), nil
	case service.LocalResponseCacheClearTypeByModel:
		return stringInList(meta.Model, scope.Models), nil
	case service.LocalResponseCacheClearTypeByGroup:
		return int64InList(meta.GroupID, scope.GroupIDs), nil
	case service.LocalResponseCacheClearTypeByAPIKey:
		return int64InList(meta.APIKeyID, scope.APIKeyIDs), nil
	default:
		return false, nil
	}
}

func (c *gatewayCache) getLocalResponseByRedisKey(ctx context.Context, redisKey string) (*service.LocalResponseCacheEntry, error) {
	payload, err := c.rdb.Get(ctx, redisKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var entry service.LocalResponseCacheEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return nil, nil
	}
	return &entry, nil
}

func parseLocalResponseCacheKeyMeta(redisKey string) (localResponseCacheKeyMeta, bool) {
	trimmed := strings.TrimPrefix(redisKey, localResponseCachePrefix)
	if trimmed == redisKey || !strings.HasPrefix(trimmed, "cache:"+service.LocalResponseCacheRuleVersion+":") {
		return localResponseCacheKeyMeta{}, false
	}
	body := strings.TrimPrefix(trimmed, "cache:"+service.LocalResponseCacheRuleVersion+":")
	parts := strings.Split(body, ":")
	if len(parts) < 6 {
		return localResponseCacheKeyMeta{}, false
	}
	apiKeyID, err := parseLocalResponseCacheInt64(parts[1])
	if err != nil {
		return localResponseCacheKeyMeta{}, false
	}
	groupID, err := parseLocalResponseCacheInt64(parts[2])
	if err != nil {
		return localResponseCacheKeyMeta{}, false
	}
	return localResponseCacheKeyMeta{
		Platform: parts[0],
		APIKeyID: apiKeyID,
		GroupID:  groupID,
		Model:    parts[len(parts)-2],
	}, true
}

func parseLocalResponseCacheInt64(raw string) (int64, error) {
	var value int64
	_, err := fmt.Sscan(raw, &value)
	return value, err
}

func stringInList(value string, values []string) bool {
	value = strings.TrimSpace(value)
	for _, item := range values {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func int64InList(value int64, values []int64) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
