package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// V03-T04：续写中断错误必须带上已观测用量（只供财务证据与耗尽收费），
// 且 partialStreamUsageResult 对续写中断错误返回 nil（用户扣费不在中断尝试上发生）。

func TestGatewayService_ForwardContinuationInterruptCarriesObservedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	AttachStreamContinuation(c)

	body := []byte(`{"model":"claude-sonnet-4-6","stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	parsed, err := ParseGatewayRequest(body, PlatformAnthropic)
	require.NoError(t, err)

	upstreamSSE := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1000,"cache_read_input_tokens":20}}}`,
		"",
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		"",
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":300}}`,
		"",
		"",
	}, "\n")
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       &streamReadCloser{payload: []byte(upstreamSSE), err: io.ErrUnexpectedEOF},
	}}
	svc := newForwardPartialUsageServiceForTest(upstream)

	result, err := svc.Forward(context.Background(), c, newAnthropicOAuthAccountForPartialUsageTest(), parsed)
	require.Nil(t, result, "续写中断属于失败转移错误，必须保持 result=nil")
	var foErr *UpstreamFailoverError
	require.True(t, errors.As(err, &foErr))
	require.True(t, foErr.SafeToFailoverAfterWrite)
	require.Equal(t, GatewayFailureReasonStreamInterrupted, foErr.Reason)
	require.NotNil(t, foErr.ObservedUsage)
	require.Equal(t, 1000, foErr.ObservedUsage.InputTokens)
	require.Equal(t, 20, foErr.ObservedUsage.CacheReadInputTokens)
	require.Equal(t, 300, foErr.ObservedUsage.OutputTokens)
	require.Equal(t, "claude-sonnet-4-6", foErr.ObservedModel)

	observed := foErr.ObservedForwardResult()
	require.NotNil(t, observed)
	require.True(t, observed.Stream)
	require.Equal(t, 1000, observed.Usage.InputTokens)
	require.Equal(t, 300, observed.Usage.OutputTokens)
}

func TestPartialStreamUsageResult_ContinuationFailoverReturnsNil(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	foErr := sc.NewStreamInterruptedFailover("upstream stream disconnected")
	attachObservedClaudeUsage(foErr, &ClaudeUsage{InputTokens: 1000, OutputTokens: 300}, "claude-sonnet-4-6", "claude-sonnet-4-6")
	require.NotNil(t, foErr.ObservedUsage)

	streamResult := &streamingResult{usage: &ClaudeUsage{InputTokens: 1000, OutputTokens: 300}}
	resp := &http.Response{Header: http.Header{}}
	require.Nil(t, partialStreamUsageResult(resp, streamResult, "claude-sonnet-4-6", "claude-sonnet-4-6", time.Now(), foErr))
}

func TestAttachObservedClaudeUsage_NoTokensLeavesNil(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	foErr := sc.NewStreamInterruptedFailover("x")
	attachObservedClaudeUsage(foErr, &ClaudeUsage{}, "m", "m")
	require.Nil(t, foErr.ObservedUsage)
	require.Nil(t, foErr.ObservedForwardResult())
}

func TestAttachObservedOpenAIUsage_ConvertsToClaudeShape(t *testing.T) {
	sc := &StreamContinuation{openIndex: -1}
	foErr := sc.NewStreamInterruptedFailover("x")
	attachObservedOpenAIUsage(foErr, &OpenAIUsage{InputTokens: 1200, CacheReadInputTokens: 200, OutputTokens: 50}, "gpt-5.4", "gpt-5.4")

	require.NotNil(t, foErr.ObservedOpenAIUsage)
	require.Equal(t, 1200, foErr.ObservedOpenAIUsage.InputTokens)
	require.NotNil(t, foErr.ObservedUsage)
	require.Equal(t, 1000, foErr.ObservedUsage.InputTokens, "Claude 口径 input 不含缓存读")
	require.Equal(t, 200, foErr.ObservedUsage.CacheReadInputTokens)
	require.Equal(t, 50, foErr.ObservedUsage.OutputTokens)

	result := foErr.ObservedOpenAIForwardResult()
	require.NotNil(t, result)
	require.Equal(t, 1200, result.Usage.InputTokens)
	require.Equal(t, "gpt-5.4", result.Model)
}

func TestBuildBillableUsageAttemptFromUsage(t *testing.T) {
	attempt, ok := BuildBillableUsageAttemptFromUsage(&ClaudeUsage{InputTokens: 1000, OutputTokens: 300, CacheReadInputTokens: 5}, "", 1, 42, nil, "claude-sonnet-4-6", nil, nil, time.Time{})
	require.True(t, ok)
	require.EqualValues(t, 1000, attempt.InputTokens)
	require.EqualValues(t, 300, attempt.OutputTokens)
	require.EqualValues(t, 5, attempt.CacheReadTokens)
	require.True(t, attempt.Billable)
	require.Equal(t, 1, attempt.AttemptNo)

	_, ok = BuildBillableUsageAttemptFromUsage(&ClaudeUsage{}, "", 1, 42, nil, "m", nil, nil, time.Time{})
	require.False(t, ok, "无观测 token 不生成尝试记录")
	_, ok = BuildBillableUsageAttemptFromUsage(nil, "", 1, 42, nil, "m", nil, nil, time.Time{})
	require.False(t, ok)
	_, ok = BuildBillableUsageAttemptFromUsage(&ClaudeUsage{InputTokens: 1}, "", 1, 0, nil, "m", nil, nil, time.Time{})
	require.False(t, ok, "账号缺失不生成尝试记录")
}
