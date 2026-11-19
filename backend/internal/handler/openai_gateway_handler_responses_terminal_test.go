package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The native Responses service already wrote its terminal event. Forward then
// returns the stream failure and the outer handler must not append a second,
// generic response.failed (AC13 captures showed created, failed, failed).
func TestEnsureForwardErrorResponse_DoesNotDuplicateWrittenTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
	c.Set("openai_responses_terminal_event_written", true)
	_, err := c.Writer.WriteString("data: " + `{"type":"response.created","response":{"id":"resp_w4","status":"in_progress","created_at":1794848626}}` + "\n\n")
	require.NoError(t, err)
	_, err = c.Writer.WriteString("event: response.failed\ndata: " + `{"type":"response.failed","sequence_number":1,"response":{"id":"resp_w4","object":"response","status":"failed","created_at":1794848626,"output":[],"error":{"code":"content_policy","message":"W4 synthetic error"}}}` + "\n\n")
	require.NoError(t, err)
	before := w.Body.String()
	require.True(t, service.ResponsesTerminalEventWritten(c))

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, true)

	require.True(t, wrote, "caller still returns normally")
	require.Equal(t, before, w.Body.String(), "no second terminal event may be appended")
	require.Equal(t, 1, strings.Count(w.Body.String(), `"type":"response.failed"`))
	require.Contains(t, w.Body.String(), `"code":"content_policy"`, "the original failure detail is retained")
}

func TestEnsureForwardErrorResponse_KeepsFallbackBeforeTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
	_, _ = c.Writer.WriteString(":\n\n")

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, 1, strings.Count(w.Body.String(), `"type":"response.failed"`))
	var payload map[string]any
	for _, line := range strings.Split(w.Body.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		require.NoError(t, json.Unmarshal([]byte(data), &payload))
	}
	errorBody, ok := payload["response"].(map[string]any)["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorBody["code"])
	assert.Equal(t, "Upstream request failed", errorBody["message"])
}
