package service_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// These tests use a real HTTP upstream, the real Forward method, and the real
// handler FailoverState. Candidate enumeration is deliberately fixed, so this
// does NOT assert the authenticated handler, scheduler, or usage persistence.
func TestV043ResponsesFaultInjectionAcceptance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough_%v", passthrough), func(t *testing.T) {
			t.Run("overload_then_sse429_then_success", func(t *testing.T) {
				v043RunForwardFaultScript(t, passthrough, 0, []v043FaultStep{
					{status: 200, body: v043SSE(`{"type":"response.failed","response":{"id":"failed_1","error":{"type":"server_error","code":"server_is_overloaded","message":"server is overloaded"}}}`)},
					{status: 200, body: v043SSE(`{"type":"response.failed","response":{"id":"failed_2","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"capacity temporarily exceeded"}}}`)},
					{status: 200, body: v043SuccessSSE},
				}, []int{1, 1, 1})
			})
			t.Run("queued_metadata_does_not_commit_failed_output", func(t *testing.T) {
				v043RunForwardFaultScript(t, passthrough, 0, []v043FaultStep{
					{status: 200, body: v043SSE(`{"type":"response.queued","response":{"id":"failed_queued","status":"queued"}}`) + v043SSE(`{"type":"response.failed","response":{"id":"failed_queued","error":{"type":"server_error","message":"The selected model is at capacity"}}}`)},
					{status: 200, body: v043SuccessSSE},
				}, []int{1, 1})
			})
			t.Run("502_retries_are_bounded_and_401_switches_immediately", func(t *testing.T) {
				v043RunForwardFaultScript(t, passthrough, 2, []v043FaultStep{
					{status: 502, body: `{"error":{"type":"server_error","message":"upstream temporarily unavailable"}}`},
					{status: 401, body: `{"error":{"type":"authentication_error","message":"invalid API key"}}`},
					{status: 200, body: v043SuccessSSE},
				}, []int{3, 1, 1})
			})
			t.Run("empty_completed_is_not_success", func(t *testing.T) {
				v043RunForwardFaultScript(t, passthrough, 0, []v043FaultStep{
					{status: 200, body: v043SSE(`{"type":"response.completed","response":{"id":"empty_completed","status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)},
					{status: 200, body: v043SuccessSSE},
				}, []int{1, 1})
			})
		})
	}
}

func TestV043ResponsesNativeFirstOutputTimeoutAcceptance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	timeoutField := reflect.ValueOf(&cfg.Gateway).Elem().FieldByName("OpenAIFirstOutputTimeoutSeconds")
	require.True(t, timeoutField.IsValid(), "V04 first-output config is not implemented in this baseline")
	require.True(t, timeoutField.CanSet())
	timeoutField.SetInt(2)
	v043RunForwardFaultScriptWithConfig(t, false, cfg, []v043FaultStep{
		{status: 200, body: v043SuccessSSE, delay: 3 * time.Second},
		{status: 200, body: v043SuccessSSE},
	}, []int{1, 1})
}

const v043SuccessSSE = "data: {\"type\":\"response.output_text.delta\",\"response_id\":\"successful_response\",\"item_id\":\"item_ok\",\"output_index\":0,\"content_index\":0,\"delta\":\"verified-success\"}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"successful_response\",\"model\":\"gpt-4o\",\"status\":\"completed\",\"output\":[{\"id\":\"item_ok\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"verified-success\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"total_tokens\":3}}}\n\n"

func v043SSE(data string) string { return "data: " + data + "\n\n" }

type v043FaultStep struct {
	status int
	body   string
	delay  time.Duration
}

func v043RunForwardFaultScript(t *testing.T, passthrough bool, retries int, steps []v043FaultStep, wantHits []int) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Gateway.FailoverSameAccountRetries = retries
	v043RunForwardFaultScriptWithConfig(t, passthrough, cfg, steps, wantHits)
}
func v043RunForwardFaultScriptWithConfig(t *testing.T, passthrough bool, cfg *config.Config, steps []v043FaultStep, wantHits []int) {
	t.Helper()
	cfg.Gateway.FailoverOnAnyError = true
	hits := make([]int, len(steps))
	var mu sync.Mutex
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.Header.Get("X-V043-Test-Account"))
		if err != nil || id < 1 || id > len(steps) {
			http.Error(w, "bad test account", 500)
			return
		}
		mu.Lock()
		hits[id-1]++
		mu.Unlock()
		step := steps[id-1]
		if step.status == 200 {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		w.Header().Set("X-Request-Id", fmt.Sprintf("v043-attempt-%d", id))
		w.WriteHeader(step.status)
		if step.delay > 0 {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-time.After(step.delay):
			case <-r.Context().Done():
				return
			}
		}
		_, _ = io.WriteString(w, step.body)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	transport := &v043LoopbackUpstream{target: target, client: upstream.Client()}
	svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, transport, nil, nil, nil, nil, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, ctxkey.Group, &service.Group{ID: 1, FailoverOnAnyError: true})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-4o","stream":true,"instructions":"test fixture only","input":"say okay"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	state := handler.NewFailoverState(len(steps), false).WithAnyErrorFailover(ctx, cfg)
	cooldown := &v043UnschedulerSpy{}
	success := false
	for attempt := 0; attempt < 20; attempt++ {
		id := int64(0)
		for i := range steps {
			if _, excluded := state.FailedAccountIDs[int64(i+1)]; !excluded {
				id = int64(i + 1)
				break
			}
		}
		require.NotZero(t, id, "all accounts exhausted before scripted success")
		account := &service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test-only-key"}, Extra: map[string]any{"openai_passthrough": passthrough, "openai_responses_mode": "force_responses"}}
		result, forwardErr := svc.Forward(ctx, c, account, body)
		if forwardErr == nil {
			require.NotNil(t, result)
			require.Equal(t, int64(len(steps)), id, "failed or empty attempt must not be accepted as success")
			success = true
			break
		}
		require.Nil(t, result, "failure must not return a successful usage result")
		var failoverErr *service.UpstreamFailoverError
		require.True(t, errors.As(forwardErr, &failoverErr), "account %d: error is not eligible for failover: %v", id, forwardErr)
		require.Empty(t, recorder.Body.String(), "pre-output error must not leak SSE from failed accounts")
		require.Equal(t, handler.FailoverContinue, state.HandleFailoverError(ctx, cooldown, id, service.PlatformOpenAI, failoverErr))
	}
	require.True(t, success, "bounded failover loop did not reach success")
	require.Contains(t, recorder.Body.String(), "verified-success")
	require.NotContains(t, recorder.Body.String(), "failed_queued")
	require.NotContains(t, recorder.Body.String(), "server_is_overloaded")
	require.NotContains(t, recorder.Body.String(), "empty_completed")
	mu.Lock()
	gotHits := append([]int(nil), hits...)
	mu.Unlock()
	require.Equal(t, wantHits, gotHits, "per-account attempt counts")
	t.Logf("V043_FAULT_METRICS passthrough=%v hits=%v switches=%d temp_unschedule_calls=%d", passthrough, gotHits, state.SwitchCount, cooldown.calls)
}

type v043LoopbackUpstream struct {
	target *url.URL
	client *http.Client
}

func (u *v043LoopbackUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	copyReq := req.Clone(req.Context())
	target := *req.URL
	copyReq.URL = &target
	copyReq.URL.Scheme = u.target.Scheme
	copyReq.URL.Host = u.target.Host
	copyReq.Host = u.target.Host
	copyReq.Header.Set("X-V043-Test-Account", strconv.FormatInt(accountID, 10))
	// Rewriting only the injected transport keeps all provider request building in
	// production code while guaranteeing that tests never contact real providers.
	return u.client.Do(copyReq)
}
func (u *v043LoopbackUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

type v043UnschedulerSpy struct{ calls int }

func (s *v043UnschedulerSpy) TempUnscheduleRetryableError(_ context.Context, _ int64, _ *service.UpstreamFailoverError) {
	s.calls++
}

// This exercises native Responses continuation over HTTP, not just the text
// rewriting helper. Anthropic and authenticated-handler continuation remain
// separate acceptance gates.
func TestV043ResponsesNativeInterruptedContinuationAcceptance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prefix := strings.Repeat("abcde", 40) // exactly 200 characters before interruption
	suffix := "continued-without-duplicate"
	var mu sync.Mutex
	hits := make([]int, 2)
	var secondRequest []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.Header.Get("X-V043-Test-Account"))
		if err != nil || id < 1 || id > 2 {
			http.Error(w, "bad test account", 500)
			return
		}
		requestBody, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits[id-1]++
		if id == 2 {
			secondRequest = append([]byte(nil), requestBody...)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, v043SSE(fmt.Sprintf(`{"type":"response.created","sequence_number":0,"response":{"id":"resp_%d","status":"in_progress"}}`, id)))
		_, _ = io.WriteString(w, v043SSE(fmt.Sprintf(`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_%d","type":"message","role":"assistant","content":[]}}`, id)))
		text := prefix
		if id == 2 {
			text = suffix
		}
		_, _ = io.WriteString(w, v043SSE(fmt.Sprintf(`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_%d","output_index":0,"content_index":0,"delta":%q}`, id, text)))
		w.(http.Flusher).Flush()
		if id == 1 {
			return // EOF without a terminal event is the injected interruption.
		}
		_, _ = io.WriteString(w, v043SSE(fmt.Sprintf(`{"type":"response.completed","sequence_number":3,"response":{"id":"resp_2","status":"completed","output":[{"id":"msg_2","type":"message","role":"assistant","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":4,"output_tokens":2}}}`, suffix)))
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	cfg := &config.Config{}
	cfg.Gateway.FailoverOnAnyError = true
	transport := &v043LoopbackUpstream{target: target, client: upstream.Client()}
	svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, transport, nil, nil, nil, nil, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, ctxkey.Group, &service.Group{ID: 1, FailoverOnAnyError: true})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-4o","stream":true,"instructions":"test fixture only","input":"continue the answer"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	continuation := service.AttachStreamContinuation(c)
	state := handler.NewFailoverState(2, false).WithAnyErrorFailover(ctx, cfg)
	for id := int64(1); id <= 2; id++ {
		account := &service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test-only-key"}, Extra: map[string]any{"openai_responses_mode": "force_responses"}}
		result, forwardErr := svc.Forward(ctx, c, account, body)
		if id == 1 {
			var failoverErr *service.UpstreamFailoverError
			require.True(t, errors.As(forwardErr, &failoverErr), "interrupted stream must be eligible for continuation: %v", forwardErr)
			require.True(t, failoverErr.SafeToFailoverAfterWrite)
			require.Equal(t, prefix, continuation.EmittedText())
			require.Equal(t, handler.FailoverContinue, state.HandleFailoverError(ctx, &v043UnschedulerSpy{}, id, service.PlatformOpenAI, failoverErr))
		} else {
			require.NoError(t, forwardErr)
			require.NotNil(t, result)
		}
	}
	var deltas strings.Builder
	var completed gjson.Result
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		event := gjson.Parse(strings.TrimPrefix(line, "data: "))
		if event.Get("type").String() == "response.output_text.delta" {
			deltas.WriteString(event.Get("delta").String())
		}
		if event.Get("type").String() == "response.completed" {
			completed = event
		}
		require.NotEqual(t, "error", event.Get("type").String(), "must not leak terminal error during continuation")
	}
	require.Equal(t, prefix+suffix, deltas.String(), "client-visible deltas must contain no duplicated prefix")
	require.Equal(t, prefix+suffix, completed.Get("response.output.0.content.0.text").String())
	require.Equal(t, "resp_1", completed.Get("response.id").String(), "response identity must survive account switch")
	mu.Lock()
	gotHits := append([]int(nil), hits...)
	requestCopy := append([]byte(nil), secondRequest...)
	mu.Unlock()
	require.Equal(t, []int{1, 1}, gotHits)
	input := gjson.GetBytes(requestCopy, "input").Array()
	require.GreaterOrEqual(t, len(input), 3)
	require.Equal(t, "assistant", input[len(input)-2].Get("role").String())
	require.Equal(t, prefix, input[len(input)-2].Get("content.0.text").String())
	require.Equal(t, "developer", input[len(input)-1].Get("role").String())
	t.Logf("V043_CONTINUATION_METRICS hits=%v prefix_chars=%d delta_chars=%d switches=%d", gotHits, len(prefix), len(deltas.String()), state.SwitchCount)
}
