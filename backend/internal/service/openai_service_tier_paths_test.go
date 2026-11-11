package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V03-T13：OpenAI 各路径服务档位只降不升（移植上游 75faedda9 的 OpenAI 子路径）。
//
// 每条路径都走真实的响应处理函数拿到 OpenAIForwardResult，再交给 RecordUsage，
// 按 AC24 口径断言 usage_logs.service_tier 与金额。gpt-5.4、输入 1000、输出 1000：
//   - default：1000×2.5e-6 + 1000×1.5e-5 = 0.01750000
//   - priority：1000×5e-6 + 1000×3e-5 = 0.03500000
//   - flex：0.01750000 × 0.5 = 0.00875000
// ActualCost 再乘默认倍率 1.1。

const (
	tierPathCostDefault  = 0.0175
	tierPathCostPriority = 0.035
	tierPathCostFlex     = 0.00875
	tierPathRateMultiple = 1.1
)

type tierPathCase struct {
	name      string
	requested string // request service_tier ("" = none)
	echoed    string // tier echoed by non-terminal events (must be ignored)
	declared  string // tier declared by the terminal event / response body
	wantTier  string // usage_logs.service_tier ("" = NULL)
	wantCost  float64
}

// tierPathCases is the shared matrix: downgrade honoured, more expensive /
// unknown / missing declarations keep the requested tier.
var tierPathCases = []tierPathCase{
	{name: "priority served as default bills default", requested: "priority", echoed: "priority", declared: "default", wantTier: "default", wantCost: tierPathCostDefault},
	{name: "priority served as flex bills flex", requested: "priority", echoed: "priority", declared: "flex", wantTier: "flex", wantCost: tierPathCostFlex},
	{name: "unknown declared tier keeps priority", requested: "priority", echoed: "priority", declared: "turbo", wantTier: "priority", wantCost: tierPathCostPriority},
	{name: "missing declaration keeps priority", requested: "priority", echoed: "priority", declared: "", wantTier: "priority", wantCost: tierPathCostPriority},
	{name: "flex never raised to default", requested: "flex", echoed: "flex", declared: "default", wantTier: "flex", wantCost: tierPathCostFlex},
	{name: "untiered request never raised to priority", requested: "", echoed: "", declared: "priority", wantTier: "", wantCost: tierPathCostDefault},
}

func tierPathJSONField(tier string) string {
	if tier == "" {
		return ""
	}
	return `,"service_tier":"` + tier + `"`
}

// tierPathResponsesSSE builds a Responses API stream whose response.created
// echoes `echoed` and whose response.completed declares `declared`.
func tierPathResponsesSSE(id, echoed, declared string) string {
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"` + id + `","model":"gpt-5.4","status":"in_progress","output":[]` + tierPathJSONField(echoed) + `}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"` + id + `","object":"response","model":"gpt-5.4","status":"completed"` + tierPathJSONField(declared) + `,"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1000,"output_tokens":1000,"total_tokens":2000}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
}

// tierPathChatSSE builds a Chat Completions stream; every chunk carries the
// processing tier (OpenAI repeats service_tier on each chunk).
func tierPathChatSSE(declared string) string {
	field := tierPathJSONField(declared)
	return strings.Join([]string{
		`data: {"id":"chatcmpl_tier","object":"chat.completion.chunk","model":"gpt-5.4"` + field + `,"choices":[{"index":0,"delta":{"role":"assistant","content":"ok"}}]}`,
		"",
		`data: {"id":"chatcmpl_tier","object":"chat.completion.chunk","model":"gpt-5.4"` + field + `,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_tier","object":"chat.completion.chunk","model":"gpt-5.4"` + field + `,"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":1000,"total_tokens":2000}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
}

func tierPathChatJSON(declared string) string {
	return `{"id":"chatcmpl_tier","object":"chat.completion","model":"gpt-5.4"` + tierPathJSONField(declared) + `,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":1000,"total_tokens":2000}}`
}

func tierPathResponsesJSON(declared string) string {
	return `{"id":"resp_tier_json","object":"response","model":"gpt-5.4","status":"completed"` + tierPathJSONField(declared) + `,"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1000,"output_tokens":1000,"total_tokens":2000}}`
}

func tierPathHTTPUpstream(contentType, body string) *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}, "x-request-id": []string{"rid_tier_path"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
}

func tierPathConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true}}}
}

// tierPathAPIKeyAccount is a public OpenAI API key account: its response tier
// is authoritative (the Codex OAuth exception does not apply).
func tierPathAPIKeyAccount(extra map[string]any) *Account {
	return &Account{
		ID:          7301,
		Name:        "openai-apikey-tier",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
		Extra: extra,
	}
}

func tierPathForceResponses() map[string]any {
	return map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses)}
}

func tierPathGinContext(path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

// recordTierPathUsage runs the real RecordUsage for a forward result and
// returns the usage log it wrote.
func recordTierPathUsage(t *testing.T, account *Account, result *OpenAIForwardResult) *UsageLog {
	t.Helper()
	require.NotNil(t, result)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result:  result,
		APIKey:  &APIKey{ID: 1301},
		User:    &User{ID: 2301},
		Account: account,
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	return usageRepo.lastLog
}

func assertTierPathBilling(t *testing.T, tc tierPathCase, log *UsageLog) {
	t.Helper()
	if tc.wantTier == "" {
		require.Nil(t, log.ServiceTier, "untiered request must stay untiered")
	} else {
		require.NotNil(t, log.ServiceTier)
		require.Equal(t, tc.wantTier, *log.ServiceTier, "usage_logs.service_tier must record the billed tier")
	}
	require.InDelta(t, tc.wantCost, log.TotalCost, 1e-12)
	require.InDelta(t, tc.wantCost*tierPathRateMultiple, log.ActualCost, 1e-12)
}

// openai_gateway_chat_completions.go：/v1/chat/completions → Responses 上游，流式与缓冲两条。
func TestServiceTierPath_ChatCompletionsViaResponses(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range tierPathCases {
			name := "buffered/" + tc.name
			if stream {
				name = "stream/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stream":` + boolJSON(stream) + tierPathJSONField(tc.requested) + `}`)
				c, _ := tierPathGinContext("/v1/chat/completions", body)
				account := tierPathAPIKeyAccount(tierPathForceResponses())
				svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: tierPathHTTPUpstream("text/event-stream", tierPathResponsesSSE("resp_cc_tier", tc.echoed, tc.declared))}

				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				require.NoError(t, err)
				assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
			})
		}
	}
}

// openai_gateway_chat_completions_raw.go：原生 Chat Completions 上游，流式与非流式。
func TestServiceTierPath_RawChatCompletions(t *testing.T) {
	cases := append([]tierPathCase(nil), tierPathCases...)
	for _, stream := range []bool{true, false} {
		for _, tc := range cases {
			name := "json/" + tc.name
			if stream {
				name = "stream/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stream":` + boolJSON(stream) + tierPathJSONField(tc.requested) + `}`)
				c, _ := tierPathGinContext("/v1/chat/completions", body)
				account := tierPathAPIKeyAccount(nil)
				upstream := tierPathHTTPUpstream("application/json", tierPathChatJSON(tc.declared))
				if stream {
					upstream = tierPathHTTPUpstream("text/event-stream", tierPathChatSSE(tc.declared))
				}
				svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: upstream}

				result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
				require.NoError(t, err)
				assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
			})
		}
	}

	t.Run("stream/conflicting chunk declarations keep priority", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stream":true,"service_tier":"priority"}`)
		c, _ := tierPathGinContext("/v1/chat/completions", body)
		account := tierPathAPIKeyAccount(nil)
		upstreamBody := strings.Join([]string{
			`data: {"id":"chatcmpl_tier","object":"chat.completion.chunk","model":"gpt-5.4","service_tier":"priority","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			"",
			`data: {"id":"chatcmpl_tier","object":"chat.completion.chunk","model":"gpt-5.4","service_tier":"default","choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":1000,"total_tokens":2000}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")
		svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: tierPathHTTPUpstream("text/event-stream", upstreamBody)}

		result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
		require.NoError(t, err)
		assertTierPathBilling(t, tierPathCase{wantTier: "priority", wantCost: tierPathCostPriority}, recordTierPathUsage(t, account, result))
	})
}

// openai_gateway_messages.go：/v1/messages → Responses 上游。Anthropic 协议只能经
// fast-mode beta 头请求 priority，因此只取 priority / 无档位的用例。
func TestServiceTierPath_AnthropicMessagesViaResponses(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range tierPathCases {
			if tc.requested != "priority" && tc.requested != "" {
				continue
			}
			name := "buffered/" + tc.name
			if stream {
				name = "stream/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":` + boolJSON(stream) + `}`)
				c, _ := tierPathGinContext("/v1/messages", body)
				if tc.requested == "priority" {
					c.Request.Header.Set("anthropic-beta", "fast-mode-2026-02-01")
				}
				account := tierPathAPIKeyAccount(tierPathForceResponses())
				svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: tierPathHTTPUpstream("text/event-stream", tierPathResponsesSSE("resp_msg_tier", tc.echoed, tc.declared))}

				result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
				require.NoError(t, err)
				assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
			})
		}
	}
}

// openai_gateway_service.go Forward（上游 openai_gateway_forward.go）：/v1/responses 原生路径。
func TestServiceTierPath_ResponsesForward(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range tierPathCases {
			name := "json/" + tc.name
			if stream {
				name = "stream/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","input":"hi","stream":` + boolJSON(stream) + tierPathJSONField(tc.requested) + `}`)
				c, _ := tierPathGinContext("/v1/responses", body)
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
				account := tierPathAPIKeyAccount(nil)
				upstream := tierPathHTTPUpstream("application/json", tierPathResponsesJSON(tc.declared))
				if stream {
					upstream = tierPathHTTPUpstream("text/event-stream", tierPathResponsesSSE("resp_fwd_tier", tc.echoed, tc.declared))
				}
				svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}

				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
			})
		}
	}
}

// openai_gateway_service.go forwardOpenAIPassthrough（上游 openai_gateway_passthrough.go）。
func TestServiceTierPath_ResponsesPassthrough(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range tierPathCases {
			name := "json/" + tc.name
			if stream {
				name = "stream/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","input":"hi","stream":` + boolJSON(stream) + tierPathJSONField(tc.requested) + `}`)
				c, _ := tierPathGinContext("/v1/responses", body)
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
				account := tierPathAPIKeyAccount(map[string]any{"openai_passthrough": true})
				upstream := tierPathHTTPUpstream("application/json", tierPathResponsesJSON(tc.declared))
				if stream {
					upstream = tierPathHTTPUpstream("text/event-stream", tierPathResponsesSSE("resp_pt_tier", tc.echoed, tc.declared))
				}
				svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}

				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
			})
		}
	}
}

// openai_gateway_responses_chat_fallback.go：/v1/responses 被强制转发到 Chat Completions 上游。
// 上游 75faedda9 列表外，同类漏洞一并补上（上游对应 openai_gateway_cc_pipeline.go）。
func TestServiceTierPath_ResponsesViaChatFallback(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range tierPathCases {
			name := "json/" + tc.name
			if stream {
				name = "stream/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","input":"hi","stream":` + boolJSON(stream) + tierPathJSONField(tc.requested) + `}`)
				c, _ := tierPathGinContext("/v1/responses", body)
				account := tierPathAPIKeyAccount(map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)})
				upstream := tierPathHTTPUpstream("application/json", tierPathChatJSON(tc.declared))
				if stream {
					upstream = tierPathHTTPUpstream("text/event-stream", tierPathChatSSE(tc.declared))
				}
				svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: upstream}

				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
			})
		}
	}
}

// Codex OAuth 例外（上游 3a9070359/50ad6e2e5）：ChatGPT 私有端点对实际 priority 也回 default，
// 不可信，按请求档位计费；声明 flex 时仍可降档。
func TestServiceTierPath_CodexOAuthDefaultNotAuthoritative(t *testing.T) {
	oauthAccount := &Account{
		ID:          7302,
		Name:        "openai-oauth-tier",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}
	cases := []tierPathCase{
		{name: "default declaration ignored", requested: "priority", echoed: "priority", declared: "default", wantTier: "priority", wantCost: tierPathCostPriority},
		{name: "flex declaration still lowers", requested: "priority", echoed: "priority", declared: "flex", wantTier: "flex", wantCost: tierPathCostFlex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stream":true` + tierPathJSONField(tc.requested) + `}`)
			c, _ := tierPathGinContext("/v1/chat/completions", body)
			svc := &OpenAIGatewayService{httpUpstream: tierPathHTTPUpstream("text/event-stream", tierPathResponsesSSE("resp_oauth_tier", tc.echoed, tc.declared))}

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, oauthAccount, body, "", "")
			require.NoError(t, err)
			assertTierPathBilling(t, tc, recordTierPathUsage(t, oauthAccount, result))
		})
	}
}

func boolJSON(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func tierPathWSConfig() *config.Config {
	cfg := tierPathConfig()
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	return cfg
}

func tierPathWSCreated(id, echoed string) []byte {
	return []byte(`{"type":"response.created","response":{"id":"` + id + `","model":"gpt-5.4"` + tierPathJSONField(echoed) + `}}`)
}

func tierPathWSCompleted(id, declared string) []byte {
	return []byte(`{"type":"response.completed","response":{"id":"` + id + `","model":"gpt-5.4"` + tierPathJSONField(declared) + `,"usage":{"input_tokens":1000,"output_tokens":1000}}}`)
}

// runTierPathIngress drives ProxyResponsesWebSocketFromClient through one
// client turn per request and returns the per-turn forward results.
func runTierPathIngress(t *testing.T, svc *OpenAIGatewayService, account *Account, requests []string) []*OpenAIForwardResult {
	t.Helper()
	resultCh := make(chan *OpenAIForwardResult, len(requests)+1)
	hooks := &OpenAIWSIngressHooks{
		AfterTurn: func(_ int, result *OpenAIForwardResult, turnErr error) {
			if turnErr == nil && result != nil {
				resultCh <- result
			}
		},
	}
	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "unit-test-agent/1.0")
		ginCtx.Request = req

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			serverErrCh <- errors.New("unsupported websocket client message type")
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, hooks)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	results := make([]*OpenAIForwardResult, 0, len(requests))
	for _, request := range requests {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(request)))
		cancelWrite()
		for {
			readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
			_, event, readErr := clientConn.Read(readCtx)
			cancelRead()
			require.NoError(t, readErr)
			if gjson.GetBytes(event, "type").String() == "response.completed" {
				break
			}
		}
		select {
		case result := <-resultCh:
			results = append(results, result)
		case <-time.After(3 * time.Second):
			t.Fatal("未收到 turn 结果回调")
		}
	}
	_ = clientConn.Close(coderws.StatusNormalClosure, "done")
	select {
	case <-serverErrCh:
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
	}
	return results
}

func tierPathCreateRequest(requested string, previousResponseID string) string {
	prev := ""
	if previousResponseID != "" {
		prev = `,"previous_response_id":"` + previousResponseID + `"`
	}
	return `{"type":"response.create","model":"gpt-5.4","stream":false` + tierPathJSONField(requested) + prev + `}`
}

// openai_ws_forwarder.go forwardOpenAIWSV2（上游 openai_ws_forwarder_v2.go）：HTTP 入站走 WSv2 上游。
func TestServiceTierPath_WSv2Forwarder(t *testing.T) {
	for _, tc := range tierPathCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tierPathWSConfig()
			captureConn := &openAIWSCaptureConn{events: [][]byte{
				tierPathWSCreated("resp_wsv2_tier", tc.echoed),
				tierPathWSCompleted("resp_wsv2_tier", tc.declared),
			}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
			account := tierPathAPIKeyAccount(map[string]any{"responses_websockets_v2_enabled": true})

			body := []byte(`{"model":"gpt-5.4","stream":false,"input":[{"type":"input_text","text":"hi"}]` + tierPathJSONField(tc.requested) + `}`)
			c, _ := tierPathGinContext("/openai/v1/responses", body)
			c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.OpenAIWSMode, "must be served over WSv2")
			require.Nil(t, upstream.lastReq, "WSv2 success must not fall back to HTTP")
			assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
		})
	}
}

func newTierPathCtxPoolService(events [][]byte) *OpenAIGatewayService {
	cfg := tierPathWSConfig()
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: &openAIWSCaptureConn{events: events}})
	return &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
}

// openai_ws_forwarder.go ProxyResponsesWebSocketFromClient ctx_pool 模式（上游 openai_ws_forwarder_ingress.go）。
func TestServiceTierPath_WSIngressCtxPool(t *testing.T) {
	account := tierPathAPIKeyAccount(map[string]any{"responses_websockets_v2_enabled": true})
	for _, tc := range tierPathCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTierPathCtxPoolService([][]byte{
				tierPathWSCreated("resp_ingress_tier", tc.echoed),
				tierPathWSCompleted("resp_ingress_tier", tc.declared),
			})
			results := runTierPathIngress(t, svc, account, []string{tierPathCreateRequest(tc.requested, "")})
			require.Len(t, results, 1)
			assertTierPathBilling(t, tc, recordTierPathUsage(t, account, results[0]))
		})
	}

	t.Run("a turn without declaration does not inherit the previous downgrade", func(t *testing.T) {
		svc := newTierPathCtxPoolService([][]byte{
			tierPathWSCompleted("resp_ingress_turn_1", "default"),
			tierPathWSCompleted("resp_ingress_turn_2", ""),
		})
		results := runTierPathIngress(t, svc, account, []string{
			tierPathCreateRequest("priority", ""),
			tierPathCreateRequest("priority", "resp_ingress_turn_1"),
		})
		require.Len(t, results, 2)
		assertTierPathBilling(t, tierPathCase{wantTier: "default", wantCost: tierPathCostDefault}, recordTierPathUsage(t, account, results[0]))
		assertTierPathBilling(t, tierPathCase{wantTier: "priority", wantCost: tierPathCostPriority}, recordTierPathUsage(t, account, results[1]))
	})
}

// openai_ws_http_bridge.go proxyOpenAIWSHTTPBridgeTurn：WS 入站经 HTTP SSE 上游。
func TestServiceTierPath_WSHTTPBridge(t *testing.T) {
	for _, tc := range tierPathCases {
		t.Run(tc.name, func(t *testing.T) {
			account := tierPathAPIKeyAccount(nil)
			svc := &OpenAIGatewayService{cfg: tierPathConfig(), httpUpstream: tierPathHTTPUpstream("text/event-stream", tierPathResponsesSSE("resp_bridge_tier", tc.echoed, tc.declared)), toolCorrector: NewCodexToolCorrector()}
			c, _ := tierPathGinContext("/v1/responses", nil)
			payload := []byte(tierPathCreateRequest(tc.requested, ""))

			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
				context.Background(), c, account, "sk-test", payload, len(payload),
				"gpt-5.4", "", "", "", "", 1, func([]byte) error { return nil },
			)
			require.NoError(t, err)
			assertTierPathBilling(t, tc, recordTierPathUsage(t, account, result))
		})
	}
}

func newTierPathPassthroughService(events [][]byte) *OpenAIGatewayService {
	cfg := tierPathWSConfig()
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	return &OpenAIGatewayService{
		cfg:                       cfg,
		httpUpstream:              &httpUpstreamRecorder{},
		cache:                     &stubGatewayCache{},
		openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:             NewCodexToolCorrector(),
		openaiWSPassthroughDialer: &openAIWSCaptureDialer{conn: &openAIWSCaptureConn{events: events}},
	}
}

// openai_ws_v2_passthrough_adapter.go + openai_ws_v2/passthrough_relay.go：ingress 透传模式。
func TestServiceTierPath_WSIngressPassthroughRelay(t *testing.T) {
	account := tierPathAPIKeyAccount(map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough})
	for _, tc := range tierPathCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTierPathPassthroughService([][]byte{
				tierPathWSCreated("resp_relay_tier", tc.echoed),
				tierPathWSCompleted("resp_relay_tier", tc.declared),
			})
			results := runTierPathIngress(t, svc, account, []string{tierPathCreateRequest(tc.requested, "")})
			require.Len(t, results, 1)
			assertTierPathBilling(t, tc, recordTierPathUsage(t, account, results[0]))
		})
	}

	t.Run("a turn without declaration does not inherit the previous downgrade", func(t *testing.T) {
		svc := newTierPathPassthroughService([][]byte{
			tierPathWSCompleted("resp_relay_turn_1", "default"),
			tierPathWSCompleted("resp_relay_turn_2", ""),
		})
		results := runTierPathIngress(t, svc, account, []string{
			tierPathCreateRequest("priority", ""),
			tierPathCreateRequest("priority", "resp_relay_turn_1"),
		})
		require.Len(t, results, 2)
		assertTierPathBilling(t, tierPathCase{wantTier: "default", wantCost: tierPathCostDefault}, recordTierPathUsage(t, account, results[0]))
		assertTierPathBilling(t, tierPathCase{wantTier: "priority", wantCost: tierPathCostPriority}, recordTierPathUsage(t, account, results[1]))
	})
}
