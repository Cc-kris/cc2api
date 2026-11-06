package service

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// CNProviderRateLimitWindow captures provider rate-limit headers without
// exposing credentials or response bodies.
type CNProviderRateLimitWindow struct {
	Limit     *int64     `json:"limit,omitempty"`
	Remaining *int64     `json:"remaining,omitempty"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

// ParseCNProviderRateLimitHeaders accepts common OpenAI-compatible spellings.
func ParseCNProviderRateLimitHeaders(headers http.Header) map[string]CNProviderRateLimitWindow {
	result := make(map[string]CNProviderRateLimitWindow)
	for _, dimension := range []string{"requests", "tokens"} {
		window := CNProviderRateLimitWindow{
			Limit:     parseHeaderInt64(headers, "x-ratelimit-limit-"+dimension, "x-rate-limit-limit-"+dimension),
			Remaining: parseHeaderInt64(headers, "x-ratelimit-remaining-"+dimension, "x-rate-limit-remaining-"+dimension),
			ResetAt:   parseHeaderTime(headers, "x-ratelimit-reset-"+dimension, "x-rate-limit-reset-"+dimension),
		}
		if window.Limit != nil || window.Remaining != nil || window.ResetAt != nil {
			result[dimension] = window
		}
	}
	return result
}

func parseHeaderInt64(headers http.Header, keys ...string) *int64 {
	for _, key := range keys {
		value := strings.TrimSpace(headers.Get(key))
		if value == "" {
			continue
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err == nil && n >= 0 {
			return &n
		}
	}
	return nil
}

func parseHeaderTime(headers http.Header, keys ...string) *time.Time {
	for _, key := range keys {
		value := strings.TrimSpace(headers.Get(key))
		if value == "" {
			continue
		}
		if duration, err := time.ParseDuration(value); err == nil {
			t := time.Now().Add(duration)
			return &t
		}
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
			// Providers use both Unix seconds and a remaining-seconds delta.
			// Current Unix timestamps are large; small integers are treated as
			// durations so a header such as "60" cannot be mistaken for 1970.
			var t time.Time
			if seconds < time.Now().Unix()/2 {
				t = time.Now().Add(time.Duration(seconds) * time.Second)
			} else {
				t = time.Unix(seconds, 0)
			}
			return &t
		}
		if t, err := time.Parse(time.RFC3339, value); err == nil {
			return &t
		}
	}
	return nil
}

// 国产供应商（kimi/zhipu/deepseek）的响应式冷却辅助（V02-R10，移植自上游
// ratelimit_cn_providers.go，去掉 OpenCode/Command Code/MiniMax 分支与配额快照）。
//
// 与 openai/anthropic 不同：
//   - 余额不足是「可恢复」状态（充值后即可继续用），不能走 handleAuthError
//     永久置 status=error。这里改为 SetTempUnschedulable 固定时长，到期自动恢复。
//   - 配额窗口耗尽的 403 是窗口到期后自动恢复的限流信号，按默认时长临时冷却。
//
// CCAI 没有上游的 Coding Plan（credentials.account_mode）概念，配额 403 不按套餐
// 区分，一律默认冷却；套餐区分随 V09 引入 account_mode 后恢复。

// cnBalanceLowReasonPrefix 是余额不足临时停调 reason 的稳定前缀。
const cnBalanceLowReasonPrefix = "cn_balance_low"

// cnBalanceLowCooldown 是余额不足临时停调时长。上游是 2× 余额检测周期（默认
// 周期 10 分钟），由周期任务在余额恢复后提前清除；CCAI 没有该周期任务，
// 写死上游默认值，到期自动恢复调度。
const cnBalanceLowCooldown = 20 * time.Minute

const kimiConcurrentRequestLimitMessage = "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."

const cnConcurrencyLimitReasonPrefix = "cn_concurrency_limit"

func isCNProviderConcurrencyLimit403(account *Account, upstreamMsg string) bool {
	return account != nil && account.Platform == PlatformKimi &&
		strings.TrimSpace(upstreamMsg) == kimiConcurrentRequestLimitMessage
}

// cnQuotaExhausted403ErrorType 是 Kimi 配额窗口耗尽的 403 错误类型。
// 与并发限制 403（见 isCNProviderConcurrencyLimit403）不同，这是 5h/weekly
// 窗口耗尽的限流信号：窗口到期后自动恢复，不能落入通用 403 升级计数
// （连续 3 次永久 SetError）。
const cnQuotaExhausted403ErrorType = "access_terminated_error"

// cnQuotaExhaustedReasonPrefix 是配额耗尽 403 临时停调 reason 的稳定前缀。
const cnQuotaExhaustedReasonPrefix = "cn_quota_exhausted"

// isCNProviderQuotaExhausted403 识别国产供应商配额窗口耗尽的 403。
// 先做廉价的文案子串匹配（usage limit / quota will reset），未命中再解析
// 响应体匹配结构化错误类型（Kimi 的 access_terminated_error），以兼容同族
// 供应商的文案变体。与并发限制文案互斥（调用方先判并发）。
//
// 上游只对 Coding Plan 账号生效；CCAI 没有 account_mode，不区分套餐。代价是
// 非 Coding Plan 账号遇到同类硬上限时会按默认时长反复冷却而不是被禁用。
func isCNProviderQuotaExhausted403(account *Account, responseBody []byte, upstreamMsg string) bool {
	if account == nil || !account.IsCNProvider() {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(upstreamMsg))
	if strings.Contains(msg, "usage limit") || strings.Contains(msg, "quota will reset") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(gjson.GetBytes(responseBody, "error.type").String()), cnQuotaExhausted403ErrorType)
}

// handleCNProviderQuotaExhausted403 把配额窗口耗尽的 403 按默认 403 冷却时长
// 临时停调，不写账号 error 状态。上游有配额快照时会冷却到窗口重置点，CCAI
// 不移植快照，统一走默认时长。
func (s *RateLimitService) handleCNProviderQuotaExhausted403(
	ctx context.Context,
	account *Account,
	upstreamMsg string,
) {
	// upstreamMsg 由调用方（HandleUpstreamError）从同一响应体提取，此处直接复用；
	// 为空时仅存前缀，保证 reason 稳定可检索。
	reason := cnQuotaExhaustedReasonPrefix
	if msg := strings.TrimSpace(upstreamMsg); msg != "" {
		reason += ": " + msg
	}
	until := time.Now().Add(time.Duration(openAI403CooldownMinutesDefault) * time.Minute)
	s.setCNProviderTempUnschedulable(ctx, account, until, cnQuotaExhaustedReasonPrefix, reason, "cn_quota_exhausted_temp_unschedulable")
}

func (s *RateLimitService) handleCNProviderConcurrencyLimit403(
	ctx context.Context,
	account *Account,
) {
	s.apply429FallbackRateLimit(ctx, account, cnConcurrencyLimitReasonPrefix)
}

// cnBalanceLowReason 构造余额不足临时停调的 reason（带稳定前缀）。
func cnBalanceLowReason(upstreamMsg string) string {
	if upstreamMsg = strings.TrimSpace(upstreamMsg); upstreamMsg != "" {
		return cnBalanceLowReasonPrefix + ": " + upstreamMsg
	}
	return cnBalanceLowReasonPrefix + ": 余额不足，账号临时停调"
}

// cnProviderResponseIndicatesInsufficientBalance 通过响应体文案识别余额不足
// （智谱 payg 无独立余额端点，仅能靠响应文案识别）。
func cnProviderResponseIndicatesInsufficientBalance(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	s := strings.ToLower(string(body))
	return strings.Contains(s, "余额不足") ||
		strings.Contains(s, "insufficient balance") ||
		strings.Contains(s, "insufficient_credit") ||
		strings.Contains(s, "balance is not enough") ||
		strings.Contains(s, "no enough balance")
}

// handleCNProviderInsufficientBalance 把余额不足标记为可恢复的临时停调
// （cnBalanceLowCooldown），到期自动恢复调度，不写账号 error 状态。
// 上游还会写 balance_low 快照供周期余额检测任务清除；CCAI 没有该任务，不写。
func (s *RateLimitService) handleCNProviderInsufficientBalance(
	ctx context.Context,
	account *Account,
	upstreamMsg string,
) {
	until := time.Now().Add(cnBalanceLowCooldown)
	s.setCNProviderTempUnschedulable(ctx, account, until, "cn_insufficient_balance", cnBalanceLowReason(upstreamMsg), "cn_provider_insufficient_balance")
}

func (s *RateLimitService) setCNProviderTempUnschedulable(ctx context.Context, account *Account, until time.Time, notifyReason, storeReason, logEvent string) {
	s.notifyAccountSchedulingBlocked(account, until, notifyReason)
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, storeReason); err != nil {
		slog.Warn(notifyReason+"_set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
		return
	}
	slog.Info(logEvent,
		"account_id", account.ID,
		"platform", account.Platform,
		"until", until.UTC(),
	)
}

// applyCNProviderReactive429 处理国产供应商的 429 响应。
// 返回 true 表示已处理（调用方应 return），false 表示未命中、继续走默认 429 逻辑。
// 只处理余额不足；频率型 429 和窗口耗尽 429 继续走默认逻辑（没有重置头时为
// apply429FallbackRateLimit 秒级冷却）。上游 de908ed6d 的频率判定修的是上游
// 「冷却到 5h 窗口」的问题，CCAI 没有该逻辑，不移植。
func (s *RateLimitService) applyCNProviderReactive429(
	ctx context.Context,
	account *Account,
	responseBody []byte,
) bool {
	if !account.IsCNProvider() {
		return false
	}
	if cnProviderResponseIndicatesInsufficientBalance(responseBody) {
		s.handleCNProviderInsufficientBalance(ctx, account, extractUpstreamErrorMessage(responseBody))
		return true
	}
	return false
}
