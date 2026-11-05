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
	"github.com/tidwall/gjson"
)

// V02-T04：流内 response.failed 限流按 429 进入 failover（上游 85a27fae3、37238c098）。

// 拷自上游 37238c098 的 TestOpenAIStreamSemanticStatusHonorsErrorStatusAlias，
// 只保留限流相关样本（401/403/529 属于别的需求，CCAI 的 openAIStreamFailureStatus 不提升）。
func TestOpenAIStreamSemanticStatusHonorsErrorStatusAlias(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		status       int
		wantFailover bool
	}{
		{
			name:         "error.status rate limited",
			body:         `{"type":"error","error":{"code":"server_error","status":429,"message":"upstream is busy"}}`,
			status:       http.StatusTooManyRequests,
			wantFailover: true,
		},
		{
			name:         "response.error.status_code rate limited",
			body:         `{"type":"response.failed","response":{"error":{"code":"server_error","status_code":429,"message":"upstream is busy"}}}`,
			status:       http.StatusTooManyRequests,
			wantFailover: true,
		},
		{
			name:         "rate_limit_exceeded code",
			body:         `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Concurrency limit exceeded"}}}`,
			status:       http.StatusTooManyRequests,
			wantFailover: true,
		},
		{
			// CCAI 的 nonRetryableMarkers 含 invalid_request，429 优先保证仍会 failover。
			name:         "invalid_request_error with status 429",
			body:         `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","status":429,"message":"too many requests"}}}`,
			status:       http.StatusTooManyRequests,
			wantFailover: true,
		},
		{
			name:   "401 is not promoted",
			body:   `{"type":"response.failed","response":{"error":{"code":"server_error","status":401,"message":"upstream rejected the key"}}}`,
			status: http.StatusBadGateway,
			// 保持 CCAI 原有判定：无 non-retryable 标记即 failover。
			wantFailover: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte(tt.body)
			message := extractOpenAISSEErrorMessage(payload)
			require.Equal(t, tt.status, openAIStreamFailureStatus(payload, message))
			require.Equal(t, tt.wantFailover, openAIStreamFailedEventShouldFailover(payload, message))
		})
	}
}

// 拷自上游 37238c098：上游把 status 报成 5xx 时仍然是通用上游故障，不冒充限流。
func TestOpenAIStreamErrorStatusAliasKeepsGeneric5xxUnclassified(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_error","status":500,"type":"server_error","message":"rv_shape_invalid: temporary response validation failure after partial output; retry your request if your client can recover partial output."},"id":"resp_0","object":"response","status":"failed"},"sequence_number":80}`)
	message := extractOpenAISSEErrorMessage(payload)

	require.Equal(t, http.StatusBadGateway, openAIStreamFailureStatus(payload, message))
	require.True(t, openAIStreamFailedEventShouldFailover(payload, message))
}

func TestOpenAIStreamFailureStatusInvalidPayloadIs502(t *testing.T) {
	require.Equal(t, http.StatusBadGateway, openAIStreamFailureStatus(nil, "rate_limit"))
	require.Equal(t, http.StatusBadGateway, openAIStreamFailureStatus([]byte("not json"), "rate_limit"))
}

func newOpenAIStreamRateLimitTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
	}}
}

func openAIStreamRateLimitSSE(errorJSON string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"id":"resp_1","status":"failed","error":` + errorJSON + `}}`,
		"",
	}, "\n")))
}

// 拷自上游 85a27fae3：API Key 池模式，首包前限流 → 429 failover，带 Retry-After，可同号重试。
func TestOpenAIStreamingResponseFailedBeforeOutputRateLimitUsesPoolRetryPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOpenAIStreamRateLimitTestService()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       openAIStreamRateLimitSSE(`{"code":"rate_limit_exceeded","message":"Concurrency limit exceeded for account, please retry later"}`),
		Header: http.Header{
			"X-Request-Id": []string{"rid-rate-limit-failed"},
			"Retry-After":  []string{"1"},
		},
	}
	account := &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Name: "pool-account",
		Credentials: map[string]any{
			"pool_mode":                    true,
			"pool_mode_retry_count":        float64(1),
			"pool_mode_retry_status_codes": []any{float64(http.StatusTooManyRequests)},
		},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.Equal(t, "1", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.Equal(t, "rate_limit_error", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
	require.Contains(t, string(failoverErr.ResponseBody), "Concurrency limit exceeded")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())

	opsVal, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	opsEvents, ok := opsVal.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, opsEvents)
	require.Equal(t, http.StatusTooManyRequests, opsEvents[len(opsEvents)-1].UpstreamStatusCode)
}

// AC38：OAuth 非透传首包前 429（含 invalid_request_error + status 429 样本），
// failover 状态码 429、不写账号冷却（上游 85a27fae3 的 DoesNotBlockAccountScheduling）。
func TestOpenAIStreamingResponseFailedRateLimitOAuthDoesNotBlockAccountScheduling(t *testing.T) {
	samples := map[string]string{
		"rate_limit_exceeded":     `{"code":"rate_limit_exceeded","message":"Concurrency limit exceeded for account, please retry later"}`,
		"invalid_request_with429": `{"type":"invalid_request_error","status":429,"message":"too many requests"}`,
	}
	for name, errJSON := range samples {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			svc := newOpenAIStreamRateLimitTestService()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       openAIStreamRateLimitSSE(errJSON),
				Header: http.Header{
					"X-Codex-Primary-Used-Percent":        []string{"12"},
					"X-Codex-Primary-Reset-After-Seconds": []string{"604800"},
					"Retry-After":                         []string{"1"},
				},
			}
			account := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Name: "oauth-account"}

			_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
			require.Equal(t, "rate_limit_error", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Empty(t, rec.Body.String())
		})
	}
}

// 透传流同样按 429 进入 failover。
func TestOpenAIStreamingPassthroughFailedBeforeOutputRateLimitReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOpenAIStreamRateLimitTestService()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       openAIStreamRateLimitSSE(`{"code":"rate_limit_exceeded","message":"slow down"}`),
		Header:     http.Header{"Retry-After": []string{"2"}},
	}
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "2", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

// AC39：非限流的 response.failed 保持 502、upstream_error。
func TestOpenAIStreamingResponseFailedNonRateLimitKeeps502(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOpenAIStreamRateLimitTestService()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       openAIStreamRateLimitSSE(`{"code":"server_error","status":500,"message":"temporary failure"}`),
		Header:     http.Header{},
	}
	account := &Account{ID: 13, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, "upstream_error", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
}

// AC65：任意错误换号关闭时，OAuth 流内 429 的 failover 状态码进入 429 风暴提前终止判断；
// 改前（502）不会。
func TestOpenAIStreamRateLimitFailoverFeedsOAuth429StormStop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOpenAIStreamRateLimitTestService()
	account := &Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for i := 0; i < openAIOAuth429StormThreshold; i++ {
		svc.recordOpenAIOAuth429()
	}

	run := func(errJSON string) *UpstreamFailoverError {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Body: openAIStreamRateLimitSSE(errJSON), Header: http.Header{}}
		_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		return failoverErr
	}

	state := OpenAIOAuth429FailoverState{}
	rateLimited := run(`{"code":"rate_limit_exceeded","message":"slow down"}`)
	require.True(t, svc.ShouldStopOpenAIOAuth429Failover(account, rateLimited.StatusCode, 1, &state))
	generic := run(`{"code":"server_error","message":"temporary failure"}`)
	require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, generic.StatusCode, 1, &state))
}
