package service

import (
	"context"
	"github.com/robfig/cron/v3"
	"log/slog"
	"sync"
	"time"
)

// ContentNewsRuntime wakes the DB-backed worker. Source next_run_at and the
// automation row remain the source of truth, so this interval is only a
// scheduler heartbeat and not a product quota.
type ContentNewsRuntime struct {
	worker *ContentNewsWorker
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewContentNewsRuntime(worker *ContentNewsWorker) *ContentNewsRuntime {
	return &ContentNewsRuntime{worker: worker}
}

func (r *ContentNewsRuntime) Start(parent context.Context) {
	if r == nil || r.worker == nil {
		return
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel, r.done = cancel, make(chan struct{})
	done := r.done
	r.mu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var next time.Time
		var cronExpr string
		var cronLocation *time.Location
		for {
			if err := r.worker.ProcessQueuedRuns(ctx, 10); err != nil && ctx.Err() == nil {
				slog.Error("content news queued run failed", "error", err)
			}
			var policy *ContentNewsWorkerPolicy
			var policyErr error
			if r.worker.policy != nil {
				policy, policyErr = r.worker.policy.GetContentNewsWorkerPolicy(ctx)
			}
			now := time.Now().UTC()
			if policyErr == nil && (r.worker.policy == nil || (policy != nil && policy.Enabled)) {
				if policy == nil {
					policy = &ContentNewsWorkerPolicy{FetchCron: "0 * * * * *"}
				}
				location := time.UTC
				if policy.Timezone != "" {
					if loaded, err := time.LoadLocation(policy.Timezone); err == nil {
						location = loaded
					}
				}
				if cronExpr != policy.FetchCron || cronLocation == nil || cronLocation.String() != location.String() {
					next = time.Time{}
					cronExpr = policy.FetchCron
					cronLocation = location
				}
				localNow := now.In(location)
				if next.IsZero() || !localNow.Before(next) {
					if _, err := r.worker.RunDueFetches(ctx, 100); err != nil && ctx.Err() == nil {
						slog.Error("content news scheduled fetch failed", "error", err)
					}
					parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
					if schedule, err := parser.Parse(policy.FetchCron); err == nil {
						next = schedule.Next(localNow)
					} else {
						next = localNow.Add(time.Minute)
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (r *ContentNewsRuntime) Stop(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel, r.done = nil, nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
}
