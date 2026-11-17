package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// v0.3.16 回归：HTTP /responses 允许「工具输出缺 call_id / 缺 item_reference」的请求
// 继续进入选号与上游转发，由上游裁定，而不是在选号前直接 400。
// 背景：上游为 apikey 型第三方中继时容忍度不同，旧行为会把本可成功的请求变成
// 不可重试、不计费的 400（v0.3.15 线上事故）。
func TestValidateFunctionCallOutputRequest_DoesNotRejectBeforeAccountSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		body string
	}{
		{
			name: "function_call_output 缺 call_id",
			body: `{"model":"gpt-5","stream":true,"input":[{"type":"function_call_output","output":"ok"}]}`,
		},
		{
			name: "function_call_output 有 call_id 但无 item_reference",
			body: `{"model":"gpt-5","stream":true,"input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`,
		},
		{
			name: "tool_search_output 缺 call_id",
			body: `{"model":"gpt-5","stream":true,"input":[{"type":"tool_search_output","output":"ok"}]}`,
		},
		{
			name: "mcp_tool_call_output 缺 call_id",
			body: `{"model":"gpt-5","stream":true,"input":[{"type":"mcp_tool_call_output","output":"ok"}]}`,
		},
		{
			name: "custom_tool_call_output 缺 call_id",
			body: `{"model":"gpt-5","stream":true,"input":[{"type":"custom_tool_call_output","output":"ok"}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

			h := &OpenAIGatewayHandler{}
			got := h.validateFunctionCallOutputRequest(c, []byte(tc.body), zap.NewNop())

			require.True(t, got, "预校验必须放行，交由上游裁定")
			require.Equal(t, 0, w.Body.Len(), "放行时不得写出任何错误响应体")
			require.Equal(t, 200, w.Code, "放行时不得改写状态码")
		})
	}
}

// 受影响的请求仍应写入解析缓存，供后续转发复用，避免重复 Unmarshal。
func TestValidateFunctionCallOutputRequest_CachesParsedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	body := `{"model":"gpt-5","input":[{"type":"function_call_output","output":"ok"}]}`
	h := &OpenAIGatewayHandler{}
	require.True(t, h.validateFunctionCallOutputRequest(c, []byte(body), zap.NewNop()))

	cached, ok := c.Get("openai_parsed_request_body")
	require.True(t, ok, "解析结果应写入 context 缓存")
	require.NotNil(t, cached)
}
