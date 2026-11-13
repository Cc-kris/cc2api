package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V05-T13 / 上游 40e1cc14b：Vertex Anthropic 路径的 anthropic-beta 白名单过滤。

func newVertexBetaTestContext(t *testing.T, anthropicBeta string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if anthropicBeta != "" {
		c.Request.Header.Set("Anthropic-Beta", anthropicBeta)
	}
	return c
}

func newVertexBetaServiceAccount(id int64) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformAnthropic,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"project_id": "vertex-proj",
			"location":   "us-east5",
		},
	}
}

// 复刻线上 400：近期 Claude Code CLI 透传的整份 anthropic-beta header 里含 Vertex
// 不接受的 token（advisor-tool / prompt-caching-scope / redact-thinking /
// thinking-token-count）。Vertex builder 必须剥掉它们，否则上游 HTTP 400（issue #3358）。
func TestVertexBetaFilter_StripsUnsupportedClaudeCodeTokens(t *testing.T) {
	c := newVertexBetaTestContext(t,
		"claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,"+
			"advisor-tool-2026-03-01,prompt-caching-scope-2026-01-05,"+
			"redact-thinking-2026-02-12,thinking-token-count-2026-05-13,"+
			"context-management-2025-06-27")

	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	req, err := svc.buildUpstreamRequest(
		context.Background(), c, newVertexBetaServiceAccount(401), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
	require.NoError(t, err)

	outBeta := getHeaderRaw(req.Header, "anthropic-beta")

	// Vertex 拒绝的 token 必须全部剥掉。
	for _, bad := range []string{
		"advisor-tool-2026-03-01",
		"prompt-caching-scope-2026-01-05",
		"redact-thinking-2026-02-12",
		"thinking-token-count-2026-05-13",
		// 客户端身份 beta：Vertex service_account 不需要，亦不在白名单。
		"claude-code-20250219",
		"oauth-2025-04-20",
	} {
		require.False(t, containsBetaToken(outBeta, bad),
			"token %q 必须被剥离；实际 outgoing beta=%q", bad, outBeta)
	}

	// 白名单内的 token 必须保留。
	for _, keep := range []string{
		"interleaved-thinking-2025-05-14",
		"context-management-2025-06-27",
	} {
		require.True(t, containsBetaToken(outBeta, keep),
			"token %q 应保留；实际 outgoing beta=%q", keep, outBeta)
	}
}

// 全部 token 都不受 Vertex 支持时，outgoing header 不应下发 anthropic-beta。
func TestVertexBetaFilter_DropsHeaderWhenAllUnsupported(t *testing.T) {
	c := newVertexBetaTestContext(t,
		"prompt-caching-scope-2026-01-05,redact-thinking-2026-02-12")

	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	req, err := svc.buildUpstreamRequest(
		context.Background(), c, newVertexBetaServiceAccount(402), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
	require.NoError(t, err)

	require.Empty(t, getHeaderRaw(req.Header, "anthropic-beta"),
		"所有 token 被剥离后不应残留 anthropic-beta header")
}

// 客户端传入的 fallbacks 字段在 Vertex 路径也必须剥除（最终 beta 不含 fallback token）。
func TestVertexBetaFilter_StripsFallbackFields(t *testing.T) {
	c := newVertexBetaTestContext(t, "interleaved-thinking-2025-05-14")

	body := []byte(`{"model":"claude-opus-4-7","fallbacks":"default","fallback_credit_token":"tok_123","messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	req, err := svc.buildUpstreamRequest(
		context.Background(), c, newVertexBetaServiceAccount(403), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
	require.NoError(t, err)

	got := readRequestBodyForTest(t, req)
	require.False(t, gjson.GetBytes(got, "fallbacks").Exists())
	require.False(t, gjson.GetBytes(got, "fallback_credit_token").Exists())
}

// BetaPolicy block 规则在 Vertex 路径同样生效：管理员 block 某 token，客户端带它 → 直接报错。
func TestVertexBetaFilter_BlocksViaBetaPolicy(t *testing.T) {
	settings := &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken:    "context-management-2025-06-27",
				Action:       BetaPolicyActionBlock,
				Scope:        BetaPolicyScopeAll,
				ErrorMessage: "context management is blocked",
			},
		},
	}
	raw, err := json.Marshal(settings)
	require.NoError(t, err)

	svc := &GatewayService{
		settingService: NewSettingService(
			&betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
			}},
			&config.Config{},
		),
	}

	c := newVertexBetaTestContext(t,
		"interleaved-thinking-2025-05-14,context-management-2025-06-27")
	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)

	_, err = svc.buildUpstreamRequest(
		context.Background(), c, newVertexBetaServiceAccount(404), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
	require.Error(t, err)
	var blocked *BetaBlockedError
	require.True(t, errors.As(err, &blocked), "expected *BetaBlockedError, got %T", err)
	require.Equal(t, "context management is blocked", err.Error())
}

// filterVertexBetaTokens 单元测试：白名单过滤 + drop 集合 + 去重 + 空输入。
func TestFilterVertexBetaTokens(t *testing.T) {
	t.Run("whitelist filters unsupported", func(t *testing.T) {
		out := filterVertexBetaTokens(
			"interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,context-management-2025-06-27",
			nil,
		)
		require.Equal(t, "interleaved-thinking-2025-05-14,context-management-2025-06-27", out)
	})

	t.Run("drop set strips before whitelist", func(t *testing.T) {
		out := filterVertexBetaTokens(
			"interleaved-thinking-2025-05-14,context-management-2025-06-27",
			map[string]struct{}{"context-management-2025-06-27": {}},
		)
		require.Equal(t, "interleaved-thinking-2025-05-14", out)
	})

	t.Run("dedupe", func(t *testing.T) {
		out := filterVertexBetaTokens(
			"context-1m-2025-08-07,context-1m-2025-08-07",
			nil,
		)
		require.Equal(t, "context-1m-2025-08-07", out)
	})

	t.Run("empty input", func(t *testing.T) {
		require.Empty(t, filterVertexBetaTokens("", nil))
		require.Empty(t, filterVertexBetaTokens("prompt-caching-scope-2026-01-05", nil))
	})
}
