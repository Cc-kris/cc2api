package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func responsesContinuationAnthropicEvents(messageID, text string, complete bool) string {
	lines := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"` + messageID + `","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":10,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + text + `"}}`,
		``,
	}
	if complete {
		lines = append(lines,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		)
	}
	return strings.Join(lines, "\n")
}

func responsesContinuationSSEData(body string) []string {
	var data []string
	for _, block := range strings.Split(body, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				data = append(data, strings.TrimPrefix(line, "data: "))
			}
		}
	}
	return data
}

func TestHandleResponsesStreamingResponse_ContinuesAfterReadErrorInOneLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	sc := AttachStreamContinuation(c)
	svc := &GatewayService{}

	result, err := svc.handleResponsesStreamingResponse(
		&http.Response{
			Header: http.Header{"x-request-id": []string{"attempt-a"}},
			Body:   &streamReadCloser{payload: []byte(responsesContinuationAnthropicEvents("msg_A", "ALPHA ", false)), err: io.ErrUnexpectedEOF},
		},
		c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
	)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.True(t, failover.SafeToFailoverAfterWrite)
	require.Equal(t, GatewayFailureReasonStreamInterrupted, failover.Reason)
	require.Equal(t, 10, failover.ObservedUsage.InputTokens)
	require.Equal(t, 1, failover.ObservedUsage.OutputTokens)
	require.Equal(t, "ALPHA ", sc.EmittedText())
	require.NotContains(t, rec.Body.String(), "response.completed")
	require.NotContains(t, rec.Body.String(), "response.output_item.done")

	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"continue"}],"stream":true}`)
	continued := sc.BuildAnthropicContinuationBody(body)
	messages := gjson.GetBytes(continued, "messages").Array()
	require.Len(t, messages, 2)
	require.Equal(t, "assistant", messages[1].Get("role").String())
	require.Equal(t, "ALPHA", messages[1].Get("content.0.text").String())

	result, err = svc.handleResponsesStreamingResponse(
		&http.Response{
			Header: http.Header{"x-request-id": []string{"attempt-b"}},
			Body:   io.NopCloser(strings.NewReader(responsesContinuationAnthropicEvents("msg_B", "BETA", true))),
		},
		c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, "ALPHA BETA", sc.EmittedText())

	events := responsesContinuationSSEData(rec.Body.String())
	var created, completed, deltas int
	var previousSequence int64 = -1
	for _, data := range events {
		eventType := gjson.Get(data, "type").String()
		require.Greater(t, gjson.Get(data, "sequence_number").Int(), previousSequence, data)
		previousSequence = gjson.Get(data, "sequence_number").Int()
		switch eventType {
		case "response.created":
			created++
		case "response.completed":
			completed++
			require.Equal(t, "msg_A", gjson.Get(data, "response.id").String())
			require.Equal(t, "ALPHA BETA", gjson.Get(data, "response.output.0.content.0.text").String())
		case "response.output_text.delta":
			deltas++
			require.Equal(t, int64(0), gjson.Get(data, "output_index").Int())
			require.Equal(t, int64(0), gjson.Get(data, "content_index").Int())
			if deltas == 1 {
				require.Equal(t, "ALPHA ", gjson.Get(data, "delta").String())
			} else {
				require.Equal(t, "BETA", gjson.Get(data, "delta").String())
				require.Equal(t, gjson.Get(events[1], "item.id").String(), gjson.Get(data, "item_id").String())
			}
		}
	}
	require.Equal(t, 1, created)
	require.Equal(t, 1, completed)
	require.Equal(t, 2, deltas)
}

func TestHandleResponsesStreamingResponse_CleanTerminalEOFDoesNotRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	sc := AttachStreamContinuation(c)

	result, err := (&GatewayService{}).handleResponsesStreamingResponse(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(responsesContinuationAnthropicEvents("msg_clean", "DONE", true)))},
		c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.ClientDisconnect)
	require.Zero(t, sc.interruptions)
	require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.completed\n"))
	require.Equal(t, "DONE", gjson.Get(responsesContinuationSSEData(rec.Body.String())[len(responsesContinuationSSEData(rec.Body.String()))-1], "response.output.0.content.0.text").String())
}

func TestHandleResponsesStreamingResponse_IncompleteCleanEOFAfterOutputDoesNotFinalize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	sc := AttachStreamContinuation(c)

	result, err := (&GatewayService{}).handleResponsesStreamingResponse(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(responsesContinuationAnthropicEvents("msg_truncated", "ALPHA", false)))},
		c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
	)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Nil(t, result)
	require.Equal(t, GatewayFailureReasonStreamInterrupted, failover.Reason)
	require.Equal(t, "ALPHA", sc.EmittedText())
	require.NotContains(t, rec.Body.String(), "response.completed")
}

func TestHandleResponsesStreamingResponse_IgnoresNegativeContentBlockIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_negative","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":-1,"delta":{"type":"text_delta","text":"MUST_NOT_LEAK"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"valid"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	_, err := (&GatewayService{}).handleResponsesStreamingResponse(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))},
		c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
	)
	require.NoError(t, err)
	require.NotContains(t, rec.Body.String(), "MUST_NOT_LEAK")
	require.Contains(t, rec.Body.String(), `"delta":"valid"`)
}

func TestHandleResponsesStreamingResponse_ClientDisconnectDoesNotReturnFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: "partial"}
	c, _ := gin.CreateTestContext(writer)
	AttachStreamContinuation(c)

	result, err := (&GatewayService{}).handleResponsesStreamingResponse(
		&http.Response{
			Header: http.Header{},
			Body:   &streamReadCloser{payload: []byte(responsesContinuationAnthropicEvents("msg_disconnect", "partial", false)), err: errors.New("connection reset")},
		},
		c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.NotContains(t, writer.Body.String(), "response.completed")
}
