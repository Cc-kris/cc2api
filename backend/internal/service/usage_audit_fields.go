package service

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// UsageAuditFields 是 V03-R11 的用量审计字段（迁移 230）。
// 只做落库和展示，绝不参与计费金额计算。
//
// 移植自上游 db0bff82c（upstream_response_model / mismatch）、1c0cb24c7（session_id）、
// 11ada80d5（requested_reasoning_effort）、de27905e8（upstream_request_id）。
type UsageAuditFields struct {
	// UpstreamResponseModel 是上游成功响应里声明的模型（客户端改写和协议转换之前）。
	UpstreamResponseModel *string
	// UpstreamModelMismatch 为 nil 表示没有观测到上游模型；否则比较响应模型与实际发送模型。
	UpstreamModelMismatch *bool
	// SessionID 只存客户端显式传的会话标识请求头；从不存粘性会话哈希、prompt_cache_key 或内容派生值。
	SessionID *string
	// RequestedReasoningEffort 是客户端请求的推理强度（任何映射/规范化之前）。
	RequestedReasoningEffort *string
	// UpstreamRequestID 是直接上游在响应头中声明的请求 ID（见 upstream_request_id.go）。
	// 与财务 usage_upstream_attempts.upstream_billing_request_id（上游计费单号）是两个不同字段。
	UpstreamRequestID *string
}

const (
	// maxPersistedSessionIDLength 与 usage_logs.session_id VARCHAR(255) 对齐；超长直接丢弃，避免截断后不同会话混为一个。
	maxPersistedSessionIDLength = 255
	// upstreamResponseModelMaxLength 与 usage_logs.upstream_response_model VARCHAR(200) 对齐。
	upstreamResponseModelMaxLength = 200
	// maxRequestedReasoningEffortLength 与 usage_logs.requested_reasoning_effort VARCHAR(20) 对齐。
	maxRequestedReasoningEffortLength = 20
)

// clientSessionIDHeaders 是客户端显式会话标识请求头，按优先级排列。
// 只读请求头，不读请求体，因此 prompt_cache_key 和粘性会话哈希不可能落进 session_id。
func clientSessionIDHeaders() []string {
	return []string{
		"session_id",
		"conversation_id",
		openCodeSessionAffinityHeader,
		openCodeSessionIDHeader,
		openCodeNativeSessionHeader,
		codeBuddyConversationHeader,
		claudeCodeSessionHeader,
	}
}

// ExtractClientSessionID 从请求头解析客户端显式会话标识，用于 usage_logs.session_id。
// 它不影响粘性路由、账号选择、request_id 或上游缓存。没有合法值时返回 ""。
func ExtractClientSessionID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	for _, header := range clientSessionIDHeaders() {
		if sessionID := sanitizeSessionID(c.GetHeader(header)); sessionID != "" {
			return sessionID
		}
	}
	if isGrokRequestContext(c) {
		if sessionID := sanitizeSessionID(c.GetHeader(grokConversationIDHeader)); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

// sanitizeSessionID 去掉首尾空白；含控制字符、非法 UTF-8 或超过列宽的值整体丢弃。
func sanitizeSessionID(raw string) string {
	if !utf8.ValidString(raw) {
		return ""
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	count := 0
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return ""
		}
		count++
		if count > maxPersistedSessionIDLength {
			return ""
		}
	}
	return trimmed
}

const requestedReasoningEffortGinKey = "usage_audit_requested_reasoning_effort"

// CaptureRequestedReasoningEffort 从入站原始请求体取客户端请求的推理强度（任何改写之前），存到 gin 上下文。
// 依次读 reasoning.effort、reasoning_effort、output_config.effort。没有显式值时不存，
// 记账时回落到实际转发值（含模型后缀推导）。
func CaptureRequestedReasoningEffort(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	if _, exists := c.Get(requestedReasoningEffortGinKey); exists {
		return
	}
	raw := firstTrimmedGJSONString(body, "reasoning.effort", "reasoning_effort", "output_config.effort")
	if effort := normalizeRequestedReasoningEffort(raw); effort != "" {
		c.Set(requestedReasoningEffortGinKey, effort)
	}
}

// RequestedReasoningEffortFromGin 返回 CaptureRequestedReasoningEffort 存下的值，没有时为 nil。
func RequestedReasoningEffortFromGin(c *gin.Context) *string {
	if c == nil {
		return nil
	}
	value, ok := c.Get(requestedReasoningEffortGinKey)
	if !ok {
		return nil
	}
	effort, _ := value.(string)
	if effort == "" {
		return nil
	}
	return &effort
}

// normalizeRequestedReasoningEffort 只做小写和去空白，保留客户端原值（例如 max、minimal 不被改写），
// 过长或含非字母字符的值丢弃。
func normalizeRequestedReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || len(value) > maxRequestedReasoningEffortLength {
		return ""
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && r != '-' && r != '_' {
			return ""
		}
	}
	return value
}

// coalesceRequestedReasoningEffort 优先取客户端请求值，缺失时回落到实际转发值。
func coalesceRequestedReasoningEffort(requested, forwarded *string) *string {
	if requested != nil {
		if v := strings.TrimSpace(*requested); v != "" {
			return &v
		}
	}
	if forwarded != nil {
		if v := strings.TrimSpace(*forwarded); v != "" {
			return &v
		}
	}
	return nil
}

// normalizeObservedUpstreamResponseModel 去空白并按 rune 截断到列宽。
func normalizeObservedUpstreamResponseModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if runes := []rune(model); len(runes) > upstreamResponseModelMaxLength {
		model = string(runes[:upstreamResponseModelMaxLength])
	}
	return model
}

// upstreamModelMismatch 三态：没有响应模型 → nil；否则比较（大小写不敏感）发送模型与响应模型。
func upstreamModelMismatch(sentModel, responseModel string) *bool {
	responseModel = strings.TrimSpace(responseModel)
	if responseModel == "" {
		return nil
	}
	sentModel = strings.TrimSpace(sentModel)
	mismatch := sentModel == "" || !strings.EqualFold(sentModel, responseModel)
	return &mismatch
}

// upstreamSentModel 返回实际发给上游的模型：有映射取映射后的，否则取请求模型。
func upstreamSentModel(requestedModel, upstreamModel string) string {
	if sent := strings.TrimSpace(upstreamModel); sent != "" {
		return sent
	}
	return strings.TrimSpace(requestedModel)
}

// buildUsageAuditFields 汇总一条用量记录的审计字段。只读输入，不改动任何计费相关值。
func buildUsageAuditFields(sentModel, responseModel, sessionID string, requestedEffort, forwardedEffort *string) UsageAuditFields {
	responseModel = normalizeObservedUpstreamResponseModel(responseModel)
	return UsageAuditFields{
		UpstreamResponseModel:    optionalTrimmedStringPtr(responseModel),
		UpstreamModelMismatch:    upstreamModelMismatch(sentModel, responseModel),
		SessionID:                optionalTrimmedStringPtr(sanitizeSessionID(sessionID)),
		RequestedReasoningEffort: coalesceRequestedReasoningEffort(requestedEffort, forwardedEffort),
	}
}

const upstreamResponseModelObserverContextKey = "usage_audit_upstream_response_model"

// upstreamResponseModelObserver 记录一次转发尝试里上游响应声明的模型。
// 终止事件的声明优先，否则保留第一次声明。观测只读，不影响转发和计费。
type upstreamResponseModelObserver struct {
	first    string
	terminal string
	// headers 是本次尝试直接上游的响应头快照，用于解析 upstream_request_id（V03-T17）。
	headers http.Header
}

// observeUpstreamResponseHeaders 记下本次转发尝试直接上游的响应头（只读快照）。
func observeUpstreamResponseHeaders(c *gin.Context, h http.Header) {
	if c == nil || h == nil {
		return
	}
	ensureUpstreamResponseModelObserver(c).headers = h.Clone()
}

// ObservedUpstreamResponseHeaders 返回当前尝试观测到的上游响应头，没有时为 nil。
func ObservedUpstreamResponseHeaders(c *gin.Context) http.Header {
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		return nil
	}
	return observer.headers
}

func (o *upstreamResponseModelObserver) Observe(model string, terminal bool) {
	if o == nil {
		return
	}
	model = normalizeObservedUpstreamResponseModel(model)
	if model == "" {
		return
	}
	if terminal {
		o.terminal = model
		return
	}
	if o.first == "" {
		o.first = model
	}
}

func (o *upstreamResponseModelObserver) Model() string {
	if o == nil {
		return ""
	}
	if o.terminal != "" {
		return o.terminal
	}
	return o.first
}

// beginUpstreamResponseModelObservation 为一次转发尝试新建观测器（换号重试时覆盖上一次的）。
func beginUpstreamResponseModelObservation(c *gin.Context) *upstreamResponseModelObserver {
	observer := &upstreamResponseModelObserver{}
	if c != nil {
		c.Set(upstreamResponseModelObserverContextKey, observer)
	}
	return observer
}

func upstreamResponseModelObserverFromContext(c *gin.Context) *upstreamResponseModelObserver {
	if c == nil {
		return nil
	}
	value, ok := c.Get(upstreamResponseModelObserverContextKey)
	if !ok {
		return nil
	}
	observer, _ := value.(*upstreamResponseModelObserver)
	return observer
}

// ensureUpstreamResponseModelObserver 返回当前尝试的观测器，没有时新建。
func ensureUpstreamResponseModelObserver(c *gin.Context) *upstreamResponseModelObserver {
	if observer := upstreamResponseModelObserverFromContext(c); observer != nil {
		return observer
	}
	return beginUpstreamResponseModelObservation(c)
}

// ObservedUpstreamResponseModel 返回当前尝试观测到的上游响应模型（handler 在提交用量前读取）。
func ObservedUpstreamResponseModel(c *gin.Context) string {
	return upstreamResponseModelObserverFromContext(c).Model()
}

// observeAnthropicResponseModel 从 Anthropic 响应体或 SSE data 里取 message.model / model。
func observeAnthropicResponseModel(c *gin.Context, payload []byte) {
	model := firstTrimmedGJSONString(payload, "message.model", "model")
	ensureUpstreamResponseModelObserver(c).Observe(model, false)
}

// observeOpenAIResponseModel 从 OpenAI 响应体或 SSE data 里取 response.model / model；终止事件优先。
func observeOpenAIResponseModel(c *gin.Context, payload []byte, eventType string) {
	model := firstTrimmedGJSONString(payload, "response.model", "model")
	ensureUpstreamResponseModelObserver(c).Observe(model, isUpstreamResponseModelTerminalEvent(eventType))
}

func firstTrimmedGJSONString(payload []byte, paths ...string) string {
	if len(payload) == 0 {
		return ""
	}
	for _, path := range paths {
		value := gjson.GetBytes(payload, path)
		if value.Exists() && value.Type == gjson.String {
			if text := strings.TrimSpace(value.String()); text != "" {
				return text
			}
		}
	}
	return ""
}

func isUpstreamResponseModelTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

// observeOpenAIResponseBody 观测非流式响应体；部分上游对非流式请求也返回 SSE 帧，逐帧观测。
func observeOpenAIResponseBody(c *gin.Context, body []byte) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return
	}
	if strings.HasPrefix(trimmed, "{") {
		observeOpenAIResponseModel(c, body, gjson.GetBytes(body, "type").String())
		return
	}
	for _, line := range strings.Split(trimmed, "\n") {
		if data, ok := extractOpenAISSEDataLine(line); ok {
			observeOpenAIResponseModel(c, []byte(data), gjson.Get(data, "type").String())
		}
	}
}
