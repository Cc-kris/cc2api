package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type UpstreamFinanceSyncRuntime struct {
	service *UpstreamFinanceSyncService
	repo    UpstreamFinanceSyncRepository
	owner   string
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func ProvideUpstreamFinanceSyncRuntime(service *UpstreamFinanceSyncService, repo UpstreamFinanceSyncRepository) *UpstreamFinanceSyncRuntime {
	host, _ := os.Hostname()
	runtime := &UpstreamFinanceSyncRuntime{service: service, repo: repo, owner: fmt.Sprintf("%s:%d", host, os.Getpid())}
	runtime.Start()
	return runtime
}

func (r *UpstreamFinanceSyncRuntime) Start() {
	if r == nil || r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.run(ctx)
	}()
}

func (r *UpstreamFinanceSyncRuntime) Stop() {
	if r == nil || r.cancel == nil {
		return
	}
	r.cancel()
	r.wg.Wait()
	r.cancel = nil
}

func (r *UpstreamFinanceSyncRuntime) run(ctx context.Context) {
	queueTicker := time.NewTicker(2 * time.Second)
	scheduleTicker := time.NewTicker(time.Minute)
	defer queueTicker.Stop()
	defer scheduleTicker.Stop()
	r.enqueueDue(ctx)
	r.drainOne(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-queueTicker.C:
			r.drainOne(ctx)
		case <-scheduleTicker.C:
			r.enqueueDue(ctx)
		}
	}
}

func (r *UpstreamFinanceSyncRuntime) enqueueDue(ctx context.Context) {
	requests, err := r.repo.ListDueSyncRequests(ctx, time.Now().UTC())
	if err != nil {
		if ctx.Err() == nil {
			logger.LegacyPrintf("service.finance", "[Finance] list due upstream syncs failed: %v", err)
		}
		return
	}
	for _, request := range requests {
		if _, _, err = r.service.Enqueue(ctx, request.WalletID, request.SyncType, nil); err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("service.finance", "[Finance] enqueue upstream sync failed: wallet=%d type=%s err=%v", request.WalletID, request.SyncType, err)
		}
	}
}

func (r *UpstreamFinanceSyncRuntime) drainOne(ctx context.Context) {
	processed, err := r.service.RunNext(ctx, r.owner)
	if err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.finance", "[Finance] upstream sync failed: %v", err)
	}
	if processed && ctx.Err() == nil {
		// Continue promptly without a busy loop; the next ticker claims another job.
		return
	}
}
