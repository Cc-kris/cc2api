package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newUsageAuditTestContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	return c
}

// readRequestBodyWithDiagnostics 在入站时记下客户端原始推理强度，后续改写不影响。
func TestReadRequestBodyCapturesRequestedReasoningEffort(t *testing.T) {
	c := newUsageAuditTestContext(t, `{"model":"gpt-5.4","reasoning":{"effort":"minimal"}}`)
	_, err := readRequestBodyWithDiagnostics(c)
	require.NoError(t, err)

	result := &service.OpenAIForwardResult{Model: "gpt-5.4"}
	stampOpenAIUsageAudit(c, result)
	require.NotNil(t, result.RequestedReasoningEffort)
	require.Equal(t, "minimal", *result.RequestedReasoningEffort)
}

func TestStampOpenAIUsageAudit_KeepsServiceValuesAndToleratesNil(t *testing.T) {
	c := newUsageAuditTestContext(t, `{}`)
	ws := &service.OpenAIForwardResult{Model: "gpt-5.4", OpenAIWSMode: true}
	stampOpenAIUsageAudit(c, ws)
	require.Equal(t, "", ws.UpstreamResponseModel)
	require.Nil(t, ws.RequestedReasoningEffort)

	// 已由 service 填好的值不覆盖。
	httpResult := &service.OpenAIForwardResult{Model: "gpt-5.4", UpstreamResponseModel: "gpt-5.4-2026"}
	stampOpenAIUsageAudit(c, httpResult)
	require.Equal(t, "gpt-5.4-2026", httpResult.UpstreamResponseModel)

	stampOpenAIUsageAudit(c, nil)
	stampForwardUsageAudit(c, nil)
}
