//go:build unit || integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// This is an accelerated, repeatable read-path measurement, not AC20's full
// mixed outbox/mutation storm or evidence from a production-sized database.
// V043_LOAD_REALTIME=1 paces the balanced case at 200 reads/second for 60 seconds.
// V043_EXPECT_MEMO=1 additionally enforces the memo contract after integration.
func TestV043SchedulerReadLoadMeasurement(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr(), DisableIdentity: true})
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.Ping(ctx).Err())
	cache := repository.NewSchedulerCache(client)
	hook := &v043RedisCounter{counts: make(map[string]int64)}
	client.AddHook(hook)

	const accountCount, groupCount = 5000, 200
	accounts := make([]service.Account, accountCount)
	for i := range accounts {
		accounts[i] = service.Account{
			ID: int64(i + 1), Name: fmt.Sprintf("load-account-%d", i+1),
			Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1,
			GroupIDs:    []int64{int64(i/(accountCount/groupCount) + 1)},
			Credentials: map[string]any{"api_key": "test-only-key"},
			Extra:       map[string]any{"model_rate_limits": map[string]any{}},
		}
	}
	buckets := make([]service.SchedulerBucket, groupCount)
	for i := range buckets {
		buckets[i] = service.SchedulerBucket{GroupID: int64(i + 1), Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
		v043SetSnapshot(t, ctx, cache, buckets[i], accounts[i*25:(i+1)*25])
		got, hit, err := cache.GetSnapshot(ctx, buckets[i])
		require.NoError(t, err)
		require.True(t, hit)
		require.Len(t, got, 25)
	}

	for _, tc := range []struct {
		name        string
		buckets     []service.SchedulerBucket
		size, reads int
	}{
		{"balanced_5000_accounts_200_groups", buckets, 25, 12000},
		{"dense_5000_account_bucket", []service.SchedulerBucket{{GroupID: 1000, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}}, 5000, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.size == 5000 {
				v043SetSnapshot(t, ctx, cache, tc.buckets[0], accounts)
				_, hit, err := cache.GetSnapshot(ctx, tc.buckets[0])
				require.NoError(t, err)
				require.True(t, hit)
			}
			// Snapshot writes advance shared metadata revisions. Warm only after
			// every setup write, otherwise the later buckets invalidate earlier ones.
			for _, bucket := range tc.buckets {
				got, hit, err := cache.GetSnapshot(ctx, bucket)
				require.NoError(t, err)
				require.True(t, hit)
				require.Len(t, got, tc.size)
			}
			hook.reset()
			start := time.Now()
			var tick *time.Ticker
			realtime := os.Getenv("V043_LOAD_REALTIME") == "1" && tc.size == 25
			if realtime {
				tick = time.NewTicker(time.Second / 200)
				defer tick.Stop()
			}
			for i := 0; i < tc.reads; i++ {
				if tick != nil {
					<-tick.C
				}
				got, hit, err := cache.GetSnapshot(ctx, tc.buckets[i%len(tc.buckets)])
				require.NoError(t, err)
				require.True(t, hit)
				require.Len(t, got, tc.size)
			}
			counts, roles, keyRoles := hook.snapshot()
			total := int64(0)
			for _, n := range counts {
				total += n
			}
			metrics := struct {
				Case           string           `json:"case"`
				Accounts       int              `json:"accounts"`
				Groups         int              `json:"groups"`
				Reads          int              `json:"reads"`
				Realtime       bool             `json:"realtime"`
				ElapsedMS      int64            `json:"elapsed_ms"`
				Commands       int64            `json:"commands"`
				CommandsByName map[string]int64 `json:"commands_by_name"`
				CommandsByRole map[string]int64 `json:"commands_by_key_role"`
				KeysByRole     map[string]int64 `json:"keys_by_role"`
			}{tc.name, accountCount, len(tc.buckets), tc.reads, realtime, time.Since(start).Milliseconds(), total, counts, roles, keyRoles}
			encoded, err := json.Marshal(metrics)
			require.NoError(t, err)
			t.Logf("V043_LOAD_METRICS %s", encoded)
			if os.Getenv("V043_EXPECT_MEMO") == "1" {
				require.Zero(t, counts["zrange"], "warm memo must not reload IDs")
				require.Zero(t, keyRoles["account_metadata"], "warm memo must not reload JSON metadata payloads")
				require.Zero(t, keyRoles["account_payload"], "warm memo must not reload full account payloads")
				require.GreaterOrEqual(t, roles["bucket_control"], int64(tc.reads), "memo must validate bucket/revision freshness")
				require.Equal(t, int64(tc.reads*tc.size), keyRoles["last_used"], "memo must preserve per-account last_used freshness")
				// MGET is valid for ready/active/revisions and live last_used.
				// Total-command reduction remains a separately reported AC20 gate;
				// this contract must not silently replace its >=80% threshold.
			}
		})
	}

	t.Run("top_level_objects_are_call_owned", func(t *testing.T) {
		first, hit, err := cache.GetSnapshot(ctx, buckets[0])
		require.NoError(t, err)
		require.True(t, hit)
		second, hit, err := cache.GetSnapshot(ctx, buckets[0])
		require.NoError(t, err)
		require.True(t, hit)
		require.NotSame(t, first[0], second[0], "cached nested values may be shared, but mutable Account structs must not be")
		want := second[0].Name
		first[0].Name = "request-local-change"
		third, hit, err := cache.GetSnapshot(ctx, buckets[0])
		require.NoError(t, err)
		require.True(t, hit)
		require.Equal(t, want, third[0].Name, "one request must not mutate another request's top-level fields")
	})
}

// v043SetSnapshot isolates the one intentional API difference so the exact same
// read workload can run before and after the bucket-fencing migration.
func v043SetSnapshot(t *testing.T, ctx context.Context, cache service.SchedulerCache, bucket service.SchedulerBucket, accounts []service.Account) {
	t.Helper()
	setter := reflect.ValueOf(cache).MethodByName("SetSnapshot")
	args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(bucket)}
	switch setter.Type().NumIn() {
	case 3:
	case 4:
		capture := reflect.ValueOf(cache).MethodByName("CaptureBucketWriteToken")
		require.True(t, capture.IsValid(), "tokenized setter requires capture API")
		result := capture.Call(args)
		require.Len(t, result, 2)
		require.True(t, result[1].IsNil(), "capture failed: %v", result[1])
		args = append(args, result[0])
	default:
		t.Fatalf("unexpected SetSnapshot arity %d", setter.Type().NumIn())
	}
	args = append(args, reflect.ValueOf(accounts))
	result := setter.Call(args)
	require.Len(t, result, 1)
	if !result[0].IsNil() {
		t.Fatal(result[0].Interface())
	}
}

type v043RedisCounter struct {
	mu       sync.Mutex
	counts   map[string]int64
	roles    map[string]int64
	keyRoles map[string]int64
}

func (h *v043RedisCounter) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h *v043RedisCounter) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error { h.add(cmd); return next(ctx, cmd) }
}
func (h *v043RedisCounter) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			h.add(cmd)
		}
		return next(ctx, cmds)
	}
}
func (h *v043RedisCounter) add(cmd redis.Cmder) {
	h.mu.Lock()
	defer h.mu.Unlock()
	name := strings.ToLower(cmd.Name())
	h.counts[name]++
	if h.roles == nil {
		h.roles = make(map[string]int64)
		h.keyRoles = make(map[string]int64)
	}
	args := cmd.Args()
	var keys []any
	switch name {
	case "get", "zrange":
		if len(args) > 1 {
			keys = args[1:2]
		}
	case "mget":
		keys = args[1:]
	}
	seen := make(map[string]bool)
	for _, arg := range keys {
		role := v043SchedulerKeyRole(fmt.Sprint(arg))
		h.keyRoles[role]++
		seen[role] = true
	}
	for role := range seen {
		h.roles[role]++
	}
}

func v043SchedulerKeyRole(key string) string {
	switch {
	case strings.HasPrefix(key, "sched:acc:last_used:"):
		return "last_used"
	case strings.HasPrefix(key, "sched:ready:"), strings.HasPrefix(key, "sched:active:"), key == "sched:meta:rev", strings.HasPrefix(key, "sched:meta:rev:"):
		return "bucket_control"
	case strings.HasPrefix(key, "sched:meta:"):
		return "account_metadata"
	case strings.HasPrefix(key, "sched:acc:"):
		return "account_payload"
	case strings.HasPrefix(key, "sched:"):
		return "snapshot_ids"
	default:
		return "other"
	}
}

func (h *v043RedisCounter) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counts = make(map[string]int64)
	h.roles = make(map[string]int64)
	h.keyRoles = make(map[string]int64)
}
func (h *v043RedisCounter) snapshot() (map[string]int64, map[string]int64, map[string]int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clone := func(src map[string]int64) map[string]int64 {
		result := make(map[string]int64, len(src))
		for k, v := range src {
			result[k] = v
		}
		return result
	}
	return clone(h.counts), clone(h.roles), clone(h.keyRoles)
}
