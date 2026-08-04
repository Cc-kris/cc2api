package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	OpsErrorCategoryClient      = "client"
	OpsErrorCategoryPlatform    = "platform"
	OpsErrorCategoryUpstream    = "upstream"
	OpsErrorCategoryAccountPool = "account_pool"
	OpsErrorCategoryRateLimit   = "rate_limit"
	OpsErrorCategoryPermission  = "permission"
	OpsErrorCategoryBalance     = "balance"
	OpsErrorCategoryConfig      = "config"
	OpsErrorCategorySlowRequest = "slow_request"
	OpsErrorCategoryUnknown     = "unknown"
)

var AllOpsErrorCategories = []string{
	OpsErrorCategoryClient,
	OpsErrorCategoryPlatform,
	OpsErrorCategoryUpstream,
	OpsErrorCategoryAccountPool,
	OpsErrorCategoryRateLimit,
	OpsErrorCategoryPermission,
	OpsErrorCategoryBalance,
	OpsErrorCategoryConfig,
	OpsErrorCategorySlowRequest,
	OpsErrorCategoryUnknown,
}

const (
	OpsClientErrorSubcategoryAuth                 = "client_auth_error"
	OpsClientErrorSubcategoryRateLimit            = "client_rate_limit_error"
	OpsClientErrorSubcategoryBalance              = "client_balance_error"
	OpsClientErrorSubcategoryGroup                = "client_group_error"
	OpsClientErrorSubcategorySubscription         = "client_subscription_error"
	OpsClientErrorSubcategoryParameter            = "client_parameter_error"
	OpsClientErrorSubcategoryModel                = "client_model_error"
	OpsClientErrorSubcategoryPath                 = "client_path_error"
	OpsClientErrorSubcategoryContext              = "client_context_error"
	OpsClientErrorSubcategoryDisconnect           = "client_disconnect_error"
	OpsClientErrorSubcategoryInsufficientEvidence = "client_insufficient_evidence"
)

var AllOpsClientErrorSubcategories = []string{
	OpsClientErrorSubcategoryAuth,
	OpsClientErrorSubcategoryRateLimit,
	OpsClientErrorSubcategoryBalance,
	OpsClientErrorSubcategoryGroup,
	OpsClientErrorSubcategorySubscription,
	OpsClientErrorSubcategoryParameter,
	OpsClientErrorSubcategoryModel,
	OpsClientErrorSubcategoryPath,
	OpsClientErrorSubcategoryContext,
	OpsClientErrorSubcategoryDisconnect,
	OpsClientErrorSubcategoryInsufficientEvidence,
}

const (
	OpsClassificationConfidenceHigh   = "high"
	OpsClassificationConfidenceMedium = "medium"
	OpsClassificationConfidenceLow    = "low"
)

const OpsClassificationReasonOpenAIImageWebSocketHTTPFallback = "生图ws降级http"

type OpsErrorClassificationInput struct {
	StatusCode           int
	UpstreamStatusCode   *int
	ErrorType            string
	ErrorPhase           string
	ErrorSource          string
	ErrorOwner           string
	ErrorMessage         string
	ErrorBody            string
	UpstreamErrorMessage string
	UpstreamErrorDetail  string
	UpstreamErrors       string
	RequestPath          string
	InboundEndpoint      string
	UpstreamEndpoint     string
	RequestedModel       string
	UpstreamModel        string
	Model                string
	IsBusinessLimited    bool
	AuthLatencyMs        *int64
	RoutingLatencyMs     *int64
	UpstreamLatencyMs    *int64
	ResponseLatencyMs    *int64
	TimeToFirstTokenMs   *int64
}

type OpsErrorClassification struct {
	ErrorCategory            string   `json:"error_category"`
	ErrorSubcategory         string   `json:"error_subcategory"`
	ClientErrorSubcategory   string   `json:"client_error_subcategory,omitempty"`
	ClassificationConfidence string   `json:"classification_confidence"`
	ClassificationReason     string   `json:"classification_reason"`
	MissingEvidence          []string `json:"missing_evidence,omitempty"`
}

func IsValidOpsErrorCategory(category string) bool {
	category = strings.TrimSpace(strings.ToLower(category))
	for _, item := range AllOpsErrorCategories {
		if category == item {
			return true
		}
	}
	return false
}

func IsValidOpsClientErrorSubcategory(subcategory string) bool {
	subcategory = strings.TrimSpace(strings.ToLower(subcategory))
	for _, item := range AllOpsClientErrorSubcategories {
		if subcategory == item {
			return true
		}
	}
	return false
}

func ClassifyOpsError(input OpsErrorClassificationInput) OpsErrorClassification {
	text := strings.ToLower(strings.Join([]string{
		input.ErrorType,
		input.ErrorPhase,
		input.ErrorSource,
		input.ErrorOwner,
		input.ErrorMessage,
		input.ErrorBody,
		input.UpstreamErrorMessage,
		input.UpstreamErrorDetail,
		input.UpstreamErrors,
		input.RequestPath,
		input.InboundEndpoint,
		input.UpstreamEndpoint,
		input.RequestedModel,
		input.UpstreamModel,
		input.Model,
	}, " "))

	status := input.StatusCode
	if input.UpstreamStatusCode != nil && *input.UpstreamStatusCode > 0 {
		status = *input.UpstreamStatusCode
	}
	hasUpstreamEvidence := input.UpstreamStatusCode != nil && *input.UpstreamStatusCode > 0 ||
		containsAny(text, "upstream_http", "provider", "upstream error", "upstream_error", "upstream_status", "upstream status") ||
		strings.EqualFold(strings.TrimSpace(input.ErrorOwner), "provider") ||
		strings.EqualFold(strings.TrimSpace(input.ErrorSource), "upstream_http") ||
		strings.EqualFold(strings.TrimSpace(input.ErrorPhase), "upstream")
	clientSide := isOpsClassificationClientSide(input, hasUpstreamEvidence)

	// A Codex image channel intentionally rejects the WebSocket upgrade with 426
	// so the client retries the same request over the HTTPS Responses endpoint.
	// This is an expected transport downgrade, not a platform failure.
	if isOpenAIImageWebSocketHTTPFallback(input, text) {
		return opsClassification(
			OpsErrorCategoryPlatform,
			"platform_internal_error",
			OpsClassificationReasonOpenAIImageWebSocketHTTPFallback,
			OpsClassificationConfidenceHigh,
		)
	}
	if isLocalImagePermissionError(input, text) {
		return clientClassification(OpsClientErrorSubcategoryGroup, "当前分组未开启生图权限", OpsClassificationConfidenceHigh)
	}

	if containsAny(text, "request body is incomplete", "incomplete_body") ||
		(containsAny(text, "unexpected eof") && strings.EqualFold(strings.TrimSpace(input.ErrorPhase), "request")) {
		return clientClassification(OpsClientErrorSubcategoryDisconnect, "请求体上传未完成，客户端连接提前中断", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "context canceled", "client canceled", "request canceled", "cancelled", "broken pipe", "connection reset", "client disconnected") {
		return clientClassification(OpsClientErrorSubcategoryDisconnect, "客户端连接中断或主动取消请求", OpsClassificationConfidenceHigh)
	}
	if isOpenAIClientDefaultModelRoutingFailure(input, hasUpstreamEvidence, text) {
		return clientClassification(OpsClientErrorSubcategoryModel, "客户端未传入有效模型，平台占位模型无法直接调度上游账号", OpsClassificationConfidenceHigh)
	}
	if isLocalAccountPoolSignal(input, text, hasUpstreamEvidence) {
		return opsClassification(OpsErrorCategoryAccountPool, "account_pool_empty", "账号池没有可用上游账号或账号调度失败", OpsClassificationConfidenceHigh)
	}

	if !hasUpstreamEvidence && containsAny(text, "this group does not allow /v1/messages dispatch") {
		return clientClassification(OpsClientErrorSubcategoryGroup, "当前分组未开启 /v1/messages 调度", OpsClassificationConfidenceHigh)
	}
	if !hasUpstreamEvidence && containsAny(text, "image generation is not enabled for this group") {
		return clientClassification(OpsClientErrorSubcategoryGroup, "当前分组未开启生图权限", OpsClassificationConfidenceHigh)
	}
	if !hasUpstreamEvidence && containsAny(text, "group_deleted", "group deleted", "分组已删除") {
		return clientClassification(OpsClientErrorSubcategoryGroup, "API Key 所属分组已删除", OpsClassificationConfidenceHigh)
	}
	if !hasUpstreamEvidence && containsAny(text, "group_disabled", "group disabled", "group inactive", "group unavailable", "group not available", "所属分组", "分组已停用", "分组不可用", "分组未启用", "分组已禁用") {
		return clientClassification(OpsClientErrorSubcategoryGroup, "API Key 绑定的分组已停用或不可用", OpsClassificationConfidenceHigh)
	}
	if !hasUpstreamEvidence && containsAny(text, "subscription_not_found", "subscription_invalid", "subscription expired", "no active subscription", "订阅不存在", "订阅无效", "订阅已过期") {
		return clientClassification(OpsClientErrorSubcategorySubscription, "订阅不存在、已过期或不满足当前分组要求", OpsClassificationConfidenceHigh)
	}
	if !hasUpstreamEvidence && strings.EqualFold(strings.TrimSpace(input.ErrorPhase), "routing") && strings.EqualFold(strings.TrimSpace(input.ErrorOwner), "platform") && strings.EqualFold(strings.TrimSpace(input.ErrorSource), "gateway") && containsAny(text, "service temporarily unavailable") {
		return opsClassification(OpsErrorCategoryAccountPool, "account_pool_empty", "平台路由没有可用上游账号或渠道", OpsClassificationConfidenceHigh)
	}

	if clientSide {
		return classifyClientSideOpsError(input, text)
	}

	if status == 429 || containsAny(text, "rate limit", "rate_limit", "too many requests", "rpm", "tpm", "concurrency", "限流", "频率限制") {
		return opsClassification(OpsErrorCategoryRateLimit, "upstream_rate_limit", "上游或平台维度触发限流", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "insufficient balance", "insufficient_balance", "balance", "quota", "credit", "usage limit", "subscription", "余额", "额度") {
		return opsClassification(OpsErrorCategoryBalance, "upstream_balance_error", "上游额度、订阅额度或余额不足", OpsClassificationConfidenceHigh)
	}
	if status == 401 || status == 403 || containsAny(text, "permission", "unauthorized", "forbidden", "access denied", "invalid api key", "invalid_api_key", "权限", "鉴权") {
		return opsClassification(OpsErrorCategoryPermission, "upstream_permission_error", "上游账号、模型或接口权限不足", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "model mapping", "no mapping", "mapped model", "channel config", "config", "cache config", "ai config", "配置", "映射") {
		return opsClassification(OpsErrorCategoryConfig, "config_model_mapping_error", "模型映射、渠道或系统配置错误", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "slow", "p99", "ttft", "time to first token", "latency", "耗时", "慢请求") || isSlowOpsError(input) {
		return opsClassification(OpsErrorCategorySlowRequest, "slow_response", "请求耗时或首 token 延迟异常", OpsClassificationConfidenceMedium)
	}
	platformOwned := strings.EqualFold(strings.TrimSpace(input.ErrorOwner), "platform")
	if hasUpstreamEvidence || (!platformOwned && status >= 500) || (!platformOwned && containsAny(text, "timeout", "overloaded", "unavailable", "bad gateway", "service unavailable", "gateway timeout")) {
		sub := "upstream_error"
		reason := "上游服务返回错误或不可用"
		if containsAny(text, "timeout", "deadline", "gateway timeout") || status == 504 {
			sub = "upstream_timeout"
			reason = "上游服务超时"
		} else if status == 502 || status == 503 || containsAny(text, "overloaded", "unavailable", "bad gateway", "service unavailable") {
			sub = "upstream_unavailable"
			reason = "上游服务不可用或过载"
		}
		return opsClassification(OpsErrorCategoryUpstream, sub, reason, OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "upstream stream ended without a terminal response event") {
		return opsClassification(OpsErrorCategoryPlatform, "platform_internal_error", "上游响应流未返回终止事件", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "panic", "internal", "database", "redis", "gateway", "platform", "平台") || strings.EqualFold(strings.TrimSpace(input.ErrorOwner), "platform") {
		sub := "platform_internal_error"
		if containsAny(text, "database", "redis", "dependency", "依赖") {
			sub = "platform_dependency_error"
		}
		return opsClassification(OpsErrorCategoryPlatform, sub, "Sub2API 平台内部处理或依赖服务异常", OpsClassificationConfidenceMedium)
	}

	return opsClassification(OpsErrorCategoryUnknown, "unknown_insufficient_evidence", "缺少足够证据，无法归入固定错误分类", OpsClassificationConfidenceLow)
}

func classifyClientSideOpsError(input OpsErrorClassificationInput, text string) OpsErrorClassification {
	status := input.StatusCode
	phase := strings.ToLower(strings.TrimSpace(input.ErrorPhase))

	if containsAny(text, "api_key_quota_exhausted", "api key 额度已用完", "quota exhausted", "配额耗尽", "user_platform_daily_quota_exhausted", "user_platform_weekly_quota_exhausted", "user_platform_monthly_quota_exhausted") {
		return clientClassification(OpsClientErrorSubcategoryBalance, clientBalanceReason(text), OpsClassificationConfidenceHigh)
	}
	if status == 429 || containsAny(text, "rate limit", "rate_limit", "too many requests", "user rate", "key rate", "group rate", "rpm", "tpm", "concurrency", "pending", "queue", "用户限流", "key 限流") {
		return clientClassification(OpsClientErrorSubcategoryRateLimit, clientRateLimitReason(text), OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "group_disabled", "group disabled", "group inactive", "group unavailable", "group not available", "所属分组", "分组已停用", "分组不可用", "分组未启用", "分组已禁用") {
		return clientClassification(OpsClientErrorSubcategoryGroup, "API Key 绑定的分组已停用或不可用", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "subscription_not_found", "subscription_invalid", "subscription expired", "no active subscription", "订阅不存在", "订阅无效", "订阅已过期") {
		return clientClassification(OpsClientErrorSubcategorySubscription, "订阅不存在、已过期或不满足当前分组要求", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "insufficient balance", "insufficient_balance", "insufficient quota", "quota exhausted", "api_key_quota_exhausted", "usage_limit_exceeded", "balance", "余额不足", "额度不足", "配额耗尽", "用量限制") {
		return clientClassification(OpsClientErrorSubcategoryBalance, clientBalanceReason(text), OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "context length", "context window", "maximum context", "max_tokens", "input tokens", "output tokens", "token limit", "上下文", "超限") {
		return clientClassification(OpsClientErrorSubcategoryContext, "输入上下文或输出上限超过模型配置", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "model not found", "model unavailable", "model does not exist", "unsupported model", "no mapping", "model mapping", "no available channel", "无可用渠道", "模型不存在", "模型不可用", "模型权限") {
		return clientClassification(OpsClientErrorSubcategoryModel, "请求模型不存在、无映射、无可用渠道或无模型权限", OpsClassificationConfidenceHigh)
	}
	if status == 404 || status == 405 || containsAny(text, "not found", "route not found", "method not allowed", "unsupported method", "路径不存在", "方法不支持") {
		return clientClassification(OpsClientErrorSubcategoryPath, "请求路径不存在或 HTTP 方法不支持", OpsClassificationConfidenceHigh)
	}
	if status == 400 || status == 422 || containsAny(text, "invalid request", "invalid_request", "validation", "missing required", "bad request", "json", "request body", "parameter", "param", "参数", "请求体") {
		return clientClassification(OpsClientErrorSubcategoryParameter, "请求参数校验失败或请求体格式错误", OpsClassificationConfidenceHigh)
	}
	if status == 401 || phase == "auth" || containsAny(text, "invalid api key", "invalid_api_key", "api_key_required", "api key required", "api_key_disabled", "api_key_expired", "key disabled", "key missing", "unauthorized", "forbidden", "access denied", "鉴权", "认证", "key 无效", "key 禁用") {
		return clientClassification(OpsClientErrorSubcategoryAuth, clientAuthReason(text), OpsClassificationConfidenceHigh)
	}

	return OpsErrorClassification{
		ErrorCategory:            OpsErrorCategoryClient,
		ErrorSubcategory:         OpsClientErrorSubcategoryInsufficientEvidence,
		ClientErrorSubcategory:   OpsClientErrorSubcategoryInsufficientEvidence,
		ClassificationConfidence: OpsClassificationConfidenceLow,
		ClassificationReason:     "客户端请求错误缺少可判断具体子类的必需字段",
		MissingEvidence:          []string{"status_code", "error_code", "request_path", "requested_model", "validation_error"},
	}
}

func clientRateLimitReason(text string) string {
	switch {
	case containsAny(text, "group requests-per-minute", "group requests per minute", "group rpm", "group_rpm_exceeded", "分组 rpm"):
		return "分组 RPM 限流"
	case containsAny(text, "api key requests-per-minute", "api key requests per minute", "api key rpm", "api_key_rpm_exceeded", "key_rpm_exceeded", "key rpm", "key rate", "api key 5小时限额已用完"):
		return "API Key 限流"
	case containsAny(text, "user requests-per-minute", "user requests per minute", "user_rpm_exceeded", "user requests per minute", "user rpm", "user rate"):
		return "用户 RPM 限流"
	case containsAny(text, "concurrency", "并发"):
		return "并发数超限"
	case containsAny(text, "pending", "queue"):
		return "请求排队数超限"
	default:
		return "用户、API Key 或分组触发限流"
	}
}

func clientBalanceReason(text string) string {
	switch {
	case containsAny(text, "insufficient account balance", "insufficient balance", "insufficient_balance", "余额不足"):
		return "用户余额不足"
	case containsAny(text, "api_key_quota_exhausted", "api key 额度已用完"):
		return "API Key 配额已用完"
	case containsAny(text, "usage_limit_exceeded", "insufficient quota", "额度不足", "用量限制"):
		return "用户或订阅额度已用完"
	case containsAny(text, "quota exhausted", "配额耗尽", "user_platform_daily_quota_exhausted", "user_platform_weekly_quota_exhausted", "user_platform_monthly_quota_exhausted"):
		return "用户或订阅额度已用完"
	default:
		return "客户端余额或配额不足"
	}
}

func isLocalAccountPoolSignal(input OpsErrorClassificationInput, text string, hasUpstreamEvidence bool) bool {
	if !containsAny(text, "no available accounts", "no available account", "no available compatible accounts", "account pool", "账号池", "账号不可用", "无可用账号", "account scheduler", "scheduling account") {
		return false
	}
	if !hasUpstreamEvidence {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(input.ErrorPhase), "routing") &&
		strings.EqualFold(strings.TrimSpace(input.ErrorOwner), "platform") &&
		strings.EqualFold(strings.TrimSpace(input.ErrorSource), "gateway")
}

func clientAuthReason(text string) string {
	switch {
	case containsAny(text, "api_key_disabled", "api key is disabled", "key disabled", "key 禁用"):
		return "API Key 已禁用"
	case containsAny(text, "api_key_expired", "api key 已过期"):
		return "API Key 已过期"
	case containsAny(text, "api_key_required", "api key required", "key missing"):
		return "请求未提供 API Key"
	case containsAny(text, "invalid api key", "invalid_api_key", "key 无效"):
		return "API Key 无效"
	case containsAny(text, "access denied"):
		return "API Key 访问被拒绝"
	default:
		return "客户端鉴权失败"
	}
}

// BuildOpsErrorSummary keeps the classified layer/reason visible while adding
// the first concrete sanitized detail available in the error record.
func BuildOpsErrorSummary(reason, message, upstreamMessage, upstreamDetail string) string {
	reason = strings.TrimSpace(reason)
	for _, candidate := range []string{upstreamDetail, upstreamMessage, message} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || isGenericOpsSummaryCandidate(candidate) || strings.EqualFold(candidate, reason) {
			continue
		}
		candidate = strings.TrimSpace(logredact.RedactResponseBody(candidate, 500))
		if candidate == "" {
			continue
		}
		if reason == "" {
			return truncateOpsSummary(candidate)
		}
		return truncateOpsSummary(reason + "：" + candidate)
	}
	if reason != "" {
		return truncateOpsSummary(reason)
	}
	return "暂无摘要"
}

func isGenericOpsSummaryCandidate(candidate string) bool {
	switch strings.ToLower(strings.TrimSpace(candidate)) {
	case "api_error", "upstream_error", "openai_error", "upstream request failed", "upstream service temporarily unavailable", "service temporarily unavailable":
		return true
	default:
		return false
	}
}

func truncateOpsSummary(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= 160 {
		return string(runes)
	}
	return string(runes[:160])
}

func ApplyOpsErrorClassificationToLog(log *OpsErrorLog) {
	if log == nil {
		return
	}
	classification := ClassifyOpsError(OpsErrorClassificationInput{
		StatusCode:       log.StatusCode,
		ErrorType:        log.Type,
		ErrorPhase:       log.Phase,
		ErrorSource:      log.Source,
		ErrorOwner:       log.Owner,
		ErrorMessage:     log.Message,
		RequestPath:      log.RequestPath,
		InboundEndpoint:  log.InboundEndpoint,
		UpstreamEndpoint: log.UpstreamEndpoint,
		RequestedModel:   log.RequestedModel,
		UpstreamModel:    log.UpstreamModel,
		Model:            log.Model,
	})
	log.SetClassification(classification)
}

func ApplyOpsErrorClassificationToDetail(detail *OpsErrorLogDetail) {
	if detail == nil {
		return
	}
	classification := ClassifyOpsError(OpsErrorClassificationInput{
		StatusCode:           detail.StatusCode,
		UpstreamStatusCode:   detail.UpstreamStatusCode,
		ErrorType:            detail.Type,
		ErrorPhase:           detail.Phase,
		ErrorSource:          detail.Source,
		ErrorOwner:           detail.Owner,
		ErrorMessage:         detail.Message,
		ErrorBody:            detail.ErrorBody,
		UpstreamErrorMessage: detail.UpstreamErrorMessage,
		UpstreamErrorDetail:  detail.UpstreamErrorDetail,
		UpstreamErrors:       detail.UpstreamErrors,
		RequestPath:          detail.RequestPath,
		InboundEndpoint:      detail.InboundEndpoint,
		UpstreamEndpoint:     detail.UpstreamEndpoint,
		RequestedModel:       detail.RequestedModel,
		UpstreamModel:        detail.UpstreamModel,
		Model:                detail.Model,
		IsBusinessLimited:    detail.IsBusinessLimited,
		AuthLatencyMs:        detail.AuthLatencyMs,
		RoutingLatencyMs:     detail.RoutingLatencyMs,
		UpstreamLatencyMs:    detail.UpstreamLatencyMs,
		ResponseLatencyMs:    detail.ResponseLatencyMs,
		TimeToFirstTokenMs:   detail.TimeToFirstTokenMs,
	})
	detail.SetClassification(classification)
}

func (l *OpsErrorLog) SetClassification(classification OpsErrorClassification) {
	if l == nil {
		return
	}
	l.ErrorCategory = classification.ErrorCategory
	l.ErrorSubcategory = classification.ErrorSubcategory
	if classification.ClientErrorSubcategory != "" {
		subcategory := classification.ClientErrorSubcategory
		l.ClientErrorSubcategory = &subcategory
	} else {
		l.ClientErrorSubcategory = nil
	}
	l.ClassificationConfidence = classification.ClassificationConfidence
	l.ClassificationReason = classification.ClassificationReason
	l.ClassificationMissingEvidence = append([]string(nil), classification.MissingEvidence...)
}

func opsClassification(category, subcategory, reason, confidence string) OpsErrorClassification {
	return OpsErrorClassification{
		ErrorCategory:            category,
		ErrorSubcategory:         subcategory,
		ClassificationConfidence: confidence,
		ClassificationReason:     reason,
	}
}

func clientClassification(subcategory, reason, confidence string) OpsErrorClassification {
	return OpsErrorClassification{
		ErrorCategory:            OpsErrorCategoryClient,
		ErrorSubcategory:         subcategory,
		ClientErrorSubcategory:   subcategory,
		ClassificationConfidence: confidence,
		ClassificationReason:     reason,
	}
}

func isOpsClassificationClientSide(input OpsErrorClassificationInput, hasUpstreamEvidence bool) bool {
	owner := strings.ToLower(strings.TrimSpace(input.ErrorOwner))
	source := strings.ToLower(strings.TrimSpace(input.ErrorSource))
	phase := strings.ToLower(strings.TrimSpace(input.ErrorPhase))
	if hasUpstreamEvidence && owner != "client" && source != "client_request" {
		return false
	}
	return owner == "client" || source == "client_request" || phase == "auth" || phase == "request"
}

func isSlowOpsError(input OpsErrorClassificationInput) bool {
	return int64GreaterOrEqual(input.TimeToFirstTokenMs, 30000) || int64GreaterOrEqual(input.ResponseLatencyMs, 120000) || int64GreaterOrEqual(input.UpstreamLatencyMs, 120000)
}

func isOpenAIClientDefaultModelRoutingFailure(input OpsErrorClassificationInput, hasUpstreamEvidence bool, text string) bool {
	if hasUpstreamEvidence {
		return false
	}
	phase := strings.ToLower(strings.TrimSpace(input.ErrorPhase))
	owner := strings.ToLower(strings.TrimSpace(input.ErrorOwner))
	source := strings.ToLower(strings.TrimSpace(input.ErrorSource))
	if phase != "routing" || owner != "platform" || source != "gateway" {
		return false
	}
	model := strings.ToLower(strings.TrimSpace(input.Model))
	requestedModel := strings.ToLower(strings.TrimSpace(input.RequestedModel))
	if model != "codex-current" && requestedModel != "codex-current" {
		return false
	}
	return containsAny(text,
		"service temporarily unavailable",
		"model is required",
		"missing required field model",
		"no available accounts",
		"no available account",
	)
}

func isOpenAIImageWebSocketHTTPFallback(input OpsErrorClassificationInput, text string) bool {
	statusCode := input.StatusCode
	if input.UpstreamStatusCode != nil && *input.UpstreamStatusCode > 0 {
		statusCode = *input.UpstreamStatusCode
	}
	if statusCode != 426 {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(input.ErrorMessage))
	if !strings.Contains(message, "codex image channels require https responses transport") {
		return false
	}
	return containsAny(text, "websocket_transport_unsupported") ||
		containsAny(strings.ToLower(input.RequestPath), "/openai/v1/responses")
}

func isLocalImagePermissionError(input OpsErrorClassificationInput, text string) bool {
	if !containsAny(text, "image generation is not enabled for this group") {
		return false
	}
	statusCode := input.StatusCode
	if input.UpstreamStatusCode != nil && *input.UpstreamStatusCode > 0 {
		statusCode = *input.UpstreamStatusCode
	}
	if statusCode != 403 || strings.TrimSpace(input.UpstreamErrorDetail) != "" {
		return false
	}
	upstreamMessage := strings.TrimSpace(input.UpstreamErrorMessage)
	return upstreamMessage == "" || strings.Contains(strings.ToLower(upstreamMessage), "image generation is not enabled for this group")
}

func int64GreaterOrEqual(v *int64, threshold int64) bool {
	return v != nil && *v >= threshold
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
