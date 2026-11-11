package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 移植自上游 94c7d6b02（去掉依赖 anthropicNativeLinePump 的上游空闲超时用例，本地没有该行泵）。
// 覆盖：Anthropic 账号 chat/responses 兼容流式在客户端中途断开后，继续读完上游合并最终 usage 再计费。

// disconnectOnMarkerResponseWriter 在首次写入包含 marker 的数据时返回错误，模拟客户端中途断开。
type disconnectOnMarkerResponseWriter struct {
	*httptest.ResponseRecorder
	marker            string
	failed            bool
	writesAfterError  int
	flushesAfterError int
}

func (writer *disconnectOnMarkerResponseWriter) Write(data []byte) (int, error) {
	if writer.failed {
		writer.writesAfterError++
		return 0, errors.New("client disconnected")
	}
	if strings.Contains(string(data), writer.marker) {
		writer.failed = true
		return 0, errors.New("client disconnected")
	}
	return writer.ResponseRecorder.Write(data)
}

func (writer *disconnectOnMarkerResponseWriter) WriteString(data string) (int, error) {
	return writer.Write([]byte(data))
}

func (writer *disconnectOnMarkerResponseWriter) Flush() {
	if writer.failed {
		writer.flushesAfterError++
	}
	writer.ResponseRecorder.Flush()
}

// anthropicStreamWithMidStreamText 构造一条完整的 Anthropic 流：message_start 输入 inputTokens，
// 正文 first/second，message_delta 最终输出 outputTokens。
func anthropicStreamWithMidStreamText(model string, inputTokens, cacheRead, cacheCreate, outputTokens int) string {
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_disc","type":"message","role":"assistant","content":[],"model":"` + model + `","usage":{"input_tokens":` + strconv.Itoa(inputTokens) + `,"cache_read_input_tokens":` + strconv.Itoa(cacheRead) + `,"cache_creation_input_tokens":` + strconv.Itoa(cacheCreate) + `,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"second"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":` + strconv.Itoa(outputTokens) + `}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
}

func TestHandleCCStreamingFromAnthropic_ClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: "second"}
	c, _ := gin.CreateTestContext(writer)

	result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(anthropicStreamWithMidStreamText("claude-sonnet-4-6", 20, 11, 4, 42)))},
		c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now(), true,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, writer.failed)
	require.True(t, result.ClientDisconnect)
	require.Zero(t, writer.writesAfterError)
	require.Zero(t, writer.flushesAfterError)

	body := writer.Body.String()
	require.Contains(t, body, `"content":"first"`)
	require.NotContains(t, body, "second")
	require.NotContains(t, body, "[DONE]")

	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 42, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.FirstTokenMs)
}

func TestHandleResponsesStreamingResponse_ClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: "second"}
	c, _ := gin.CreateTestContext(writer)

	result, err := (&GatewayService{}).handleResponsesStreamingResponse(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(anthropicStreamWithMidStreamText("claude-sonnet-4-6", 20, 11, 4, 42)))},
		c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, writer.failed)
	require.True(t, result.ClientDisconnect)
	require.Zero(t, writer.writesAfterError)
	require.Zero(t, writer.flushesAfterError)

	body := writer.Body.String()
	require.Contains(t, body, `"delta":"first"`)
	require.NotContains(t, body, "second")
	require.NotContains(t, body, "response.completed")

	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 42, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.FirstTokenMs)
}

// V03-AC20：claude-opus-4-7，客户端收到部分输出后断开；上游最终输入 2000、输出 800。
// 服务端读完上游，按完整用量计费：2000×5e-6 + 800×2.5e-5 = 0.03（两条兼容路径一致）。
func TestAnthropicCompatStreaming_ClientDisconnectBillsFullUpstreamUsage(t *testing.T) {
	billing := NewBillingService(nil, nil)
	handlers := map[string]func(*GatewayService, *http.Response, *gin.Context) (*ForwardResult, error){
		"chat_completions": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-7", "claude-opus-4-7", nil, time.Now(), true)
		},
		"responses": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleResponsesStreamingResponse(resp, c, "claude-opus-4-7", "claude-opus-4-7", nil, time.Now())
		},
	}
	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: "second"}
			c, _ := gin.CreateTestContext(writer)
			resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(anthropicStreamWithMidStreamText("claude-opus-4-7", 2000, 0, 0, 800)))}

			result, err := handle(&GatewayService{}, resp, c)
			require.NoError(t, err)
			require.True(t, result.ClientDisconnect)
			require.Equal(t, 2000, result.Usage.InputTokens)
			require.Equal(t, 800, result.Usage.OutputTokens)

			cost, err := billing.CalculateCost("claude-opus-4-7", UsageTokens{
				InputTokens:  result.Usage.InputTokens,
				OutputTokens: result.Usage.OutputTokens,
			}, 1)
			require.NoError(t, err)
			require.InDelta(t, 0.03, cost.ActualCost, 1e-12)
		})
	}
}
