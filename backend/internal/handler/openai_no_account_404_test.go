package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V02-T28 / AC28（404 部分）：OpenAI 端点选号失败时，没有账号支持该模型返回 404
// model_not_found，账号都在限流返回 503，上游都不会收到请求。

func newOpenAINoAccountTestHandler(t *testing.T, accounts []service.Account, upstream *openAIHTTPPassthroughSSEStatusUpstream) *OpenAIGatewayHandler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.MaxAccountSwitches = 1
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)
	// Model diagnosis now queries persistent group membership rather than
	// the transient schedulable list; these fixtures belong to the key's group.
	for i := range accounts {
		accounts[i].GroupIDs = []int64{4301}
	}
	gatewaySvc := service.NewOpenAIGatewayService(
		&openAIWSSilentRetryAccountRepoStub{accounts: accounts},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheSvc, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	return NewOpenAIGatewayHandler(
		gatewaySvc,
		service.NewConcurrencyService(nil),
		billingCacheSvc,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, cfg,
	)
}

func newOpenAINoAccountTestContext(path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(4301)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 1901, GroupID: &groupID,
		User:  &service.User{ID: 1902, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1902, Concurrency: 0})
	return c, rec
}

func restrictedOpenAIAccount(id int64) service.Account {
	return service.Account{
		ID: id, Name: "restricted", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"base_url":      "https://api.example.test",
			"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
		},
	}
}

func TestOpenAIResponses_UnsupportedModelReturns404(t *testing.T) {
	upstream := &openAIHTTPPassthroughSSEStatusUpstream{}
	h := newOpenAINoAccountTestHandler(t, []service.Account{restrictedOpenAIAccount(9931)}, upstream)
	c, rec := newOpenAINoAccountTestContext("/openai/v1/responses", `{"model":"o9-mini","input":"hi","stream":false}`)

	h.Responses(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "model_not_found", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), `"o9-mini"`)
	require.Empty(t, upstream.calls())
}

func TestOpenAIChatCompletions_UnsupportedModelReturns404(t *testing.T) {
	upstream := &openAIHTTPPassthroughSSEStatusUpstream{}
	h := newOpenAINoAccountTestHandler(t, []service.Account{restrictedOpenAIAccount(9932)}, upstream)
	c, rec := newOpenAINoAccountTestContext("/v1/chat/completions", `{"model":"o9-mini","messages":[{"role":"user","content":"hi"}]}`)

	h.ChatCompletions(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "model_not_found", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Empty(t, upstream.calls())
}

// 支持该模型的账号都在限流：仍返回 503（客户端会重试）。
func TestOpenAIResponses_AllRateLimitedStays503(t *testing.T) {
	upstream := &openAIHTTPPassthroughSSEStatusUpstream{}
	account := restrictedOpenAIAccount(9933)
	reset := time.Now().Add(time.Hour)
	account.RateLimitResetAt = &reset
	h := newOpenAINoAccountTestHandler(t, []service.Account{account}, upstream)
	c, rec := newOpenAINoAccountTestContext("/openai/v1/responses", `{"model":"gpt-5.4","input":"hi","stream":false}`)

	h.Responses(c)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "api_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Empty(t, upstream.calls())
}
