package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func responsesContinuationAttemptSSE(messageID, text string, complete bool) string {
	body := sseEvent("message_start", `{"type":"message_start","message":{"id":"`+messageID+`","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1000,"output_tokens":1}}}`) +
		sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"`+text+`"}}`)
	if complete {
		body += sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
			sseEvent("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":500}}`) +
			sseEvent("message_stop", `{"type":"message_stop"}`)
	}
	return body
}

func (h *streamBillingHarness) serveResponsesRequest(t *testing.T, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), ctxkey.Group, h.group)
	ctx = context.WithValue(ctx, ctxkey.RequestID, "responses-client-req-1")
	c.Request = req.WithContext(ctx)
	c.Set(string(middleware.ContextKeyAPIKey), h.apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: h.apiKey.UserID, Concurrency: 10})
	h.handler.Responses(c)
	return rec
}

func (h *streamBillingHarness) serveResponses(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return h.serveResponsesRequest(t, []byte(`{"model":"claude-sonnet-4-6","max_output_tokens":1024,"stream":true,"input":"hello"}`))
}

func TestGatewayHandlerResponses_StreamContinuationSuccessBillsOnce(t *testing.T) {
	accountA := newStreamBillingAccount(701, 0, 1)
	accountB := newStreamBillingAccount(702, 0, 2)
	h := newStreamBillingHarness(t, true, []*service.Account{accountA, accountB}, map[int64][]streamBillingUpstreamStep{
		701: {{body: responsesContinuationAttemptSSE("msg_A", "ALPHA ", false), readErr: io.ErrUnexpectedEOF}},
		702: {{body: responsesContinuationAttemptSSE("msg_B", "BETA", true)}},
	})
	defer h.cleanup()

	rec := h.serveResponses(t)
	out := rec.Body.String()
	require.Contains(t, out, `"delta":"ALPHA "`)
	require.Contains(t, out, `"delta":"BETA"`)
	require.NotContains(t, out, `"delta":"ALPHA BETA"`)
	require.Equal(t, 1, strings.Count(out, "event: response.created\n"))
	require.Equal(t, 1, strings.Count(out, "event: response.completed\n"))
	require.Equal(t, 1, strings.Count(out, "event: response.output_item.done\n"))
	require.NotContains(t, out, `"error"`)

	var deltas, completed []string
	for _, block := range strings.Split(out, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			switch gjson.Get(data, "type").String() {
			case "response.output_text.delta":
				deltas = append(deltas, data)
			case "response.completed":
				completed = append(completed, data)
			}
		}
	}
	require.Len(t, deltas, 2)
	require.Len(t, completed, 1)
	require.Equal(t, gjson.Get(deltas[0], "item_id").String(), gjson.Get(deltas[1], "item_id").String())
	require.Equal(t, gjson.Get(deltas[0], "output_index").Int(), gjson.Get(deltas[1], "output_index").Int())
	require.Equal(t, gjson.Get(deltas[0], "content_index").Int(), gjson.Get(deltas[1], "content_index").Int())
	require.Less(t, gjson.Get(deltas[0], "sequence_number").Int(), gjson.Get(deltas[1], "sequence_number").Int())
	require.Equal(t, "msg_A", gjson.Get(completed[0], "response.id").String())
	require.Equal(t, "ALPHA BETA", gjson.Get(completed[0], "response.output.0.content.0.text").String())

	require.Equal(t, 1, h.upstream.callCount(701))
	require.Equal(t, 1, h.upstream.callCount(702))
	require.Len(t, h.upstream.requests, 2)
	messages := gjson.GetBytes(h.upstream.requests[1].body, "messages").Array()
	require.Len(t, messages, 2)
	require.Equal(t, "assistant", messages[1].Get("role").String())
	require.Equal(t, "ALPHA", messages[1].Get("content.0.text").String())

	logs := h.waitBilling(t, 1)
	require.Len(t, logs, 1, "two upstream attempts must produce one client usage row")
	require.Equal(t, int64(702), logs[0].AccountID)
	require.Len(t, logs[0].UpstreamAttempts, 2)
	requireSingleBalanceCharge(t, h, logs[0].ActualCost)
	_, applyCalls := h.billing.snapshot()
	require.Equal(t, 1, applyCalls)
}

func TestGatewayHandlerResponses_ConversionErrorRejectedLocally(t *testing.T) {
	account := newStreamBillingAccount(705, 0, 1)
	h := newStreamBillingHarness(t, true, []*service.Account{account}, nil)
	defer h.cleanup()

	rec := h.serveResponsesRequest(t, []byte(`{"model":"claude-sonnet-4-6","input":42,"stream":true}`))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"invalid_request"`)
	require.Zero(t, h.upstream.callCount(705), "conversion errors must never reach upstream")
}
