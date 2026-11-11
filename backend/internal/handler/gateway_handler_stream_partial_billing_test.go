package handler

import (
	"io"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// V03-AC16：分组未开启任意错误换号，上游发出 message_start（输入 1000）和输出 300 后断开，无 message_stop。
// 客户端收到错误；按已观测用量记账一次：1000×3e-6 + 300×1.5e-5 = 0.0075。
func TestGatewayHandlerMessages_StreamInterruptedBillsObservedUsageOnce(t *testing.T) {
	account := newStreamBillingAccount(101, 0, 1)
	upstreamSSE := sseEvent("message_start", `{"type":"message_start","message":{"id":"msg_a","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1000,"output_tokens":1}}}`) +
		sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`) +
		sseEvent("message_delta", `{"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":300}}`)
	h := newStreamBillingHarness(t, false, []*service.Account{account}, map[int64][]streamBillingUpstreamStep{
		101: {{body: upstreamSSE, readErr: io.ErrUnexpectedEOF}},
	})
	defer h.cleanup()

	rec := h.serve(t, "claude-sonnet-4-6")

	require.Contains(t, rec.Body.String(), "partial", "已推送内容不撤回")
	require.Contains(t, rec.Body.String(), `"type":"error"`, "客户端必须收到错误事件")

	logs := h.waitBilling(t, 1)
	require.Len(t, logs, 1)
	require.Equal(t, 1000, logs[0].InputTokens)
	require.Equal(t, 300, logs[0].OutputTokens)
	require.InDelta(t, 0.0075, logs[0].ActualCost, 1e-12)
	cmd := requireSingleBalanceCharge(t, h, 0.0075)
	require.Equal(t, int64(101), cmd.AccountID)
	require.Equal(t, 1, h.upstream.callCount(101))
}

// V03-AC17：上游在 message_start 之前就断开，不写用量记录，也不扣费（不能生成零用量记录）。
func TestGatewayHandlerMessages_StreamInterruptedBeforeUsageDoesNotBill(t *testing.T) {
	account := newStreamBillingAccount(102, 0, 1)
	h := newStreamBillingHarness(t, false, []*service.Account{account}, map[int64][]streamBillingUpstreamStep{
		102: {{body: "event: ping\ndata: {\"type\":\"ping\"}\n\n"}},
	})
	defer h.cleanup()

	_ = h.serve(t, "claude-sonnet-4-6")

	require.Empty(t, h.waitBillingNone(t))
	applied, calls := h.billing.snapshot()
	require.Empty(t, applied)
	require.Zero(t, calls)
}
