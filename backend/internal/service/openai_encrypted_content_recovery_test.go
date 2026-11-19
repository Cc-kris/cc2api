package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_Forward_HTTPRetryRecoveryDropsCompaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"bad encrypted content"}}`)),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2}}`)),
			},
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID:          10,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com",
		},
		Extra: map[string]any{"use_responses_api": true},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":[{"id":"cmp_stale","type":"compaction","encrypted_content":"gAAA"},{"type":"message","content":[{"type":"input_text","text":"hi"}]}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "compaction", gjson.GetBytes(upstream.bodies[0], "input.0.type").String())
	require.Equal(t, "message", gjson.GetBytes(upstream.bodies[1], "input.0.type").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.1").Exists())
}

func TestOpenAIGatewayService_Forward_HTTPThinkingSignatureRecoveryAndLineage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := func(status int, body string) *http.Response {
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		response(http.StatusBadRequest, `{"error":{"code":"thinking_signature_invalid","message":"The encrypted content abc could not be verified. Reason: Encrypted content could not be decrypted or parsed."}}`),
		response(http.StatusOK, `{"id":"resp_recovered","usage":{"input_tokens":1,"output_tokens":2}}`),
		response(http.StatusOK, `{"id":"resp_next","usage":{"input_tokens":1,"output_tokens":2}}`),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{ID: 10, Name: "openai-apikey", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com"},
		Extra: map[string]any{"use_responses_api": true}}
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
		return c
	}
	input := `{"model":"gpt-5","stream":false,"prompt_cache_key":"issue-7700","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"input":[{"type":"reasoning","encrypted_content":"stale","summary":[{"type":"summary_text","text":"keep"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]},{"type":"function_call_output","call_id":"call_1","output":"result"}]}`
	first, err := svc.Forward(context.Background(), newContext(), account, []byte(input))
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "stale", gjson.GetBytes(upstream.bodies[0], "input.0.encrypted_content").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.encrypted_content").Exists())
	require.Equal(t, "keep", gjson.GetBytes(upstream.bodies[1], "input.0.summary.0.text").String())
	require.Equal(t, "hello", gjson.GetBytes(upstream.bodies[1], "input.1.content.0.text").String())
	require.Equal(t, "result", gjson.GetBytes(upstream.bodies[1], "input.2.output").String())
	require.Equal(t, "lookup", gjson.GetBytes(upstream.bodies[1], "tools.0.name").String())

	second, err := svc.Forward(context.Background(), newContext(), account, []byte(input))
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Len(t, upstream.bodies, 3, "later turn should be stripped before its first upstream request")
	require.False(t, gjson.GetBytes(upstream.bodies[2], "input.0.encrypted_content").Exists())
	require.Equal(t, "result", gjson.GetBytes(upstream.bodies[2], "input.2.output").String())
}

func TestOpenAIGatewayService_Forward_HTTPThinkingSignatureDoesNotRetryUnrelatedErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ name, code, message, input string }{
		{"no encrypted reasoning", "thinking_signature_invalid", "The encrypted content could not be verified. Reason: Encrypted content could not be decrypted or parsed.", `[{"type":"message","role":"user","content":"hello"}]`},
		{"ordinary 400", "invalid_request_error", "bad request", `[{"type":"reasoning","encrypted_content":"stale"},{"type":"message","content":"hello"}]`},
		{"unrelated signature error", "thinking_signature_invalid", "Invalid signature", `[{"type":"reasoning","encrypted_content":"stale"},{"type":"message","content":"hello"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"` + tc.code + `","message":"` + tc.message + `"}}`))}}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := &Account{ID: 10, Name: "openai-apikey", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com"}, Extra: map[string]any{"use_responses_api": true}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			_, _ = svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5","stream":false,"input":`+tc.input+`}`))
			require.Len(t, upstream.bodies, 1)
		})
	}
}

func TestTrimOpenAIEncryptedReasoningItems_Compaction(t *testing.T) {
	tests := []struct {
		name      string
		itemType  string
		encrypted bool
		changed   bool
	}{
		{name: "compaction", itemType: "compaction", encrypted: true, changed: true},
		{name: "compaction summary", itemType: "compaction_summary", encrypted: true, changed: true},
		{name: "unencrypted compaction", itemType: "compaction", encrypted: false, changed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := map[string]any{"type": tt.itemType, "id": "cmp_stale"}
			if tt.encrypted {
				item["encrypted_content"] = "gAAA"
			}
			reqBody := map[string]any{"input": []any{
				item,
				map[string]any{"type": "message", "content": "hi"},
			}}

			changed := trimOpenAIEncryptedReasoningItems(reqBody)
			assert.Equal(t, tt.changed, changed)
			input, ok := reqBody["input"].([]any)
			require.True(t, ok)
			require.NotEmpty(t, input)
			first, ok := input[0].(map[string]any)
			require.True(t, ok)
			if tt.changed {
				require.Len(t, input, 1)
				assert.Equal(t, "message", first["type"])
				return
			}
			require.Len(t, input, 2)
			assert.Equal(t, tt.itemType, first["type"])
		})
	}
}
