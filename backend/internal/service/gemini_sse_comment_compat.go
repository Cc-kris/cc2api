package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// geminiClientRejectsSSEComments identifies Google GenAI SDKs that treat an
// SSE comment as invalid response data instead of ignoring it.
func geminiClientRejectsSSEComments(clientHint string) bool {
	hint := strings.ToLower(strings.TrimSpace(clientHint))
	if !strings.Contains(hint, "google-genai-sdk/") {
		return false
	}
	return strings.Contains(hint, "gl-go/") || strings.Contains(hint, "gl-python/")
}

func downstreamRejectsSSEComments(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return geminiClientRejectsSSEComments(c.GetHeader("User-Agent")) ||
		geminiClientRejectsSSEComments(c.GetHeader("X-Goog-Api-Client"))
}
