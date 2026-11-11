package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 移植自上游 2b1f12938，按本地测试夹具改写（本地没有 newGeminiVertexAPIKeyTestService，
// 也没有依赖 b6eb6c1ef 的已送达图片计数用例）。

// geminiAbortingStreamBody 先吐出 data，读完后返回 abortErr；onAbort 用于模拟客户端断开时请求 context 被取消。
type geminiAbortingStreamBody struct {
	data     *bytes.Reader
	abortErr error
	onAbort  func()
}

func (b *geminiAbortingStreamBody) Read(p []byte) (int, error) {
	n, err := b.data.Read(p)
	if err == io.EOF {
		if b.onAbort != nil {
			b.onAbort()
		}
		return n, b.abortErr
	}
	return n, err
}

func (b *geminiAbortingStreamBody) Close() error { return nil }

const (
	geminiAbortTextChunk  = `data: {"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"}}]}` + "\n\n"
	geminiAbortFinalChunk = `data: {"candidates":[{"content":{"parts":[{"text":" world"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}` + "\n\n"
	// V03-AC21：已收集输入 1000、输出 200 后客户端断开（无 finishReason）。
	geminiAbortMidUsageChunk = `data: {"candidates":[{"content":{"parts":[{"text":"partial"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":200,"totalTokenCount":1200}}` + "\n\n"
)

func geminiAbortTestAccount() *Account {
	return &Account{
		ID:          301,
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "gemini-api-key"},
		Concurrency: 1,
	}
}

// newGeminiAbortingStreamService 返回的上游流在吐完 sse 后中断；clientGone=true 时中断伴随请求 context 取消。
func newGeminiAbortingStreamService(t *testing.T, path string, reqBody []byte, sse string, clientGone bool) (*GeminiMessagesCompatService, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	reqCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(reqBody)).WithContext(reqCtx)

	body := &geminiAbortingStreamBody{data: bytes.NewReader([]byte(sse)), abortErr: io.ErrUnexpectedEOF}
	if clientGone {
		body.abortErr = context.Canceled
		body.onAbort = cancel
	}
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       body,
	}}
	svc := &GeminiMessagesCompatService{
		httpUpstream:         httpStub,
		cfg:                  &config.Config{},
		responseHeaderFilter: compileResponseHeaderFilter(&config.Config{}),
	}
	return svc, c
}

func TestGeminiForwardNative_ClientDisconnectAfterFinalChunkBillsCollectedUsage(t *testing.T) {
	reqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	svc, c := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-2.5-flash:streamGenerateContent", reqBody, geminiAbortTextChunk+geminiAbortFinalChunk, true)

	result, err := svc.ForwardNative(context.Background(), c, geminiAbortTestAccount(), "gemini-2.5-flash", "streamGenerateContent", true, reqBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Zero(t, result.ImageCount)
}

// V03-AC21：gemini-2.5-pro 已收集输入 1000、输出 200 后客户端断开，
// 按已收集用量计费 1000×1.25e-6 + 200×1e-5 = 0.00325，client_disconnect=true。
func TestGeminiForwardNative_ClientDisconnectMidStreamBillsCollectedUsage(t *testing.T) {
	reqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	svc, c := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", reqBody, geminiAbortMidUsageChunk, true)

	result, err := svc.ForwardNative(context.Background(), c, geminiAbortTestAccount(), "gemini-2.5-pro", "streamGenerateContent", true, reqBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 1000, result.Usage.InputTokens)
	require.Equal(t, 200, result.Usage.OutputTokens)

	// gemini-2.5-pro（≤200k）：输入 1.25e-6、输出 1e-5。
	pricing := &ModelPricing{InputPricePerToken: 1.25e-6, OutputPricePerToken: 1e-5}
	got := float64(result.Usage.InputTokens)*pricing.InputPricePerToken + float64(result.Usage.OutputTokens)*pricing.OutputPricePerToken
	require.InDelta(t, 0.00325, got, 1e-12)
}

func TestGeminiForwardNative_ClientDisconnectWithoutUsageStaysUnbilled(t *testing.T) {
	reqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	svc, c := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-2.5-flash:streamGenerateContent", reqBody, geminiAbortTextChunk, true)

	result, err := svc.ForwardNative(context.Background(), c, geminiAbortTestAccount(), "gemini-2.5-flash", "streamGenerateContent", true, reqBody)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}

func TestGeminiForwardNative_UpstreamReadErrorWithLiveClientStaysError(t *testing.T) {
	reqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	svc, c := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-2.5-flash:streamGenerateContent", reqBody, geminiAbortTextChunk+geminiAbortFinalChunk, false)

	result, err := svc.ForwardNative(context.Background(), c, geminiAbortTestAccount(), "gemini-2.5-flash", "streamGenerateContent", true, reqBody)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, result)
}

func TestGeminiForwardNative_ClientDisconnectIgnoresTruncatedTrailingLine(t *testing.T) {
	complete := `data: {"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":300,"totalTokenCount":1300}}` + "\n\n"
	truncated := `data: {"candidates":[{"content":{"parts":[{"text":" more"}],"role":"model"}}],"usageMetadata":{`
	reqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	svc, c := newGeminiAbortingStreamService(t, "/v1beta/models/gemini-2.5-flash:streamGenerateContent", reqBody, complete+truncated, true)

	result, err := svc.ForwardNative(context.Background(), c, geminiAbortTestAccount(), "gemini-2.5-flash", "streamGenerateContent", true, reqBody)
	require.NoError(t, err)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 1000, result.Usage.InputTokens)
	require.Equal(t, 300, result.Usage.OutputTokens)
}

func TestGeminiCompatForward_ClientDisconnectAfterFinalChunkBillsCollectedUsage(t *testing.T) {
	reqBody := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	t.Run("client gone", func(t *testing.T) {
		svc, c := newGeminiAbortingStreamService(t, "/v1/messages", reqBody, geminiAbortTextChunk+geminiAbortFinalChunk, true)
		result, err := svc.Forward(context.Background(), c, geminiAbortTestAccount(), reqBody)
		require.NoError(t, err)
		require.True(t, result.ClientDisconnect)
		require.Equal(t, 10, result.Usage.InputTokens)
		require.Equal(t, 5, result.Usage.OutputTokens)
	})

	t.Run("upstream read error with live client", func(t *testing.T) {
		svc, c := newGeminiAbortingStreamService(t, "/v1/messages", reqBody, geminiAbortTextChunk+geminiAbortFinalChunk, false)
		result, err := svc.Forward(context.Background(), c, geminiAbortTestAccount(), reqBody)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.Nil(t, result)
	})
}

func TestGeminiForwardAsChatCompletions_ClientDisconnectAfterFinalChunkBillsCollectedUsage(t *testing.T) {
	reqBody := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	t.Run("client gone", func(t *testing.T) {
		svc, c := newGeminiAbortingStreamService(t, "/v1/chat/completions", reqBody, geminiAbortTextChunk+geminiAbortFinalChunk, true)
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiAbortTestAccount(), reqBody)
		require.NoError(t, err)
		require.True(t, result.ClientDisconnect)
		require.Equal(t, 10, result.Usage.InputTokens)
		require.Equal(t, 5, result.Usage.OutputTokens)
	})

	t.Run("client gone before usage", func(t *testing.T) {
		svc, c := newGeminiAbortingStreamService(t, "/v1/chat/completions", reqBody, geminiAbortTextChunk, true)
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiAbortTestAccount(), reqBody)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	})
}

// geminiFailingResponseWriter 模拟客户端已断开：所有写入失败。
type geminiFailingResponseWriter struct {
	header http.Header
}

func (w *geminiFailingResponseWriter) Header() http.Header       { return w.header }
func (w *geminiFailingResponseWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (w *geminiFailingResponseWriter) WriteHeader(int)           {}
func (w *geminiFailingResponseWriter) Flush()                    {}

// Chat 兼容流写失败（客户端已断开）时，结果带上断开标记（修复前写死 ClientDisconnect=false）。
func TestGeminiForwardAsChatCompletions_WriteFailureMarksClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqBody := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	c, _ := gin.CreateTestContext(&geminiFailingResponseWriter{header: http.Header{}})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(geminiAbortMidUsageChunk))),
	}}
	svc := &GeminiMessagesCompatService{httpUpstream: httpStub, cfg: &config.Config{}, responseHeaderFilter: compileResponseHeaderFilter(&config.Config{})}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiAbortTestAccount(), reqBody)
	require.NoError(t, err)
	require.True(t, result.ClientDisconnect)
	require.Zero(t, result.ImageCount)
}

// 原生流写失败（客户端断开）后继续读完上游，按最终 usageMetadata 计费。
func TestGeminiForwardNative_WriteFailureDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	c, _ := gin.CreateTestContext(&geminiFailingResponseWriter{header: http.Header{}})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:streamGenerateContent", bytes.NewReader(reqBody))
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(geminiAbortTextChunk + geminiAbortFinalChunk))),
	}}
	svc := &GeminiMessagesCompatService{httpUpstream: httpStub, cfg: &config.Config{}, responseHeaderFilter: compileResponseHeaderFilter(&config.Config{})}

	result, err := svc.ForwardNative(context.Background(), c, geminiAbortTestAccount(), "gemini-2.5-flash", "streamGenerateContent", true, reqBody)
	require.NoError(t, err)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
}
