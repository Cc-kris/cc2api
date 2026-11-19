package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestHandleStreamingResponseSynthesizesContentPartsAndPreservesTerminalOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := "data: " + `{"type":"response.created","sequence_number":0,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","created_at":1794848626,"status":"in_progress","output":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.added","sequence_number":1,"output_index":1,"item":{"id":"msg_w4","type":"message","role":"assistant","status":"in_progress","content":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_text.delta","sequence_number":2,"output_index":1,"content_index":0,"delta":"W4 streamed answer"}` + "\n\n" +
		"data: " + `{"type":"response.output_text.done","sequence_number":3,"output_index":1,"content_index":0,"item_id":"msg_w4","text":"W4 streamed answer"}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"id":"rs_w4","type":"reasoning","summary":[{"type":"summary_text","text":"fixture reasoning"}],"encrypted_content":"enc-w4"}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","sequence_number":5,"output_index":1,"item":{"id":"msg_w4","type":"message","role":"assistant","status":"completed","phase":"final_answer","content":[{"type":"output_text","text":"W4 streamed answer","annotations":[],"logprobs":[]}]}}` + "\n\n" +
		"data: " + `{"type":"response.completed","sequence_number":6,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","status":"completed","output":[],"created_at":1794848626}}` + "\n\n" +
		"data: [DONE]\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstream))}
	account := &Account{ID: 413, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result)

	payloads := sseDataPayloads(recorder.Body.String())
	requireMonotonicResponsesSequence(t, payloads)
	var types []string
	var partAdded, partDone, textDelta, itemDone, completed string
	for _, payload := range payloads {
		typ := gjson.Get(payload, "type").String()
		types = append(types, typ)
		switch typ {
		case "response.content_part.added":
			require.Empty(t, partAdded, "part added must not be duplicated")
			partAdded = payload
		case "response.content_part.done":
			require.Empty(t, partDone, "part done must not be duplicated")
			partDone = payload
		case "response.output_text.delta":
			textDelta = payload
		case "response.output_item.done":
			if gjson.Get(payload, "item.type").String() == "message" {
				itemDone = payload
			}
		case "response.completed":
			completed = payload
		}
	}
	require.NotEmpty(t, partAdded)
	require.NotEmpty(t, partDone)
	require.Equal(t, "msg_w4", gjson.Get(partAdded, "item_id").String())
	require.EqualValues(t, 1, gjson.Get(partAdded, "output_index").Int())
	require.Equal(t, "", gjson.Get(partAdded, "part.text").String())
	require.True(t, gjson.Get(partAdded, "part.annotations").IsArray())
	require.True(t, gjson.Get(partAdded, "part.logprobs").IsArray())
	require.Equal(t, "W4 streamed answer", gjson.Get(partDone, "part.text").String())
	require.Less(t, indexOfString(types, "response.content_part.added"), indexOfString(types, "response.output_text.delta"))
	require.Less(t, indexOfString(types, "response.output_text.done"), indexOfString(types, "response.content_part.done"))
	require.NotEmpty(t, textDelta)
	require.NotEmpty(t, itemDone)

	require.EqualValues(t, 1794848626, gjson.Get(completed, "response.created_at").Int())
	require.Len(t, gjson.Get(completed, "response.output").Array(), 2)
	require.Equal(t, "rs_w4", gjson.Get(completed, "response.output.0.id").String())
	require.Equal(t, "enc-w4", gjson.Get(completed, "response.output.0.encrypted_content").String())
	require.Equal(t, "msg_w4", gjson.Get(completed, "response.output.1.id").String())
	require.Equal(t, "W4 streamed answer", gjson.Get(completed, "response.output.1.content.0.text").String())
}

func TestHandleStreamingResponsePassthroughSynthesizesItemOnlyContentParts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := "data: " + `{"type":"response.created","sequence_number":0,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","created_at":1794848626,"status":"in_progress","output":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","sequence_number":1,"output_index":1,"item":{"id":"msg_w4","type":"message","role":"assistant","status":"completed","phase":"final_answer","content":[{"type":"output_text","text":"W4 streamed answer","annotations":[],"logprobs":[]}]}}` + "\n\n" +
		"data: " + `{"type":"response.completed","sequence_number":2,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","status":"completed","created_at":1794848626,"output":[]}}` + "\n\n" +
		"data: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstream))}

	result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 413, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	payloads := sseDataPayloads(recorder.Body.String())
	requireMonotonicResponsesSequence(t, payloads)
	require.Len(t, payloads, 5, "created, added, done, item.done and completed must each appear once")
	require.Equal(t, "response.content_part.added", gjson.Get(payloads[1], "type").String())
	require.Equal(t, "response.content_part.done", gjson.Get(payloads[2], "type").String())
	require.Equal(t, "response.output_item.done", gjson.Get(payloads[3], "type").String())
	require.Equal(t, "msg_w4", gjson.Get(payloads[1], "item_id").String())
	require.Equal(t, "W4 streamed answer", gjson.Get(payloads[2], "part.text").String())
	require.Equal(t, "msg_w4", gjson.Get(payloads[4], "response.output.0.id").String())
	require.Equal(t, "W4 streamed answer", gjson.Get(payloads[4], "response.output.0.content.0.text").String())
}

func TestHandleStreamingResponsePassthroughSynthesizesMissingContentPartLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := "data: " + `{"type":"response.created","sequence_number":0,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","created_at":1794848626,"status":"in_progress","output":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_w4","type":"message","role":"assistant","status":"in_progress","content":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"delta":"W4 missing part"}` + "\n\n" +
		"data: " + `{"type":"response.output_text.done","sequence_number":3,"output_index":0,"item_id":"msg_w4","content_index":0,"text":"W4 missing part"}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"id":"msg_w4","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"W4 missing part","annotations":[],"logprobs":[]}]}}` + "\n\n" +
		"data: " + `{"type":"response.completed","sequence_number":5,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","created_at":1794848626,"status":"completed","output":[]}}` + "\n\n" +
		"data: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstream))}

	_, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 413, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	payloads := sseDataPayloads(recorder.Body.String())
	requireMonotonicResponsesSequence(t, payloads)
	require.Len(t, payloads, 8)
	for index, expected := range []string{"response.created", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"} {
		require.Equal(t, expected, gjson.Get(payloads[index], "type").String())
	}
}

func TestHandleStreamingResponsePassthroughPreservesExistingContentPartDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := "data: " + `{"type":"response.created","sequence_number":0,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","created_at":1794848626,"status":"in_progress","output":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.added","sequence_number":1,"output_index":1,"item":{"id":"msg_w4","type":"message","role":"assistant","status":"in_progress","content":[]}}` + "\n\n" +
		"data: " + `{"type":"response.content_part.added","sequence_number":2,"output_index":1,"item_id":"msg_w4","content_index":0,"part":{"type":"output_text","text":"","annotations":[],"logprobs":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_text.delta","sequence_number":3,"output_index":1,"item_id":"msg_w4","content_index":0,"delta":"W4 independently rebuilt answer"}` + "\n\n" +
		"data: " + `{"type":"response.output_text.done","sequence_number":4,"output_index":1,"item_id":"msg_w4","content_index":0,"text":"W4 independently rebuilt answer"}` + "\n\n" +
		"data: " + `{"type":"response.content_part.done","sequence_number":5,"output_index":1,"item_id":"msg_w4","content_index":0,"part":{"type":"output_text","text":"W4 independently rebuilt answer","annotations":[],"logprobs":[]}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","sequence_number":6,"output_index":1,"item":{"id":"msg_w4","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"W4 independently rebuilt answer","annotations":[],"logprobs":[]}]}}` + "\n\n" +
		"data: " + `{"type":"response.completed","sequence_number":7,"response":{"id":"resp_w4","object":"response","model":"gpt-5.5","created_at":1794848626,"status":"completed","output":[]}}` + "\n\n" +
		"data: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstream))}

	result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 413, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	payloads := sseDataPayloads(recorder.Body.String())
	requireMonotonicResponsesSequence(t, payloads)
	require.Len(t, payloads, 8, "upstream part.done must be forwarded exactly once")
	require.Equal(t, "response.content_part.done", gjson.Get(payloads[5], "type").String())
	require.Equal(t, "response.output_item.done", gjson.Get(payloads[6], "type").String())
	require.Equal(t, "W4 independently rebuilt answer", gjson.Get(payloads[7], "response.output.0.content.0.text").String())
}

func requireMonotonicResponsesSequence(t *testing.T, payloads []string) {
	t.Helper()
	previous := int64(-1)
	for _, payload := range payloads {
		sequence := gjson.Get(payload, "sequence_number")
		require.Equal(t, gjson.Number, sequence.Type, payload)
		require.Greater(t, sequence.Int(), previous, payload)
		previous = sequence.Int()
	}
}

func indexOfString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
