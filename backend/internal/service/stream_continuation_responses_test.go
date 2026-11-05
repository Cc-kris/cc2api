package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 场景：账号 A 输出 "Hello wor" 后断开；账号 B 续写 "ld!"。
// 客户端应看到同一个 response.id / item_id / output_index，sequence_number 单调递增，
// 终止事件里的正文是拼接后的完整文本。
func TestStreamContinuation_ResponsesStitchText(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	sc.BeginAttempt()
	sc.beginResponsesAttempt()

	attemptA := []string{
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_A","status":"in_progress"}}`,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_A","type":"message","role":"assistant","content":[]}}`,
		`{"type":"response.content_part.added","sequence_number":2,"item_id":"msg_A","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,
		`{"type":"response.output_text.delta","sequence_number":3,"item_id":"msg_A","output_index":0,"content_index":0,"delta":"Hello "}`,
		`{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_A","output_index":0,"content_index":0,"delta":"wor"}`,
	}
	for i, d := range attemptA {
		_, keep := sc.RewriteResponsesData(d, i > 0)
		require.True(t, keep)
	}
	require.True(t, sc.Active())
	require.True(t, sc.Recoverable())
	require.Equal(t, "Hello wor", sc.EmittedText())

	// 续写请求体：assistant 预填 + developer 续写指令
	body := sc.BuildResponsesContinuationBody([]byte(`{"model":"gpt-x","input":"say hello","stream":true}`))
	input := gjson.GetBytes(body, "input").Array()
	require.Len(t, input, 3)
	require.Equal(t, "assistant", input[1].Get("role").String())
	require.Equal(t, "Hello wor", input[1].Get("content.0.text").String())
	require.Equal(t, "developer", input[2].Get("role").String())

	// 账号 B
	sc.BeginAttempt()
	sc.beginResponsesAttempt()
	attemptB := []string{
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_B","status":"in_progress"}}`,
		`{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_B"}}`,
		`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"msg_B","type":"message","role":"assistant","content":[]}}`,
		`{"type":"response.content_part.added","sequence_number":3,"item_id":"msg_B","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,
		`{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_B","output_index":0,"content_index":0,"delta":"ld!"}`,
		`{"type":"response.output_text.done","sequence_number":5,"item_id":"msg_B","output_index":0,"content_index":0,"text":"ld!"}`,
		`{"type":"response.output_item.done","sequence_number":6,"output_index":0,"item":{"id":"msg_B","type":"message","role":"assistant","content":[{"type":"output_text","text":"ld!"}]}}`,
		`{"type":"response.completed","sequence_number":7,"response":{"id":"resp_B","status":"completed","output":[{"id":"msg_B","type":"message","role":"assistant","content":[{"type":"output_text","text":"ld!"}]}]}}`,
	}
	var emitted []string
	for _, d := range attemptB {
		out, keep := sc.RewriteResponsesData(d, true)
		if keep {
			emitted = append(emitted, out)
		}
	}
	// created / in_progress / 重复的 output_item.added / content_part.added 被丢弃
	require.Len(t, emitted, 4)

	lastSeq := int64(4)
	for _, e := range emitted {
		seq := gjson.Get(e, "sequence_number").Int()
		require.Greater(t, seq, lastSeq, e)
		lastSeq = seq
		if id := gjson.Get(e, "item_id"); id.Exists() {
			require.Equal(t, "msg_A", id.String())
		}
		if idx := gjson.Get(e, "output_index"); idx.Exists() {
			require.Equal(t, int64(0), idx.Int())
		}
	}
	require.Equal(t, "ld!", gjson.Get(emitted[0], "delta").String())
	require.Equal(t, "Hello world!", gjson.Get(emitted[1], "text").String())
	require.Equal(t, "msg_A", gjson.Get(emitted[2], "item.id").String())
	require.Equal(t, "Hello world!", gjson.Get(emitted[2], "item.content.0.text").String())

	final := emitted[3]
	require.Equal(t, "resp_A", gjson.Get(final, "response.id").String())
	require.Equal(t, "Hello world!", gjson.Get(final, "response.output.0.content.0.text").String())
	require.Equal(t, "Hello world!", sc.EmittedText())
	require.False(t, sc.Recoverable(), "completed 之后不再续写")
}

func TestStreamContinuation_ResponsesFunctionCallNotRecoverable(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	sc.BeginAttempt()
	sc.beginResponsesAttempt()
	_, _ = sc.RewriteResponsesData(`{"type":"response.created","sequence_number":0,"response":{"id":"r"}}`, false)
	_, _ = sc.RewriteResponsesData(`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"fc_1","type":"function_call","name":"run"}}`, true)
	require.True(t, sc.Active())
	require.False(t, sc.Recoverable())
}

func TestStreamContinuation_ResponsesPreambleOnlyNotActive(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	sc.BeginAttempt()
	sc.beginResponsesAttempt()
	_, _ = sc.RewriteResponsesData(`{"type":"response.created","sequence_number":0,"response":{"id":"r"}}`, false)
	require.False(t, sc.Active(), "只发了 preamble，换号应从头生成")
	require.True(t, strings.Contains(string(sc.BuildResponsesContinuationBody([]byte(`{"input":"x"}`))), `"input":"x"`))
}
