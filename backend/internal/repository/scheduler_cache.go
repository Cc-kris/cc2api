package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/tidwall/gjson"
)

const (
	schedulerLastUsedUpdateChunkSize           = 256
	schedulerAccountLastUsedPrefix             = "sched:acc:last_used:"
	schedulerAccountMetaPlatformRevisionPrefix = "sched:meta:rev:"
	schedulerAccountMetaGroupRevisionPrefix    = "sched:meta:rev:group:"
	schedulerAccountMetaRevisionKey            = "sched:meta:rev"
	schedulerBucketSetKey                      = "sched:buckets"
	schedulerOutboxWatermarkKey                = "sched:outbox:watermark"
	schedulerAccountPrefix                     = "sched:acc:"
	schedulerAccountMetaPrefix                 = "sched:meta:"
	schedulerActivePrefix                      = "sched:active:"
	schedulerReadyPrefix                       = "sched:ready:"
	schedulerVersionPrefix                     = "sched:ver:"
	schedulerEpochPrefix                       = "sched:epoch:"
	schedulerRetiredPrefix                     = "sched:retired:"
	schedulerSnapshotPrefix                    = "sched:"
	schedulerLockPrefix                        = "sched:lock:"

	defaultSchedulerSnapshotMGetChunkSize  = 128
	defaultSchedulerSnapshotWriteChunkSize = 256

	// snapshotGraceTTLSeconds 旧快照过期的宽限期（秒）。
	// 替代立即 DEL，让正在读取旧版本的 reader 有足够时间完成 ZRANGE。
	snapshotGraceTTLSeconds = 60

	// schedulerSnapshotCacheMaxAge 是进程内快照缓存条目的最长复用时长，
	// 限定了未推进修订号的精简记录改写在本进程的最长可见延迟，与 outbox_lag_rebuild_seconds 默认 10 秒的陈旧容忍度相同。
	schedulerSnapshotCacheMaxAge = 10 * time.Second
	// schedulerSnapshotCacheEvictAge：写入条目时，加载时间早于该时长的其它条目一并清除，
	// 已退休分组的条目不会长期驻留。
	schedulerSnapshotCacheEvictAge = 10 * schedulerSnapshotCacheMaxAge

	schedulerAccountMetaRevisionBytes        = 16
	schedulerAccountMetaRevisionWriteTimeout = 3 * time.Second
)

const (
	schedulerGroupLifecycleLockPrefix      = "sched:group:lifecycle-lock:"
	schedulerGroupLifecycleOwnerTokenBytes = 16
)

var readSchedulerSnapshotScript = redis.NewScript(`
local tagCount = tonumber(ARGV[1])
local values = redis.call('MGET', unpack(KEYS, 1, tagCount))
local lastUsed = {}
for index = tagCount + 1, #KEYS do
    lastUsed[index - tagCount] = redis.call('GET', KEYS[index])
end
return {values, lastUsed}
`)

var updateSchedulerLastUsedScript = redis.NewScript(`
local updated = 0
for index = 1, #ARGV do
    local key_index = (index - 1) * 2 + 1
    local candidate = tonumber(ARGV[index])
    if candidate == nil then
        return redis.error_reply('invalid last_used value')
    end
    if redis.call('EXISTS', KEYS[key_index]) == 1 then
        local current = tonumber(redis.call('GET', KEYS[key_index + 1]))
        if current == nil or candidate > current then
            redis.call('SET', KEYS[key_index + 1], ARGV[index])
            updated = updated + 1
        end
    end
end
return updated
`)

var (
	captureBucketWriteTokenScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then
    return -1
end

local currentEpoch = redis.call('GET', KEYS[1])
if currentEpoch == false then
    redis.call('SET', KEYS[1], '1')
    return 1
end

local parsedEpoch = tonumber(currentEpoch)
if parsedEpoch == nil or parsedEpoch < 1 then
    return -2
end
return parsedEpoch
`)

	allocateSnapshotVersionScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then
    return -1
end

local currentEpoch = tonumber(redis.call('GET', KEYS[1]))
local expectedEpoch = tonumber(ARGV[1])
if currentEpoch == nil or expectedEpoch == nil or currentEpoch ~= expectedEpoch then
    return -2
end

return redis.call('INCR', KEYS[3])
`)

	retireBucketScript = redis.NewScript(`
local retired = redis.call('GET', KEYS[2])
local currentEpoch = tonumber(redis.call('GET', KEYS[1])) or 0

if retired == false then
    currentEpoch = currentEpoch + 1
    if currentEpoch < 1 then
        currentEpoch = 1
    end
    redis.call('SET', KEYS[1], tostring(currentEpoch))
    redis.call('SET', KEYS[2], tostring(currentEpoch))
elseif currentEpoch < 1 then
    currentEpoch = tonumber(retired) or 1
    redis.call('SET', KEYS[1], tostring(currentEpoch))
end

redis.call('SREM', KEYS[3], ARGV[1])
local currentActive = redis.call('GET', KEYS[5])
if currentActive ~= false then
    redis.call('EXPIRE', ARGV[2] .. currentActive, tonumber(ARGV[3]))
end
redis.call('DEL', KEYS[4], KEYS[5])
return currentEpoch
`)

	reopenBucketScript = redis.NewScript(`
local currentEpochRaw = redis.call('GET', KEYS[1])
local currentEpoch = tonumber(currentEpochRaw)
local retiredEpochRaw = redis.call('GET', KEYS[2])

if retiredEpochRaw == false then
    if currentEpochRaw == false then
        redis.call('SET', KEYS[1], '1')
        return 1
    end
    if currentEpoch == nil or currentEpoch < 1 then
        return -2
    end
    return currentEpoch
end

local retiredEpoch = tonumber(retiredEpochRaw)
if retiredEpoch == nil or retiredEpoch < 1 then
    return -2
end
if currentEpoch == nil or currentEpoch < retiredEpoch then
    currentEpoch = retiredEpoch
end

redis.call('SET', KEYS[1], tostring(currentEpoch))
redis.call('DEL', KEYS[2])
redis.call('SREM', KEYS[3], ARGV[1])
local currentActive = redis.call('GET', KEYS[5])
if currentActive ~= false then
    redis.call('EXPIRE', ARGV[2] .. currentActive, tonumber(ARGV[3]))
end
redis.call('DEL', KEYS[4], KEYS[5])
return currentEpoch
`)

	releaseGroupLifecycleLeaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end
return 0
`)

	// activateSnapshotScript 原子 CAS 切换快照版本。
	// 仅当新版本号 >= 当前激活版本时才切换，防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 设置宽限期而非立即 DEL，避免与 reader 竞态。
	//
	// KEYS[1] = activeKey     (sched:active:{bucket})
	// KEYS[2] = readyKey      (sched:ready:{bucket})
	// KEYS[3] = bucketSetKey  (sched:buckets)
	// KEYS[4] = snapshotKey   (新写入的快照 key)
	// KEYS[5] = epochKey
	// KEYS[6] = retiredKey
	// ARGV[1] = 新版本号字符串
	// ARGV[2] = bucket 字符串 (用于 SADD)
	// ARGV[3] = 快照 key 前缀 (用于构造旧快照 key)
	// ARGV[4] = 宽限期 TTL 秒数
	// ARGV[5] = writer epoch
	//
	// 返回 1 = 已激活, 0 = 版本过旧未激活
	activateSnapshotScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[6]) == 1 then
    redis.call('DEL', KEYS[4])
    return -1
end

local currentEpoch = tonumber(redis.call('GET', KEYS[5]))
local expectedEpoch = tonumber(ARGV[5])
if currentEpoch == nil or expectedEpoch == nil or currentEpoch ~= expectedEpoch then
    redis.call('DEL', KEYS[4])
    return -2
end

local currentActive = redis.call('GET', KEYS[1])
local newVersion = tonumber(ARGV[1])

if currentActive ~= false then
	local curVersion = tonumber(currentActive)
	if curVersion and newVersion < curVersion then
		redis.call('DEL', KEYS[4])
		return 0
	end
end

redis.call('SET', KEYS[1], ARGV[1])
redis.call('SET', KEYS[2], '1')
redis.call('SADD', KEYS[3], ARGV[2])

if currentActive ~= false and currentActive ~= ARGV[1] then
	redis.call('EXPIRE', ARGV[3] .. currentActive, tonumber(ARGV[4]))
end

return 1
`)
)

type schedulerCache struct {
	rdb            *redis.Client
	mgetChunkSize  int
	writeChunkSize int

	// now 是进程内快照缓存判定条目年龄的时钟。
	now func() time.Time

	snapshotMu      sync.RWMutex
	snapshotEntries map[string]*schedulerSnapshotEntry

	// globalRevisionPending 与 globalRevisionWriting 合并写入出错后的后台全局修订号写入，见 scheduleGlobalAccountMetaRevision。
	globalRevisionPending atomic.Bool
	globalRevisionWriting atomic.Bool
}

// schedulerSnapshotEntry 是一个 bucket 的已解码快照。
// revisions 依次是全局、平台与 owning-group 修订号（见 schedulerSnapshotTagKeys）。
// accounts 是按快照顺序排列的母本，发布后只读，指针不外泄；
// 每次读取都按值复制出本次请求独占的副本再套用 last_used。
type schedulerSnapshotEntry struct {
	active       string
	revisions    []string
	loadedAt     time.Time
	accounts     []*service.Account
	lastUsedKeys []string
}

func NewSchedulerCache(rdb *redis.Client) service.SchedulerCache {
	return newSchedulerCacheWithChunkSizes(rdb, defaultSchedulerSnapshotMGetChunkSize, defaultSchedulerSnapshotWriteChunkSize)
}

func newSchedulerCacheWithChunkSizes(rdb *redis.Client, mgetChunkSize, writeChunkSize int) service.SchedulerCache {
	if mgetChunkSize <= 0 {
		mgetChunkSize = defaultSchedulerSnapshotMGetChunkSize
	}
	if writeChunkSize <= 0 {
		writeChunkSize = defaultSchedulerSnapshotWriteChunkSize
	}
	return &schedulerCache{
		rdb:             rdb,
		mgetChunkSize:   mgetChunkSize,
		writeChunkSize:  writeChunkSize,
		now:             time.Now,
		snapshotEntries: make(map[string]*schedulerSnapshotEntry),
	}
}

// GetSnapshot 读取 bucket 的激活快照。解码后的精简记录按 bucket 常驻进程内存，
// 只有 ready、active 版本、全局修订号与该 bucket 所涉平台及 owning-group 修订号都与 Redis 一致且未超过最长复用时长时才复用；
// 否则重新从 Redis 加载。last_used 每次都从 Redis 读取。
// 返回的账号对象由本次调用独占，可以修改其顶层字段；map、切片底层数组与指针指向的值与其它调用共享，不得原地修改。
func (c *schedulerCache) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	bucketKey := bucket.String()
	tagKeys := schedulerSnapshotTagKeys(bucket)
	now := c.now()
	entry := c.lookupSnapshotEntry(bucketKey)
	keys := append([]string(nil), tagKeys...)
	if entry != nil {
		keys = append(keys, entry.lastUsedKeys...)
	}
	values, err := readSchedulerSnapshotScript.Run(ctx, c.rdb, keys, len(tagKeys)).Slice()
	if err != nil {
		return nil, false, err
	}
	if len(values) != 2 {
		return nil, false, fmt.Errorf("unexpected scheduler snapshot read result count: got %d, want 2", len(values))
	}
	tags, ok := values[0].([]any)
	if !ok || len(tags) != len(tagKeys) {
		return nil, false, fmt.Errorf("unexpected scheduler snapshot tag result: got %T/%d, want %d", values[0], len(tags), len(tagKeys))
	}
	var lastUsedValues []any
	if entry != nil {
		lastUsedValues, ok = values[1].([]any)
		if !ok || len(lastUsedValues) != len(entry.accounts) {
			return nil, false, fmt.Errorf("unexpected scheduler last_used result: got %T/%d, want %d", values[1], len(lastUsedValues), len(entry.accounts))
		}
	}
	if err != nil {
		return nil, false, err
	}
	if len(tags) != len(tagKeys) {
		return nil, false, fmt.Errorf("unexpected scheduler snapshot tag count: got %d, want %d", len(tags), len(tagKeys))
	}
	readyVal, err := schedulerCacheOptionalString(tags[0])
	if err != nil {
		return nil, false, err
	}
	activeVal, err := schedulerCacheOptionalString(tags[1])
	if err != nil {
		return nil, false, err
	}
	// 修订号缺失（nil）按空字符串比较：它是一个独立取值，不会与任何写入过的随机修订号相等。
	revisions := make([]string, 0, len(tags)-2)
	for _, tag := range tags[2:] {
		revision, err := schedulerCacheOptionalString(tag)
		if err != nil {
			return nil, false, err
		}
		revisions = append(revisions, revision)
	}
	if readyVal != "1" || activeVal == "" {
		c.dropSnapshotEntry(bucketKey)
		return nil, false, nil
	}

	if entry != nil && entry.matches(activeVal, revisions, now) {
		accounts, err := snapshotAccountsFromEntry(entry, lastUsedValues)
		if err != nil {
			return nil, false, err
		}
		return accounts, true, nil
	}

	ids, err := c.rdb.ZRange(ctx, schedulerSnapshotKey(bucket, activeVal), 0, -1).Result()
	if err != nil {
		return nil, false, err
	}
	if len(ids) == 0 {
		// 空快照视为缓存未命中，触发数据库回退查询
		// 这解决了新分组创建后立即绑定账号时的竞态条件问题
		c.dropSnapshotEntry(bucketKey)
		return nil, false, nil
	}

	metadataKeys := make([]string, 0, len(ids))
	lastUsedKeys := make([]string, 0, len(ids))
	for _, id := range ids {
		metadataKeys = append(metadataKeys, schedulerAccountMetaKey(id))
		lastUsedKeys = append(lastUsedKeys, schedulerLastUsedKey(id))
	}
	metadataValues, err := c.mgetChunked(ctx, metadataKeys)
	if err != nil {
		return nil, false, err
	}

	masters := make([]*service.Account, 0, len(metadataValues))
	for _, val := range metadataValues {
		if val == nil {
			c.dropSnapshotEntry(bucketKey)
			return nil, false, nil
		}
		account, err := decodeCachedAccount(val)
		if err != nil {
			return nil, false, err
		}
		masters = append(masters, account)
	}
	// 母本发布进缓存前由本 goroutine 完成预处理，发布后只读。
	for _, master := range masters {
		master.PrepareSharedSnapshot()
	}

	// 条目记录的是加载前读到的 (active, 修订号) 与当时的时钟：精简记录读取晚于这组取值，
	// 之后若再有改写，修订号或 active 必然不同，或在最长复用时长后过期。
	entry = &schedulerSnapshotEntry{
		active:       activeVal,
		revisions:    revisions,
		loadedAt:     now,
		accounts:     masters,
		lastUsedKeys: lastUsedKeys,
	}
	c.storeSnapshotEntry(bucketKey, entry)

	accounts, err := c.snapshotAccountsFromEntry(ctx, entry)
	if err != nil {
		return nil, false, err
	}
	return accounts, true, nil
}

func (c *schedulerCache) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accounts []service.Account) error {
	if !token.ValidFor(bucket) {
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	}
	// 分配版本与激活指针是两个 fencing 边界；中间写入的数据只有通过第二次校验才能发布。
	version, err := c.allocateSnapshotVersion(ctx, bucket, token)
	if err != nil {
		return err
	}
	// 快照成员最终只依赖可编码账号的有序 ID；直接复用 ID 路径，避免为
	// 随后立即丢弃的完整 Account 再分配一份临时切片。
	if _, err := c.writeSnapshotVersionAndReturnAccountIDs(ctx, bucket, version, accounts); err != nil {
		return err
	}
	return c.activateSnapshotVersion(ctx, bucket, token, version)
}

func (c *schedulerCache) activateSnapshotVersion(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, version string) error {
	// A lost global tag can mean Redis was flushed while an equal active version
	// was recreated. Never reuse a process-local memo across that boundary.
	if err := c.rdb.SetNX(ctx, schedulerAccountMetaRevisionKey, newSchedulerAccountMetaRevision(), 0).Err(); err != nil {
		return err
	}
	snapshotKey := schedulerSnapshotKey(bucket, version)
	// Phase 2: 原子 CAS 切换版本，同时再次校验退休状态与 writer epoch。
	// Lua 脚本保证：仅当新版本 >= 当前激活版本时才切换 active 指针，
	// 防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 宽限期而非立即 DEL，避免 reader 竞态。
	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	readyKey := schedulerBucketKey(schedulerReadyPrefix, bucket)
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)

	keys := []string{
		activeKey,
		readyKey,
		schedulerBucketSetKey,
		snapshotKey,
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
	}
	args := []any{version, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds, token.Epoch}

	result, err := activateSnapshotScript.Run(ctx, c.rdb, keys, args...).Int64()
	if err != nil {
		return err
	}
	return schedulerBucketWriteResultError(result, bucket)
}

func schedulerBucketWriteResultError(result int64, bucket service.SchedulerBucket) error {
	switch result {
	case -1:
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketRetired, bucket.String())
	case -2:
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	default:
		return nil
	}
}

func (c *schedulerCache) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	id := strconv.FormatInt(accountID, 10)
	values, err := c.rdb.MGet(ctx, schedulerAccountKey(id), schedulerLastUsedKey(id)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) != 2 || values[0] == nil {
		return nil, nil
	}
	account, err := decodeCachedAccount(values[0])
	if err != nil {
		return nil, err
	}
	if err := applySchedulerLastUsed(account, values[1]); err != nil {
		return nil, err
	}
	return account, nil
}

func (c *schedulerCache) SetAccount(ctx context.Context, account *service.Account) error {
	if account == nil || account.ID <= 0 {
		return nil
	}
	accountIDs, err := c.writeAccountIDsScoped(ctx, []service.Account{*account}, false)
	if err != nil {
		return err
	}
	if len(accountIDs) == 0 {
		return c.DeleteAccount(ctx, account.ID)
	}
	return nil
}

func (c *schedulerCache) DeleteAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
	}
	id := strconv.FormatInt(accountID, 10)
	// 删除精简记录与推进全局修订号在同一个 MULTI 中原子执行：读到删除结果的读者也读到新修订号，
	// 进程内快照缓存不会给出已删除的账号。
	pipe := c.rdb.TxPipeline()
	pipe.Del(ctx, schedulerAccountKey(id), schedulerAccountMetaKey(id), schedulerLastUsedKey(id))
	pipe.Set(ctx, schedulerAccountMetaRevisionKey, newSchedulerAccountMetaRevision(), 0)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *schedulerCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}

	pipe := c.rdb.Pipeline()
	queued := 0
	metaDeleted := false
	keys := make([]string, 0, schedulerLastUsedUpdateChunkSize*2)
	args := make([]any, 0, schedulerLastUsedUpdateChunkSize)
	queueBatch := func() {
		if len(args) == 0 {
			return
		}
		updateSchedulerLastUsedScript.Eval(ctx, pipe, keys, args...)
		queued++
		keys = make([]string, 0, schedulerLastUsedUpdateChunkSize*2)
		args = make([]any, 0, schedulerLastUsedUpdateChunkSize)
	}
	for id, usedAt := range updates {
		if id <= 0 {
			continue
		}
		millis, err := schedulerLastUsedMillis(usedAt)
		if err != nil {
			slog.Warn("scheduler cache removes account with unencodable payload",
				"account_id", id,
				"error", err,
			)
			idText := strconv.FormatInt(id, 10)
			pipe.Del(ctx, schedulerAccountKey(idText), schedulerAccountMetaKey(idText), schedulerLastUsedKey(idText))
			queued++
			metaDeleted = true
			continue
		}
		idText := strconv.FormatInt(id, 10)
		keys = append(keys, schedulerAccountKey(idText), schedulerLastUsedKey(idText))
		args = append(args, millis)
		if len(args) >= schedulerLastUsedUpdateChunkSize {
			queueBatch()
		}
	}
	queueBatch()
	if queued == 0 {
		return nil
	}
	if metaDeleted {
		// 被删账号的平台未知，全局修订号排在同一 pipeline 的 DEL 之后。
		pipe.Set(ctx, schedulerAccountMetaRevisionKey, newSchedulerAccountMetaRevision(), 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *schedulerCache) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	key := schedulerBucketKey(schedulerLockPrefix, bucket)
	return c.rdb.SetNX(ctx, key, time.Now().UnixNano(), ttl).Result()
}

func (c *schedulerCache) UnlockBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	key := schedulerBucketKey(schedulerLockPrefix, bucket)
	return c.rdb.Del(ctx, key).Err()
}

func (c *schedulerCache) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	raw, err := c.rdb.SMembers(ctx, schedulerBucketSetKey).Result()
	if err != nil {
		return nil, err
	}
	out := make([]service.SchedulerBucket, 0, len(raw))
	for _, entry := range raw {
		bucket, ok := service.ParseSchedulerBucket(entry)
		if !ok {
			continue
		}
		out = append(out, bucket)
	}
	return out, nil
}

func (c *schedulerCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	val, err := c.rdb.Get(ctx, schedulerOutboxWatermarkKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (c *schedulerCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	return c.rdb.Set(ctx, schedulerOutboxWatermarkKey, strconv.FormatInt(id, 10), 0).Err()
}

func schedulerBucketKey(prefix string, bucket service.SchedulerBucket) string {
	return fmt.Sprintf("%s%d:%s:%s", prefix, bucket.GroupID, bucket.Platform, bucket.Mode)
}

func schedulerGroupLifecycleLockKey(groupID int64) string {
	return schedulerGroupLifecycleLockPrefix + strconv.FormatInt(groupID, 10)
}

func schedulerSnapshotKey(bucket service.SchedulerBucket, version string) string {
	return fmt.Sprintf("%s%d:%s:%s:v%s", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode, version)
}

func schedulerAccountKey(id string) string {
	return schedulerAccountPrefix + id
}

func schedulerAccountMetaKey(id string) string {
	return schedulerAccountMetaPrefix + id
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func decodeCachedAccount(val any) (*service.Account, error) {
	var payload []byte
	switch raw := val.(type) {
	case string:
		payload = []byte(raw)
	case []byte:
		payload = raw
	default:
		return nil, fmt.Errorf("unexpected account cache type: %T", val)
	}
	var account service.Account
	if err := json.Unmarshal(payload, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

func (c *schedulerCache) writeAccounts(ctx context.Context, accounts []service.Account) error {
	_, err := c.writeAccountIDs(ctx, accounts)
	return err
}

func (c *schedulerCache) writeAccountIDs(ctx context.Context, accounts []service.Account) ([]int64, error) {
	return c.writeAccountIDsScoped(ctx, accounts, true)
}

func (c *schedulerCache) writeAccountIDsScoped(ctx context.Context, accounts []service.Account, platformRevision bool) ([]int64, error) {
	if len(accounts) == 0 {
		return nil, nil
	}

	type metaWrite struct {
		platform string
		groups   []int64
		previous *redis.StringCmd
	}
	pipe := c.rdb.Pipeline()
	fencePipe := c.rdb.Pipeline()
	accountIDs := make([]int64, 0, len(accounts))
	platforms := make([]string, 0, 1)
	groups := make([]int64, 0, 2)
	hasExplicitGroups := false
	for _, account := range accounts {
		if len(schedulerMetadataAccountGroups(account)) > 0 {
			hasExplicitGroups = true
			break
		}
	}
	writes := make([]metaWrite, 0, min(len(accounts), c.writeChunkSize))
	pending := 0
	execFailed := false
	platformChanged := false
	groupChanged := false
	fallbackPlatformRevision := false
	defer func() {
		if execFailed {
			c.scheduleGlobalAccountMetaRevision(ctx)
		}
	}()
	exec := func() error {
		cmds, err := pipe.Exec(ctx)
		if err != nil && !errors.Is(err, redis.Nil) {
			execFailed = true
			return err
		}
		for _, cmd := range cmds {
			if cmdErr := cmd.Err(); cmdErr != nil && !errors.Is(cmdErr, redis.Nil) {
				execFailed = true
				return cmdErr
			}
		}
		for _, write := range writes {
			previous, hasPrevious, changed := schedulerPreviousMetadataGroups(write.previous)
			if changed || (hasPrevious && previous.Platform != write.platform) {
				platformChanged = true
			}
			if len(write.groups) > 0 {
				hasExplicitGroups = true
			}
			if !hasPrevious || len(write.groups) == 0 || write.platform == service.PlatformAntigravity {
				if hasPrevious && len(write.groups) == 0 && !slices.Contains(groups, 0) {
					groups = append(groups, 0)
				}
				// Before a full hydration there is no trustworthy old ownership to
				// compare. Antigravity also participates in other platforms' mixed
				// buckets, so preserve the platform fence for both cases.
				fallbackPlatformRevision = true
			}
			previousGroups := make([]int64, 0, 1)
			for _, groupID := range previous.Groups {
				if !slices.Contains(write.groups, groupID) {
					previousGroups = append(previousGroups, groupID)
					fencePipe.Set(ctx, schedulerAccountMetaRevisionKeyForGroup(groupID), newSchedulerAccountMetaRevision(), 0)
				}
				if !slices.Contains(groups, groupID) {
					groups = append(groups, groupID)
				}
			}
			if hasPrevious && !slices.Equal(previous.Groups, write.groups) {
				if !hasExplicitGroups {
					platformChanged = true // Legacy ungrouped payload needs a global fence.
				} else {
					groupChanged = true
				}
			}
		}
		if _, err := fencePipe.Exec(ctx); err != nil {
			execFailed = true
			return err
		}
		writes = writes[:0]
		pending = 0
		return nil
	}
	for _, account := range accounts {
		fullPayload, metaPayload, err := marshalSchedulerCacheAccount(account)
		if err != nil {
			slog.Warn("scheduler cache skips account with unencodable payload", "account_id", account.ID, "error", err)
			continue
		}
		if pending >= c.writeChunkSize {
			if err := exec(); err != nil {
				return nil, err
			}
		}
		id := strconv.FormatInt(account.ID, 10)
		pipe.Set(ctx, schedulerAccountKey(id), fullPayload, 0)
		previous := pipe.Get(ctx, schedulerAccountMetaKey(id))
		pipe.Set(ctx, schedulerAccountMetaKey(id), metaPayload, 0)
		writes = append(writes, metaWrite{
			platform: account.Platform,
			groups:   schedulerMetadataAccountGroups(account),
			previous: previous,
		})
		accountIDs = append(accountIDs, account.ID)
		if !slices.Contains(platforms, account.Platform) {
			platforms = append(platforms, account.Platform)
		}
		accountGroups := schedulerMetadataAccountGroups(account)
		if len(accountGroups) == 0 {
			accountGroups = []int64{0}
		}
		for _, groupID := range accountGroups {
			if !slices.Contains(groups, groupID) {
				groups = append(groups, groupID)
			}
			if !platformRevision {
				pipe.Set(ctx, schedulerAccountMetaRevisionKeyForGroup(groupID), newSchedulerAccountMetaRevision(), 0)
			}
		}
		if !platformRevision && len(schedulerMetadataAccountGroups(account)) > 0 {
			pipe.Set(ctx, schedulerAccountMetaRevisionKeyForGroup(0), newSchedulerAccountMetaRevision(), 0)
		}
		pending++
	}
	if pending == 0 {
		return accountIDs, nil
	}
	// Group 0 also represents simple mode, where buckets can contain every group.
	// Keep the existing ungrouped-write command shape; only add this tag when the
	// caller actually supplied group ownership.
	if hasExplicitGroups && !slices.Contains(groups, 0) {
		groups = append(groups, 0)
	}
	for _, platform := range platforms {
		// Bucket publication already activates the bucket containing the refreshed
		// members. Owning-group fences cover other OpenAI/Grok buckets; a platform
		// fence is only needed when ownership cannot be trusted or has changed.
		if platformRevision && !schedulerGroupScopedPlatform(platform) {
			pipe.Set(ctx, schedulerAccountMetaRevisionKeyForPlatform(platform), newSchedulerAccountMetaRevision(), 0)
		}
	}
	for _, groupID := range groups {
		if !platformRevision || hasExplicitGroups {
			pipe.Set(ctx, schedulerAccountMetaRevisionKeyForGroup(groupID), newSchedulerAccountMetaRevision(), 0)
		}
	}
	if err := exec(); err != nil {
		return nil, err
	}
	if platformRevision && (fallbackPlatformRevision || platformChanged || groupChanged) {
		for _, platform := range platforms {
			if !schedulerGroupScopedPlatform(platform) {
				continue
			}
			if err := c.rdb.Set(ctx, schedulerAccountMetaRevisionKeyForPlatform(platform), newSchedulerAccountMetaRevision(), 0).Err(); err != nil {
				return nil, err
			}
		}
	}
	if !platformRevision && (fallbackPlatformRevision || platformChanged || groupChanged || len(accounts) == 1 && accounts[0].Platform != service.PlatformOpenAI && accounts[0].Platform != service.PlatformGrok) {
		// Ownership and platform changes are known only after the old metadata GET returns.
		// OpenAI/Grok scoped writes already fenced every owning group; fencing the
		// whole platform would invalidate unrelated buckets for an in-place update.
		for _, platform := range platforms {
			if err := c.rdb.Set(ctx, schedulerAccountMetaRevisionKeyForPlatform(platform), newSchedulerAccountMetaRevision(), 0).Err(); err != nil {
				return nil, err
			}
		}
	}
	if platformChanged {
		c.advanceGlobalAccountMetaRevision(ctx)
	}
	return accountIDs, nil
}

func (c *schedulerCache) mgetChunked(ctx context.Context, keys []string) ([]any, error) {
	if len(keys) == 0 {
		return []any{}, nil
	}

	out := make([]any, 0, len(keys))
	chunkSize := c.mgetChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultSchedulerSnapshotMGetChunkSize
	}
	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		part, err := c.rdb.MGet(ctx, keys[start:end]...).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func buildSchedulerMetadataAccount(account service.Account) service.Account {
	return service.Account{
		ID:                      account.ID,
		Name:                    account.Name,
		Platform:                account.Platform,
		Type:                    account.Type,
		Concurrency:             account.Concurrency,
		LoadFactor:              account.LoadFactor,
		Priority:                account.Priority,
		RateMultiplier:          account.RateMultiplier,
		Status:                  account.Status,
		LastUsedAt:              account.LastUsedAt,
		ExpiresAt:               account.ExpiresAt,
		AutoPauseOnExpired:      account.AutoPauseOnExpired,
		Schedulable:             account.Schedulable,
		RateLimitedAt:           account.RateLimitedAt,
		RateLimitResetAt:        account.RateLimitResetAt,
		OverloadUntil:           account.OverloadUntil,
		TempUnschedulableUntil:  account.TempUnschedulableUntil,
		TempUnschedulableReason: account.TempUnschedulableReason,
		SessionWindowStart:      account.SessionWindowStart,
		SessionWindowEnd:        account.SessionWindowEnd,
		SessionWindowStatus:     account.SessionWindowStatus,
		AccountGroups:           filterSchedulerAccountGroups(account.AccountGroups),
		GroupIDs:                filterSchedulerGroupIDs(account.GroupIDs, account.AccountGroups),
		Credentials:             filterSchedulerCredentials(account.Credentials),
		Extra:                   filterSchedulerExtra(account.Extra),
	}
}

func filterSchedulerAccountGroups(accountGroups []service.AccountGroup) []service.AccountGroup {
	if len(accountGroups) == 0 {
		return nil
	}

	filtered := make([]service.AccountGroup, 0, len(accountGroups))
	for _, ag := range accountGroups {
		if ag.GroupID <= 0 {
			continue
		}
		filtered = append(filtered, service.AccountGroup{
			AccountID: ag.AccountID,
			GroupID:   ag.GroupID,
			Priority:  ag.Priority,
			CreatedAt: ag.CreatedAt,
		})
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerGroupIDs(groupIDs []int64, accountGroups []service.AccountGroup) []int64 {
	if len(groupIDs) == 0 && len(accountGroups) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(groupIDs)+len(accountGroups))
	filtered := make([]int64, 0, len(groupIDs)+len(accountGroups))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		filtered = append(filtered, id)
	}
	for _, ag := range accountGroups {
		if ag.GroupID <= 0 {
			continue
		}
		if _, ok := seen[ag.GroupID]; ok {
			continue
		}
		seen[ag.GroupID] = struct{}{}
		filtered = append(filtered, ag.GroupID)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerCredentials(credentials map[string]any) map[string]any {
	if len(credentials) == 0 {
		return nil
	}
	keys := []string{"model_mapping", "api_key", "project_id", "oauth_type", "plan_type", "account_scheduling_threshold", "email", "email_address", "chatgpt_account_id", "account_id", "workspace_id", "chatgpt_workspace_id", "organization_id", "org_id"}
	filtered := make(map[string]any)
	for _, key := range keys {
		if value, ok := credentials[key]; ok && value != nil {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	keys := []string{
		// Threshold filters consume the shared metadata projection, not full records.
		"session_window_utilization",
		"passive_usage_7d_utilization",
		"passive_usage_7d_reset",
		"passive_usage_7d_oi_utilization",
		"passive_usage_7d_oi_reset",
		"codex_5h_used_percent",
		"codex_7d_used_percent",
		"codex_5h_reset_at",
		"codex_7d_reset_at",
		"codex_5h_reset_after_seconds",
		"codex_7d_reset_after_seconds",
		"codex_usage_updated_at",
		"auto_pause_5h_threshold",
		"auto_pause_7d_threshold",
		"auto_pause_5h_disabled",
		"auto_pause_7d_disabled",
		"mixed_scheduling",
		"model_rate_limits",
		"codex_primary_used_percent",
		"codex_secondary_used_percent",
		"codex_primary_reset_at",
		"codex_secondary_reset_at",
		"codex_primary_reset_after_seconds",
		"codex_secondary_reset_after_seconds",
		"grok_sched_utilization",
		"grok_sched_reset_at",
		"email",
		"email_address",
		"chatgpt_account_id",
		"account_id",
		"workspace_id",
		"chatgpt_workspace_id",
		"organization_id",
		"org_id",
		"window_cost_limit",
		"window_cost_sticky_reserve",
		"max_sessions",
		"session_idle_timeout_minutes",
		"openai_oauth_responses_websockets_v2_enabled",
		"openai_oauth_responses_websockets_v2_mode",
		"openai_apikey_responses_websockets_v2_enabled",
		"openai_apikey_responses_websockets_v2_mode",
		"responses_websockets_v2_enabled",
		"openai_ws_enabled",
		"openai_ws_force_http",
		"openai_responses_mode",
		"openai_responses_supported",
		service.OpenAIStructuredOutputModeExtraKey,
		service.GrokMediaEligibleExtraKey,
		"grok_billing_snapshot",
		"grok_client_tool_cache_enabled",
	}
	filtered := make(map[string]any)
	for _, key := range keys {
		if value, ok := extra[key]; ok && value != nil {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func (c *schedulerCache) CaptureBucketWriteToken(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	result, err := captureBucketWriteTokenScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
	}).Int64()
	if err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	if err := schedulerBucketWriteResultError(result, bucket); err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: result}, nil
}

func (c *schedulerCache) RetireBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)
	result, err := retireBucketScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
		schedulerBucketSetKey,
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerActivePrefix, bucket),
	}, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds).Int64()
	if err != nil {
		return err
	}
	if result < 1 {
		return fmt.Errorf("retire scheduler bucket %s returned invalid epoch %d", bucket.String(), result)
	}
	return nil
}

func (c *schedulerCache) ReopenBucket(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)
	result, err := reopenBucketScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
		schedulerBucketSetKey,
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerActivePrefix, bucket),
	}, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds).Int64()
	if err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	if err := schedulerBucketWriteResultError(result, bucket); err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: result}, nil
}

func (c *schedulerCache) allocateSnapshotVersion(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken) (string, error) {
	result, err := allocateSnapshotVersionScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
		schedulerBucketKey(schedulerVersionPrefix, bucket),
	}, token.Epoch).Int64()
	if err != nil {
		return "", err
	}
	if err := schedulerBucketWriteResultError(result, bucket); err != nil {
		return "", err
	}
	return strconv.FormatInt(result, 10), nil
}

func (c *schedulerCache) TryAcquireGroupLifecycleLease(ctx context.Context, groupID int64, ttl time.Duration) (service.SchedulerGroupLifecycleLease, bool, error) {
	if groupID <= 0 {
		return service.SchedulerGroupLifecycleLease{}, false, fmt.Errorf("%w: group id must be positive", service.ErrSchedulerGroupLifecycleLeaseInvalid)
	}
	if ttl <= 0 {
		return service.SchedulerGroupLifecycleLease{}, false, fmt.Errorf("%w: ttl must be positive", service.ErrSchedulerGroupLifecycleLeaseInvalid)
	}
	ownerToken, err := newSchedulerGroupLifecycleOwnerToken()
	if err != nil {
		return service.SchedulerGroupLifecycleLease{}, false, err
	}
	acquired, err := c.rdb.SetNX(ctx, schedulerGroupLifecycleLockKey(groupID), ownerToken, ttl).Result()
	if err != nil {
		return service.SchedulerGroupLifecycleLease{}, false, err
	}
	if !acquired {
		return service.SchedulerGroupLifecycleLease{}, false, nil
	}
	return service.SchedulerGroupLifecycleLease{GroupID: groupID, OwnerToken: ownerToken}, true, nil
}

func (c *schedulerCache) ReleaseGroupLifecycleLease(ctx context.Context, lease service.SchedulerGroupLifecycleLease) error {
	if !lease.ValidFor(lease.GroupID) {
		return service.ErrSchedulerGroupLifecycleLeaseInvalid
	}
	result, err := releaseGroupLifecycleLeaseScript.Run(
		ctx,
		c.rdb,
		[]string{schedulerGroupLifecycleLockKey(lease.GroupID)},
		lease.OwnerToken,
	).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return fmt.Errorf("%w: group=%d", service.ErrSchedulerGroupLifecycleLeaseLost, lease.GroupID)
	}
	if result != 1 {
		return fmt.Errorf("release scheduler group lifecycle lease returned %d", result)
	}
	return nil
}

func newSchedulerGroupLifecycleOwnerToken() (string, error) {
	raw := make([]byte, schedulerGroupLifecycleOwnerTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate scheduler group lifecycle owner token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func (e *schedulerSnapshotEntry) matches(active string, revisions []string, now time.Time) bool {
	if e.active != active || !slices.Equal(e.revisions, revisions) {
		return false
	}
	age := now.Sub(e.loadedAt)
	return age >= 0 && age < schedulerSnapshotCacheMaxAge
}

func (c *schedulerCache) snapshotAccountsFromEntry(ctx context.Context, entry *schedulerSnapshotEntry) ([]*service.Account, error) {
	lastUsedValues, err := c.mgetChunked(ctx, entry.lastUsedKeys)
	if err != nil {
		return nil, err
	}
	return snapshotAccountsFromEntry(entry, lastUsedValues)
}

func snapshotAccountsFromEntry(entry *schedulerSnapshotEntry, lastUsedValues []any) ([]*service.Account, error) {
	if len(lastUsedValues) != len(entry.accounts) {
		return nil, fmt.Errorf("unexpected scheduler last_used value count: got %d, want %d", len(lastUsedValues), len(entry.accounts))
	}
	slab := make([]service.Account, len(entry.accounts))
	accounts := make([]*service.Account, len(entry.accounts))
	for i, master := range entry.accounts {
		slab[i] = *master
		account := &slab[i]
		if err := applySchedulerLastUsed(account, lastUsedValues[i]); err != nil {
			return nil, err
		}
		accounts[i] = account
	}
	return accounts, nil
}

func (c *schedulerCache) lookupSnapshotEntry(bucketKey string) *schedulerSnapshotEntry {
	c.snapshotMu.RLock()
	defer c.snapshotMu.RUnlock()
	return c.snapshotEntries[bucketKey]
}

func (c *schedulerCache) storeSnapshotEntry(bucketKey string, entry *schedulerSnapshotEntry) {
	c.snapshotMu.Lock()
	defer c.snapshotMu.Unlock()
	if current := c.snapshotEntries[bucketKey]; current == nil || !current.loadedAt.After(entry.loadedAt) {
		c.snapshotEntries[bucketKey] = entry
	}
	for key, other := range c.snapshotEntries {
		if key != bucketKey && entry.loadedAt.Sub(other.loadedAt) > schedulerSnapshotCacheEvictAge {
			delete(c.snapshotEntries, key)
		}
	}
}

func (c *schedulerCache) dropSnapshotEntry(bucketKey string) {
	c.snapshotMu.RLock()
	_, ok := c.snapshotEntries[bucketKey]
	c.snapshotMu.RUnlock()
	if !ok {
		return
	}
	c.snapshotMu.Lock()
	delete(c.snapshotEntries, bucketKey)
	c.snapshotMu.Unlock()
}

func schedulerSnapshotTagKeys(bucket service.SchedulerBucket) []string {
	keys := []string{
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerActivePrefix, bucket),
		schedulerAccountMetaRevisionKey,
		schedulerAccountMetaRevisionKeyForPlatform(bucket.Platform),
	}
	if schedulerGroupScopedPlatform(bucket.Platform) {
		keys = append(keys, schedulerAccountMetaRevisionKeyForGroup(bucket.GroupID))
	}
	if bucket.Mode == service.SchedulerModeMixed && bucket.Platform != service.PlatformAntigravity {
		keys = append(keys, schedulerAccountMetaRevisionKeyForPlatform(service.PlatformAntigravity))
	}
	return keys
}

func schedulerGroupScopedPlatform(platform string) bool {
	return platform == service.PlatformOpenAI || platform == service.PlatformGrok
}

func schedulerAccountMetaRevisionKeyForGroup(groupID int64) string {
	return schedulerAccountMetaGroupRevisionPrefix + strconv.FormatInt(groupID, 10)
}

func schedulerAccountMetaRevisionKeyForPlatform(platform string) string {
	if platform == "" {
		return schedulerAccountMetaRevisionKey
	}
	return schedulerAccountMetaPlatformRevisionPrefix + platform
}

func schedulerCacheOptionalString(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", fmt.Errorf("unexpected scheduler cache value type: %T", value)
	}
}

func schedulerLastUsedKey(id string) string {
	return schedulerAccountLastUsedPrefix + id
}

func schedulerLastUsedMillis(value time.Time) (int64, error) {
	if _, err := value.MarshalJSON(); err != nil {
		return 0, err
	}
	return value.UTC().UnixMilli(), nil
}

func applySchedulerLastUsed(account *service.Account, value any) error {
	if account == nil || value == nil {
		return nil
	}
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	default:
		return fmt.Errorf("unexpected last_used cache type: %T", value)
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid last_used cache value %q: %w", raw, err)
	}
	lastUsedAt := time.UnixMilli(millis).UTC()
	if account.LastUsedAt == nil || lastUsedAt.After(*account.LastUsedAt) {
		account.LastUsedAt = ptrTime(lastUsedAt)
	}
	return nil
}

type schedulerPreviousMetadata struct {
	Platform string
	Groups   []int64
}

func schedulerMetadataAccountGroups(account service.Account) []int64 {
	return filterSchedulerGroupIDs(account.GroupIDs, account.AccountGroups)
}

func schedulerPreviousMetadataGroups(previous *redis.StringCmd) (schedulerPreviousMetadata, bool, bool) {
	raw, err := previous.Result()
	if errors.Is(err, redis.Nil) {
		return schedulerPreviousMetadata{}, false, false
	}
	if err != nil || !gjson.Valid(raw) {
		return schedulerPreviousMetadata{}, false, true
	}
	platform := gjson.Get(raw, "Platform")
	groups := filterSchedulerGroupIDs(
		gjsonInt64Slice(gjson.Get(raw, "GroupIDs").Array()),
		gjsonAccountGroups(gjson.Get(raw, "AccountGroups").Array()),
	)
	return schedulerPreviousMetadata{Platform: platform.Str, Groups: groups}, true, platform.Type != gjson.String
}

func gjsonInt64Slice(values []gjson.Result) []int64 {
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		if value.Type == gjson.Number {
			ids = append(ids, value.Int())
		}
	}
	return ids
}

func gjsonAccountGroups(values []gjson.Result) []service.AccountGroup {
	groups := make([]service.AccountGroup, 0, len(values))
	for _, value := range values {
		groupID := value.Get("GroupID")
		if groupID.Type == gjson.Number {
			groups = append(groups, service.AccountGroup{GroupID: groupID.Int()})
		}
	}
	return groups
}

func (c *schedulerCache) scheduleGlobalAccountMetaRevision(ctx context.Context) {
	c.globalRevisionPending.Store(true)
	if !c.globalRevisionWriting.CompareAndSwap(false, true) {
		return
	}
	go func() {
		for {
			c.globalRevisionPending.Store(false)
			c.advanceGlobalAccountMetaRevision(ctx)
			c.globalRevisionWriting.Store(false)
			if !c.globalRevisionPending.Load() || !c.globalRevisionWriting.CompareAndSwap(false, true) {
				return
			}
		}
	}()
}

func (c *schedulerCache) advanceGlobalAccountMetaRevision(ctx context.Context) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), schedulerAccountMetaRevisionWriteTimeout)
	defer cancel()
	if err := c.rdb.Set(writeCtx, schedulerAccountMetaRevisionKey, newSchedulerAccountMetaRevision(), 0).Err(); err != nil {
		slog.Warn("scheduler cache failed to advance account metadata revision", "error", err)
	}
}

func newSchedulerAccountMetaRevision() string {
	raw := make([]byte, schedulerAccountMetaRevisionBytes)
	// crypto/rand.Read 读取失败时直接终止进程，不会返回错误。
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func marshalSchedulerCacheAccount(account service.Account) ([]byte, []byte, error) {
	fullPayload, err := json.Marshal(account)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal account: %w", err)
	}
	metaPayload, err := json.Marshal(buildSchedulerMetadataAccount(account))
	if err != nil {
		return nil, nil, fmt.Errorf("marshal account metadata: %w", err)
	}
	return fullPayload, metaPayload, nil
}

func (c *schedulerCache) SetSnapshotAndReturnAccountIDs(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accounts []service.Account) ([]int64, error) {
	if !token.ValidFor(bucket) {
		return nil, fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	}
	// 分配版本与激活指针是两个 fencing 边界；中间写入的数据只有通过第二次校验才能发布。
	version, err := c.allocateSnapshotVersion(ctx, bucket, token)
	if err != nil {
		return nil, err
	}
	accountIDs, err := c.writeSnapshotVersionAndReturnAccountIDs(ctx, bucket, version, accounts)
	if err != nil {
		return nil, err
	}
	if err := c.activateSnapshotVersion(ctx, bucket, token, version); err != nil {
		return nil, err
	}
	return accountIDs, nil
}

func (c *schedulerCache) SetSnapshotByAccountIDs(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accountIDs []int64) error {
	if !token.ValidFor(bucket) {
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	}
	version, err := c.allocateSnapshotVersion(ctx, bucket, token)
	if err != nil {
		return err
	}
	if err := c.writeSnapshotAccountIDs(ctx, bucket, version, accountIDs); err != nil {
		return err
	}
	return c.activateSnapshotVersion(ctx, bucket, token, version)
}

func (c *schedulerCache) writeSnapshotVersionAndReturnAccountIDs(ctx context.Context, bucket service.SchedulerBucket, version string, accounts []service.Account) ([]int64, error) {
	accountIDs, err := c.writeAccountIDs(ctx, accounts)
	if err != nil {
		return nil, err
	}
	if err := c.writeSnapshotAccountIDs(ctx, bucket, version, accountIDs); err != nil {
		return nil, err
	}
	return accountIDs, nil
}

func (c *schedulerCache) writeSnapshotAccountIDs(ctx context.Context, bucket service.SchedulerBucket, version string, accountIDs []int64) error {
	members := schedulerSnapshotMembers(accountIDs)
	return c.writeSnapshotMembers(ctx, bucket, version, members)
}

func schedulerSnapshotMembers(accountIDs []int64) []redis.Z {
	if len(accountIDs) == 0 {
		return nil
	}
	// 使用序号作为 score，保持数据库返回的排序语义；重复 ID 继续交由 Redis ZADD
	// 按最后一个 score 覆盖，与直接从账号切片构造成员时的行为一致。
	members := make([]redis.Z, 0, len(accountIDs))
	for idx, accountID := range accountIDs {
		members = append(members, redis.Z{
			Score:  float64(idx),
			Member: strconv.FormatInt(accountID, 10),
		})
	}
	return members
}

func (c *schedulerCache) writeSnapshotMembers(ctx context.Context, bucket service.SchedulerBucket, version string, members []redis.Z) error {
	if len(members) == 0 {
		return nil
	}
	snapshotKey := schedulerSnapshotKey(bucket, version)
	pipe := c.rdb.Pipeline()
	for start := 0; start < len(members); start += c.writeChunkSize {
		end := start + c.writeChunkSize
		if end > len(members) {
			end = len(members)
		}
		pipe.ZAdd(ctx, snapshotKey, members[start:end]...)
	}
	_, err := pipe.Exec(ctx)
	return err
}
