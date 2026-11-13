//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const cloudflareMessagesTestBaseURL = "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai"

func TestAnthropicMessagesURL_EntryPoints(t *testing.T) {
	for _, tt := range []struct {
		name    string
		baseURL string
		query   string
	}{
		{"cloudflare REST", cloudflareMessagesTestBaseURL, ""},
		{"cloudflare REST trailing slash", cloudflareMessagesTestBaseURL + "/", ""},
		{"anthropic default", "", "beta=true"},
		{"anthropic explicit", "https://api.anthropic.com", "beta=true"},
		{"custom upstream", "https://anthropic.example.com", "beta=true"},
		{"cloudflare native gateway", "https://gateway.ai.cloudflare.com/v1/account/gateway/anthropic", "beta=true"},
		{"lookalike host", "https://api.cloudflare.com.example.com/client/v4/accounts/account/ai", "beta=true"},
		{"other cloudflare path", "https://api.cloudflare.com/client/v4/accounts/account/ai-gateway", "beta=true"},
		{"extra cloudflare path segment", cloudflareMessagesTestBaseURL + "/other", "beta=true"},
		{"missing cloudflare account", "https://api.cloudflare.com/client/v4/accounts//ai", "beta=true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := newAnthropicAPIKeyAccountForTest()
			account.Credentials["base_url"] = tt.baseURL
			body := []byte(`{"model":"claude-haiku-4-5","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
			baseURL := strings.TrimRight(tt.baseURL, "/")
			if baseURL == "" {
				baseURL = "https://api.anthropic.com"
			}
			wantURL := baseURL + "/v1/messages"
			if tt.query != "" {
				wantURL += "?" + tt.query
			}
			checkRequest := func(t *testing.T, req *http.Request) {
				t.Helper()
				require.NotNil(t, req)
				require.Equal(t, wantURL, req.URL.String())
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "upstream-anthropic-key", getHeaderRaw(req.Header, "x-api-key"))
				require.Equal(t, "2023-06-01", getHeaderRaw(req.Header, "anthropic-version"))
			}
			svc := &GatewayService{cfg: &config.Config{}}
			t.Run("forward", func(t *testing.T) {
				c, _ := newTestContext()
				req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "upstream-anthropic-key", "apikey", "claude-haiku-4-5", false, false)
				require.NoError(t, err)
				checkRequest(t, req)
			})
			t.Run("passthrough", func(t *testing.T) {
				c, _ := newTestContext()
				req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "upstream-anthropic-key")
				require.NoError(t, err)
				checkRequest(t, req)
			})
			t.Run("account test", func(t *testing.T) {
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-haiku-4-5\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\n"))}}
				testSvc := &AccountTestService{cfg: &config.Config{}, httpUpstream: upstream}
				c, rec := newTestContext()
				require.NoError(t, testSvc.testClaudeAccountConnection(c, account, "claude-haiku-4-5"))
				checkRequest(t, upstream.lastReq)
				require.Equal(t, "claude-haiku-4-5", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Contains(t, rec.Body.String(), `"type":"content"`)
				require.Contains(t, rec.Body.String(), `"text":"ok"`)
			})
		})
	}
}

// Simulate the REST endpoint's strict query handling without contacting Cloudflare.
type cloudflareMessagesTestUpstream struct {
	anthropicHTTPUpstreamRecorder
}

func (u *cloudflareMessagesTestUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if req.URL.RawQuery != "" {
		u.resp = &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"errors":[{"code":"unrecognized_keys","keys":["query"]}],"success":false}`))}
		return u.Do(req, proxyURL, accountID, concurrency)
	}
	u.resp = &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-haiku-4-5\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\n"))}
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestAccountTestService_CloudflareMessagesRejectsBetaQuery(t *testing.T) {
	account := newAnthropicAPIKeyAccountForTest()
	account.Credentials["base_url"] = cloudflareMessagesTestBaseURL
	upstream := &cloudflareMessagesTestUpstream{}
	svc := &AccountTestService{cfg: &config.Config{}, httpUpstream: upstream}
	c, rec := newTestContext()
	require.NoError(t, svc.testClaudeAccountConnection(c, account, "claude-haiku-4-5"))
	require.Contains(t, rec.Body.String(), `"text":"ok"`)
	require.NotContains(t, rec.Body.String(), "API returned 400")
}

func TestAnthropicMessagesURL_RespectsURLAllowlist(t *testing.T) {
	account := newAnthropicAPIKeyAccountForTest()
	account.Credentials["base_url"] = cloudflareMessagesTestBaseURL
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"api.anthropic.com"}
	svc := &GatewayService{cfg: cfg}
	body := []byte(`{"model":"claude-haiku-4-5","messages":[]}`)
	c, _ := newTestContext()
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "test-key", "apikey", "claude-haiku-4-5", false, false)
	require.ErrorContains(t, err, "host is not allowed")
	require.Nil(t, req)
	req, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "test-key")
	require.ErrorContains(t, err, "host is not allowed")
	require.Nil(t, req)
	upstream := &anthropicHTTPUpstreamRecorder{}
	testSvc := &AccountTestService{cfg: cfg, httpUpstream: upstream}
	c, rec := newTestContext()
	require.Error(t, testSvc.testClaudeAccountConnection(c, account, "claude-haiku-4-5"))
	require.Contains(t, rec.Body.String(), "host is not allowed")
	require.Nil(t, upstream.lastReq)
}
