package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

const opsRequestBodyDiagnosticsKey = "ops_request_body_diagnostics"

type requestBodyDiagnostics struct {
	Kind           string `json:"kind"`
	ContentLength  int64  `json:"content_length"`
	BytesRead      int64  `json:"bytes_read"`
	ReadDurationMs int64  `json:"read_duration_ms"`
	Encoding       string `json:"encoding,omitempty"`
	Cause          string `json:"cause,omitempty"`
}

func readRequestBodyWithDiagnostics(c *gin.Context) ([]byte, error) {
	startedAt := time.Now()
	body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		setOpsRequestBodyDiagnostics(c, err, time.Since(startedAt))
	}
	return body, err
}

func requestBodyReadClientMessage(err error) string {
	if err == nil {
		return "Failed to read request body"
	}
	if _, ok := extractMaxBytesError(err); ok {
		return "Failed to read request body"
	}
	info, ok := httputil.RequestBodyReadErrorInfo(err)
	if !ok || info == nil {
		return "Failed to read request body"
	}
	switch info.Kind {
	case httputil.RequestBodyReadClientDisconnected:
		return "Request body upload was interrupted before completion"
	case httputil.RequestBodyReadIncompleteBody:
		return "Request body is incomplete"
	case httputil.RequestBodyReadTimeout:
		return "Request body upload timed out"
	case httputil.RequestBodyUnsupportedEncoding:
		return "Unsupported request Content-Encoding"
	case httputil.RequestBodyDecodeFailed:
		return "Failed to decode request body"
	default:
		return "Failed to read request body"
	}
}

func setOpsRequestBodyDiagnostics(c *gin.Context, err error, readDuration time.Duration) {
	if c == nil || err == nil {
		return
	}
	diagnostics := requestBodyDiagnostics{
		Kind:           "read_failed",
		ContentLength:  -1,
		ReadDurationMs: readDuration.Milliseconds(),
	}
	if c.Request != nil {
		diagnostics.ContentLength = c.Request.ContentLength
	}
	if maxErr, ok := extractMaxBytesError(err); ok && maxErr != nil {
		diagnostics.Kind = "too_large"
		diagnostics.Cause = sanitizeRequestBodyDiagnosticCause(err.Error())
		c.Set(opsRequestBodyDiagnosticsKey, diagnostics)
		return
	}
	if info, ok := httputil.RequestBodyReadErrorInfo(err); ok && info != nil {
		diagnostics.Kind = string(info.Kind)
		diagnostics.BytesRead = info.BytesRead
		diagnostics.ContentLength = info.ContentLength
		diagnostics.Encoding = strings.TrimSpace(info.Encoding)
		if info.Err != nil {
			diagnostics.Cause = sanitizeRequestBodyDiagnosticCause(info.Err.Error())
		}
	} else {
		diagnostics.Cause = sanitizeRequestBodyDiagnosticCause(err.Error())
	}
	c.Set(opsRequestBodyDiagnosticsKey, diagnostics)
}

func sanitizeRequestBodyDiagnosticCause(cause string) string {
	cause = strings.TrimSpace(cause)
	if cause == "" {
		return ""
	}
	const maxLen = 512
	if len(cause) > maxLen {
		cause = cause[:maxLen]
	}
	return cause
}

func requestBodyDiagnosticsErrorBody(responseBody string, diagnostics any) string {
	if diagnostics == nil {
		return responseBody
	}
	payload := map[string]any{
		"response":    json.RawMessage(responseBody),
		"diagnostics": diagnostics,
	}
	if !json.Valid([]byte(responseBody)) {
		payload["response"] = responseBody
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return responseBody
	}
	return string(out)
}

func writeRequestBodyReadError(c *gin.Context, status int, errType string, err error, write func(status int, errType string, message string)) {
	if write == nil {
		return
	}
	if maxErr, ok := extractMaxBytesError(err); ok {
		write(http.StatusRequestEntityTooLarge, errType, buildBodyTooLargeMessage(maxErr.Limit))
		return
	}
	write(status, errType, requestBodyReadClientMessage(err))
}
