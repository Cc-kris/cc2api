package service

import (
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

func TestGeminiClientRejectsSSEComments(t *testing.T) {
	for _, tc := range []struct {
		name string
		hint string
		want bool
	}{
		{"go-genai", "google-genai-sdk/1.71.0 gl-go/go1.28", true},
		{"python-genai", "google-genai-sdk/1.20.0 gl-python/3.12.4", true},
		{"js-genai", "google-genai-sdk/1.9.0 gl-node/22.3.0", false},
		{"gemini-cli", "GeminiCLI/0.60.0 (darwin; arm64)", false},
		{"curl", "curl/8.7.1", false},
		{"empty", "", false},
		{"case-insensitive", "Google-GenAI-SDK/1.0.0 GL-Go/go1.27", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, geminiClientRejectsSSEComments(tc.hint))
		})
	}
}

func TestDownstreamRejectsSSECommentsReadsBothHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{Header: http.Header{}}
	require.False(t, downstreamRejectsSSEComments(c))
	c.Request.Header.Set("X-Goog-Api-Client", "google-genai-sdk/1.71.0 gl-go/go1.28")
	require.True(t, downstreamRejectsSSEComments(c))
	c.Request.Header.Del("X-Goog-Api-Client")
	c.Request.Header.Set("User-Agent", "google-genai-sdk/1.71.0 gl-go/go1.28")
	require.True(t, downstreamRejectsSSEComments(c))
	require.False(t, downstreamRejectsSSEComments(nil))
}

func TestAntigravityGeminiStreamSkipsCommentKeepaliveForGoGenai(t *testing.T) {
	for _, tc := range []struct {
		userAgent string
		comments  bool
	}{{"curl/8.7.1", true}, {"google-genai-sdk/1.71.0 gl-go/go1.28", false}} {
		t.Run(tc.userAgent, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamKeepaliveInterval: 1}})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Request.Header.Set("User-Agent", tc.userAgent)
			reader, writer := io.Pipe()
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
			done := make(chan error, 1)
			go func() {
				_, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
				done <- err
			}()
			_, err := io.WriteString(writer, `data: {"response":{"responseId":"resp_1","candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1}}}`+"\n\n")
			require.NoError(t, err)
			time.Sleep(1200 * time.Millisecond)
			require.NoError(t, writer.Close())
			require.NoError(t, <-done)
			require.NoError(t, reader.Close())
			out := recorder.Body.String()
			require.Contains(t, out, `"text":"partial"`)
			require.Equal(t, tc.comments, strings.Contains(out, ":\n\n"))
		})
	}
}
