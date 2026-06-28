package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDownloadFormBearerTokenSetsAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/download", downloadFormBearerToken(), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetHeader("Authorization"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader("auth_token=form-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "Bearer form-token", rec.Body.String())
}

func TestDownloadFormBearerTokenKeepsExistingAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/download", downloadFormBearerToken(), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetHeader("Authorization"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader("auth_token=form-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer header-token")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "Bearer header-token", rec.Body.String())
}

func TestRegisterUserRoutesIncludesPostVideoDownloadEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{UserVideo: handler.NewUserVideoGenerationHandler(nil, nil, nil)}
	v1 := router.Group("/api/v1")
	RegisterUserRoutes(v1, handlers, func(c *gin.Context) { c.Next() }, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/video-generations/task-1/download", strings.NewReader("api_key_id=1&auth_token=form-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestRegisterUserRoutesIncludesVideoHistoryEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{UserVideo: handler.NewUserVideoGenerationHandler(nil, nil, nil)}
	v1 := router.Group("/api/v1")
	RegisterUserRoutes(v1, handlers, func(c *gin.Context) { c.Next() }, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/user/video-generations/history"},
		{method: http.MethodPut, path: "/api/v1/user/video-generations/history/session-1", body: `{"summary":"摘要","generationCount":1,"messages":[{"id":"m1","role":"user","content":"生成视频"}]}`},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(rec, req)
		require.NotEqual(t, http.StatusNotFound, rec.Code)
	}
}
