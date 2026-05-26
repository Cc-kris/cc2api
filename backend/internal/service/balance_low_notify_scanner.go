package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// BalanceLowNotifyScanner periodically scans users for low balance notifications.
type BalanceLowNotifyScanner struct {
	service  *BalanceNotifyService
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewBalanceLowNotifyScanner(service *BalanceNotifyService, interval time.Duration) *BalanceLowNotifyScanner {
	return &BalanceLowNotifyScanner{
		service:  service,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (s *BalanceLowNotifyScanner) Start() {
	if s == nil || s.service == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *BalanceLowNotifyScanner) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *BalanceLowNotifyScanner) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stats, err := s.service.ScanBalanceLowUsers(ctx)
	if err != nil {
		slog.Error("scan balance low users failed", "error", err)
		return
	}
	if stats.Recovered > 0 || stats.Matched > 0 || stats.Sent > 0 {
		slog.Info("balance low notify scan finished", "recovered", stats.Recovered, "matched", stats.Matched, "marked", stats.Marked, "sent", stats.Sent)
	}
}
