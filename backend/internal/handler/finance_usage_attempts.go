package handler

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const financeUsageAttemptsContextKey = "finance_usage_upstream_attempts"

func appendBillableUsageAttemptFromFailover(c *gin.Context, account *service.Account, upstreamModel string, failover *service.UpstreamFailoverError) {
	if c == nil || account == nil || failover == nil {
		return
	}
	attempts := usageUpstreamAttemptsSnapshot(c)
	attempt, ok := service.BuildBillableUsageAttemptFromResponse(
		failover.ResponseBody,
		"",
		len(attempts)+1,
		account.ID,
		nil,
		strings.TrimSpace(upstreamModel),
		nil,
		service.CloneDecimalSnapshot(account.UpstreamCostMultiplier),
		time.Now(),
	)
	if !ok {
		// 流式续写中断：ResponseBody 是本地合成的错误 JSON，不带 usage。
		// 回退到中断时的已观测用量，只记财务成本证据（不扣用户钱）。
		observedModel := strings.TrimSpace(failover.ObservedUpstreamModel)
		if observedModel == "" {
			observedModel = strings.TrimSpace(upstreamModel)
		}
		attempt, ok = service.BuildBillableUsageAttemptFromUsage(
			failover.ObservedUsage,
			"",
			len(attempts)+1,
			account.ID,
			nil,
			observedModel,
			nil,
			service.CloneDecimalSnapshot(account.UpstreamCostMultiplier),
			time.Now(),
		)
	}
	if !ok {
		return
	}
	service.ApplyAccountFinanceEvidenceToAttempt(&attempt, account)
	_ = service.ApplyFinanceRequestChargeToAttempt(&attempt, failover.ResponseBody, account)
	attempts = append(attempts, attempt)
	c.Set(financeUsageAttemptsContextKey, attempts)
}

func usageUpstreamAttemptsSnapshot(c *gin.Context) []service.UsageUpstreamAttempt {
	if c == nil {
		return nil
	}
	value, ok := c.Get(financeUsageAttemptsContextKey)
	if !ok {
		return nil
	}
	attempts, ok := value.([]service.UsageUpstreamAttempt)
	if !ok {
		return nil
	}
	return service.CloneUsageUpstreamAttempts(attempts)
}
