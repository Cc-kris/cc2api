package service

import (
	"context"
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
	"github.com/tidwall/sjson"
)

// 移植自上游 994ca26e9 / ca0882593 的 gateway_billing_header_test.go。
// CCAI 的 buildUpstreamRequest / buildCountTokensRequest 只返回 *http.Request，
// 出站 body 从 req.Body 读取。

func resetGatewayForwardingSettingsCacheForTest(t *testing.T) {
	t.Helper()
	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	t.Cleanup(func() {
		gatewayForwardingSF.Forget("gateway_forwarding")
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	})
}

func buildClaudeOAuthWireRequestForTest(t *testing.T, svc *GatewayService, endpoint string, body []byte, mimic bool) (*http.Request, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	var (
		req *http.Request
		err error
	)
	if endpoint == "messages" {
		req, err = svc.buildUpstreamRequest(context.Background(), c, account,
			body, "test-token", "oauth", "claude-haiku-4-5", false, mimic)
	} else {
		req, err = svc.buildCountTokensRequest(context.Background(), c, account,
			body, "test-token", "oauth", "claude-haiku-4-5", mimic)
	}
	require.NoError(t, err)
	wireBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	return req, wireBody
}

func TestSyncBillingHeaderVersion_RecomputesSuffixAndIsIdempotent(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.df2; cc_entrypoint=cli;"}],"messages":[{"role":"user","content":"hello world"}]}`)
	version := "2.1.22"
	result := syncBillingHeaderVersion(body, "claude-cli/"+version)
	require.Contains(t, gjson.GetBytes(result, "system.0.text").String(),
		"cc_version="+version+"."+computeClaudeCodeFingerprint(body, version)+";")
	require.Equal(t, string(result), string(syncBillingHeaderVersion(result, "claude-cli/"+version)))
	require.JSONEq(t, gjson.GetBytes(body, "messages").Raw, gjson.GetBytes(result, "messages").Raw)
}

func TestSyncBillingHeaderVersion_UnicodeSuffix(t *testing.T) {
	for _, initialVersion := range []string{"2.1.81.df2", "2.1.283.000", "2.1.283.c7e"} {
		t.Run(initialVersion, func(t *testing.T) {
			body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=` + initialVersion + `; cc_entrypoint=cli;"},{"type":"text","text":"Keep this prompt."}],"messages":[{"role":"user","content":"这是一个用于验证中文指纹计算的测试消息，请保持一致。"}],"unknown":{"keep":true}}`)
			result := syncBillingHeaderVersion(body, "claude-cli/2.1.283")
			// Node.js reference output; do not derive this from the helper under test.
			require.Equal(t, "x-anthropic-billing-header: cc_version=2.1.283.c7e; cc_entrypoint=cli;", gjson.GetBytes(result, "system.0.text").String())
			require.Equal(t, result, syncBillingHeaderVersion(result, "claude-cli/2.1.283"))
			if initialVersion == "2.1.283.c7e" {
				require.Equal(t, body, result)
			}
			for _, path := range []string{"messages", "system.1", "unknown"} {
				require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(result, path).Raw)
			}
		})
	}
}

func TestBuildOAuthRequest_BillingMatchesWireUserAgent(t *testing.T) {
	for _, endpoint := range []string{"messages", "count_tokens"} {
		for _, tc := range []struct {
			name      string
			mimic     bool
			identity  bool
			disableFP bool
		}{
			{name: "mimic_overrides_cached_version", mimic: true, identity: true},
			{name: "mimic_without_identity", mimic: true},
			{name: "mimic_with_fingerprint_disabled", mimic: true, identity: true, disableFP: true},
			{name: "passthrough_uses_cached_version", identity: true},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				resetGatewayForwardingSettingsCacheForTest(t)
				body := []byte(`{"model":"claude-haiku-4-5","system":[{"type":"text","text":""}],"messages":[{"role":"user","content":"这是一个用于验证中文指纹计算的测试消息，请保持一致。"}]}`)
				billing, err := buildBillingAttributionText(body, "2.1.81")
				require.NoError(t, err)
				body, err = sjson.SetBytes(body, "system.0.text", billing)
				require.NoError(t, err)

				cfg := &config.Config{}
				svc := &GatewayService{cfg: cfg}
				cachedUA := "claude-cli/2.9.0 (external, cli)"
				if tc.identity {
					svc.identityService = NewIdentityService(&stubIdentityCache{fingerprint: &Fingerprint{
						UserAgent: cachedUA, ClientID: "test-client", UpdatedAt: time.Now().Unix(),
					}})
				}
				if tc.disableFP {
					svc.settingService = NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{
						SettingKeyEnableFingerprintUnification: "false",
					}}, cfg)
				}
				req, wireBody := buildClaudeOAuthWireRequestForTest(t, svc, endpoint, body, tc.mimic)
				wantUA := cachedUA
				if tc.mimic {
					wantUA = claude.DefaultHeaders()["User-Agent"]
				}
				require.Equal(t, wantUA, getHeaderRaw(req.Header, "User-Agent"))
				version := ExtractCLIVersion(wantUA)
				require.Contains(t, gjson.GetBytes(wireBody, "system.0.text").String(),
					"cc_version="+version+"."+computeClaudeCodeFingerprint(wireBody, version)+";")
				if !tc.mimic {
					// Independently calculated with Node.js for the cached 2.9.0 UA.
					require.Contains(t, gjson.GetBytes(wireBody, "system.0.text").String(), "cc_version=2.9.0.ac8;")
				}
			})
		}
	}
}
