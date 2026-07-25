package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GrokCountTokens handles Anthropic-compatible count_tokens requests locally.
// It deliberately avoids account selection, billing checks, credentials, and upstream calls.
func (h *OpenAIGatewayHandler) GrokCountTokens(c *gin.Context) {
	body, err := readRequestBodyWithDiagnostics(c)
	if err != nil {
		writeRequestBodyReadError(c, http.StatusBadRequest, "invalid_request_error", err, func(status int, errType string, message string) {
			h.anthropicErrorResponse(c, status, errType, message)
		})
		return
	}
	if len(body) == 0 {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	parsedReq, err := service.ParseGatewayRequest(body, domain.PlatformAnthropic)
	if err != nil {
		requestLogger(c, "handler.openai_gateway.grok_count_tokens").Warn("grok_count_tokens.parse_failed", zap.Error(err))
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	if parsedReq.Model == "" {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	estimated, err := service.EstimateGrokCountTokens(parsedReq.Body)
	if err != nil {
		requestLogger(c, "handler.openai_gateway.grok_count_tokens").Warn("grok_count_tokens.local_estimate_failed", zap.Error(err))
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	setOpsRequestContext(c, parsedReq.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(false, false)))
	c.JSON(http.StatusOK, gin.H{"input_tokens": estimated})
}
