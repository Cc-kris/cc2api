//go:build integration

package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Real service lifecycle + real PostgreSQL repositories + real Redis repository.
// No production DB or shared API runtime is used. The only failure fake is a
// switchable error at the outbox read port; healthy calls reach PostgreSQL.
// Reads exercise ListSchedulableAccounts, not the final account scoring picker.
func TestV043SchedulerMixedStormAcceptance(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	f := v043NewStormFixture(t)
	duration := 12 * time.Second
	if os.Getenv("V043_STORM_REALTIME") == "1" {
		duration = 60 * time.Second
	}
	metrics := v043StormMetrics{Case: "mixed_mutation_expiry_disable", Accounts: 5000, Groups: 200, Reads: 12000, RequestedDurationMS: duration.Milliseconds()}
	startFull := f.cache.fullEntries.Load()
	f.commands.reset()
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	producerErr := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(duration / 2000)
		defer ticker.Stop()
		rng := rand.New(rand.NewSource(43020))
		for i := 0; i < 2000; i++ {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				producerErr <- ctx.Err()
				return
			}
			id := int64(rng.Intn(5000) + 1)
			if err := f.accounts.UpdateExtra(ctx, id, map[string]any{"v043_storm_revision": i}); err != nil {
				producerErr <- err
				return
			}
			if i%10 == 0 {
				expiredID := int64(i/10 + 1)
				if _, err := f.db.ExecContext(ctx, "UPDATE accounts SET expires_at=NOW()-INTERVAL '1 second', auto_pause_on_expired=TRUE WHERE id=$1", expiredID); err != nil {
					producerErr <- err
					return
				}
				n, err := f.accounts.AutoPauseExpiredAccounts(ctx, time.Now())
				if err != nil || n != 1 {
					producerErr <- fmt.Errorf("expiry %d affected %d: %v", expiredID, n, err)
					return
				}
			}
			if i%100 == 0 {
				groupID := int64(i/100 + 1)
				group, err := f.groups.GetByID(ctx, groupID)
				if err != nil {
					producerErr <- err
					return
				}
				group.Status = "disabled"
				if err := f.groups.Update(ctx, group); err != nil {
					producerErr <- err
					return
				}
			}
		}
		producerErr <- nil
	}()

	stopSample := make(chan struct{})
	samplerDone := make(chan struct{})
	var peak atomic.Int64
	var sampleErr atomic.Bool
	go func() {
		defer close(samplerDone)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				var n int64
				if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE id>$1", f.cache.watermark.Load()).Scan(&n); err != nil {
					sampleErr.Store(true)
					return
				}
				for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
				}
			case <-stopSample:
				return
			}
		}
	}()
	readTicker := time.NewTicker(duration / time.Duration(metrics.Reads))
	for i := 0; i < metrics.Reads; i++ {
		<-readTicker.C
		// Disabled groups are not eligible for scheduling. Distribute queries
		// over the remaining groups; disabled bucket lifecycle is checked below.
		groupID := int64(i%180 + 21)
		accounts, _, err := f.svc.ListSchedulableAccounts(context.WithValue(ctx, v043StormReadKey{}, true), &groupID, service.PlatformOpenAI, true)
		if err != nil {
			metrics.ReadErrors++
		}
		if len(accounts) != 25 {
			metrics.WrongReadSizes++
		}
	}
	readTicker.Stop()
	mutationErr := <-producerErr
	metrics.ProducerElapsedMS = time.Since(started).Milliseconds()
	f.waitDrained(t)
	close(stopSample)
	<-samplerDone
	f.svc.Stop()
	metrics.ElapsedMS = time.Since(started).Milliseconds()
	metrics.FullRebuildEntries = f.cache.fullEntries.Load() - startFull
	metrics.PendingPeakSampled = peak.Load()
	metrics.SampleFailed = sampleErr.Load()
	metrics.Commands, metrics.ReadCommands, metrics.CommandsByName = f.commands.snapshot()
	require.NoError(t, f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE schedulable=FALSE AND id BETWEEN 1 AND 200").Scan(&metrics.ExpiredAccounts))
	require.NoError(t, f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM groups WHERE status='disabled'").Scan(&metrics.DisabledGroups))
	for id := int64(1); id <= 20; id++ {
		_, hit, err := f.rawCache.GetSnapshot(ctx, service.SchedulerBucket{GroupID: id, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeForced})
		if hit || err != nil {
			metrics.DisabledBucketsStillReadable++
		}
	}
	v043EmitStormMetrics(t, metrics)
	require.NoError(t, mutationErr)
	require.Zero(t, metrics.ReadErrors)
	require.Zero(t, metrics.WrongReadSizes)
	require.False(t, metrics.SampleFailed)
	require.Equal(t, int64(200), metrics.ExpiredAccounts)
	require.Equal(t, int64(20), metrics.DisabledGroups)
	if os.Getenv("V043_STORM_ENFORCE") == "1" {
		require.LessOrEqual(t, metrics.FullRebuildEntries, int64(2), "AC20 full rebuild gate; startup excluded")
		require.Zero(t, metrics.DisabledBucketsStillReadable, "disabled groups must not retain readable snapshots")
		path := os.Getenv("V043_STORM_BASELINE")
		require.NotEmpty(t, path, "strict AC20 comparison requires actual baseline metrics, not guessed numbers")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var baseline v043StormMetrics
		require.NoError(t, json.Unmarshal(data, &baseline))
		require.Equal(t, metrics.Case, baseline.Case)
		require.Equal(t, metrics.Reads, baseline.Reads)
		require.Equal(t, metrics.RequestedDurationMS, baseline.RequestedDurationMS, "compare the same pacing")
		require.Positive(t, baseline.ReadCommands)
		require.LessOrEqual(t, metrics.ReadCommands*5, baseline.ReadCommands, "original AC20 total READ command reduction >=80%; not metadata-only")
		// Sampling can underestimate the true peak. Passing this is necessary,
		// not sufficient for an exact outbox peak gate; report the limitation.
		require.Positive(t, baseline.PendingPeakSampled)
		require.LessOrEqual(t, metrics.PendingPeakSampled*5, baseline.PendingPeakSampled, "AC20 sampled pending peak <=20% of measured baseline")
	}
}

func TestV043SchedulerOutboxReadFailureRecoveryAcceptance(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	f := v043NewStormFixture(t)
	before := f.cache.fullEntries.Load()
	f.commands.reset()
	f.outbox.fail.Store(true)
	require.NoError(t, f.accounts.UpdateExtra(context.Background(), 300, map[string]any{"v043_fault_revision": 1}))
	// Keep the original 30-second failure period even in accelerated storm mode.
	failureStarted := time.Now()
	deadline := time.NewTimer(30 * time.Second)
	<-deadline.C
	failedPolls := f.outbox.failedPolls.Load()
	during := f.cache.fullEntries.Load() - before
	f.outbox.fail.Store(false)
	f.waitDrained(t)
	polls := f.outbox.polls.Load()
	require.Eventually(t, func() bool { return f.outbox.polls.Load() >= polls+3 }, 10*time.Second, 50*time.Millisecond)
	f.svc.Stop()
	after := f.cache.fullEntries.Load() - before
	commands, readCommands, byName := f.commands.snapshot()
	metrics := v043StormMetrics{Case: "outbox_read_failure_30s_then_recovery", Accounts: 5000, Groups: 200, ElapsedMS: time.Since(failureStarted).Milliseconds(), FullRebuildEntries: after, DuringFailureRebuildEntries: during, FailedPolls: failedPolls, Commands: commands, ReadCommands: readCommands, CommandsByName: byName}
	v043EmitStormMetrics(t, metrics)
	require.GreaterOrEqual(t, failedPolls, int64(20), "service must actually experience repeated outbox read failures")
	require.Equal(t, int64(1), during, "AC21 requires exactly one compensation rebuild during the failure period")
	require.Equal(t, during, after, "recovery must not add a further full rebuild")
}

type v043StormMetrics struct {
	Case                         string           `json:"case"`
	Accounts                     int              `json:"accounts"`
	Groups                       int              `json:"groups"`
	Reads                        int              `json:"reads"`
	RequestedDurationMS          int64            `json:"requested_duration_ms"`
	ProducerElapsedMS            int64            `json:"producer_and_reads_elapsed_ms"`
	ElapsedMS                    int64            `json:"elapsed_ms"`
	Commands                     int64            `json:"commands"`
	ReadCommands                 int64            `json:"read_commands"`
	CommandsByName               map[string]int64 `json:"commands_by_name"`
	FullRebuildEntries           int64            `json:"full_rebuild_entries"`
	PendingPeakSampled           int64            `json:"pending_peak_sampled"`
	SampleFailed                 bool             `json:"sample_failed"`
	ReadErrors                   int              `json:"read_errors"`
	WrongReadSizes               int              `json:"wrong_read_sizes"`
	ExpiredAccounts              int64            `json:"expired_accounts"`
	DisabledGroups               int64            `json:"disabled_groups"`
	DisabledBucketsStillReadable int              `json:"disabled_buckets_still_readable"`
	DuringFailureRebuildEntries  int64            `json:"during_failure_rebuild_entries"`
	FailedPolls                  int64            `json:"failed_polls"`
}

func v043EmitStormMetrics(t *testing.T, m v043StormMetrics) {
	t.Helper()
	data, err := json.Marshal(m)
	require.NoError(t, err)
	t.Logf("V043_STORM_METRICS %s", data)
	if path := os.Getenv("V043_STORM_OUTPUT"); path != "" {
		// Separate suffixes prevent the recovery test replacing storm baseline.
		require.NoError(t, os.WriteFile(path+"-"+m.Case+".json", data, 0600))
	}
}

type v043StormFixture struct {
	db       *sql.DB
	accounts service.AccountRepository
	groups   service.GroupRepository
	rawCache service.SchedulerCache
	cache    *v043StormCacheObserver
	outbox   *v043StormOutboxFault
	commands *v043StormCommandCounter
	svc      *service.SchedulerSnapshotService
}

func v043NewStormFixture(t *testing.T) *v043StormFixture {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:18.1-alpine3.23", tcpostgres.WithDatabase("v043_storm"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("fixture-only-password"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err, "dedicated Docker PostgreSQL is required; this test must not silently skip")
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(12)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.PingContext(ctx))
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	// Only this freshly-created disposable database is seeded. No production
	// credentials, shared databases, or external URLs are accepted by the test.
	_, err = db.ExecContext(ctx, `DELETE FROM groups;
INSERT INTO groups(id,name,platform,status) SELECT n,'v043-group-'||n,'openai','active' FROM generate_series(1,200) AS n;
INSERT INTO accounts(id,name,platform,type,credentials,extra,schedulable,status) SELECT n,'v043-account-'||n,'openai','apikey','{"api_key":"fixture-only"}'::jsonb,'{}'::jsonb,TRUE,'active' FROM generate_series(1,5000) AS n;
INSERT INTO account_groups(account_id,group_id) SELECT n,(n-1)/25+1 FROM generate_series(1,5000) AS n;`)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr(), DisableIdentity: true})
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.Ping(ctx).Err())
	counter := &v043StormCommandCounter{}
	counter.reset()
	rdb.AddHook(counter)
	rawCache := repository.NewSchedulerCache(rdb)
	cache := &v043StormCacheObserver{SchedulerCache: rawCache}
	outbox := &v043StormOutboxFault{SchedulerOutboxRepository: repository.NewSchedulerOutboxRepository(db)}
	accounts := repository.NewAccountRepository(client, db, rawCache)
	groups := repository.NewGroupRepository(client, db)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.OutboxPollIntervalSeconds = 1
	cfg.Gateway.Scheduling.OutboxLagRebuildSeconds = 5
	cfg.Gateway.Scheduling.OutboxLagRebuildFailures = 2
	cfg.Gateway.Scheduling.OutboxBacklogRebuildRows = 10000
	cfg.Gateway.Scheduling.FullRebuildIntervalSeconds = 0
	svc := service.NewSchedulerSnapshotService(cache, outbox, accounts, groups, cfg)
	f := &v043StormFixture{db: db, accounts: accounts, groups: groups, rawCache: rawCache, cache: cache, outbox: outbox, commands: counter, svc: svc}
	svc.Start()
	t.Cleanup(svc.Stop)
	// Empty initial Redis means the complete canonical bucket count observes
	// startup publication, rather than mistaking the first warm bucket for done.
	expectedBuckets := 410 // old service: 10 ungrouped + 200*(single+forced)
	if reflect.ValueOf(rawCache).MethodByName("CaptureBucketWriteToken").IsValid() {
		expectedBuckets = 12 * 201 // W1: five platforms, plus two mixed, for 0..200
	}
	require.Eventually(t, func() bool {
		buckets, err := rawCache.ListBuckets(ctx)
		return err == nil && len(buckets) >= expectedBuckets
	}, 2*time.Minute, 100*time.Millisecond, "startup must publish every canonical bucket")
	f.waitDrained(t)
	t.Logf("V043_STORM_SETUP accounts=5000 groups=200 canonical_buckets=%d startup_full_entries=%d", expectedBuckets, cache.fullEntries.Load())
	return f
}

func (f *v043StormFixture) waitDrained(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		max, err := f.outbox.SchedulerOutboxRepository.MaxID(context.Background())
		return err == nil && f.cache.watermark.Load() >= max
	}, 2*time.Minute, 50*time.Millisecond, "real outbox watermark must catch up")
}

// Counts actual full-rebuild entry calls, NOT manufactured successful rebuilds.
// Group retirement also calls ListBuckets, so caller filtering is essential.
// Stop() joins service workers before metrics are finalized.
type v043StormCacheObserver struct {
	service.SchedulerCache
	fullEntries atomic.Int64
	watermark   atomic.Int64
}

func (c *v043StormCacheObserver) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	if pc, _, _, ok := runtime.Caller(1); ok {
		name := runtime.FuncForPC(pc).Name()
		if strings.HasSuffix(name, ".runInitialRebuild") || strings.HasSuffix(name, ".triggerFullRebuild") || strings.HasSuffix(name, ".rebuildFullSnapshot") {
			c.fullEntries.Add(1)
		}
	}
	return c.SchedulerCache.ListBuckets(ctx)
}
func (c *v043StormCacheObserver) SetOutboxWatermark(ctx context.Context, id int64) error {
	if err := c.SchedulerCache.SetOutboxWatermark(ctx, id); err != nil {
		return err
	}
	c.watermark.Store(id)
	return nil
}

type v043StormOutboxFault struct {
	service.SchedulerOutboxRepository
	fail        atomic.Bool
	polls       atomic.Int64
	failedPolls atomic.Int64
}

func (r *v043StormOutboxFault) read(ctx context.Context, after int64, limit int) ([]service.SchedulerOutboxEvent, error) {
	r.polls.Add(1)
	if r.fail.Load() {
		r.failedPolls.Add(1)
		return nil, errors.New("v043 intentional outbox read outage")
	}
	// Adapt only the renamed outbox public port across baseline/W1 APIs.
	method := reflect.ValueOf(r.SchedulerOutboxRepository).MethodByName("ListAfterAndReleaseDedup")
	if !method.IsValid() {
		method = reflect.ValueOf(r.SchedulerOutboxRepository).MethodByName("ListAfter")
	}
	result := method.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(after), reflect.ValueOf(limit)})
	if !result[1].IsNil() {
		return nil, result[1].Interface().(error)
	}
	return result[0].Interface().([]service.SchedulerOutboxEvent), nil
}
func (r *v043StormOutboxFault) ListAfter(ctx context.Context, after int64, limit int) ([]service.SchedulerOutboxEvent, error) {
	return r.read(ctx, after, limit)
}
func (r *v043StormOutboxFault) ListAfterAndReleaseDedup(ctx context.Context, after int64, limit int) ([]service.SchedulerOutboxEvent, error) {
	return r.read(ctx, after, limit)
}

type v043StormReadKey struct{}
type v043StormCommandCounter struct {
	mu     sync.Mutex
	total  int64
	reads  int64
	byName map[string]int64
}

func (h *v043StormCommandCounter) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h *v043StormCommandCounter) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error { h.add(ctx, cmd); return next(ctx, cmd) }
}
func (h *v043StormCommandCounter) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			h.add(ctx, cmd)
		}
		return next(ctx, cmds)
	}
}
func (h *v043StormCommandCounter) add(ctx context.Context, cmd redis.Cmder) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total++
	h.byName[strings.ToLower(cmd.Name())]++
	if ctx.Value(v043StormReadKey{}) == true {
		h.reads++
	}
}
func (h *v043StormCommandCounter) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total, h.reads = 0, 0
	h.byName = make(map[string]int64)
}
func (h *v043StormCommandCounter) snapshot() (int64, int64, map[string]int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	byName := make(map[string]int64, len(h.byName))
	for k, v := range h.byName {
		byName[k] = v
	}
	return h.total, h.reads, byName
}
