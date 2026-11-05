package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGeminiModelPathGuardContext(t *testing.T, method, path string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(`{"contents":[]}`))
	c.Params = params
	groupID := int64(1)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      1,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformGemini},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
	return c, w
}

// 已解码的模型名（gin 交给 handler 的 Param 已是百分号解码后的结果）。
var nonConformingGeminiModels = []string{
	"..",
	".",
	"../../x",
	"gemini-2.5-pro/../../x",
	"gemini-2.5-pro?a=b",
	"gemini-2.5-pro#frag",
	"gemini\x00pro",
	"gemini\r\npro",
	`..\..\x`,
}

// TestGeminiV1BetaGetModelRejectsNonConformingModel 锁定：GET /v1beta/models/{model}
// 的模型名不合规时直接 400，不选账号、不发上游请求（handler 的服务依赖均为 nil，
// 若越过校验会 panic）。
func TestGeminiV1BetaGetModelRejectsNonConformingModel(t *testing.T) {
	h := &GatewayHandler{}
	for _, model := range nonConformingGeminiModels {
		c, w := newGeminiModelPathGuardContext(t, http.MethodGet, "/v1beta/models/x", gin.Params{{Key: "model", Value: model}})
		h.GeminiV1BetaGetModel(c)
		require.Equal(t, http.StatusBadRequest, w.Code, "model=%q", model)
		require.Contains(t, w.Body.String(), "Invalid model in URL", "model=%q", model)
	}
}

// TestGeminiV1BetaModelsRejectsNonConformingModel 锁定：POST /v1beta/models/{model}:{action}
// 的模型名不合规时在读请求体、渠道映射、调度之前直接 400。
func TestGeminiV1BetaModelsRejectsNonConformingModel(t *testing.T) {
	h := &GatewayHandler{}
	for _, model := range nonConformingGeminiModels {
		c, w := newGeminiModelPathGuardContext(t, http.MethodPost, "/v1beta/models/x:generateContent",
			gin.Params{{Key: "modelAction", Value: "/" + model + ":generateContent"}})
		h.GeminiV1BetaModels(c)
		require.Equal(t, http.StatusBadRequest, w.Code, "model=%q", model)
		require.Contains(t, w.Body.String(), "Invalid model in URL", "model=%q", model)
	}
}
