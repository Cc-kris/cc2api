//go:build unit

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
)

// Keep the legacy tests intact, while checking the unaffected contracts with
// inputs/assertions consistent with the explicitly changed V04 behavior.
func TestV043NonEmptyCompletedPreservesHeaderOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{ResponseHeaders: config.ResponseHeaderConfig{Enabled: false}}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
		"Cache-Control": {"upstream"}, "X-Request-Id": {"req-123"}, "Content-Type": {"application/custom"},
	}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	require.NoError(t, err)
	require.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "req-123", rec.Header().Get("X-Request-Id"))
	require.Contains(t, rec.Body.String(), "hello")
}

func TestV043Instructions400PreservesSafeDiagnosticLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?trace=1", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("OpenAI-Beta", "assistants=v2")
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}, "x-request-id": {"rid-upstream"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Missing required parameter: 'instructions'","type":"invalid_request_error","param":"instructions","code":"missing_required_parameter"}}`))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 1001, Name: "diagnostic fixture", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}, Status: StatusActive, Schedulable: true}
	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.1-codex","stream":false,"input":[{"type":"text","text":"hello"}],"prompt_cache_key":"pc-forward","access_token":"secret-token"}`))
	require.Error(t, err)
	require.Equal(t, 400, rec.Code)
	require.Contains(t, rec.Body.String(), "missing_required_parameter")
	require.Contains(t, rec.Body.String(), `"param":"instructions"`)
	require.True(t, logSink.ContainsMessageAtLevel("OpenAI 上游返回 Instructions are required，已记录请求详情用于排查", "warn"))
	require.True(t, logSink.ContainsFieldValue("request_user_agent", "codex_cli_rs/0.1.0"))
	require.True(t, logSink.ContainsFieldValue("request_model", "gpt-5.1-codex"))
	require.True(t, logSink.ContainsFieldValue("request_headers", "openai-beta"))
	require.True(t, logSink.ContainsField("request_body_size"))
	require.False(t, logSink.ContainsField("request_body_preview"))
}

func TestV043HTTP529PassthroughFailoverWithoutCooldown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		t.Run(accountType, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 529, Header: http.Header{"Content-Type": {"application/json"}, "x-request-id": {"rid-failover"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"server overloaded","type":"server_error"}}`))}}
			repo := &openAIPassthroughFailoverRepo{}
			rate := &RateLimitService{accountRepo: repo, cfg: &config.Config{RateLimit: config.RateLimitConfig{OverloadCooldownMinutes: 10}}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, rateLimitService: rate}
			account := &Account{ID: 123, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test", "access_token": "oauth-token", "chatgpt_account_id": "fixture"}, Extra: map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true}
			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.2","stream":false,"instructions":"fixture","input":"hi"}`))
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, 529, failure.StatusCode)
			require.False(t, c.Writer.Written())
			require.Empty(t, repo.overloadCalls)
			require.Empty(t, repo.rateLimitCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			value, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			events, ok := value.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.NotEmpty(t, events)
			event := events[len(events)-1]
			require.True(t, event.Passthrough)
			require.Equal(t, "failover", event.Kind)
			require.Equal(t, 529, event.UpstreamStatusCode)
		})
	}
}
