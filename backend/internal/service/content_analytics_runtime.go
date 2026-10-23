package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type ContentAnalyticsRuntime struct {
	service  *ContentAnalyticsService
	timezone string
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewContentAnalyticsRuntime(s *ContentAnalyticsService, timezone string) *ContentAnalyticsRuntime {
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	return &ContentAnalyticsRuntime{service: s, timezone: timezone}
}

func (r *ContentAnalyticsRuntime) Start(parent context.Context) {
	if r == nil || r.service == nil {
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
		// BI freshness target is five minutes. The initial aggregation runs
		// immediately, then the ticker keeps the daily rollups within that SLA.
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			if err := r.service.Aggregate(ctx, time.Now().UTC(), r.timezone); err != nil && ctx.Err() == nil {
				slog.Error("content analytics aggregation failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (r *ContentAnalyticsRuntime) Stop(ctx context.Context) {
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
