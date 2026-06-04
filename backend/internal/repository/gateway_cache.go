package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
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
