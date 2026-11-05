package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGatewayRoutesTestRouter() *gin.Engine {
	return newGatewayRoutesTestRouterForPlatform(service.PlatformOpenAI)
}

func newGatewayRoutesTestRouterForPlatform(platform string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: platform},
			})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		&config.Config{Gateway: config.GatewayConfig{MaxBodySize: config.DefaultMaxRequestBodySize}},
	)

	return router
}

func TestGatewayRoutesGrokCountTokensEstimatesLocally(t *testing.T) {
	router := newGatewayRoutesTestRouterForPlatform(service.PlatformGrok)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"grok-4","messages":[{"role":"user","content":"hello world"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"input_tokens":`)
}

func TestGatewayRoutesOpenAIResponsesCompactPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/responses/compact",
		"/responses/compact",
		"/backend-api/codex/responses",
		"/backend-api/codex/responses/compact",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI responses handler", path)
	}
}

// TestGatewayRoutesResponsesSubpathRejectsNonConformingSubpaths 端到端锁定不变式：
// /responses/*subpath 的子路径会被转发到上游同名端点之后，因此不合规的子路径必须
// 在入口就被拒绝，不得进入调度与转发流程。三个入口（/v1、裸 /responses、
// Codex 直连 /backend-api/codex）行为一致。
func TestGatewayRoutesResponsesSubpathRejectsNonConformingSubpaths(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	longSegment := strings.Repeat("a", 129)
	deepSuffix := strings.Repeat("/a", 9)
	subpaths := []string{
		"/../../x/y",
		"/..%2f..%2fx/y",
		"/%2e%2e/%2e%2e/x",
		"/%2e%2e%2fx",
		`/..\..\x`,
		"/%3fa=b",
		"/x%23frag",
		"/compact%2f..",
		"/x%2f..%2fy",
		"/%00x",
		"//double",
		"/compact//detail",
		"/" + longSegment,
		deepSuffix,
	}
	for _, prefix := range []string{"/v1/responses", "/responses", "/backend-api/codex/responses"} {
		for _, subpath := range subpaths {
			path := prefix + subpath
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code, "path=%s must be rejected at the edge", path)
			require.Contains(t, w.Body.String(), "not_found_error", "path=%s", path)
			require.Contains(t, w.Body.String(), "Unsupported responses subpath", "path=%s", path)
		}
	}
}

// TestGatewayRoutesResponsesSubpathAllowsConformingSubpaths 锁定正向行为：
// compact、compact/detail、{id}/cancel 不被守卫拦截。
func TestGatewayRoutesResponsesSubpathAllowsConformingSubpaths(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, prefix := range []string{"/v1/responses", "/responses", "/backend-api/codex/responses"} {
		// 业务层看到的是解码后的 URL.Path，编码后的 `/` 解码成合规的多段路径时照常放行。
		for _, subpath := range []string{"/compact", "/compact/detail", "/compact%2fdetail", "/resp_abc123/cancel", "/" + strings.Repeat("a", 128), strings.Repeat("/a", 8)} {
			path := prefix + subpath
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
			require.NotContains(t, w.Body.String(), "Unsupported responses subpath", "path=%s must pass the guard", path)
		}
	}
}

func TestGatewayRoutesOpenAIImagesPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-image-2","prompt":"draw a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI images handler", path)
	}
}

func TestGatewayRoutesOpenAIVideoCreationPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouterForPlatform(service.PlatformOpenAI)
	routes := router.Routes()
	registered := make(map[string]bool, len(routes))
	for _, route := range routes {
		registered[route.Method+" "+route.Path] = true
	}

	require.True(t, registered[http.MethodPost+" /v1/videos"])
	require.True(t, registered[http.MethodPost+" /videos"])

	// 别名必须落到 videoGenerationHandler，而不是 gin 默认 404：
	// 非 Grok 分组会得到平台不支持的业务错误体，据此区分路由是否命中。
	for _, path := range []string{"/v1/videos", "/videos"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"MiniMax-H3","prompt":"a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "path=%s", path)
		require.Contains(t, w.Body.String(), "Videos API is not supported for this platform", "path=%s should hit videoGenerationHandler", path)
	}
}

func TestGatewayRoutesCodexModelsPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()
	routes := router.Routes()
	registered := make(map[string]bool, len(routes))
	for _, route := range routes {
		registered[route.Method+" "+route.Path] = true
	}

	require.True(t, registered[http.MethodGet+" /v1/models"])
	require.True(t, registered[http.MethodGet+" /backend-api/codex/models"])
}
