package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func oauthSnapshotInput(t *testing.T, model, ua string, genuine bool) ([]byte, *ParsedRequest) {
	t.Helper()
	input := map[string]any{
		"model":      model,
		"max_tokens": 128,
		"system": []any{
			map[string]any{"type": "text", "text": "Today’s date is 2026/07/01."},
		},
		"messages": []any{map[string]any{
			"role":    "user",
			"content": "<system-reminder>\nToday’s date is 2026/07/01.\n</system-reminder>\nhello world",
		}},
	}
	if model == "claude-haiku-4-5" {
		input["system"] = []any{
			map[string]any{"type": "text", "text": "stable client prompt", "cache_control": map[string]any{"type": "ephemeral"}},
			map[string]any{"type": "text", "text": "second client prompt", "cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
		}
	}
	if genuine {
		input["system"] = []any{
			map[string]any{"type": "text", "text": claudeCodeSystemPrompt},
			map[string]any{"type": "text", "text": "Today’s date is 2026/07/01."},
		}
		input["metadata"] = map[string]any{"user_id": claudeCodeMetadataUserIDJSON}
	}
	body, err := json.Marshal(input)
	require.NoError(t, err)
	return body, &ParsedRequest{Body: body, Model: model, MetadataUserID: gjson.GetBytes(body, "metadata.user_id").String()}
}

func TestGatewayOAuthOutboundBodySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		model       string
		ua          string
		genuine     bool
		proxied     bool
		wantBlocks  int
		wantTTL     string
		passthrough bool
	}{
		{name: "non CC sonnet", model: "claude-sonnet-5-5", ua: "third-party/1.0", wantBlocks: 3, wantTTL: "5m"},
		{name: "non CC haiku with system breakpoints", model: "claude-haiku-4-5", ua: "third-party/1.0", wantBlocks: 3, wantTTL: "5m"},
		{name: "non CC fable", model: "claude-fable-5", ua: "third-party/1.0", wantBlocks: 2},
		{name: "genuine CC passthrough", model: "claude-sonnet-5-5", ua: "claude-cli/2.1.258 (external, cli)", genuine: true, passthrough: true},
		{name: "proxied CC billing passthrough", model: "claude-sonnet-5-5", ua: "Go-http-client/2.0", proxied: true, passthrough: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGatewayForwardingSettingsCacheForTest(t)
			body, parsed := oauthSnapshotInput(t, tt.model, tt.ua, tt.genuine || tt.proxied)
			if tt.proxied {
				billing, err := buildBillingAttributionText(body, "2.1.258")
				require.NoError(t, err)
				var input map[string]any
				require.NoError(t, json.Unmarshal(body, &input))
				input["system"] = []any{
					map[string]any{"type": "text", "text": billing},
					map[string]any{"type": "text", "text": "Today’s date is 2026/07/01."},
				}
				body, err = json.Marshal(input)
				require.NoError(t, err)
				parsed.Body = body
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("User-Agent", tt.ua)
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(&streamReadCloser{payload: []byte(
					`{"id":"msg_0123456789012345678901","type":"message","role":"assistant","content":[],"model":"claude-sonnet-5-5","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
				)}),
			}}
			svc := &GatewayService{
				cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
				httpUpstream:     upstream,
				rateLimitService: &RateLimitService{},
				settingService:   NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{}}, &config.Config{}),
				identityService: NewIdentityService(&stubIdentityCache{fingerprint: &Fingerprint{
					UserAgent: "claude-cli/2.1.258 (external, cli)", ClientID: "snapshot-test", UpdatedAt: time.Now().Unix(),
				}}),
			}
			account := &Account{ID: 611, Name: "oauth-snapshot", Platform: PlatformAnthropic, Type: AccountTypeOAuth,
				Concurrency: 1, Credentials: map[string]any{"access_token": "test-token"}, Status: StatusActive, Schedulable: true}
			_, err := svc.Forward(context.Background(), c, account, parsed)
			require.NoError(t, err)

			out := upstream.lastBody
			require.NotContains(t, string(out), "cch=")
			require.Contains(t, string(out), "Today's date is 2026-07-01.")
			require.NotContains(t, string(out), "2026/07/01")
			_, messagesCC, toolsCC, systemCC := collectCacheControlPaths(out)
			require.LessOrEqual(t, len(messagesCC)+len(toolsCC)+len(systemCC), maxCacheControlBlocks)
			system := gjson.GetBytes(out, "system").Array()
			if tt.passthrough {
				if tt.proxied {
					require.Contains(t, system[0].Get("text").String(), claudeCodeBillingHeaderPrefix)
				} else {
					require.Contains(t, system[0].Get("text").String(), claudeCodeSystemPrompt)
				}
				return
			}
			require.Len(t, system, tt.wantBlocks)
			billing := system[0].Get("text").String()
			require.Contains(t, billing, "cc_version="+claude.CLIVersion()+"."+computeClaudeCodeFingerprint(body, claude.CLIVersion())+";")
			require.Equal(t, claude.DefaultUserAgent(), getHeaderRaw(upstream.lastReq.Header, "User-Agent"))
			require.Contains(t, billing, computeClaudeCodeFingerprint(body, claude.CLIVersion()))
			if tt.wantTTL != "" {
				require.Equal(t, tt.wantTTL, system[2].Get("cache_control.ttl").String())
				require.False(t, system[1].Get("cache_control").Exists())
			} else {
				for _, block := range system {
					require.False(t, block.Get("cache_control").Exists())
				}
			}
		})
	}
}

func TestGatewayOAuthCountTokensBodySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGatewayForwardingSettingsCacheForTest(t)
	_, parsed := oauthSnapshotInput(t, "claude-haiku-4-5", "third-party/1.0", false)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("User-Agent", "third-party/1.0")
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(&streamReadCloser{payload: []byte(`{"input_tokens":42}`)}),
	}}
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
		settingService:   NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{}}, &config.Config{}),
	}
	account := &Account{ID: 612, Name: "count-snapshot", Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Concurrency: 1, Credentials: map[string]any{"access_token": "test-token"}, Status: StatusActive, Schedulable: true}
	require.NoError(t, svc.ForwardCountTokens(context.Background(), c, account, parsed))
	out := upstream.lastBody
	_, messagesCC, toolsCC, systemCC := collectCacheControlPaths(out)
	require.LessOrEqual(t, len(messagesCC)+len(toolsCC)+len(systemCC), maxCacheControlBlocks)
	// count_tokens intentionally has neither system injection nor dateline normalization.
	require.Contains(t, string(out), "2026/07/01")
	require.NotContains(t, string(out), "cch=")
}
