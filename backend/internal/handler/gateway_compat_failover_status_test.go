package handler

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCompatFailoverExhaustedMatchesNativeHTTPStatus(t *testing.T) {
	h := &GatewayHandler{}
	for _, status := range []int{400, 401, 403, 410, 429, 500, 502, 503, 504, 529} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			lastErr := &service.UpstreamFailoverError{StatusCode: status, ResponseBody: []byte(`{"error":{"message":"Service moved"}}`)}
			native := httptest.NewRecorder()
			nc, _ := gin.CreateTestContext(native)
			nc.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			h.handleFailoverExhausted(nc, lastErr, service.PlatformAnthropic, false)
			responses := httptest.NewRecorder()
			rc, _ := gin.CreateTestContext(responses)
			rc.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			h.handleResponsesFailoverExhausted(rc, lastErr, false)
			chat := httptest.NewRecorder()
			cc, _ := gin.CreateTestContext(chat)
			cc.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			h.handleCCFailoverExhausted(cc, lastErr, false)
			require.Equal(t, native.Code, responses.Code)
			require.Equal(t, native.Code, chat.Code)
		})
	}
}
