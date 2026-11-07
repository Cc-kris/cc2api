package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// V02-T15：Codex 出站身份统一。六条 OAuth 出站路径（HTTP 转发、透传、WS、用量探测、
// 模型清单探测、账号测试）都用同一个版本来源，收口放在每条路径最后。
//
// 本文件的用例改写进程级解析器，不得加 t.Parallel()。

const codexIdentityTestVersion = "0.150.0"

var codexIdentityTestUA = "codex_cli_rs/" + codexIdentityTestVersion + codexCLIUserAgentSuffix

func withCodexIdentityTestResolver(t *testing.T) {
	t.Helper()
	SetCodexCanonicalUserAgentResolver(func() string { return codexIdentityTestUA })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
}

func requireCodexIdentityTestHeaders(t *testing.T, h http.Header) {
	t.Helper()
	require.Equal(t, codexIdentityTestUA, h.Get("User-Agent"))
	require.Equal(t, openai.CodexCLIOriginator, h.Get("Originator"))
	require.Equal(t, codexIdentityTestVersion, h.Get("Version"))
}

func newCodexIdentityTestOAuthAccount() *Account {
	return &Account{
		ID: 501, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Extra:       map[string]any{},
	}
}

func newCodexIdentityTestContext(t *testing.T, ua, originator, version string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if ua != "" {
		c.Request.Header.Set("User-Agent", ua)
	}
	if originator != "" {
		c.Request.Header.Set("originator", originator)
	}
	if version != "" {
		c.Request.Header.Set("version", version)
	}
	return c
}

// AC20 / AC21：HTTP 转发非 compact 请求也写 version；客户端带陈旧 TUI 身份和 version 0.125.0 时整体替换。
func TestCodexIdentityHTTPForwardNonCompact(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newCodexIdentityTestContext(t, "codex-tui/0.125.0 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.125.0)", "codex-tui", "0.125.0")

	req, err := svc.buildUpstreamRequest(context.Background(), c, newCodexIdentityTestOAuthAccount(), []byte(`{"model":"gpt-5.4"}`), "tok", true, "", true)
	require.NoError(t, err)
	requireCodexIdentityTestHeaders(t, req.Header)
}

// AC20：浏览器 UA 也被收口吸收，不再依赖 overrideBrowserUserAgent。
func TestCodexIdentityHTTPForwardBrowserUA(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newCodexIdentityTestContext(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "", "")

	req, err := svc.buildUpstreamRequest(context.Background(), c, newCodexIdentityTestOAuthAccount(), []byte(`{"model":"gpt-5.4"}`), "tok", true, "", false)
	require.NoError(t, err)
	requireCodexIdentityTestHeaders(t, req.Header)
}

// AC20：ForceCodexCLI 时忽略账号 UA，用规范身份。
func TestCodexIdentityHTTPForwardForceCodexCLIIgnoresAccountUA(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: true}}}
	account := newCodexIdentityTestOAuthAccount()
	account.Credentials["user_agent"] = "codex_vscode/0.120.0 (Ubuntu 22.4.0; x86_64) vscode"
	c := newCodexIdentityTestContext(t, "", "", "")

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "tok", true, "", true)
	require.NoError(t, err)
	requireCodexIdentityTestHeaders(t, req.Header)
}

// AC21：账号级自定义 UA 只保留指纹，版本段重建、originator 按首段配对。
func TestCodexIdentityHTTPForwardAccountUAKeepsFingerprint(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := newCodexIdentityTestOAuthAccount()
	account.Credentials["user_agent"] = "codex_vscode/0.120.0 (Mac OS X 14.0; arm64) vscode"
	c := newCodexIdentityTestContext(t, "", "", "")

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "tok", true, "", true)
	require.NoError(t, err)
	require.Equal(t, "codex_vscode/"+codexIdentityTestVersion+" (Mac OS X 14.0; arm64) vscode", req.Header.Get("User-Agent"))
	require.Equal(t, "codex_vscode", req.Header.Get("Originator"))
	require.Equal(t, codexIdentityTestVersion, req.Header.Get("Version"))
}

// AC20 例外 / AC63：full 档账号跳过收口，出站 UA 仍是账号 UA，版本段不重建。
func TestCodexIdentityFullModeSkipsEnforcement(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	const pinnedUA = "codex_cli_rs/0.130.0 (Mac OS X 14.0; arm64) iTerm"
	account := newCodexIdentityTestOAuthAccount()
	account.Credentials["user_agent"] = pinnedUA
	account.Extra[CodexIdentityModeExtraKey] = string(OpenAICodexIdentityModeFull)
	c := newCodexIdentityTestContext(t, "", "", "")

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "tok", true, "", true)
	require.NoError(t, err)
	require.Equal(t, pinnedUA, req.Header.Get("User-Agent"))
	require.Empty(t, req.Header.Get("Version"), "full 档跳过收口，非 compact 请求保持不写 version")

	headers, _ := svc.buildOpenAIWSHeaders(c, account, "tok", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "", "")
	require.Equal(t, pinnedUA, headers.Get("User-Agent"))

	// full 档但账号没填 UA：full 档不写 UA，照常收口。
	account.Credentials["user_agent"] = ""
	req, err = svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "tok", true, "", true)
	require.NoError(t, err)
	requireCodexIdentityTestHeaders(t, req.Header)
}

// AC63：其他三档照常收口。
func TestCodexIdentityNonFullModesEnforce(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	for _, mode := range []OpenAICodexIdentityMode{OpenAICodexIdentityModeDisabled, OpenAICodexIdentityModeDevice, OpenAICodexIdentityModeSession} {
		t.Run(string(mode), func(t *testing.T) {
			account := newCodexIdentityTestOAuthAccount()
			account.Credentials["user_agent"] = "codex_cli_rs/0.130.0 (Mac OS X 14.0; arm64) iTerm"
			account.Credentials["device_id"] = "dev-1"
			account.Extra[CodexIdentityModeExtraKey] = string(mode)
			c := newCodexIdentityTestContext(t, "", "", "")
			req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "tok", true, "", true)
			require.NoError(t, err)
			require.Equal(t, "codex_cli_rs/"+codexIdentityTestVersion+" (Mac OS X 14.0; arm64) iTerm", req.Header.Get("User-Agent"))
			require.Equal(t, codexIdentityTestVersion, req.Header.Get("Version"))
		})
	}
}

// AC20 例外：Messages 桥不发 originator，也不写 version。
func TestCodexIdentityMessagesBridgeSkipsEnforcement(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newCodexIdentityTestContext(t, "claude-cli/2.1.0 (external, cli)", "", "")
	setOpenAICompatMessagesBridgeContext(c, true)

	req, err := svc.buildUpstreamRequest(context.Background(), c, newCodexIdentityTestOAuthAccount(), []byte(`{"model":"gpt-5.4"}`), "tok", true, "", false)
	require.NoError(t, err)
	require.Empty(t, req.Header.Get("Originator"))
	require.Empty(t, req.Header.Get("Version"))
	require.Equal(t, "claude-cli/2.1.0 (external, cli)", req.Header.Get("User-Agent"))
}

// AC20：透传路径。
func TestCodexIdentityPassthrough(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newCodexIdentityTestContext(t, "codex_cli_rs/0.1.0", "", "0.1.0")

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, newCodexIdentityTestOAuthAccount(), []byte(`{"model":"gpt-5.4"}`), "tok")
	require.NoError(t, err)
	requireCodexIdentityTestHeaders(t, req.Header)
}

// AC20：WS 握手带 version 头（CCAI 原来不发）。
func TestCodexIdentityWSHandshake(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newCodexIdentityTestContext(t, "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)", "codex-tui", "")

	headers, _ := svc.buildOpenAIWSHeaders(c, newCodexIdentityTestOAuthAccount(), "tok", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "", "")
	requireCodexIdentityTestHeaders(t, headers)
}

// API Key 账号不受收口影响。
func TestCodexIdentityAPIKeyUntouched(t *testing.T) {
	withCodexIdentityTestResolver(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}
	c := newCodexIdentityTestContext(t, "my-sdk/1.0", "", "")

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "tok", true, "", false)
	require.NoError(t, err)
	require.Equal(t, "my-sdk/1.0", req.Header.Get("User-Agent"))
	require.Empty(t, req.Header.Get("Version"))
}

// AC20：用量探测。
func TestCodexIdentityUsageProbe(t *testing.T) {
	withCodexIdentityTestResolver(t)
	h := make(http.Header)
	(&AccountUsageService{}).applyOpenAICodexProbeIdentity(context.Background(), h, newCodexIdentityTestOAuthAccount())
	requireCodexIdentityTestHeaders(t, h)
}

// AC20：模型清单探测。客户端 version 低于门槛时 version 头用生效版本，查询参数原样。
func TestCodexIdentityModelsManifestProbe(t *testing.T) {
	withCodexIdentityTestResolver(t)
	var got http.Header
	var gotClientVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		gotClientVersion = r.URL.Query().Get("client_version")
		_, _ = io.WriteString(w, codexModelsNativeTestManifest("gpt-5.6"))
	}))
	defer server.Close()
	oldURL := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL + "/backend-api/codex/models"
	t.Cleanup(func() { chatgptCodexModelsURL = oldURL })

	svc := &OpenAIGatewayService{}
	_, err := svc.FetchCodexModelsManifest(context.Background(), newCodexIdentityTestOAuthAccount(), "", "")
	require.NoError(t, err)
	requireCodexIdentityTestHeaders(t, got)
	require.Equal(t, codexIdentityTestVersion, gotClientVersion)

	_, err = svc.FetchCodexModelsManifest(context.Background(), newCodexIdentityTestOAuthAccount(), "0.125.0", "")
	require.NoError(t, err)
	require.Equal(t, codexIdentityTestVersion, got.Get("Version"))
	require.Equal(t, "0.125.0", gotClientVersion)
}

// AC20：账号测试（普通测试和 compact 探测）。
func TestCodexIdentityAccountTest(t *testing.T) {
	withCodexIdentityTestResolver(t)
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{AccountTestModeDefault, AccountTestModeCompact} {
		t.Run(mode, func(t *testing.T) {
			account := *newCodexIdentityTestOAuthAccount()
			body := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
			contentType := "text/event-stream"
			if mode == AccountTestModeCompact {
				body = `{"id":"cmp_probe","status":"completed"}`
				contentType = "application/json"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}}
			svc := &AccountTestService{
				accountRepo:  &snapshotUpdateAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}, updateExtraCalls: make(chan map[string]any, 4)},
				httpUpstream: upstream,
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/501/test", bytes.NewReader(nil))

			_ = svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", mode)
			require.NotNil(t, upstream.lastReq)
			requireCodexIdentityTestHeaders(t, upstream.lastReq.Header)
		})
	}
}

// AC21：管理员面板 UA 为 codex-tui/0.125.0 时，规范 UA 版本段重建；收口再归一为 CLI。
func TestCodexIdentitySettingPanelUAVersionRebuilt(t *testing.T) {
	repo := &compactTestSettingRepo{values: map[string]string{
		SettingKeyOpenAICodexUserAgent: "codex-tui/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.125.0)",
	}}
	settings := NewSettingService(repo, &config.Config{})
	canonical := settings.GetOpenAICodexCanonicalUserAgent(context.Background())
	require.Equal(t, "codex-tui/"+codexCLIVersion+" (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; "+codexCLIVersion+")", canonical)

	SetCodexCanonicalUserAgentResolver(func() string { return canonical })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	h := make(http.Header)
	h.Set("originator", "codex_cli_rs")
	enforceCodexIdentityHeaders(h)
	require.Equal(t, openai.CodexCLIOriginator, h.Get("originator"))
	require.Equal(t, "codex_cli_rs/"+codexCLIVersion+" (Ubuntu 22.4.0; x86_64) xterm-256color", h.Get("user-agent"))
	require.Equal(t, codexCLIVersion, h.Get("version"))
}

// AC22：管理员版本带 CRLF 注入被拒绝，生效版本不变。
func TestCodexIdentityRejectsInjectedVersion(t *testing.T) {
	require.Empty(t, NormalizeCodexClientVersion("0.150.0\r\nX-Evil: 1"))
	require.Equal(t, codexCLIUserAgent, buildCodexCLIUserAgent("0.150.0\r\nX-Evil: 1"))

	SetCodexCanonicalUserAgentResolver(func() string { return "codex_cli_rs/0.150.0\r\nX-Evil: 1" })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	require.Equal(t, codexCLIVersion, CodexCanonicalClientVersion())
	require.Equal(t, codexCLIUserAgent, CodexCanonicalUserAgent())
}

// 默认 UA 改为 CLI 身份（不再是 codex-tui）。
func TestDefaultOpenAICodexUserAgentIsCLI(t *testing.T) {
	require.Equal(t, "codex_cli_rs/0.146.0 (Ubuntu 22.4.0; x86_64) xterm-256color", DefaultOpenAICodexUserAgent)
	require.Equal(t, codexCLIUserAgent, DefaultOpenAICodexUserAgent)
}
