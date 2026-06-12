package service

import "strings"

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

	if containsAny(text, "context canceled", "client canceled", "request canceled", "cancelled", "broken pipe", "connection reset", "client disconnected") {
		return clientClassification(OpsClientErrorSubcategoryDisconnect, "客户端连接中断或主动取消请求", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "no available accounts", "no available account", "account pool", "账号池", "账号不可用", "无可用账号", "account scheduler", "scheduling account") {
		return opsClassification(OpsErrorCategoryAccountPool, "account_pool_empty", "账号池没有可用上游账号或账号调度失败", OpsClassificationConfidenceHigh)
	}

	if !hasUpstreamEvidence && containsAny(text, "group_disabled", "group disabled", "group inactive", "group unavailable", "group not available", "所属分组", "分组已停用", "分组不可用", "分组未启用", "分组已禁用") {
		return clientClassification(OpsClientErrorSubcategoryGroup, "API Key 绑定的分组已停用或不可用", OpsClassificationConfidenceHigh)
	}
	if !hasUpstreamEvidence && containsAny(text, "subscription_not_found", "subscription_invalid", "subscription expired", "no active subscription", "订阅不存在", "订阅无效", "订阅已过期") {
		return clientClassification(OpsClientErrorSubcategorySubscription, "订阅不存在、已过期或不满足当前分组要求", OpsClassificationConfidenceHigh)
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
	if hasUpstreamEvidence || status >= 500 || (!platformOwned && containsAny(text, "timeout", "overloaded", "unavailable", "bad gateway", "service unavailable", "gateway timeout")) {
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

	if status == 429 || containsAny(text, "rate limit", "rate_limit", "too many requests", "user rate", "key rate", "group rate", "rpm", "tpm", "concurrency", "pending", "queue", "用户限流", "key 限流") {
		return clientClassification(OpsClientErrorSubcategoryRateLimit, "用户、Key 或分组维度触发限流", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "group_disabled", "group disabled", "group inactive", "group unavailable", "group not available", "所属分组", "分组已停用", "分组不可用", "分组未启用", "分组已禁用") {
		return clientClassification(OpsClientErrorSubcategoryGroup, "API Key 绑定的分组已停用或不可用", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "subscription_not_found", "subscription_invalid", "subscription expired", "no active subscription", "订阅不存在", "订阅无效", "订阅已过期") {
		return clientClassification(OpsClientErrorSubcategorySubscription, "订阅不存在、已过期或不满足当前分组要求", OpsClassificationConfidenceHigh)
	}
	if containsAny(text, "insufficient balance", "insufficient_balance", "insufficient quota", "quota exhausted", "api_key_quota_exhausted", "usage_limit_exceeded", "balance", "余额不足", "额度不足", "配额耗尽", "用量限制") {
		return clientClassification(OpsClientErrorSubcategoryBalance, "用户余额不足、Key 配额耗尽或用户额度不足", OpsClassificationConfidenceHigh)
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
		return clientClassification(OpsClientErrorSubcategoryAuth, "客户端凭证缺失、无效、禁用、过期，或被访问控制拒绝", OpsClassificationConfidenceHigh)
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
