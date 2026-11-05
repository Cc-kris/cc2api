package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// StreamContinuationContextKey 存放单次请求的流式续写状态（仅内存、仅本次请求，请求结束即丢弃）。
const StreamContinuationContextKey = "stream_continuation_state"

// GatewayFailureReasonStreamInterrupted 流式输出中途中断，可换号续写。
const GatewayFailureReasonStreamInterrupted GatewayFailureReason = "stream_interrupted"

// GatewayFailureReasonStreamContinuationUnsupported 续写进行中，但该账号的转发路径不支持续写。
const GatewayFailureReasonStreamContinuationUnsupported GatewayFailureReason = "stream_continuation_unsupported"

// StreamContinuation 记录"已经推给客户端的内容"，用于换号后让新账号从断点接着写，
// 并把新账号的流拼接到旧流之后（客户端看到的是同一条回复）。
//
// 只在内存中、只针对当前这一个请求，不落库、不写日志。
type StreamContinuation struct {
	// ---- 跨尝试累计的客户端可见状态 ----
	wroteContent     bool // 已向客户端写出过内容事件（不含 ping/心跳）
	messageStartSent bool // 已发出 message_start（Anthropic）
	sawStopReason    bool // 已收到带 stop_reason 的 message_delta，说明正文已完整
	sawToolUse       bool // 已开始输出工具调用：工具参数 JSON 无法安全拼接，不再续写
	nextIndex        int  // 下一个客户端可见内容块 index
	openIndex        int  // 当前未结束的内容块（-1 表示没有）
	openType         string
	text             strings.Builder // 已输出的正文文本（所有 text 块拼接）

	// ---- 当前尝试（续写尝试）的映射状态 ----
	continuing         bool
	indexMap           map[int]int
	merged             bool   // 续写的第一个 text 块已并入旧的未结束 text 块
	mergedUpstreamIdx  int    // 被并入的上游 index
	pendingStripPrefix string // 预填时去掉的尾部空白，续写首个 delta 需去重
	firstDeltaDone     bool
	interruptions      int

	// OpenAI Responses 协议的续写状态（仅 /v1/responses 路径使用）
	resp *responsesContState
}

// AttachStreamContinuation 为本次请求开启流式续写状态。
func AttachStreamContinuation(c *gin.Context) *StreamContinuation {
	if c == nil {
		return nil
	}
	sc := &StreamContinuation{openIndex: -1}
	c.Set(StreamContinuationContextKey, sc)
	return sc
}

// StreamContinuationFromContext 返回本次请求的续写状态；未开启时返回 nil。
func StreamContinuationFromContext(c *gin.Context) *StreamContinuation {
	if c == nil {
		return nil
	}
	v, ok := c.Get(StreamContinuationContextKey)
	if !ok {
		return nil
	}
	sc, _ := v.(*StreamContinuation)
	return sc
}

// Active 报告是否已有内容推给客户端（下一次尝试必须是"续写"而不是从头生成）。
func (sc *StreamContinuation) Active() bool {
	return sc != nil && sc.wroteContent
}

// Recoverable 报告中途断开后是否还能续写。
func (sc *StreamContinuation) Recoverable() bool {
	return sc != nil && !sc.sawToolUse && !sc.sawStopReason
}

// EmittedText 返回已推给客户端的正文（测试与诊断用）。
func (sc *StreamContinuation) EmittedText() string {
	if sc == nil {
		return ""
	}
	return sc.text.String()
}

// NewStreamInterruptedFailover 构造"流式中途中断、可换号续写"的 failover 错误。
func (sc *StreamContinuation) NewStreamInterruptedFailover(message string) *UpstreamFailoverError {
	if sc != nil {
		sc.interruptions++
	}
	body, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": "upstream_disconnected", "message": message},
	})
	return &UpstreamFailoverError{
		StatusCode:               http.StatusBadGateway,
		ResponseBody:             body,
		SafeToFailoverAfterWrite: true,
		Reason:                   GatewayFailureReasonStreamInterrupted,
	}
}

// NewStreamContinuationUnsupportedFailover 构造"该账号不支持续写"的 failover 错误。
func NewStreamContinuationUnsupportedFailover() *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:               http.StatusBadGateway,
		ResponseBody:             []byte(`{"type":"error","error":{"type":"upstream_error","message":"Upstream stream interrupted"}}`),
		SafeToFailoverAfterWrite: true,
		Reason:                   GatewayFailureReasonStreamContinuationUnsupported,
	}
}

// BeginAttempt 在每次开始处理上游流时调用。
func (sc *StreamContinuation) BeginAttempt() {
	if sc == nil {
		return
	}
	sc.continuing = sc.wroteContent
	sc.indexMap = make(map[int]int)
	sc.merged = false
	sc.mergedUpstreamIdx = -1
	sc.firstDeltaDone = false
	sc.beginResponsesAttempt()
}

// ---------------------------------------------------------------------------
// Anthropic Messages
// ---------------------------------------------------------------------------

// BuildAnthropicContinuationBody 在已有输出时，把已输出正文作为 assistant 预填追加到请求末尾，
// 让新账号从断点接着写。没有正文时返回原请求（新账号从头生成，旧的 message_start 会被复用）。
func (sc *StreamContinuation) BuildAnthropicContinuationBody(body []byte) []byte {
	if !sc.Active() {
		return body
	}
	text := sc.text.String()
	trimmed := strings.TrimRight(text, " \t\r\n")
	sc.pendingStripPrefix = text[len(trimmed):]
	if trimmed == "" {
		return body
	}
	out := body
	// 开启 thinking 时 Claude 不支持 assistant 预填；续写只需要继续正文。
	if gjson.GetBytes(out, "thinking").Exists() {
		if v, err := sjson.DeleteBytes(out, "thinking"); err == nil {
			out = v
		}
	}
	msgs := gjson.GetBytes(out, "messages")
	if !msgs.IsArray() {
		return body
	}
	arr := msgs.Array()
	if n := len(arr); n > 0 && arr[n-1].Get("role").String() == "assistant" {
		last := arr[n-1].Get("content")
		path := "messages." + itoa(n-1) + ".content"
		if last.Type == gjson.String {
			if v, err := sjson.SetBytes(out, path, last.String()+trimmed); err == nil {
				return v
			}
			return body
		}
		if v, err := sjson.SetBytes(out, path+".-1", map[string]any{"type": "text", "text": trimmed}); err == nil {
			return v
		}
		return body
	}
	v, err := sjson.SetBytes(out, "messages.-1", map[string]any{
		"role":    "assistant",
		"content": []map[string]any{{"type": "text", "text": trimmed}},
	})
	if err != nil {
		return body
	}
	return v
}

// RewriteAnthropicBlock 处理一个即将写给客户端的 SSE 块：
// 首次尝试时只记录状态；续写尝试时丢弃重复的 message_start、重排内容块 index、
// 把续写的第一个 text 块并入旧的未结束 text 块，并去掉预填尾部空白的重复。
// 返回空字符串表示该块不写给客户端。
func (sc *StreamContinuation) RewriteAnthropicBlock(block string) string {
	if sc == nil {
		return block
	}
	eventName, data := splitSSEBlock(block)
	if data == "" || data == "[DONE]" || !gjson.Valid(data) {
		return block
	}
	evType := gjson.Get(data, "type").String()
	if eventName == "" {
		eventName = evType
	}
	if evType == "ping" || eventName == "ping" {
		return block
	}

	if !sc.continuing {
		sc.trackAnthropic(evType, data)
		sc.wroteContent = true
		return block
	}

	prefix := ""
	switch evType {
	case "message_start":
		if sc.messageStartSent {
			return ""
		}
		sc.messageStartSent = true
		sc.wroteContent = true
		return block
	case "content_block_start":
		upIdx := int(gjson.Get(data, "index").Int())
		blockType := gjson.Get(data, "content_block.type").String()
		if !sc.merged && sc.openIndex >= 0 && sc.openType == "text" && blockType == "text" {
			sc.merged = true
			sc.mergedUpstreamIdx = upIdx
			sc.indexMap[upIdx] = sc.openIndex
			return ""
		}
		if sc.openIndex >= 0 {
			// 关闭旧的未结束块（例如中途断开的 thinking），新内容从下一个 index 开始
			prefix = anthropicBlockStopEvent(sc.openIndex)
			sc.openIndex = -1
			sc.openType = ""
		}
		newIdx := sc.nextIndex
		sc.nextIndex++
		sc.indexMap[upIdx] = newIdx
		sc.openIndex = newIdx
		sc.openType = blockType
		if blockType == "tool_use" || blockType == "server_tool_use" {
			sc.sawToolUse = true
		}
		data = setJSONInt(data, "index", newIdx)
	case "content_block_delta":
		upIdx := int(gjson.Get(data, "index").Int())
		idx, ok := sc.indexMap[upIdx]
		if !ok {
			idx = upIdx
		}
		data = setJSONInt(data, "index", idx)
		if gjson.Get(data, "delta.type").String() == "text_delta" {
			delta := gjson.Get(data, "delta.text").String()
			if sc.merged && upIdx == sc.mergedUpstreamIdx && !sc.firstDeltaDone && delta != "" {
				sc.firstDeltaDone = true
				if p := sc.pendingStripPrefix; p != "" {
					if strings.HasPrefix(delta, p) {
						delta = delta[len(p):]
					} else {
						delta = strings.TrimLeft(delta, " \t\r\n")
					}
					if v, err := sjson.Set(data, "delta.text", delta); err == nil {
						data = v
					}
				}
			}
			sc.text.WriteString(delta)
		}
	case "content_block_stop":
		upIdx := int(gjson.Get(data, "index").Int())
		idx, ok := sc.indexMap[upIdx]
		if !ok {
			idx = upIdx
		}
		if idx == sc.openIndex {
			sc.openIndex = -1
			sc.openType = ""
		}
		data = setJSONInt(data, "index", idx)
	default:
		sc.trackAnthropic(evType, data)
	}
	sc.wroteContent = true
	return prefix + buildSSEBlock(eventName, data)
}

func (sc *StreamContinuation) trackAnthropic(evType, data string) {
	switch evType {
	case "message_start":
		sc.messageStartSent = true
	case "content_block_start":
		idx := int(gjson.Get(data, "index").Int())
		sc.openIndex = idx
		sc.openType = gjson.Get(data, "content_block.type").String()
		if sc.openType == "tool_use" || sc.openType == "server_tool_use" {
			sc.sawToolUse = true
		}
		if idx+1 > sc.nextIndex {
			sc.nextIndex = idx + 1
		}
	case "content_block_delta":
		if gjson.Get(data, "delta.type").String() == "text_delta" {
			sc.text.WriteString(gjson.Get(data, "delta.text").String())
		}
	case "content_block_stop":
		if int(gjson.Get(data, "index").Int()) == sc.openIndex {
			sc.openIndex = -1
			sc.openType = ""
		}
	case "message_delta":
		if gjson.Get(data, "delta.stop_reason").String() != "" {
			sc.sawStopReason = true
		}
	}
}

func anthropicBlockStopEvent(index int) string {
	return "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":" + itoa(index) + "}\n\n"
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func splitSSEBlock(block string) (eventName, data string) {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			continue
		}
		if data == "" && strings.HasPrefix(trimmed, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		}
	}
	return eventName, data
}

func buildSSEBlock(eventName, data string) string {
	out := ""
	if eventName != "" {
		out = "event: " + eventName + "\n"
	}
	return out + "data: " + data + "\n\n"
}

func setJSONInt(data, path string, v int) string {
	if out, err := sjson.Set(data, path, v); err == nil {
		return out
	}
	return data
}

func itoa(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
