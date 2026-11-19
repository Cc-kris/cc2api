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

func TestHandleStreamingResponsePassthroughBareErrorTerminalWrittenOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := "data: " + `{"type":"response.created","sequence_number":0,"response":{"id":"resp_e2e_error","object":"response","model":"gpt-5.5","status":"in_progress","output":[],"created_at":1794848626}}` + "\n\n" +
		"event: error\n" +
		"data: " + `{"type":"error","sequence_number":1,"error":{"code":"content_policy","message":"W4 synthetic error"}}` + "\n\n" +
		"data: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstream))}

	result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 413, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.Error(t, err)
	require.NotNil(t, result)
	require.True(t, ResponsesTerminalEventWritten(c))

	var failures []string
	for _, payload := range sseDataPayloads(recorder.Body.String()) {
		if gjson.Get(payload, "type").String() == "response.failed" {
			failures = append(failures, payload)
		}
	}
	require.Len(t, failures, 1, "service terminal failure is the only response.failed; the handler must not add another")
	failed := failures[0]
	require.Equal(t, "resp_e2e_error", gjson.Get(failed, "response.id").String())
	require.EqualValues(t, 1794848626, gjson.Get(failed, "response.created_at").Int())
	require.EqualValues(t, 1, gjson.Get(failed, "sequence_number").Int())
	require.Equal(t, "content_policy", gjson.Get(failed, "response.error.code").String())
	require.Equal(t, "W4 synthetic error", gjson.Get(failed, "response.error.message").String())

	wrapResponsesTerminalWriter(c)
	_, writeErr := c.Writer.WriteString("event: response.failed\ndata: " + `{"type":"response.failed","response":{"id":"resp_other","status":"failed","error":{"message":"Upstream request failed"}}}` + "\n\n")
	require.NoError(t, writeErr)
	require.Len(t, sseDataPayloads(recorder.Body.String()), 2)
}

func TestOpenAIResponsesTerminalWriterDelegatesStateWithoutPooledWriterPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	pooled := &terminalStatePooledWriter{ResponseWriter: c.Writer}
	c.Writer = pooled
	wrapResponsesTerminalWriter(c)
	defer restoreResponsesTerminalWriter(c)
	_, err := c.Writer.WriteString("data: " + `{"type":"response.completed","response":{"id":"resp_1","status":"completed"}}` + "\n\n")
	require.NoError(t, err)
	restoreResponsesTerminalWriter(c)
	require.NotPanics(t, func() {
		require.Equal(t, http.StatusOK, c.Writer.Status())
		require.True(t, c.Writer.Written())
		require.Positive(t, c.Writer.Size())
	})
	pooled.ResponseWriter = nil
}

type terminalStatePooledWriter struct {
	gin.ResponseWriter
}

func (w *terminalStatePooledWriter) Status() int {
	if w.ResponseWriter == nil {
		panic("released writer")
	}
	return w.ResponseWriter.Status()
}

func (w *terminalStatePooledWriter) Written() bool {
	if w.ResponseWriter == nil {
		panic("released writer")
	}
	return w.ResponseWriter.Written()
}

func (w *terminalStatePooledWriter) Size() int {
	if w.ResponseWriter == nil {
		panic("released writer")
	}
	return w.ResponseWriter.Size()
}

func TestOpenAIResponsesTerminalWriterPreservesIncompleteFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	wrapResponsesTerminalWriter(c)
	_, err := c.Writer.WriteString("data: " + `{"type":"response.created","response":{"id":"resp_1"}}` + "\n\n")
	require.NoError(t, err)
	require.False(t, ResponsesTerminalEventWritten(c))
	_, err = c.Writer.WriteString("event: response.failed\ndata: " + `{"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"upstream_error","message":"upstream disconnected"}}}` + "\n\n")
	require.NoError(t, err)
	require.True(t, ResponsesTerminalEventWritten(c))
}
