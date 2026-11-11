package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// claude-sonnet-4-6：输入 3e-6、缓存读 3e-7、输出 1.5e-5。
// 强制缓存计费把 InputTokens 挪到 CacheReadInputTokens（gateway_service.go RecordUsage）。
func sonnetCostForTest(input, output int, forceCacheBilling bool) float64 {
	cacheRead := 0
	if forceCacheBilling {
		cacheRead, input = input, 0
	}
	return float64(input)*3e-6 + float64(cacheRead)*3e-7 + float64(output)*1.5e-5
}

// V03-AC04：粘性会话绑定账号 A，A 返回 RetryableOnSameAccount 后同账号重试成功，不触发强制缓存计费。
func TestHandleFailoverError_SameAccountRetryNoForceCacheBilling(t *testing.T) {
	mock := &mockTempUnscheduler{}
	fs := NewFailoverState(3, true)

	action := fs.HandleFailoverError(ctxWithGroupFailover(false), mock, 100, "anthropic",
		&service.UpstreamFailoverError{StatusCode: 529, RetryableOnSameAccount: true})

	require.Equal(t, FailoverContinue, action)
	require.Equal(t, 1, fs.SameAccountRetryCount[100])
	require.Zero(t, fs.SwitchCount)
	require.False(t, fs.ForceCacheBilling)
	require.InDelta(t, 0.006, sonnetCostForTest(1000, 200, fs.ForceCacheBilling), 1e-12)
}

// V03-AC05：分组开启任意错误换号，failover_same_account_retries=2；A 返回普通 500，
// 同账号重试一次后成功，不触发强制缓存计费。
func TestHandleFailoverError_AnyErrorSameAccountRetryNoForceCacheBilling(t *testing.T) {
	mock := &mockTempUnscheduler{}
	ctx := ctxWithGroupFailover(true)
	fs := NewFailoverState(3, true).WithAnyErrorFailover(ctx, anyErrorCfg(2))

	action := fs.HandleFailoverError(ctx, mock, 100, "anthropic", &service.UpstreamFailoverError{StatusCode: 500})

	require.Equal(t, FailoverContinue, action)
	require.Equal(t, 1, fs.SameAccountRetryCount[100])
	require.Zero(t, fs.SwitchCount)
	require.NotContains(t, fs.FailedAccountIDs, int64(100))
	require.False(t, fs.ForceCacheBilling)
	require.InDelta(t, 0.006, sonnetCostForTest(1000, 200, fs.ForceCacheBilling), 1e-12)
}

// 任意错误换号：两次同账号重试期间都不设置，第三次真正换号时才设置。
func TestHandleFailoverError_AnyErrorRetriesExhaustedSwitchSetsForceCacheBilling(t *testing.T) {
	mock := &mockTempUnscheduler{}
	ctx := ctxWithGroupFailover(true)
	fs := NewFailoverState(3, true).WithAnyErrorFailover(ctx, anyErrorCfg(2))
	errA := &service.UpstreamFailoverError{StatusCode: 500}

	for i := 0; i < 2; i++ {
		require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 100, "anthropic", errA))
		require.False(t, fs.ForceCacheBilling, "第 %d 次同账号重试不应强制缓存计费", i+1)
		require.Zero(t, fs.SwitchCount)
	}

	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 100, "anthropic", errA))
	require.Equal(t, 1, fs.SwitchCount)
	require.Contains(t, fs.FailedAccountIDs, int64(100))
	require.True(t, fs.ForceCacheBilling)
	// V03-AC06：换到 B 成功后按缓存读计费。
	require.InDelta(t, 0.0033, sonnetCostForTest(1000, 200, fs.ForceCacheBilling), 1e-12)
}

// V03-AC06：RetryableOnSameAccount 重试用尽后切到 B，强制缓存计费仍生效。
func TestHandleFailoverError_SameAccountRetriesExhaustedSwitchForcesCacheBilling(t *testing.T) {
	mock := &mockTempUnscheduler{}
	fs := NewFailoverState(3, true)
	errA := &service.UpstreamFailoverError{StatusCode: 529, RetryableOnSameAccount: true}

	for i := 0; i < maxSameAccountRetries; i++ {
		require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctxWithGroupFailover(false), mock, 100, "anthropic", errA))
		require.False(t, fs.ForceCacheBilling)
	}
	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctxWithGroupFailover(false), mock, 100, "anthropic", errA))
	require.Equal(t, 1, fs.SwitchCount)
	require.Len(t, mock.calls, 1)
	require.True(t, fs.ForceCacheBilling)
	require.InDelta(t, 0.0033, sonnetCostForTest(1000, 200, fs.ForceCacheBilling), 1e-12)
}

// 任意错误换号下的确定性错误（401/403/429 等）不重试同账号，直接换号：粘性会话照常强制缓存计费。
func TestHandleFailoverError_AnyErrorDeterministicErrorSwitchForcesCacheBilling(t *testing.T) {
	mock := &mockTempUnscheduler{}
	ctx := ctxWithGroupFailover(true)
	fs := NewFailoverState(3, true).WithAnyErrorFailover(ctx, anyErrorCfg(2))

	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 100, "anthropic", &service.UpstreamFailoverError{StatusCode: 401}))
	require.Zero(t, fs.SameAccountRetryCount[100])
	require.Equal(t, 1, fs.SwitchCount)
	require.True(t, fs.ForceCacheBilling)
}

// 任意错误换号同账号重试时，上游显式 ForceCacheBilling 仍生效。
func TestHandleFailoverError_AnyErrorSameAccountRetryKeepsExplicitForceCacheBilling(t *testing.T) {
	mock := &mockTempUnscheduler{}
	ctx := ctxWithGroupFailover(true)
	fs := NewFailoverState(3, false).WithAnyErrorFailover(ctx, anyErrorCfg(2))

	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 100, "anthropic",
		&service.UpstreamFailoverError{StatusCode: 500, ForceCacheBilling: true}))
	require.Zero(t, fs.SwitchCount)
	require.True(t, fs.ForceCacheBilling)
}

// 任意错误换号关闭（分组开关关）时，普通 500 直接换号，粘性会话强制缓存计费（行为不变）。
func TestHandleFailoverError_AnyErrorDisabledSwitchForcesCacheBilling(t *testing.T) {
	mock := &mockTempUnscheduler{}
	ctx := ctxWithGroupFailover(false)
	fs := NewFailoverState(3, true).WithAnyErrorFailover(ctx, anyErrorCfg(2))

	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 100, "anthropic", &service.UpstreamFailoverError{StatusCode: 500}))
	require.Equal(t, 1, fs.SwitchCount)
	require.True(t, fs.ForceCacheBilling)
}
