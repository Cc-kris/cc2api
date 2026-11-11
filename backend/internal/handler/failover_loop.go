package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TempUnscheduler 用于 HandleFailoverError 中同账号重试耗尽后的临时封禁。
// GatewayService 隐式实现此接口。
type TempUnscheduler interface {
	TempUnscheduleRetryableError(ctx context.Context, accountID int64, failoverErr *service.UpstreamFailoverError)
}

// FailoverAction 表示 failover 错误处理后的下一步动作
type FailoverAction int

const (
	// FailoverContinue 继续循环（同账号重试或切换账号，调用方统一 continue）
	FailoverContinue FailoverAction = iota
	// FailoverExhausted 切换次数耗尽（调用方应返回错误响应）
	FailoverExhausted
	// FailoverCanceled context 已取消（调用方应直接 return）
	FailoverCanceled
)

const (
	// maxSameAccountRetries 同账号重试次数上限（针对 RetryableOnSameAccount 错误）
	maxSameAccountRetries = 2
	// sameAccountRetryDelay 同账号重试间隔
	sameAccountRetryDelay = 500 * time.Millisecond
	// singleAccountBackoffDelay 单账号分组 503 退避重试固定延时。
	// Service 层在 SingleAccountRetry 模式下已做充分原地重试（最多 3 次、总等待 30s），
	// Handler 层只需短暂间隔后重新进入 Service 层即可。
	singleAccountBackoffDelay = 2 * time.Second
)

// FailoverState 跨循环迭代共享的 failover 状态
type FailoverState struct {
	SwitchCount           int
	MaxSwitches           int
	FailedAccountIDs      map[int64]struct{}
	SameAccountRetryCount map[int64]int
	LastFailoverErr       *service.UpstreamFailoverError
	ForceCacheBilling     bool
	hasBoundSession       bool

	// LastObservedResult 是最后一个带已观测用量的续写中断尝试（UpstreamFailoverError.ObservedUsage）。
	// 续写全部失败（换号耗尽）且已向客户端输出过内容时，按它向用户收费一次；
	// 续写成功时不使用（最终尝试正常返回 result，按成功路径记账）。
	LastObservedResult *service.ForwardResult
	// LastObservedAccountID 是 LastObservedResult 对应的账号。
	LastObservedAccountID int64
	// LastObservedForceCacheBilling 是该尝试发生时的强制缓存计费状态（本次失败触发的换号不影响它）。
	LastObservedForceCacheBilling bool

	// anyErrorCfg 非 nil 且开启 FailoverOnAnyError 时，任意错误都会依次尝试分组内所有账号。
	anyErrorCfg *config.Config
	// backoffMaxSwitches 单账号 503 退避重试的上限，保持原配置值，避免放开切换上限后退避循环过长。
	backoffMaxSwitches int
}

// NewFailoverState 创建 failover 状态
func NewFailoverState(maxSwitches int, hasBoundSession bool) *FailoverState {
	return &FailoverState{
		MaxSwitches:           maxSwitches,
		FailedAccountIDs:      make(map[int64]struct{}),
		SameAccountRetryCount: make(map[int64]int),
		hasBoundSession:       hasBoundSession,
		backoffMaxSwitches:    maxSwitches,
	}
}

// WithAnyErrorFailover 按配置开启"任意错误都切换账号直到分组内账号全部尝试"模式。
// cfg 为 nil 或开关关闭时不改变任何行为。
// 是否生效由全局总开关 + ctx 中请求所属分组的 failover_on_any_error 共同决定。
func (s *FailoverState) WithAnyErrorFailover(ctx context.Context, cfg *config.Config) *FailoverState {
	if s == nil || !service.FailoverOnAnyErrorEnabled(ctx, cfg) {
		return s
	}
	s.anyErrorCfg = cfg
	s.MaxSwitches = anyErrorMaxSwitchesSafetyCap
	return s
}

// HandleFailoverError 处理 UpstreamFailoverError，返回下一步动作。
// 包含：缓存计费判断、同账号重试、临时封禁、切换计数、Antigravity 延时。
func (s *FailoverState) HandleFailoverError(
	ctx context.Context,
	gatewayService TempUnscheduler,
	accountID int64,
	platform string,
	failoverErr *service.UpstreamFailoverError,
) FailoverAction {
	s.LastFailoverErr = failoverErr
	if observed := failoverErr.ObservedForwardResult(); observed != nil {
		s.LastObservedResult = observed
		s.LastObservedAccountID = accountID
		s.LastObservedForceCacheBilling = s.ForceCacheBilling
	}

	// 缓存计费判断：同账号重试不算切换账号，粘性会话仅在实际切换时强制缓存计费。
	// 两个同账号重试分支（RetryableOnSameAccount、任意错误换号）的判断与下方保持一致。
	if needForceCacheBilling(s.hasBoundSession, failoverErr, s.willRetrySameAccount(accountID, failoverErr)) {
		s.ForceCacheBilling = true
	}

	// 同账号重试：对 RetryableOnSameAccount 的临时性错误，先在同一账号上重试
	if failoverErr.RetryableOnSameAccount && s.SameAccountRetryCount[accountID] < maxSameAccountRetries {
		s.SameAccountRetryCount[accountID]++
		logger.FromContext(ctx).Warn("gateway.failover_same_account_retry",
			zap.Int64("account_id", accountID),
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.Int("same_account_retry_count", s.SameAccountRetryCount[accountID]),
			zap.Int("same_account_retry_max", maxSameAccountRetries),
		)
		if !sleepWithContext(ctx, sameAccountRetryDelay) {
			return FailoverCanceled
		}
		return FailoverContinue
	}

	// FailoverOnAnyError：非 RetryableOnSameAccount 的错误，切换前也先在同账号重试（确定性错误除外）
	if !failoverErr.RetryableOnSameAccount && s.anyErrorCfg != nil {
		limit := anyErrorSameAccountRetryLimitEnabled(s.anyErrorCfg, failoverErr)
		if s.SameAccountRetryCount[accountID] < limit {
			s.SameAccountRetryCount[accountID]++
			logger.FromContext(ctx).Warn("gateway.failover_any_error_same_account_retry",
				zap.Int64("account_id", accountID),
				zap.Int("upstream_status", failoverErr.StatusCode),
				zap.Int("same_account_retry_count", s.SameAccountRetryCount[accountID]),
				zap.Int("same_account_retry_max", limit),
			)
			if !sleepWithContext(ctx, sameAccountRetryDelay) {
				return FailoverCanceled
			}
			return FailoverContinue
		}
	}

	// 同账号重试用尽，执行临时封禁
	if failoverErr.RetryableOnSameAccount {
		gatewayService.TempUnscheduleRetryableError(ctx, accountID, failoverErr)
	}

	// 加入失败列表
	s.FailedAccountIDs[accountID] = struct{}{}

	// 检查是否耗尽
	if s.SwitchCount >= s.MaxSwitches {
		return FailoverExhausted
	}

	// 递增切换计数
	s.SwitchCount++
	logger.FromContext(ctx).Warn("gateway.failover_switch_account",
		zap.Int64("account_id", accountID),
		zap.Int("upstream_status", failoverErr.StatusCode),
		zap.Int("switch_count", s.SwitchCount),
		zap.Int("max_switches", s.MaxSwitches),
	)

	// Antigravity 平台换号线性递增延时
	if platform == service.PlatformAntigravity {
		delay := time.Duration(s.SwitchCount-1) * time.Second
		if s.anyErrorCfg != nil && delay > anyErrorAntigravityMaxSwitchDelaySeconds*time.Second {
			delay = anyErrorAntigravityMaxSwitchDelaySeconds * time.Second
		}
		if !sleepWithContext(ctx, delay) {
			return FailoverCanceled
		}
	}

	return FailoverContinue
}

// HandleSelectionExhausted 处理选号失败（所有候选账号都在排除列表中）时的退避重试决策。
// 针对 Antigravity 单账号分组的 503 (MODEL_CAPACITY_EXHAUSTED) 场景：
// 清除排除列表、等待退避后重新选号。
//
// 返回 FailoverContinue 时，调用方应设置 SingleAccountRetry context 并 continue。
// 返回 FailoverExhausted 时，调用方应返回错误响应。
// 返回 FailoverCanceled 时，调用方应直接 return。
func (s *FailoverState) HandleSelectionExhausted(ctx context.Context) FailoverAction {
	if s.LastFailoverErr != nil &&
		s.LastFailoverErr.StatusCode == http.StatusServiceUnavailable &&
		s.SwitchCount <= s.backoffMaxSwitches {

		logger.FromContext(ctx).Warn("gateway.failover_single_account_backoff",
			zap.Duration("backoff_delay", singleAccountBackoffDelay),
			zap.Int("switch_count", s.SwitchCount),
			zap.Int("max_switches", s.MaxSwitches),
		)
		if !sleepWithContext(ctx, singleAccountBackoffDelay) {
			return FailoverCanceled
		}
		logger.FromContext(ctx).Warn("gateway.failover_single_account_retry",
			zap.Int("switch_count", s.SwitchCount),
			zap.Int("max_switches", s.MaxSwitches),
		)
		s.FailedAccountIDs = make(map[int64]struct{})
		return FailoverContinue
	}
	return FailoverExhausted
}

// willRetrySameAccount 判断本次 HandleFailoverError 是否会走同账号重试分支
// （不修改计数）。条件必须与 HandleFailoverError 中两个同账号重试分支一致：
//   - 上游标记 RetryableOnSameAccount 且未达 maxSameAccountRetries；
//   - 任意错误换号开启、非 RetryableOnSameAccount、未达 anyErrorSameAccountRetryLimitEnabled。
func (s *FailoverState) willRetrySameAccount(accountID int64, failoverErr *service.UpstreamFailoverError) bool {
	if failoverErr == nil {
		return false
	}
	retryCount := s.SameAccountRetryCount[accountID]
	if failoverErr.RetryableOnSameAccount {
		return retryCount < maxSameAccountRetries
	}
	if s.anyErrorCfg != nil {
		return retryCount < anyErrorSameAccountRetryLimitEnabled(s.anyErrorCfg, failoverErr)
	}
	return false
}

// needForceCacheBilling 判断 failover 时是否需要强制缓存计费。
// 粘性会话实际切换账号、或上游明确标记时，将 input_tokens 转为 cache_read 计费。
// 同账号重试（sameAccountRetry=true）不触发粘性会话的强制缓存计费。
func needForceCacheBilling(hasBoundSession bool, failoverErr *service.UpstreamFailoverError, sameAccountRetry bool) bool {
	return (hasBoundSession && !sameAccountRetry) || (failoverErr != nil && failoverErr.ForceCacheBilling)
}

// failoverClientGone stops account switching once the downstream request is gone.
func failoverClientGone(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.Context().Err() != nil
}

// sleepWithContext 等待指定时长，返回 false 表示 context 已取消。
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
