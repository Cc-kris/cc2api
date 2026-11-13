package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestServiceRetryBudgetExhausted(t *testing.T) {
	for _, retryable := range []bool{false, true} {
		e := &service.UpstreamFailoverError{StatusCode: 502, RetryableOnSameAccount: retryable, SameAccountRetryBudgetExhausted: true}
		a := &service.Account{ID: 44, Type: service.AccountTypeAPIKey}
		require.Zero(t, sameAccountRetryLimit(context.Background(), anyErrorCfg(5), a, e))
		state := NewFailoverState(3, false).WithAnyErrorFailover(context.Background(), anyErrorCfg(5))
		require.False(t, state.willRetrySameAccount(a.ID, e, a))
		require.Equal(t, FailoverContinue, state.HandleFailoverError(context.Background(), &mockTempUnscheduler{}, a.ID, service.PlatformGemini, e, a))
		require.Contains(t, state.FailedAccountIDs, a.ID)
	}
	// Without the explicit metadata, unrelated/early failures retain the contract.
	require.Equal(t, 5, sameAccountRetryLimit(context.Background(), anyErrorCfg(5), nil, &service.UpstreamFailoverError{StatusCode: 502}))
}

func TestPoolLegacyHTTPStatusBudget(t *testing.T) {
	for _, tc := range []struct {
		name                string
		codes               []int
		count, status, want int
	}{
		{"default", nil, 2, 502, 2}, {"empty", []int{}, 2, 502, 0}, {"zero", []int{502}, 0, 502, 0}, {"two", []int{502}, 2, 502, 2}, {"three", []int{502}, 3, 502, 3}, {"excluded401", []int{502}, 2, 401, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &service.Account{ID: 45, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true, "pool_mode_retry_count": tc.count}}
			if tc.codes != nil {
				a.Credentials["pool_mode_retry_status_codes"] = tc.codes
			}
			e := &service.UpstreamFailoverError{StatusCode: tc.status, ResponseBody: []byte(`{"error":{"message":"upstream failed"}}`), RetryableOnSameAccount: a.IsPoolModeRetryableStatus(tc.status)}
			require.Equal(t, tc.want, sameAccountRetryLimit(context.Background(), anyErrorCfg(5), a, e))
			state := NewFailoverState(3, false).WithAnyErrorFailover(context.Background(), anyErrorCfg(5))
			state.SameAccountRetryCount[a.ID] = tc.want
			require.False(t, state.willRetrySameAccount(a.ID, e, a))
			require.Equal(t, FailoverContinue, state.HandleFailoverError(context.Background(), &mockTempUnscheduler{}, a.ID, service.PlatformOpenAI, e, a))
			require.Contains(t, state.FailedAccountIDs, a.ID)
			// The HTTP pool policy must not suppress classified pre-output stream errors.
			stream := &service.UpstreamFailoverError{StatusCode: 502, Reason: service.GatewayFailureReasonPreOutput, ResponseBody: e.ResponseBody}
			require.False(t, isPoolHTTPStatusFailure(a, stream))
			require.Equal(t, 5, sameAccountRetryLimit(context.Background(), anyErrorCfg(5), a, stream))
		})
	}
}
