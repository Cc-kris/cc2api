package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// V02-T17：Codex 客户端的 session-id（连字符）头参与粘性和会话隔离，
// 但客户端原始 session-id 不能发到上游，出站只写隔离后的 session_id。

// 上游 8e60d5747 的表格用例：session-id 优先级最高。
func TestOpenAIGatewayService_CodexSessionIDHeaderPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	body := []byte(`{"prompt_cache_key":"body-session"}`)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "codex-session")
	c.Request.Header.Set("session_id", "generic-session")
	c.Request.Header.Set("conversation_id", "generic-conversation")
	require.Equal(t, "codex-session", svc.ExtractSessionID(c, body))

	withSessionID := svc.GenerateSessionHash(c, body)
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c2.Request.Header.Set("session_id", "codex-session")
	require.Equal(t, withSessionID, svc.GenerateSessionHash(c2, body), "两种拼写同值应得到同一粘性哈希")

	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	c3.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c3.Request.Header.Set("session_id", "generic-session")
	require.Equal(t, "generic-session", svc.ExtractSessionID(c3, body))
	require.Equal(t, "body-session", svc.ExtractSessionID(newSessionIDTestContext(t), body))
}

// 拷自上游 8e60d5747：预热与业务请求的会话哈希一致，重连后仍命中同一账号。
func TestOpenAIGatewayService_CodexSessionIDKeepsReconnectHashStable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "codex-reconnect-session")

	svc := &OpenAIGatewayService{}
	warmup := []byte(`{
		"type":"response.create",
		"model":"gpt-5.6-sol",
		"generate":false,
		"tools":[{"type":"custom","name":"exec"}],
		"input":[{"role":"user","content":"warmup"}]
	}`)
	business := []byte(`{
		"type":"response.create",
		"model":"gpt-5.6-sol",
		"input":[{"role":"user","content":"install codex"}]
	}`)

	require.Equal(t, svc.GenerateSessionHash(c, warmup), svc.GenerateSessionHash(c, business))
	require.Equal(t, "codex-reconnect-session", svc.ExtractSessionID(c, business))
}

func newSessionIDTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	return c
}

func requireNoRawSessionIDHeader(t *testing.T, h http.Header, raw string) {
	t.Helper()
	for key, values := range h {
		if http.CanonicalHeaderKey(key) == "Session-Id" {
			t.Fatalf("出站请求不应带 session-id 头，实际 %s=%v", key, values)
		}
		for _, v := range values {
			require.NotEqual(t, raw, v, "客户端原始会话 ID 不能出现在出站头 %s", key)
		}
	}
}

// AC24 HTTP 转发：客户端 session-id 不出站；session_id 是按 apiKey 隔离后的值。
func TestOpenAIForwardDoesNotLeakRawSessionIDHeader(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newSessionIDTestContext(t)
	c.Request.Header.Set("session-id", "raw-codex-session")
	c.Set("api_key", &APIKey{ID: 77})

	req, err := svc.buildUpstreamRequest(context.Background(), c, newCodexIdentityTestOAuthAccount(), []byte(`{"model":"gpt-5.4"}`), "tok", true, "raw-codex-session", true)
	require.NoError(t, err)
	requireNoRawSessionIDHeader(t, req.Header, "raw-codex-session")
	require.Equal(t, isolateOpenAISessionID(getAPIKeyIDFromContext(c), "raw-codex-session"), req.Header.Get("session_id"))

	// API Key 账号同样不透传 session-id（不在白名单）。
	apiKeyAccount := &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}
	req, err = svc.buildUpstreamRequest(context.Background(), c, apiKeyAccount, []byte(`{"model":"gpt-5.4"}`), "tok", true, "", false)
	require.NoError(t, err)
	requireNoRawSessionIDHeader(t, req.Header, "raw-codex-session")
}

// AC24 透传：session_id 为空时读 session-id 参与隔离；原始 session-id 不出站。
func TestOpenAIPassthroughReadsSessionIDHeaderAndIsolates(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newSessionIDTestContext(t)
	c.Request.Header.Set("session-id", "raw-codex-session")

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, newCodexIdentityTestOAuthAccount(), []byte(`{"model":"gpt-5.4"}`), "tok")
	require.NoError(t, err)
	requireNoRawSessionIDHeader(t, req.Header, "raw-codex-session")
	require.Equal(t, isolateOpenAISessionID(getAPIKeyIDFromContext(c), "raw-codex-session"), req.Header.Get("session_id"))

	apiKeyAccount := &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}
	req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, apiKeyAccount, []byte(`{"model":"gpt-5.4"}`), "tok")
	require.NoError(t, err)
	requireNoRawSessionIDHeader(t, req.Header, "raw-codex-session")
}

// AC24 WS：resolveOpenAIWSSessionHeaders 认 session-id；握手头只有隔离后的 session_id。
func TestOpenAIWSSessionHeadersHonorSessionIDHeader(t *testing.T) {
	c := newSessionIDTestContext(t)
	c.Request.Header.Set("session-id", "raw-codex-session")
	c.Request.Header.Set("session_id", "underscore-session")

	resolution := resolveOpenAIWSSessionHeaders(c, "prompt-key")
	require.Equal(t, "raw-codex-session", resolution.SessionID)
	require.Equal(t, "header_session-id", resolution.SessionSource)

	c2 := newSessionIDTestContext(t)
	c2.Request.Header.Set("session_id", "underscore-session")
	require.Equal(t, "header_session_id", resolveOpenAIWSSessionHeaders(c2, "").SessionSource)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	headers, _ := svc.buildOpenAIWSHeaders(c, newCodexIdentityTestOAuthAccount(), "tok", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "gpt-5.4", "")
	requireNoRawSessionIDHeader(t, headers, "raw-codex-session")
	require.Equal(t, isolateOpenAISessionID(getAPIKeyIDFromContext(c), "raw-codex-session"), headers.Get("session_id"))
}
