package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

func buildLocalResponseCacheKey(hash string) string {
	return localResponseCachePrefix + hash
}

func (c *gatewayCache) GetLocalResponse(ctx context.Context, key string) (*service.LocalResponseCacheEntry, error) {
	if c == nil || c.rdb == nil {
		return nil, redis.Nil
	}
	payload, err := c.rdb.Get(ctx, buildLocalResponseCacheKey(key)).Bytes()
	if err != nil {
		return nil, err
	}
	var entry service.LocalResponseCacheEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *gatewayCache) SetLocalResponse(ctx context.Context, key string, entry *service.LocalResponseCacheEntry, ttl time.Duration) error {
	if c == nil || c.rdb == nil || entry == nil {
		return nil
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, buildLocalResponseCacheKey(key), payload, ttl).Err()
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
		return false, nil
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
