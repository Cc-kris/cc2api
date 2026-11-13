package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPoolStatusBudgetOverridesAnyErrorRetries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count any
		codes []int
		want  int
	}{
		{"default", nil, nil, 2}, {"explicit-two", 2, []int{502}, 2},
		{"zero", 0, []int{502}, 0}, {"disabled", 2, []int{}, 0},
		{"larger-than-global", 3, []int{502}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &service.Account{ID: 42, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
			if tc.count != nil {
				a.Credentials["pool_mode_retry_count"] = tc.count
			}
			if tc.codes != nil {
				a.Credentials["pool_mode_retry_status_codes"] = tc.codes
			}
			e := &service.UpstreamFailoverError{StatusCode: 502, Reason: service.GatewayFailureReasonPoolStatus, RetryableOnSameAccount: a.IsPoolModeRetryableStatus(502)}
			cfg := anyErrorCfg(5)
			require.Equal(t, tc.want, sameAccountRetryLimit(context.Background(), cfg, a, e))
			state := NewFailoverState(3, true).WithAnyErrorFailover(context.Background(), cfg)
			state.SameAccountRetryCount[a.ID] = tc.want
			require.False(t, state.willRetrySameAccount(a.ID, e, a))
			require.Equal(t, FailoverContinue, state.HandleFailoverError(context.Background(), &mockTempUnscheduler{}, a.ID, service.PlatformAnthropic, e, a))
			require.Contains(t, state.FailedAccountIDs, a.ID)
			require.Equal(t, 1, state.SwitchCount)
			if tc.want > 0 {
				state.SameAccountRetryCount[a.ID] = tc.want - 1
				require.True(t, state.willRetrySameAccount(a.ID, e, a))
			}
		})
	}
	// A committed stream must continue on a different account even when both
	// the account and the global policy would normally permit retries.
	partial := &service.UpstreamFailoverError{StatusCode: 502, SafeToFailoverAfterWrite: true, RetryableOnSameAccount: true}
	pool := &service.Account{ID: 43, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true, "pool_mode_retry_count": 2}}
	require.Zero(t, sameAccountRetryLimit(context.Background(), anyErrorCfg(5), pool, partial))
	state := NewFailoverState(3, false).WithAnyErrorFailover(context.Background(), anyErrorCfg(5))
	require.False(t, state.willRetrySameAccount(pool.ID, partial, pool))
	require.Equal(t, FailoverContinue, state.HandleFailoverError(context.Background(), &mockTempUnscheduler{}, pool.ID, service.PlatformOpenAI, partial, pool))
	require.Contains(t, state.FailedAccountIDs, pool.ID)
	// Ordinary non-pool transient errors retain the configured retry behavior.
	require.Equal(t, 5, sameAccountRetryLimit(context.Background(), anyErrorCfg(5), nil, &service.UpstreamFailoverError{StatusCode: 502}))
}
