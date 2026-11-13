package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cowRaceAccountRepo struct {
	AccountRepository
	updates atomic.Int64
}

func (r *cowRaceAccountRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	r.updates.Add(1)
	return nil
}

type cowRaceSchedulerCache struct {
	SchedulerCache
	writes atomic.Int64
}

func (c *cowRaceSchedulerCache) SetAccount(context.Context, *Account) error {
	c.writes.Add(1)
	return nil
}

// Scheduler callers own the Account top level; only the immutable inner maps
// come from the same master. All three mutation paths must preserve that master.
func TestModelRateLimitCOWRace_LocalAntigravityPaths(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	limits := map[string]any{
		creditsExhaustedKey: map[string]any{"rate_limit_reset_at": now.Add(time.Hour).Format(time.RFC3339)},
		"existing":          map[string]any{"rate_limit_reset_at": now.Add(time.Hour).Format(time.RFC3339)},
	}
	extra := map[string]any{modelRateLimitsKey: limits, "local_metadata": "keep"}
	mapping := map[string]any{"existing": "existing"}
	for worker := 0; worker < 64; worker++ {
		model := fmt.Sprintf("model-%d", worker)
		mapping[model] = model
	}
	master := Account{ID: 77, Platform: PlatformAntigravity, Extra: extra,
		Credentials: map[string]any{"model_mapping": mapping}}
	master.PrepareSharedSnapshot()
	repo := &cowRaceAccountRepo{}
	cache := &cowRaceSchedulerCache{}
	svc := &AntigravityGatewayService{
		accountRepo:       repo,
		schedulerSnapshot: NewSchedulerSnapshotService(cache, nil, nil, nil, nil),
	}
	const workers = 64
	const iterations = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				candidate := master
				model := fmt.Sprintf("model-%d", worker)
				if worker%2 == 0 {
					if !candidate.isModelRateLimitedWithContext(ctx, "existing") || !candidate.isCreditsExhausted() {
						t.Error("shared reader lost inherited limits")
						return
					}
					if candidate.isModelRateLimitedWithContext(ctx, model) {
						t.Error("shared reader observed another request's limit")
						return
					}
					continue
				}
				switch worker % 3 {
				case 0:
					setAccountModelRateLimitSnapshot(&candidate, model, now.Add(time.Hour), "test", now)
				case 1:
					svc.updateAccountModelRateLimitInCache(ctx, &candidate, model, now.Add(time.Hour))
				case 2:
					svc.clearCreditsExhausted(ctx, &candidate)
					if candidate.isCreditsExhausted() {
						t.Error("own credits key not cleared")
					}
				}
				if worker%3 != 2 && !candidate.isModelRateLimitedWithContext(ctx, model) {
					t.Error("own model limit not applied")
				}
				if !candidate.isModelRateLimitedWithContext(ctx, "existing") {
					t.Error("unrelated limit lost")
				}
				if candidate.Extra["local_metadata"] != "keep" {
					t.Error("local metadata lost")
				}
			}
		}(worker)
	}
	close(start)
	wg.Wait()
	require.Len(t, extra, 2)
	require.Len(t, limits, 2)
	require.True(t, master.isCreditsExhausted())
	require.Positive(t, cache.writes.Load())
	require.Positive(t, repo.updates.Load())
}
