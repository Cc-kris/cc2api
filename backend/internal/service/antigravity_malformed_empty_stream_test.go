package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAntigravityMalformedFunctionCallNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name    string
		frames  string
		wantErr bool
	}{
		{"signature only", `data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL","content":{"parts":[{"thought":true,"thoughtSignature":"test-signature"}]}}]}}` + "\n\n", true},
		{"error after parts", `data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read","args":{}}}]}}]}}` + "\n\n" + `data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}}` + "\n\n", true},
		{"error before usage", `data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}}` + "\n\n" + `data: {"response":{"usageMetadata":{"promptTokenCount":1}}}` + "\n\n", true},
		{"normal tool", `data: {"response":{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"read","args":{}}}]}}]}}` + "\n\n", false},
		{"normal text", `data: {"response":{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"hello"}]}}]}}` + "\n\n", false},
	} {
		for _, protocol := range []string{"gemini", "claude"} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}})
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.frames))}
				var err error
				if protocol == "gemini" {
					_, err = svc.handleGeminiStreamToNonStreaming(c, resp, time.Now())
				} else {
					_, err = svc.handleClaudeStreamToNonStreaming(c, resp, time.Now(), "test-model")
				}
				if tc.wantErr {
					require.Error(t, err)
					if protocol == "gemini" {
						var failover *UpstreamFailoverError
						require.ErrorAs(t, err, &failover)
						require.True(t, failover.RetryableOnSameAccount)
					}
					require.Empty(t, recorder.Body.String())
				} else {
					require.NoError(t, err)
					require.Contains(t, recorder.Body.String(), `"content"`)
				}
			})
		}
	}
}

func TestAntigravityMalformedSignatureOnlyStreamTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`data: {"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL","content":{"parts":[{"thought":true,"thoughtSignature":"test-signature"}]}}]}}` + "\n\n"))}
	_, err := svc.handleClaudeStreamingResponse(c, resp, time.Now(), "test-model")
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.True(t, failover.RetryableOnSameAccount)
}
