package service

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strings"
)

func openAICodexClientSessionID(c *gin.Context, body []byte) string {
	if c != nil && c.Request != nil {
		if id := strings.TrimSpace(c.GetHeader("session-id")); id != "" {
			return id
		}
	}
	return explicitOpenAIRequestSessionID(c, body)
}

// WriteOpenAICompactSSEFailureMessage terminates a compact stream whose HTTP
// status was already committed by a keepalive comment.
func WriteOpenAICompactSSEFailureMessage(c *gin.Context, status int, errType, message string) {
	writeOpenAICompactSSEFailureMessage(c, status, errType, message)
}

const openAIWSOriginalExecutionScopeKey = "openai_ws_original_execution_scope"
const openAIStreamKeepaliveBytesKey = "openai_stream_keepalive_bytes"

func recordOpenAIStreamKeepaliveBytes(c *gin.Context, n int) {
	if c == nil || n <= 0 {
		return
	}
	previous := c.GetInt(openAIStreamKeepaliveBytesKey)
	c.Set(openAIStreamKeepaliveBytesKey, previous+n)
}

// CaptureOpenAIWSExecutionScope records only the declared execution identity,
// before normalization can remove client_metadata. Content-derived affinity is
// intentionally never used for state sharing or preemption.
func CaptureOpenAIWSExecutionScope(c *gin.Context, body []byte) {
	if c != nil {
		scope, _ := resolveOpenAIWSExecutionScope(c, body, getAPIKeyIDFromContext(c))
		c.Set(openAIWSOriginalExecutionScopeKey, scope)
	}
}

func openAIHTTPWSExecutionScope(c *gin.Context, body []byte) string {
	if c != nil {
		if value, ok := c.Get(openAIWSOriginalExecutionScopeKey); ok {
			scope, _ := value.(string)
			return scope
		}
	}
	scope, _ := resolveOpenAIWSExecutionScope(c, body, getAPIKeyIDFromContext(c))
	return scope
}

func writeOpenAIJSONResponse(c *gin.Context, status int, value any) {
	if StopOpenAICompactSSEKeepaliveCommitted(c) {
		body, _ := json.Marshal(value)
		writeOpenAICompactSSEFailure(c, status, body)
		return
	}
	c.JSON(status, value)
}

func firstOpenAIResponseAccount(accounts []*Account) *Account {
	if len(accounts) > 0 {
		return accounts[0]
	}
	return nil
}
