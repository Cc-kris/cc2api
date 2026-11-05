package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V02-T04：流内 response.failed 限流按 429 进入 failover。

// openAIHTTPPassthroughSSEStatusUpstream 按账号返回流内失败事件或正常完成。
type openAIHTTPPassthroughSSEStatusUpstream struct {
	service.HTTPUpstream
	mu         sync.Mutex
	accountIDs []int64
	failJSON   map[int64]string // 账号 → response.failed 的 error JSON；缺省返回 completed
}

func (u *openAIHTTPPassthroughSSEStatusUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.mu.Unlock()
	var body string
	if errJSON, ok := u.failJSON[accountID]; ok {
		body = strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_rate_limited"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","response":{"id":"resp_rate_limited","status":"failed","error":` + errJSON + `}}`,
			"",
		}, "\n")
	} else {
		body = strings.Join([]string{
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_ok","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"Retry-After":  []string{"1"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (u *openAIHTTPPassthroughSSEStatusUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
}

func runOpenAIPassthroughSSEStatusCase(t *testing.T, cfg *config.Config, group *service.Group, accounts []service.Account, upstream *openAIHTTPPassthroughSSEStatusUpstream) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false

	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)
	gatewaySvc := service.NewOpenAIGatewayService(
		&openAIWSSilentRetryAccountRepoStub{accounts: accounts},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheSvc, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(
		gatewaySvc,
		service.NewConcurrencyService(nil),
		billingCacheSvc,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, cfg,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol","input":"hello","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 1804, GroupID: &group.ID,
		User:  &service.User{ID: 1704, Status: service.StatusActive},
		Group: group,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1704, Concurrency: 0})

	h.Responses(c)
	return rec
}

func passthroughAPIKeyAccount(id int64, priority int, creds map[string]any) service.Account {
	base := map[string]any{"api_key": "sk-test", "base_url": "https://api.example.test"}
	for k, v := range creds {
		base[k] = v
	}
	return service.Account{
		ID: id, Name: "acct", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: priority,
		Credentials: base,
		Extra:       map[string]any{"openai_passthrough": true},
	}
}

const sseRateLimitErrorJSON = `{"code":"rate_limit_exceeded","message":"Concurrency limit exceeded for account, please retry later"}`

// AC38：拷自上游 85a27fae3。API Key 透传 + 池模式 429：同账号重试一次，最终 429 带 Retry-After。
func TestOpenAIResponses_APIKeyPassthroughSSERateLimitUsesConfiguredPoolRetry(t *testing.T) {
	group := &service.Group{ID: 4204, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	accounts := []service.Account{passthroughAPIKeyAccount(9912, 1, map[string]any{
		"pool_mode":                    true,
		"pool_mode_retry_count":        float64(1),
		"pool_mode_retry_status_codes": []any{float64(http.StatusTooManyRequests)},
	})}
	upstream := &openAIHTTPPassthroughSSEStatusUpstream{failJSON: map[int64]string{9912: sseRateLimitErrorJSON}}
	cfg := &config.Config{}
	cfg.Gateway.MaxAccountSwitches = 1

	rec := runOpenAIPassthroughSSEStatusCase(t, cfg, group, accounts, upstream)

	require.Equal(t, []int64{9912, 9912}, upstream.calls())
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	require.Equal(t, "rate_limit_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Equal(t, "Upstream rate limit exceeded, please retry later", gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
}

// AC65：任意错误换号开启时，流内 429 是确定性错误，不同号重试、直接换号；
// 对照组：流内 502 按 FailoverSameAccountRetries 先同号重试。
func TestOpenAIResponses_StreamRateLimitAnyErrorFailoverSwitchesWithoutSameAccountRetry(t *testing.T) {
	cases := []struct {
		name      string
		failJSON  string
		wantCalls []int64
	}{
		{name: "stream_429_switches_directly", failJSON: sseRateLimitErrorJSON, wantCalls: []int64{9921, 9922}},
		{name: "stream_502_retries_same_account_first", failJSON: `{"code":"server_error","message":"temporary failure"}`, wantCalls: []int64{9921, 9921, 9922}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			group := &service.Group{ID: 4205, Platform: service.PlatformOpenAI, Status: service.StatusActive, FailoverOnAnyError: true}
			accounts := []service.Account{
				passthroughAPIKeyAccount(9921, 0, nil),
				passthroughAPIKeyAccount(9922, 1, nil),
			}
			upstream := &openAIHTTPPassthroughSSEStatusUpstream{failJSON: map[int64]string{9921: tc.failJSON}}
			cfg := &config.Config{}
			cfg.Gateway.MaxAccountSwitches = 3
			cfg.Gateway.FailoverOnAnyError = true
			cfg.Gateway.FailoverSameAccountRetries = 1

			rec := runOpenAIPassthroughSSEStatusCase(t, cfg, group, accounts, upstream)

			require.Equal(t, tc.wantCalls, upstream.calls())
			require.Equal(t, http.StatusOK, rec.Code)
		})
	}
}
