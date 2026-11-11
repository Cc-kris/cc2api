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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const antigravityPoolIdentityErrorBody = `{"error":{"code":400,"message":"Permission denied on resource project projects/123456789 for consumer: projects/123456789; caller pool-sa@my-gcp-proj.iam.gserviceaccount.com","status":"FAILED_PRECONDITION","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","metadata":{"consumer":"projects/123456789"}}]}}`

func requireNoAntigravityPoolIdentity(t *testing.T, out string) {
	t.Helper()
	require.NotContains(t, out, "123456789")
	require.NotContains(t, out, "pool-sa@")
	require.NotContains(t, out, "gserviceaccount.com")
	require.NotContains(t, out, "details")
}

// TestAntigravityForwardGemini_UpstreamErrorBodyIsSanitized 锁定 V01-R09：ForwardGemini 不换号的
// 4xx 出口返回 application/json 的 Gemini 风格错误体，不含项目号和服务账号；ops 日志保留原文。
// 用 400（不触发换号）走到最后的写回分支。
func TestAntigravityForwardGemini_UpstreamErrorBodyIsSanitized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{{"role": "user", "parts": []map[string]any{{"text": "hello"}}}},
	})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))

	upstreamBody := []byte(`{"response":` + antigravityPoolIdentityErrorBody + `}`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"text/plain"}, "X-Request-Id": []string{"req-sanitize-1"}},
		Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
	}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize, LogUpstreamErrorBody: true, LogUpstreamErrorBodyMaxBytes: 4096}}
	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, cfg),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   &httpUpstreamStub{resp: resp},
	}
	account := &Account{
		ID: 7, Name: "acc-sanitize", Platform: PlatformAntigravity, Type: AccountTypeOAuth,
		Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"access_token": "token"},
	}

	_, err = svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "generateContent", false, body, false)
	require.Error(t, err)

	require.Equal(t, http.StatusBadRequest, writer.Code)
	require.Equal(t, "application/json", strings.Split(writer.Header().Get("Content-Type"), ";")[0])
	out := writer.Body.String()
	requireNoAntigravityPoolIdentity(t, out)

	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &parsed))
	require.Equal(t, 400, parsed.Error.Code)
	require.Equal(t, "FAILED_PRECONDITION", parsed.Error.Status)
	require.Contains(t, parsed.Error.Message, "Permission denied")
	require.True(t, IsResponseCommitted(c))

	// ops 日志仍保留原始 body
	detail, ok := c.Get(OpsUpstreamErrorDetailKey)
	require.True(t, ok)
	detailStr, ok := detail.(string)
	require.True(t, ok, "detail should be a string")
	require.Contains(t, detailStr, "pool-sa@my-gcp-proj.iam.gserviceaccount.com")
	require.Contains(t, detailStr, "projects/123456789")
}

// TestAntigravityForwardUpstream_UpstreamErrorBodyIsSanitized：Claude 协议的上游透传账号
// （ForwardUpstream）原先把上游错误体原样写回客户端，同样改为脱敏的 Claude 错误体。
func TestAntigravityForwardUpstream_UpstreamErrorBodyIsSanitized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1/messages", bytes.NewReader(body))

	upstreamBody := []byte(`{"type":"error","error":{"type":"permission_error","message":"consumer 987654321 projects/my-gcp-proj sa@x.iam.gserviceaccount.com denied"},"request_id":"r"}`)
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
	}
	cfg := &config.Config{Gateway: config.GatewayConfig{LogUpstreamErrorBody: true, LogUpstreamErrorBodyMaxBytes: 4096}}
	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, cfg),
		httpUpstream:   &httpUpstreamStub{resp: resp},
	}
	account := &Account{
		ID: 8, Name: "acc-upstream", Platform: PlatformAntigravity, Type: AccountTypeUpstream,
		Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"base_url": "https://upstream.example.com", "api_key": "k"},
	}

	result, err := svc.Forward(context.Background(), c, account, body, false)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, http.StatusForbidden, writer.Code)
	require.Equal(t, "application/json", strings.Split(writer.Header().Get("Content-Type"), ";")[0])
	out := writer.Body.String()
	require.NotContains(t, out, "987654321")
	require.NotContains(t, out, "my-gcp-proj")
	require.NotContains(t, out, "gserviceaccount")
	require.NotContains(t, out, "request_id")
	require.JSONEq(t, `{"type":"error","error":{"type":"permission_error","message":"consumer *** projects/*** *** denied"}}`, out)

	detail, ok := c.Get(OpsUpstreamErrorDetailKey)
	require.True(t, ok)
	detailStr, ok := detail.(string)
	require.True(t, ok, "detail should be a string")
	require.Contains(t, detailStr, "sa@x.iam.gserviceaccount.com")
}

func TestBuildAntigravityClaudeClientErrorBody_NonJSONAndUnknownType(t *testing.T) {
	out := string(buildAntigravityClaudeClientErrorBody(http.StatusTooManyRequests, []byte("quota exceeded for consumer 987654321 sa@x.iam.gserviceaccount.com")))
	require.JSONEq(t, `{"type":"error","error":{"type":"rate_limit_error","message":"Too Many Requests"}}`, out)

	out = string(buildAntigravityClaudeClientErrorBody(http.StatusBadGateway, []byte(`{"error":{"type":"<script>","message":"bad"}}`)))
	require.JSONEq(t, `{"type":"error","error":{"type":"api_error","message":"bad"}}`, out)
}
