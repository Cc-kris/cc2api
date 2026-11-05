package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func anthropicEv(name, data string) string {
	return "event: " + name + "\ndata: " + data + "\n\n"
}

// 模拟：账号 A 输出 "今天天气很好，适合 " 后断开；账号 B 续写 "出去散步。"
// 客户端应看到一条连贯的回复：只有一个 message_start，同一个 text 块 index 0。
func TestStreamContinuation_AnthropicStitchText(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}

	// ---- 账号 A ----
	sc.BeginAttempt()
	var out strings.Builder
	for _, b := range []string{
		anthropicEv("message_start", `{"type":"message_start","message":{"id":"msg_A","role":"assistant","content":[]}}`),
		anthropicEv("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		anthropicEv("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"今天天气很好，"}}`),
		anthropicEv("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"适合 "}}`),
	} {
		out.WriteString(sc.RewriteAnthropicBlock(b))
	}
	require.True(t, sc.Active())
	require.True(t, sc.Recoverable())
	require.Equal(t, "今天天气很好，适合 ", sc.EmittedText())

	// ---- 续写请求体：已输出正文作为 assistant 预填，尾部空白去掉 ----
	req := []byte(`{"model":"claude","stream":true,"thinking":{"type":"enabled"},"messages":[{"role":"user","content":"天气"}]}`)
	cont := sc.BuildAnthropicContinuationBody(req)
	require.False(t, gjson.GetBytes(cont, "thinking").Exists(), "预填时去掉 thinking")
	msgs := gjson.GetBytes(cont, "messages").Array()
	require.Len(t, msgs, 2)
	require.Equal(t, "assistant", msgs[1].Get("role").String())
	require.Equal(t, "今天天气很好，适合", msgs[1].Get("content.0.text").String())

	// ---- 账号 B ----
	sc.BeginAttempt()
	for _, b := range []string{
		anthropicEv("message_start", `{"type":"message_start","message":{"id":"msg_B","role":"assistant","content":[]}}`),
		anthropicEv("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		anthropicEv("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 出去散步。"}}`),
		anthropicEv("content_block_stop", `{"type":"content_block_stop","index":0}`),
		anthropicEv("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`),
		anthropicEv("message_stop", `{"type":"message_stop"}`),
	} {
		out.WriteString(sc.RewriteAnthropicBlock(b))
	}

	stream := out.String()
	require.Equal(t, 1, strings.Count(stream, "event: message_start"), "只保留第一次的 message_start")
	require.NotContains(t, stream, "msg_B")
	require.Equal(t, 1, strings.Count(stream, "event: content_block_start"), "续写的 text 块并入旧块")
	require.Equal(t, "今天天气很好，适合 出去散步。", sc.EmittedText())
	require.Equal(t, 1, strings.Count(stream, "event: message_stop"))
	require.False(t, sc.Recoverable(), "收到 stop_reason 后不再续写")
}

// 断开时正停在 thinking 块：续写先关闭 thinking 块，正文从新 index 开始。
func TestStreamContinuation_AnthropicClosesOpenThinkingBlock(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	sc.BeginAttempt()
	sc.RewriteAnthropicBlock(anthropicEv("message_start", `{"type":"message_start","message":{"id":"m"}}`))
	sc.RewriteAnthropicBlock(anthropicEv("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`))
	sc.RewriteAnthropicBlock(anthropicEv("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"想一想"}}`))

	sc.BeginAttempt()
	got := sc.RewriteAnthropicBlock(anthropicEv("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`))
	require.True(t, strings.HasPrefix(got, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}"))
	require.Contains(t, got, `"index":1`)
	got = sc.RewriteAnthropicBlock(anthropicEv("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"答案"}}`))
	require.Contains(t, got, `"index":1`)
}

// 已开始输出工具调用：不可续写（工具参数 JSON 无法安全拼接）。
func TestStreamContinuation_ToolUseNotRecoverable(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	sc.BeginAttempt()
	sc.RewriteAnthropicBlock(anthropicEv("message_start", `{"type":"message_start","message":{"id":"m"}}`))
	sc.RewriteAnthropicBlock(anthropicEv("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"x"}}`))
	require.True(t, sc.Active())
	require.False(t, sc.Recoverable())
}

// ping 不算内容；未写内容前不处于续写状态，请求体保持原样。
func TestStreamContinuation_PingOnlyNotActive(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	sc.BeginAttempt()
	sc.RewriteAnthropicBlock("event: ping\ndata: {\"type\":\"ping\"}\n\n")
	require.False(t, sc.Active())
	req := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, req, sc.BuildAnthropicContinuationBody(req))
}

func TestStreamInterruptedFailover_SafeAfterWrite(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	fo := sc.NewStreamInterruptedFailover("boom")
	require.True(t, fo.SafeToFailoverAfterWrite)
	require.Equal(t, GatewayFailureReasonStreamInterrupted, fo.Reason)
	require.True(t, NewStreamContinuationUnsupportedFailover().SafeToFailoverAfterWrite)
}
