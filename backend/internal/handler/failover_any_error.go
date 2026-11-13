package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// anyErrorMaxSwitchesSafetyCap 是 FailoverOnAnyError 模式下的切换安全上限。
// 正常情况下循环会因"分组内所有账号都已在排除列表"而在选号阶段自然结束，
// 这个上限只防止异常情况（例如账号列表在请求中途被持续刷新）导致无限循环。
const anyErrorMaxSwitchesSafetyCap = 512

// anyErrorAntigravityMaxSwitchDelaySeconds 限制 Antigravity 线性递增换号延时的上限，
// 避免分组账号较多时累计等待过长。
const anyErrorAntigravityMaxSwitchDelaySeconds = 5

// anyErrorFailoverEnabled 判断本次请求是否启用"任意错误换号"：
// 全局总开关 + 请求所属分组开关（分组信息从 ctx 读取）。
func anyErrorFailoverEnabled(ctx context.Context, cfg *config.Config) bool {
	return service.FailoverOnAnyErrorEnabled(ctx, cfg)
}

// effectiveMaxAccountSwitches 返回本次请求的切换上限：
// 开启 FailoverOnAnyError 时放开到安全上限，让分组内所有账号都有机会被尝试。
func effectiveMaxAccountSwitches(ctx context.Context, cfg *config.Config, configured int) int {
	if anyErrorFailoverEnabled(ctx, cfg) {
		return anyErrorMaxSwitchesSafetyCap
	}
	return configured
}

// anyErrorSameAccountRetryLimit 返回 FailoverOnAnyError 模式下切换账号前的同账号重试次数。
// 确定性错误（鉴权/余额/权限/限流/模型不存在）重试同一账号没有意义，直接切换。
func anyErrorSameAccountRetryLimit(ctx context.Context, cfg *config.Config, failoverErr *service.UpstreamFailoverError) int {
	if failoverErr == nil || !anyErrorFailoverEnabled(ctx, cfg) {
		return 0
	}
	return anyErrorSameAccountRetryLimitEnabled(cfg, failoverErr)
}

// anyErrorSameAccountRetryLimitEnabled 在已确认开关生效时计算同账号重试次数。
func anyErrorSameAccountRetryLimitEnabled(cfg *config.Config, failoverErr *service.UpstreamFailoverError) int {
	if cfg == nil || failoverErr == nil || failoverErr.SafeToFailoverAfterWrite || failoverErr.SameAccountRetryBudgetExhausted || failoverErr.Reason == service.GatewayFailureReasonPoolStatus || isDeterministicAccountError(failoverErr) {
		return 0
	}
	n := cfg.Gateway.FailoverSameAccountRetries
	if n < 0 {
		return 0
	}
	if n > 5 {
		return 5
	}
	return n
}

// sameAccountRetryLimit 合并池模式重试与 FailoverOnAnyError 重试：
// 上游已标记 RetryableOnSameAccount 时沿用账号池模式配置，否则使用全局配置。
func sameAccountRetryLimit(ctx context.Context, cfg *config.Config, account *service.Account, failoverErr *service.UpstreamFailoverError) int {
	// Once output is committed, continue on a different account rather than
	// spending the pre-output retry budget on the same broken stream.
	if failoverErr != nil && (failoverErr.SafeToFailoverAfterWrite || failoverErr.SameAccountRetryBudgetExhausted) {
		return 0
	}
	if isPoolHTTPStatusFailure(account, failoverErr) {
		if failoverErr.RetryableOnSameAccount && account.IsPoolModeRetryableStatus(failoverErr.StatusCode) {
			return account.GetPoolModeRetryCount()
		}
		return 0
	}
	limit := anyErrorSameAccountRetryLimit(ctx, cfg, failoverErr)
	if failoverErr != nil && failoverErr.RetryableOnSameAccount && account != nil {
		limit = max(limit, account.GetPoolModeRetryCount())
	}
	return limit
}

// Legacy HTTP forwarders leave Reason empty for excluded pool statuses. Keep
// their status policy separate from any-error retries without suppressing
// explicitly classified stream/pre-output/auth failures.
func isPoolHTTPStatusFailure(account *service.Account, failure *service.UpstreamFailoverError) bool {
	return account != nil && account.IsPoolMode() && failure != nil && failure.StatusCode >= 400 &&
		(failure.Reason == service.GatewayFailureReasonPoolStatus || (failure.Reason == "" && failure.Stage == "" && len(failure.ResponseBody) > 0))
}

// shouldRespectOAuth429StormStop 在 FailoverOnAnyError 模式下关闭 429 风暴提前终止，
// 保证"所有账号都试过才返回错误"。
func shouldRespectOAuth429StormStop(ctx context.Context, cfg *config.Config) bool {
	return !anyErrorFailoverEnabled(ctx, cfg)
}

func isDeterministicAccountError(failoverErr *service.UpstreamFailoverError) bool {
	switch failoverErr.StatusCode {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden,
		http.StatusNotFound, http.StatusTooManyRequests:
		return true
	case http.StatusBadRequest:
		return isModelUnavailableErrorBody(failoverErr.ResponseBody)
	}
	return false
}

// isModelUnavailableErrorBody 识别"账号不支持/不存在该模型"类错误。
func isModelUnavailableErrorBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	msg := strings.ToLower(service.ExtractUpstreamErrorMessage(body))
	if msg == "" {
		msg = strings.ToLower(string(body))
	}
	for _, kw := range []string{
		"model not found", "model_not_found", "unknown model", "model does not exist",
		"does not exist", "unsupported model", "model is not supported", "not supported model",
		"invalid model", "no such model", "model unavailable", "模型不存在", "不支持该模型", "无可用渠道",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
