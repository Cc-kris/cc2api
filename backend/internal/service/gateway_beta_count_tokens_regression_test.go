package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Haiku 5.5 validates legacy enabled thinking in ParseGatewayRequest, so keep
// these beta-sanitizer fixtures on a model whose accepted controls are unchanged.
const w8BetaGatedBody = `{"model":"","messages":[{"role":"user","content":"hello"}],"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]},"thinking":{"type":"enabled","budget_tokens":1024,"block_binding":{"prefix_mismatch_behavior":"drop_block"}},"fallbacks":"default","fallback_credit_token":"synthetic","tools":[{"name":"deferred","defer_loading":true,"cache_control":{"type":"ephemeral"}},{"name":"ordinary","cache_control":{"type":"ephemeral","ttl":"1h"}}]}`

func w8BetaContext(path, beta string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	if beta != "" {
		c.Request.Header.Set("anthropic-beta", beta)
	}
	return c
}

func w8BetaAccount() *Account {
	return &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "synthetic"}}
}

func w8AssertBetaControls(t *testing.T, body []byte, finalBeta string, wantContext, wantBinding bool) {
	t.Helper()
	require.Equal(t, containsBetaToken(finalBeta, claude.BetaContextManagement), wantContext)
	require.Equal(t, containsBetaToken(finalBeta, claude.BetaThinkingBindingControls), wantBinding)
	require.Equal(t, wantContext, gjson.GetBytes(body, "context_management").Exists())
	require.Equal(t, wantBinding, gjson.GetBytes(body, "thinking.block_binding").Exists())
	require.Equal(t, "enabled", gjson.GetBytes(body, "thinking.type").String())
	require.Equal(t, int64(1024), gjson.GetBytes(body, "thinking.budget_tokens").Int())
	// Fallback stripping remains independent from context/thinking beta controls.
	require.False(t, gjson.GetBytes(body, "fallbacks").Exists())
	require.False(t, gjson.GetBytes(body, "fallback_credit_token").Exists())
}

func w8AssertDeferredCacheControls(t *testing.T, body []byte) {
	t.Helper()
	// Deferred-tool cache cleanup remains intact, including on count_tokens.
	require.False(t, gjson.GetBytes(body, "tools.0.cache_control").Exists())
	require.True(t, gjson.GetBytes(body, "tools.0.defer_loading").Bool())
	require.Equal(t, "1h", gjson.GetBytes(body, "tools.1.cache_control.ttl").String())
}

func TestSanitizeAnthropicBodyForBetaTokens_AC19FinalBetaMatrix(t *testing.T) {
	for _, tt := range []struct {
		name        string
		beta        string
		wantContext bool
		wantBinding bool
	}{
		{name: "missing both", beta: claude.BetaStructuredOutputs},
		{name: "context only", beta: claude.BetaContextManagement, wantContext: true},
		{name: "binding only", beta: claude.BetaThinkingBindingControls, wantBinding: true},
		{name: "both with whitespace", beta: " " + claude.BetaContextManagement + " , " + claude.BetaThinkingBindingControls + " ", wantContext: true, wantBinding: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, changed := sanitizeAnthropicBodyForBetaTokens([]byte(w8BetaGatedBody), tt.beta)
			require.Equal(t, !tt.wantContext || !tt.wantBinding || !containsBetaToken(tt.beta, claude.BetaServerSideFallback), changed)
			w8AssertBetaControls(t, out, tt.beta, tt.wantContext, tt.wantBinding)
		})
	}
}

func TestAnthropicRoutes_AC19UseFinalOutgoingBeta(t *testing.T) {
	for _, tt := range []struct {
		name        string
		beta        string
		wantContext bool
		wantBinding bool
	}{
		{name: "missing", beta: claude.BetaStructuredOutputs},
		{name: "correct", beta: claude.BetaContextManagement + "," + claude.BetaThinkingBindingControls, wantContext: true, wantBinding: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, route := range []struct {
				name  string
				build func(*gin.Context, *GatewayService, *Account) (*http.Request, error)
			}{
				{name: "messages API key", build: func(c *gin.Context, svc *GatewayService, account *Account) (*http.Request, error) {
					return svc.buildUpstreamRequest(context.Background(), c, account, []byte(w8BetaGatedBody), "synthetic", "apikey", "", false, false)
				}},
				{name: "messages API key passthrough", build: func(c *gin.Context, svc *GatewayService, account *Account) (*http.Request, error) {
					return svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(w8BetaGatedBody), "synthetic")
				}},
				{name: "count_tokens API key", build: func(c *gin.Context, svc *GatewayService, account *Account) (*http.Request, error) {
					return svc.buildCountTokensRequest(context.Background(), c, account, []byte(w8BetaGatedBody), "synthetic", "apikey", "", false)
				}},
				{name: "count_tokens API key passthrough", build: func(c *gin.Context, svc *GatewayService, account *Account) (*http.Request, error) {
					return svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(w8BetaGatedBody), "synthetic")
				}},
			} {
				t.Run(route.name, func(t *testing.T) {
					c := w8BetaContext("/v1/messages", tt.beta)
					svc := &GatewayService{cfg: &config.Config{}}
					req, err := route.build(c, svc, w8BetaAccount())
					require.NoError(t, err)
					out := readRequestBodyForTest(t, req)
					w8AssertBetaControls(t, out, getHeaderRaw(req.Header, "anthropic-beta"), tt.wantContext, tt.wantBinding)
					w8AssertDeferredCacheControls(t, out)
				})
			}
		})
	}
}

func TestAnthropicRoutes_AC19PolicyDropsFinalBetaBeforeSanitize(t *testing.T) {
	raw, err := json.Marshal(&BetaPolicySettings{Rules: []BetaPolicyRule{
		{BetaToken: claude.BetaContextManagement, Action: BetaPolicyActionFilter, Scope: BetaPolicyScopeAll},
		{BetaToken: claude.BetaThinkingBindingControls, Action: BetaPolicyActionFilter, Scope: BetaPolicyScopeAll},
	}})
	require.NoError(t, err)
	beta := claude.BetaContextManagement + "," + claude.BetaThinkingBindingControls
	for _, countTokens := range []bool{false, true} {
		t.Run(map[bool]string{false: "messages", true: "count_tokens"}[countTokens], func(t *testing.T) {
			c := w8BetaContext("/v1/messages", beta)
			svc := &GatewayService{cfg: &config.Config{}, settingService: NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{SettingKeyBetaPolicySettings: string(raw)}}, &config.Config{})}
			account := w8BetaAccount()
			var req *http.Request
			if countTokens {
				req, err = svc.buildCountTokensRequest(context.Background(), c, account, []byte(w8BetaGatedBody), "synthetic", "apikey", "", false)
			} else {
				req, err = svc.buildUpstreamRequest(context.Background(), c, account, []byte(w8BetaGatedBody), "synthetic", "apikey", "", false, false)
			}
			require.NoError(t, err)
			require.NotContains(t, getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaContextManagement)
			require.NotContains(t, getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaThinkingBindingControls)
			out := readRequestBodyForTest(t, req)
			require.False(t, gjson.GetBytes(out, "context_management").Exists())
			require.False(t, gjson.GetBytes(out, "thinking.block_binding").Exists())
		})
	}
}

func TestAnthropicRoutes_AC19OAuthSyntheticFinalBeta(t *testing.T) {
	account := &Account{ID: 801, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "synthetic"}}
	for _, tt := range []struct {
		name        string
		mimic       bool
		beta        string
		wantContext bool
		wantBinding bool
	}{
		{name: "mimic injects both final tokens", mimic: true, beta: claude.BetaContextManagement, wantContext: true, wantBinding: true},
		{name: "native client without tokens", beta: claude.BetaStructuredOutputs},
		{name: "native client with tokens", beta: claude.BetaContextManagement + "," + claude.BetaThinkingBindingControls, wantContext: true, wantBinding: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := w8BetaContext("/v1/messages", tt.beta)
			svc := &GatewayService{cfg: &config.Config{}}
			req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(w8BetaGatedBody), "synthetic", "oauth", "", false, tt.mimic)
			require.NoError(t, err)
			finalBeta := getHeaderRaw(req.Header, "anthropic-beta")
			require.Equal(t, tt.wantContext, containsBetaToken(finalBeta, claude.BetaContextManagement))
			require.Equal(t, tt.wantBinding, containsBetaToken(finalBeta, claude.BetaThinkingBindingControls))
			out := readRequestBodyForTest(t, req)
			w8AssertBetaControls(t, out, finalBeta, tt.wantContext, tt.wantBinding)
			w8AssertDeferredCacheControls(t, out)
		})
	}
}

func TestAnthropicRoutes_AC19VertexUsesFilteredBeta(t *testing.T) {
	// Vertex does not accept the binding token, even when the client sends it.
	vertexBeta := filterVertexBetaTokens(claude.BetaContextManagement+","+claude.BetaThinkingBindingControls, nil)
	require.Equal(t, claude.BetaContextManagement, vertexBeta)
	body := []byte(`{"model":"","messages":[{"role":"user","content":"hello"}],"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]},"thinking":{"type":"enabled","budget_tokens":1024,"block_binding":{"prefix_mismatch_behavior":"drop_block"}},"fallbacks":"default","fallback_credit_token":"synthetic"}`)
	out, changed := sanitizeAnthropicBodyForBetaTokens(body, vertexBeta)
	require.True(t, changed)
	w8AssertBetaControls(t, out, vertexBeta, true, false)

	// A native client that omits context-management loses context_management.
	out, changed = sanitizeAnthropicBodyForBetaTokens(body, claude.BetaStructuredOutputs)
	require.True(t, changed)
	w8AssertBetaControls(t, out, claude.BetaStructuredOutputs, false, false)

	// And the filtered beta is what Vertex actually receives.
	c := w8BetaContext("/v1/messages", claude.BetaContextManagement+","+claude.BetaThinkingBindingControls)
	req, err := (&GatewayService{}).buildUpstreamRequestAnthropicVertex(context.Background(), c, newVertexBetaServiceAccount(802), body, "synthetic", "@20250929", false)
	require.NoError(t, err)
	require.Equal(t, vertexBeta, getHeaderRaw(req.Header, "anthropic-beta"))
}

func TestForwardCountTokens_AC21FiltersEachGenerationField(t *testing.T) {
	for _, field := range []string{"max_tokens", "temperature", "top_p", "top_k", "stream", "stop_sequences", "stop"} {
		t.Run(field, func(t *testing.T) {
			var input map[string]any
			require.NoError(t, json.Unmarshal([]byte(w8BetaGatedBody), &input))
			switch field {
			case "max_tokens":
				input[field] = 2048
			case "temperature", "top_p":
				input[field] = 0.2
			case "top_k":
				input[field] = 10
			case "stream":
				input[field] = true
			default:
				input[field] = []string{"STOP"}
			}
			body, err := json.Marshal(input)
			require.NoError(t, err)
			parsed, err := ParseGatewayRequest(body, domain.PlatformAnthropic)
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
			c.Request.Header.Set("anthropic-beta", claude.BetaContextManagement+","+claude.BetaThinkingBindingControls)
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"input_tokens":42}`))}}
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}, httpUpstream: upstream, rateLimitService: &RateLimitService{}}
			require.NoError(t, svc.ForwardCountTokens(context.Background(), c, w8BetaAccount(), parsed))
			for _, forbidden := range []string{"max_tokens", "temperature", "top_p", "top_k", "stream", "stop_sequences", "stop"} {
				require.False(t, gjson.GetBytes(upstream.lastBody, forbidden).Exists(), forbidden)
			}
			require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
			w8AssertBetaControls(t, upstream.lastBody, getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"), true, true)
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"input_tokens":42}`, rec.Body.String())
			require.Equal(t, `{"input_tokens":42}`, rec.Body.String(), "count_tokens semantic response is passed through unchanged")
		})
	}
}

// Guard the two write paths that used to read the unexpanded request body: the
// sanitizer must run before NewRequest, and the body must remain valid JSON.
func TestAnthropicRoutes_AC19SanitizeBeforeRequestCreation(t *testing.T) {
	c := w8BetaContext("/v1/messages", claude.BetaStructuredOutputs)
	svc := &GatewayService{cfg: &config.Config{}}
	req, err := svc.buildUpstreamRequest(context.Background(), c, w8BetaAccount(), []byte(w8BetaGatedBody), "synthetic", "apikey", "", false, false)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.NewDecoder(io.NopCloser(bytes.NewReader(readRequestBodyForTest(t, req)))).Decode(&body))
	require.Contains(t, body, "messages")
	require.NotContains(t, body, "context_management")
}
