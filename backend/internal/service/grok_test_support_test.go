package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type poolHealthRefresher struct {
	err            error
	delay          time.Duration
	startDelays    []time.Duration
	ignoreContext  bool
	cancel         context.CancelFunc
	newCredentials map[string]any
	calls          atomic.Int64
	active         atomic.Int64
	maxActive      atomic.Int64
	startMu        sync.Mutex
	startTimes     []time.Time
}

func (r *poolHealthRefresher) CacheKey(account *Account) string {
	return fmt.Sprintf("pool-health:%d", account.ID)
}

func (r *poolHealthRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
}

func (r *poolHealthRefresher) NeedsRefresh(*Account, time.Duration) bool { return true }

func (r *poolHealthRefresher) Refresh(ctx context.Context, _ *Account) (map[string]any, error) {
	r.calls.Add(1)
	active := r.active.Add(1)
	defer r.active.Add(-1)
	r.startMu.Lock()
	startIndex := len(r.startTimes)
	r.startTimes = append(r.startTimes, time.Now())
	delay := r.delay
	if startIndex < len(r.startDelays) {
		delay = r.startDelays[startIndex]
	}
	r.startMu.Unlock()
	for {
		maxActive := r.maxActive.Load()
		if active <= maxActive || r.maxActive.CompareAndSwap(maxActive, active) {
			break
		}
	}
	if r.cancel != nil {
		r.cancel()
	}
	if delay > 0 {
		if r.ignoreContext {
			time.Sleep(delay)
		} else {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.newCredentials != nil {
		credentials := make(map[string]any, len(r.newCredentials))
		for key, value := range r.newCredentials {
			credentials[key] = value
		}
		return credentials, nil
	}
	return map[string]any{"access_token": "new-token", "refresh_token": "new-refresh-token"}, nil
}

func grokPoolAccount(id int64) Account {
	return Account{
		ID: id, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "old-token", "refresh_token": "refresh-token"},
	}
}
