package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func anyErrorCfg(retries int) *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.FailoverOnAnyError = true
	cfg.Gateway.FailoverSameAccountRetries = retries
	return cfg
}

func ctxWithGroupFailover(enabled bool) context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, &service.Group{ID: 1, FailoverOnAnyError: enabled})
}

func TestEffectiveMaxAccountSwitches(t *testing.T) {
	bg := context.Background()
	require.Equal(t, 3, effectiveMaxAccountSwitches(bg, nil, 3))
	require.Equal(t, 3, effectiveMaxAccountSwitches(bg, &config.Config{}, 3))
	require.Equal(t, anyErrorMaxSwitchesSafetyCap, effectiveMaxAccountSwitches(bg, anyErrorCfg(1), 3))
}

func TestAnyErrorFailover_GroupSwitch(t *testing.T) {
	cfg := anyErrorCfg(1)
	// 分组开启 + 全局开启 → 生效
	require.True(t, anyErrorFailoverEnabled(ctxWithGroupFailover(true), cfg))
	// 分组关闭 → 不生效（即使全局开启）
	require.False(t, anyErrorFailoverEnabled(ctxWithGroupFailover(false), cfg))
	require.Equal(t, 3, effectiveMaxAccountSwitches(ctxWithGroupFailover(false), cfg, 3))
	// 全局总开关关闭 → 分组开启也不生效（紧急回滚）
	require.False(t, anyErrorFailoverEnabled(ctxWithGroupFailover(true), &config.Config{}))
	// ctx 无分组 → 按全局
	require.True(t, anyErrorFailoverEnabled(context.Background(), cfg))
}

func TestAnyErrorSameAccountRetryLimit(t *testing.T) {
	bg := context.Background()
	cfg := anyErrorCfg(1)
	require.Equal(t, 1, anyErrorSameAccountRetryLimit(bg, cfg, &service.UpstreamFailoverError{StatusCode: 500}))
	require.Equal(t, 1, anyErrorSameAccountRetryLimit(bg, cfg, &service.UpstreamFailoverError{StatusCode: 502}))
	// 确定性错误直接切换
	for _, code := range []int{401, 402, 403, 404, 429} {
		require.Equal(t, 0, anyErrorSameAccountRetryLimit(bg, cfg, &service.UpstreamFailoverError{StatusCode: code}), code)
	}
	// 400 model not found 直接切换，其他 400 先重试
	require.Equal(t, 0, anyErrorSameAccountRetryLimit(bg, cfg, &service.UpstreamFailoverError{
		StatusCode: 400, ResponseBody: []byte(`{"error":{"message":"The model gpt-01 does not exist"}}`),
	}))
	require.Equal(t, 1, anyErrorSameAccountRetryLimit(bg, cfg, &service.UpstreamFailoverError{
		StatusCode: 400, ResponseBody: []byte(`{"error":{"message":"bad things"}}`),
	}))
	// 关闭时为 0；上限截断
	require.Equal(t, 0, anyErrorSameAccountRetryLimit(bg, &config.Config{}, &service.UpstreamFailoverError{StatusCode: 500}))
	require.Equal(t, 0, anyErrorSameAccountRetryLimit(ctxWithGroupFailover(false), cfg, &service.UpstreamFailoverError{StatusCode: 500}))
	require.Equal(t, 5, anyErrorSameAccountRetryLimit(bg, anyErrorCfg(99), &service.UpstreamFailoverError{StatusCode: 500}))
	require.Equal(t, 0, anyErrorSameAccountRetryLimit(bg, anyErrorCfg(-1), &service.UpstreamFailoverError{StatusCode: 500}))
}

func TestShouldRespectOAuth429StormStop(t *testing.T) {
	bg := context.Background()
	require.True(t, shouldRespectOAuth429StormStop(bg, nil))
	require.False(t, shouldRespectOAuth429StormStop(bg, anyErrorCfg(1)))
	require.True(t, shouldRespectOAuth429StormStop(ctxWithGroupFailover(false), anyErrorCfg(1)))
}

// 场景：A 返回 500（同账号重试 1 次后切换），B 返回 404 model not found（直接切换），
// 所有账号都被排除后才耗尽。
func TestFailoverState_AnyError_RetryThenSwitchAllAccounts(t *testing.T) {
	mock := &mockTempUnscheduler{}
	ctx := ctxWithGroupFailover(true)
	fs := NewFailoverState(1, false).WithAnyErrorFailover(ctx, anyErrorCfg(1))
	require.Equal(t, anyErrorMaxSwitchesSafetyCap, fs.MaxSwitches)

	errA := &service.UpstreamFailoverError{StatusCode: 500}
	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 1, "openai", errA))
	require.NotContains(t, fs.FailedAccountIDs, int64(1))
	require.Equal(t, 0, fs.SwitchCount)
	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 1, "openai", errA))
	require.Contains(t, fs.FailedAccountIDs, int64(1))
	require.Equal(t, 1, fs.SwitchCount)

	errB := &service.UpstreamFailoverError{StatusCode: 404, ResponseBody: []byte(`{"error":{"message":"model not found"}}`)}
	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 2, "openai", errB))
	require.Contains(t, fs.FailedAccountIDs, int64(2))
	require.Equal(t, 2, fs.SwitchCount)

	errC := &service.UpstreamFailoverError{StatusCode: 403}
	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 3, "openai", errC))
	require.Len(t, fs.FailedAccountIDs, 3)
}

func TestFailoverState_AnyErrorDisabled_KeepsLegacyBehavior(t *testing.T) {
	mock := &mockTempUnscheduler{}
	for name, tc := range map[string]struct {
		ctx context.Context
		cfg *config.Config
	}{
		"global_off": {context.Background(), &config.Config{}},
		"group_off":  {ctxWithGroupFailover(false), anyErrorCfg(1)},
	} {
		t.Run(name, func(t *testing.T) {
			fs := NewFailoverState(1, false).WithAnyErrorFailover(tc.ctx, tc.cfg)
			require.Equal(t, 1, fs.MaxSwitches)
			err := &service.UpstreamFailoverError{StatusCode: 500}
			require.Equal(t, FailoverContinue, fs.HandleFailoverError(tc.ctx, mock, 1, "openai", err))
			require.Equal(t, 1, fs.SwitchCount, "关闭时不做额外同账号重试")
			require.Equal(t, FailoverExhausted, fs.HandleFailoverError(tc.ctx, mock, 2, "openai", err))
		})
	}
}

// 单账号 503 退避循环保持原上限，避免开启模式后无限退避。
func TestFailoverState_AnyError_SelectionBackoffKeepsOriginalCap(t *testing.T) {
	fs := NewFailoverState(2, false).WithAnyErrorFailover(context.Background(), anyErrorCfg(0))
	fs.LastFailoverErr = &service.UpstreamFailoverError{StatusCode: 503}
	fs.SwitchCount = 3
	require.Equal(t, FailoverExhausted, fs.HandleSelectionExhausted(context.Background()))
}
