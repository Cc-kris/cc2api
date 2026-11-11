package service

// V03-R11 用量审计字段测试。session_id 用例移植自上游 1c0cb24c7 session_id_test.go，
// mismatch / 观测器用例移植自 db0bff82c upstream_response_model_test.go，按 CCAI 精简实现适配。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newUsageAuditHeaderContext(t *testing.T, headers map[string]string, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c
}

func TestSanitizeSessionID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   \t  ", ""},
		{"trims surrounding whitespace", "  sess-123  ", "sess-123"},
		{"uuid", "550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440000"},
		{"reject LF", "sess\n123", ""},
		{"reject CRLF injection", "sess-1\r\nSet-Cookie: x=y", ""},
		{"reject NUL", "sess\x00123", ""},
		{"reject DEL", "sess\x7f123", ""},
		{"reject invalid UTF-8", string([]byte{'s', 0xff}), ""},
		{"accepts value at column bound", strings.Repeat("b", maxPersistedSessionIDLength), strings.Repeat("b", maxPersistedSessionIDLength)},
		{"rejects overlong value", strings.Repeat("a", maxPersistedSessionIDLength+1), ""},
		{"rejects overlong multibyte value", strings.Repeat("好", maxPersistedSessionIDLength+1), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, sanitizeSessionID(tc.in))
		})
	}
}

func TestExtractClientSessionID_ExplicitHeaders(t *testing.T) {
	require.Equal(t, "", ExtractClientSessionID(nil))
	require.Equal(t, "", ExtractClientSessionID(&gin.Context{}))

	for _, header := range []string{"session_id", "conversation_id", "X-Session-Affinity", "X-Session-Id", "X-OpenCode-Session", "X-Conversation-ID", "X-Claude-Code-Session-Id"} {
		c := newUsageAuditHeaderContext(t, map[string]string{header: " sess-" + header + " "}, "")
		require.Equal(t, "sess-"+header, ExtractClientSessionID(c), header)
	}

	// 优先级：session_id 先于 X-Claude-Code-Session-Id。
	c := newUsageAuditHeaderContext(t, map[string]string{"session_id": "first", "X-Claude-Code-Session-Id": "second"}, "")
	require.Equal(t, "first", ExtractClientSessionID(c))

	// 非法值跳过，继续取下一个合法头。
	c = newUsageAuditHeaderContext(t, map[string]string{"session_id": "bad\nvalue", "X-Session-Id": "good"}, "")
	require.Equal(t, "good", ExtractClientSessionID(c))
}

// AC32 反向用例：请求只有能算出粘性会话哈希的内容（metadata.user_id、prompt_cache_key），
// 没有显式会话头时，session_id 必须为空。
func TestExtractClientSessionID_StickyHashOnlyRequestStaysEmpty(t *testing.T) {
	body := `{"model":"claude-sonnet-4","prompt_cache_key":"pck-123","metadata":{"user_id":"user_abc_account__session_11111111-2222-3333-4444-555555555555"},"messages":[{"role":"user","content":"hi"}]}`
	c := newUsageAuditHeaderContext(t, map[string]string{"User-Agent": "claude-cli/2.0"}, body)
	require.Equal(t, "", ExtractClientSessionID(c))

	audit := buildUsageAuditFields("claude-sonnet-4", "", ExtractClientSessionID(c), nil, nil)
	require.Nil(t, audit.SessionID)
}

func TestUpstreamModelMismatch_ThreeStateCaseInsensitive(t *testing.T) {
	require.Nil(t, upstreamModelMismatch("gpt-5.5", ""))
	matched := upstreamModelMismatch("gpt-5.5", "GPT-5.5")
	require.NotNil(t, matched)
	require.False(t, *matched)
	mismatched := upstreamModelMismatch("gpt-5.5", "gpt-5.4")
	require.NotNil(t, mismatched)
	require.True(t, *mismatched)
	emptySent := upstreamModelMismatch("", "gpt-5.4")
	require.NotNil(t, emptySent)
	require.True(t, *emptySent)

	require.Equal(t, "mapped", upstreamSentModel("requested", " mapped "))
	require.Equal(t, "requested", upstreamSentModel(" requested ", ""))
}

func TestUpstreamResponseModelObserver_TerminalWinsAndAttemptReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	observeOpenAIResponseModel(c, []byte(`{"type":"response.created","response":{"model":"gpt-5.5"}}`), "response.created")
	observeOpenAIResponseModel(c, []byte(`{"type":"response.output_text.delta","delta":"x"}`), "response.output_text.delta")
	require.Equal(t, "gpt-5.5", ObservedUpstreamResponseModel(c))
	observeOpenAIResponseModel(c, []byte(`{"type":"response.completed","response":{"model":"gpt-5.4"}}`), "response.completed")
	require.Equal(t, "gpt-5.4", ObservedUpstreamResponseModel(c))

	// 换号：新一次尝试必须清掉上次尝试观测到的模型。
	beginUpstreamResponseModelObservation(c)
	require.Equal(t, "", ObservedUpstreamResponseModel(c))
	observeAnthropicResponseModel(c, []byte(`{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`))
	observeAnthropicResponseModel(c, []byte(`{"type":"content_block_delta"}`))
	require.Equal(t, "claude-sonnet-4-20250514", ObservedUpstreamResponseModel(c))

	beginUpstreamResponseModelObservation(c)
	observeOpenAIResponseBody(c, []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.6\"}}\n\n"))
	require.Equal(t, "gpt-5.6", ObservedUpstreamResponseModel(c))

	long := strings.Repeat("m", upstreamResponseModelMaxLength+20)
	beginUpstreamResponseModelObservation(c)
	observeAnthropicResponseModel(c, []byte(`{"model":"`+long+`"}`))
	require.Len(t, ObservedUpstreamResponseModel(c), upstreamResponseModelMaxLength)
}

func TestCaptureRequestedReasoningEffort_KeepsClientValueBeforeMapping(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"reasoning":{"effort":"Minimal"}}`, "minimal"},
		{`{"reasoning_effort":"xhigh"}`, "xhigh"},
		{`{"output_config":{"effort":"max"}}`, "max"},
		{`{"model":"gpt-5.4-high"}`, ""},
		{`{"reasoning":{"effort":"bad value!"}}`, ""},
	}
	for _, tc := range cases {
		c := newUsageAuditHeaderContext(t, nil, tc.body)
		CaptureRequestedReasoningEffort(c, []byte(tc.body))
		got := RequestedReasoningEffortFromGin(c)
		if tc.want == "" {
			require.Nil(t, got, tc.body)
			continue
		}
		require.NotNil(t, got, tc.body)
		require.Equal(t, tc.want, *got)
	}

	// 只取第一次（入站原始请求体），后续改写过的请求体不覆盖。
	c := newUsageAuditHeaderContext(t, nil, "")
	CaptureRequestedReasoningEffort(c, []byte(`{"output_config":{"effort":"max"}}`))
	CaptureRequestedReasoningEffort(c, []byte(`{"reasoning":{"effort":"xhigh"}}`))
	require.Equal(t, "max", *RequestedReasoningEffortFromGin(c))

	forwarded := "xhigh"
	requested := "max"
	require.Equal(t, "max", *coalesceRequestedReasoningEffort(&requested, &forwarded))
	require.Equal(t, "xhigh", *coalesceRequestedReasoningEffort(nil, &forwarded))
	require.Nil(t, coalesceRequestedReasoningEffort(nil, nil))
}

func newUsageAuditGatewayServiceForTest(usageRepo UsageLogRepository, userRepo UserRepository) *GatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	return NewGatewayService(
		nil, nil, usageRepo, nil, userRepo, &openAIRecordUsageSubRepoStub{}, nil, nil, cfg, nil, nil,
		NewBillingService(cfg, nil), nil, &BillingCacheService{}, nil, nil, &DeferredService{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
}

type usageAuditBillingSnapshot struct {
	input, output, cacheCreation, cacheRead, total, actual, rate float64
	deducted                                                     float64
}

func snapshotUsageAuditBilling(log *UsageLog, deducted float64) usageAuditBillingSnapshot {
	return usageAuditBillingSnapshot{
		input: log.InputCost, output: log.OutputCost, cacheCreation: log.CacheCreationCost, cacheRead: log.CacheReadCost,
		total: log.TotalCost, actual: log.ActualCost, rate: log.RateMultiplier, deducted: deducted,
	}
}

// AC31：Anthropic 路径写入 session_id、upstream_response_model、mismatch、requested_reasoning_effort；
// 费用与扣费金额和不写这些字段时完全相同。
func TestGatewayRecordUsage_AuditFieldsDoNotChangeBilling(t *testing.T) {
	run := func(withAudit bool) (*UsageLog, float64) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newUsageAuditGatewayServiceForTest(usageRepo, userRepo)
		effort := "high"
		result := &ForwardResult{
			RequestID:       "gateway_audit_billing",
			Usage:           ClaudeUsage{InputTokens: 1200, OutputTokens: 340, CacheReadInputTokens: 50},
			Model:           "claude-sonnet-4",
			UpstreamModel:   "claude-sonnet-4-20250514",
			Duration:        time.Second,
			ReasoningEffort: &effort,
		}
		input := &RecordUsageInput{
			Result:  result,
			APIKey:  &APIKey{ID: 501},
			User:    &User{ID: 601},
			Account: &Account{ID: 701, Platform: PlatformAnthropic},
		}
		if withAudit {
			requested := "max"
			result.UpstreamResponseModel = "claude-sonnet-4-5-20250929"
			result.RequestedReasoningEffort = &requested
			input.SessionID = "sess-abc"
		}
		require.NoError(t, svc.RecordUsage(context.Background(), input))
		require.NotNil(t, usageRepo.lastLog)
		return usageRepo.lastLog, userRepo.lastAmount
	}

	plain, plainDeducted := run(false)
	audited, auditedDeducted := run(true)

	require.Equal(t, snapshotUsageAuditBilling(plain, plainDeducted), snapshotUsageAuditBilling(audited, auditedDeducted))
	require.Greater(t, audited.ActualCost, 0.0)

	require.Nil(t, plain.UpstreamResponseModel)
	require.Nil(t, plain.UpstreamModelMismatch)
	require.Nil(t, plain.SessionID)
	require.Equal(t, "high", *plain.RequestedReasoningEffort, "无显式请求值时回落到转发值")

	require.Equal(t, "claude-sonnet-4-5-20250929", *audited.UpstreamResponseModel)
	require.True(t, *audited.UpstreamModelMismatch)
	require.Equal(t, "sess-abc", *audited.SessionID)
	require.Equal(t, "max", *audited.RequestedReasoningEffort)
	require.Equal(t, "high", *audited.ReasoningEffort)
}

// AC31：OpenAI 路径同样只写审计字段，不改金额。
func TestOpenAIRecordUsage_AuditFieldsDoNotChangeBilling(t *testing.T) {
	run := func(withAudit bool) (*UsageLog, float64) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
		result := &OpenAIForwardResult{
			RequestID: "openai_audit_billing",
			Usage:     OpenAIUsage{InputTokens: 900, OutputTokens: 120},
			Model:     "gpt-5.1",
			Duration:  time.Second,
		}
		input := &OpenAIRecordUsageInput{
			Result:  result,
			APIKey:  &APIKey{ID: 502},
			User:    &User{ID: 602},
			Account: &Account{ID: 702, Platform: PlatformOpenAI},
		}
		if withAudit {
			requested := "minimal"
			result.UpstreamResponseModel = "GPT-5.1"
			result.RequestedReasoningEffort = &requested
			input.SessionID = "conv-1"
		}
		require.NoError(t, svc.RecordUsage(context.Background(), input))
		require.NotNil(t, usageRepo.lastLog)
		return usageRepo.lastLog, userRepo.lastAmount
	}

	plain, plainDeducted := run(false)
	audited, auditedDeducted := run(true)

	require.Equal(t, snapshotUsageAuditBilling(plain, plainDeducted), snapshotUsageAuditBilling(audited, auditedDeducted))
	require.Greater(t, audited.ActualCost, 0.0)
	require.Equal(t, "GPT-5.1", *audited.UpstreamResponseModel)
	require.False(t, *audited.UpstreamModelMismatch, "大小写不同不算不一致")
	require.Equal(t, "conv-1", *audited.SessionID)
	require.Equal(t, "minimal", *audited.RequestedReasoningEffort)
	require.Nil(t, plain.RequestedReasoningEffort)
}
