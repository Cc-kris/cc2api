package handler

import (
	"io"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 账号 A：message_start（输入 1000）+ 正文 + message_delta（输出 300），随后连接中断（无 message_stop）。
func continuationInterruptedAttemptSSE() string {
	return sseEvent("message_start", `{"type":"message_start","message":{"id":"msg_a","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1000,"output_tokens":1}}}`) +
		sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello from A. "}}`) +
		sseEvent("message_delta", `{"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":300}}`)
}

// 账号 B：续写尝试，上报输入 1300、输出 500，正常结束。
func continuationCompletedAttemptSSE() string {
	return sseEvent("message_start", `{"type":"message_start","message":{"id":"msg_b","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1300,"output_tokens":1}}}`) +
		sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"And B finishes."}}`) +
		sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
		sseEvent("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":500}}`) +
		sseEvent("message_stop", `{"type":"message_stop"}`)
}

func findAttempt(attempts []service.UsageUpstreamAttempt, accountID int64) *service.UsageUpstreamAttempt {
	for i := range attempts {
		if attempts[i].AccountID == accountID {
			return &attempts[i]
		}
	}
	return nil
}

// V03-AC18：任意错误换号 + 非粘性会话。A 输出 300 后中断，B 续写成功（1300/500）。
// 用户只扣一次 1300×3e-6 + 500×1.5e-5 = 0.0114；用量记录 1 行；上游尝试 2 段（A 1000/300，B 1300/500）。
func TestGatewayHandlerMessages_StreamContinuationSuccessBillsOnceWithTwoAttempts(t *testing.T) {
	accountA := newStreamBillingAccount(201, 0, 1)
	accountB := newStreamBillingAccount(202, 0, 2)
	h := newStreamBillingHarness(t, true, []*service.Account{accountA, accountB}, map[int64][]streamBillingUpstreamStep{
		201: {{body: continuationInterruptedAttemptSSE(), readErr: io.ErrUnexpectedEOF}},
		202: {{body: continuationCompletedAttemptSSE()}},
	})
	defer h.cleanup()

	rec := h.serve(t, "claude-sonnet-4-6")
	out := rec.Body.String()
	require.Contains(t, out, "Hello from A. ")
	require.Contains(t, out, "And B finishes.")
	require.NotContains(t, out, `"type":"error"`, "续写成功时客户端看到一条完整回复")
	require.Equal(t, 1, h.upstream.callCount(201))
	require.Equal(t, 1, h.upstream.callCount(202))

	logs := h.waitBilling(t, 1)
	require.Len(t, logs, 1, "用量记录只有 1 行")
	log := logs[0]
	require.Equal(t, int64(202), log.AccountID)
	require.Equal(t, 1300, log.InputTokens)
	require.Equal(t, 500, log.OutputTokens)
	require.InDelta(t, 0.0114, log.ActualCost, 1e-12)
	requireSingleBalanceCharge(t, h, 0.0114)
	_, applyCalls := h.billing.snapshot()
	require.Equal(t, 1, applyCalls, "中断尝试绝不能单独调用计费")

	require.Len(t, log.UpstreamAttempts, 2, "被中断尝试和最终尝试都进入财务成本分段")
	attemptA := findAttempt(log.UpstreamAttempts, 201)
	require.NotNil(t, attemptA)
	require.EqualValues(t, 1000, attemptA.InputTokens)
	require.EqualValues(t, 300, attemptA.OutputTokens)
	require.True(t, attemptA.Billable)
	attemptB := findAttempt(log.UpstreamAttempts, 202)
	require.NotNil(t, attemptB)
	require.EqualValues(t, 1300, attemptB.InputTokens)
	require.EqualValues(t, 500, attemptB.OutputTokens)
	require.NotEqual(t, attemptA.AttemptNo, attemptB.AttemptNo)
}

// V03-AC19：同 AC18，但 B、C 都在输出前失败，换号耗尽。
// 客户端收到已输出内容和错误；按最后观测尝试 A 收费 0.0075，只扣一次。
func TestGatewayHandlerMessages_StreamContinuationExhaustedBillsLastObservedAttempt(t *testing.T) {
	accountA := newStreamBillingAccount(301, 0, 1)
	accountB := newStreamBillingAccount(302, 0, 2)
	accountC := newStreamBillingAccount(303, 0, 3)
	failBody := `{"type":"error","error":{"type":"api_error","message":"upstream boom"}}`
	h := newStreamBillingHarness(t, true, []*service.Account{accountA, accountB, accountC}, map[int64][]streamBillingUpstreamStep{
		301: {{body: continuationInterruptedAttemptSSE(), readErr: io.ErrUnexpectedEOF}},
		302: {{status: 500, body: failBody}},
		303: {{status: 500, body: failBody}},
	})
	defer h.cleanup()

	rec := h.serve(t, "claude-sonnet-4-6")
	out := rec.Body.String()
	require.Contains(t, out, "Hello from A. ", "已输出内容不撤回")
	require.Contains(t, out, `"type":"error"`, "换号耗尽后客户端收到错误事件")
	require.Equal(t, 1, h.upstream.callCount(302))
	require.Equal(t, 1, h.upstream.callCount(303))

	logs := h.waitBilling(t, 1)
	require.Len(t, logs, 1)
	log := logs[0]
	require.Equal(t, int64(301), log.AccountID)
	require.Equal(t, 1000, log.InputTokens)
	require.Equal(t, 300, log.OutputTokens)
	require.InDelta(t, 0.0075, log.ActualCost, 1e-12)
	requireSingleBalanceCharge(t, h, 0.0075)
	_, applyCalls := h.billing.snapshot()
	require.Equal(t, 1, applyCalls)

	require.Len(t, log.UpstreamAttempts, 1, "B、C 没有观测用量，不产生财务尝试段")
	require.EqualValues(t, 301, log.UpstreamAttempts[0].AccountID)
}

// 反向用例：A 在输出任何内容前就失败、B 也失败，耗尽时没有输出过内容，不收费。
func TestGatewayHandlerMessages_StreamFailoverExhaustedWithoutOutputDoesNotBill(t *testing.T) {
	accountA := newStreamBillingAccount(401, 0, 1)
	accountB := newStreamBillingAccount(402, 0, 2)
	failBody := `{"type":"error","error":{"type":"api_error","message":"upstream boom"}}`
	h := newStreamBillingHarness(t, true, []*service.Account{accountA, accountB}, map[int64][]streamBillingUpstreamStep{
		401: {{status: 500, body: failBody}},
		402: {{status: 500, body: failBody}},
	})
	defer h.cleanup()

	_ = h.serve(t, "claude-sonnet-4-6")

	require.Empty(t, h.waitBillingNone(t))
	applied, calls := h.billing.snapshot()
	require.Empty(t, applied)
	require.Zero(t, calls)
}

// FailoverState 只在续写中断错误带观测用量时更新 LastObservedResult；后续无用量的失败不覆盖它。
func TestFailoverState_TracksLastObservedContinuationAttempt(t *testing.T) {
	mock := &mockTempUnscheduler{}
	ctx := ctxWithGroupFailover(true)
	fs := NewFailoverState(3, false).WithAnyErrorFailover(ctx, anyErrorCfg(0))

	interrupted := (&service.StreamContinuation{}).NewStreamInterruptedFailover("disconnected")
	interrupted.ObservedUsage = &service.ClaudeUsage{InputTokens: 1000, OutputTokens: 300}
	interrupted.ObservedModel = "claude-sonnet-4-6"
	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 1, "anthropic", interrupted))
	require.NotNil(t, fs.LastObservedResult)
	require.Equal(t, int64(1), fs.LastObservedAccountID)
	require.Equal(t, 1000, fs.LastObservedResult.Usage.InputTokens)

	require.Equal(t, FailoverContinue, fs.HandleFailoverError(ctx, mock, 2, "anthropic", &service.UpstreamFailoverError{StatusCode: 500}))
	require.Equal(t, int64(1), fs.LastObservedAccountID, "无观测用量的失败不覆盖最后观测尝试")
	require.Equal(t, 300, fs.LastObservedResult.Usage.OutputTokens)
}
