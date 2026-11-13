package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type pollFailureGroupRepo struct{ GroupRepository }

func (*pollFailureGroupRepo) ListActive(context.Context) ([]Group, error) { return nil, nil }

type pollFailureAccountRepo struct{ AccountRepository }

func (*pollFailureAccountRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (*pollFailureAccountRepo) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	return nil, nil
}

type pollFailureRepo struct {
	*outboxCleanupRepo
	readErr   error
	healthErr error
}

func (r *pollFailureRepo) ListAfterAndReleaseDedup(ctx context.Context, id int64, limit int) ([]SchedulerOutboxEvent, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.outboxCleanupRepo.ListAfterAndReleaseDedup(ctx, id, limit)
}
func (r *pollFailureRepo) FirstCreatedAtAfter(ctx context.Context, id int64) (time.Time, bool, error) {
	if r.healthErr != nil {
		return time.Time{}, false, r.healthErr
	}
	return r.outboxCleanupRepo.FirstCreatedAtAfter(ctx, id)
}

type pollFailureCache struct {
	*outboxCleanupCache
	readErr  error
	writeErr error
}

func (c *pollFailureCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	return c.watermark, nil
}
func (c *pollFailureCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	return c.outboxCleanupCache.SetOutboxWatermark(ctx, id)
}

func TestSchedulerPollOutboxFailureBranchesCompensateOnceAndRecover(t *testing.T) {
	for _, threshold := range []int{1, 3} {
		for _, branch := range []string{"read", "handle", "watermark_read", "watermark_write"} {
			t.Run(fmt.Sprintf("%s/threshold_%d", branch, threshold), func(t *testing.T) {
				fault := errors.New("injected processing outage")
				cache := &pollFailureCache{outboxCleanupCache: &outboxCleanupCache{}}
				repo := &pollFailureRepo{outboxCleanupRepo: &outboxCleanupRepo{events: []SchedulerOutboxEvent{{ID: 1, EventType: SchedulerOutboxEventAccountLastUsed, Payload: map[string]any{"last_used": map[string]any{"1": float64(100)}}}}}}
				switch branch {
				case "read":
					repo.readErr = fault
				case "handle":
					cache.updateErr = fault
				case "watermark_read":
					cache.readErr = fault
				case "watermark_write":
					cache.writeErr = fault
				}
				cfg := &config.Config{}
				cfg.Gateway.Scheduling.OutboxLagRebuildFailures = threshold
				svc := NewSchedulerSnapshotService(cache, repo, &pollFailureAccountRepo{}, &pollFailureGroupRepo{}, cfg)
				for range threshold - 1 {
					svc.pollOutbox()
					require.Zero(t, cache.listBucketCalls, "compensation must wait for the configured failed-poll threshold")
				}
				for range 5 {
					svc.pollOutbox()
				}
				require.Equal(t, 1, cache.listBucketCalls, "real failed polls must produce one latched compensation")
				require.True(t, svc.outboxRebuildLatched)
				require.Zero(t, cache.watermark, "compensation must not consume failed events")
				repo.readErr = nil
				cache.updateErr = nil
				cache.readErr = nil
				cache.writeErr = nil
				repo.events = nil
				svc.pollOutbox()
				require.False(t, svc.outboxRebuildLatched, "healthy empty poll ends the episode")
				repo.readErr = fault
				for range threshold - 1 {
					svc.pollOutbox()
					require.Equal(t, 1, cache.listBucketCalls, "recovery must reset the failure counter")
				}
				svc.pollOutbox()
				require.Equal(t, 2, cache.listBucketCalls, "a later failure episode must compensate again")
			})
		}
	}
}

func TestSchedulerPollOutboxTotalOutageBacksOffFailedRebuild(t *testing.T) {
	fault := errors.New("database unavailable")
	cache := &outboxCleanupCache{listBucketErr: fault}
	repo := &pollFailureRepo{outboxCleanupRepo: &outboxCleanupRepo{}, readErr: fault, healthErr: fault}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.OutboxLagRebuildFailures = 1
	svc := NewSchedulerSnapshotService(cache, repo, &pollFailureAccountRepo{}, &pollFailureGroupRepo{}, cfg)
	svc.pollOutbox()
	require.Equal(t, 1, cache.listBucketCalls)
	require.False(t, svc.outboxRebuildLatched, "failed compensation cannot latch success")
	require.Equal(t, "outbox_processing", svc.outboxRebuildRetryReason)
	require.True(t, svc.outboxRebuildRetryAt.After(time.Now()))
	for range 5 {
		svc.pollOutbox()
	}
	require.Equal(t, 1, cache.listBucketCalls, "failed rebuild must obey cooldown")
	svc.lagMu.Lock()
	svc.outboxRebuildRetryAt = time.Now().Add(-time.Second)
	svc.lagMu.Unlock()
	cache.listBucketErr = nil
	svc.pollOutbox()
	require.Equal(t, 2, cache.listBucketCalls)
	require.True(t, svc.outboxRebuildLatched, "successful retry latches even if health queries remain unavailable")
	for range 5 {
		svc.pollOutbox()
	}
	require.Equal(t, 2, cache.listBucketCalls)
	repo.readErr = nil
	repo.healthErr = nil
	svc.pollOutbox()
	require.False(t, svc.outboxRebuildLatched)
	require.Zero(t, svc.outboxProcessingFailures)
}
