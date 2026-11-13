package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 本文件只收录 V05-T14 相关用例（上游 efa5a2240、1a3ecd2b9）。
// 上游同名文件中依赖 context_management 清理的用例本地未移植，不在此处。

func TestBuildCountTokensRequest_OAuthMimic_DropsInjectedMaxTokens(t *testing.T) {
	// OAuth mimicry injects max_tokens=128000 for normal messages requests. It is
	// invalid for Anthropic's count_tokens endpoint and must be stripped on wire.
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

	account := &Account{ID: 413, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-tok"},
		Status:      StatusActive, Schedulable: true,
	}
	normalized, _ := normalizeClaudeOAuthRequestBody(
		[]byte(`{"model":"claude-sonnet-4-5","messages":[]}`),
		"claude-sonnet-4-5", claudeOAuthNormalizeOptions{},
	)
	require.Equal(t, int64(128000), gjson.GetBytes(normalized, "max_tokens").Int(),
		"precondition: OAuth mimicry injects the Claude Code default")

	svc := &GatewayService{cfg: &config.Config{}}
	req, err := svc.buildCountTokensRequest(
		context.Background(), c, account, normalized,
		"oauth-tok", "oauth", "claude-sonnet-4-5", true,
	)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(readRequestBodyForTest(t, req), "max_tokens").Exists(),
		"count_tokens wire body must not contain max_tokens")
}

func TestBuildCountTokensRequest_StripsCacheControlOnlyFromLiteralDeferredTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"claude-haiku-4-5","messages":[],"tools":[{"name":"deferred","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral"}},{"name":"ordinary","custom":{"defer_loading":false},"cache_control":{"type":"ephemeral"}},{"name":"string","custom":{"defer_loading":"true"},"cache_control":{"type":"ephemeral"}},{"name":"number","custom":{"defer_loading":1},"cache_control":{"type":"ephemeral"}},{"name":"object","custom":{"defer_loading":{}},"cache_control":{"type":"ephemeral"}}]}`)

	tests := []struct {
		name      string
		account   *Account
		token     string
		tokenType string
	}{
		{
			name:      "generic API key",
			account:   &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
			token:     "sk-ant-test",
			tokenType: "apikey",
		},
		{
			name:      "recognized Claude Code OAuth without mimicry",
			account:   &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			token:     "oauth-token",
			tokenType: "oauth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
			svc := &GatewayService{cfg: &config.Config{}}

			req, err := svc.buildCountTokensRequest(
				context.Background(), c, tt.account, body,
				tt.token, tt.tokenType, "claude-haiku-4-5", false,
			)
			require.NoError(t, err)
			wireBody := readRequestBodyForTest(t, req)
			require.False(t, gjson.GetBytes(wireBody, "tools.0.cache_control").Exists())
			for idx := 1; idx < 5; idx++ {
				require.Equal(t, "ephemeral", gjson.GetBytes(wireBody, fmt.Sprintf("tools.%d.cache_control.type", idx)).String())
			}
		})
	}
}
