package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterCacheManagementRoutesIncludesClearEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		CacheConfig: adminhandler.NewCacheConfigHandler(nil, nil),
		CacheStats:  adminhandler.NewCacheStatsHandler(nil),
	}}
	v1 := router.Group("/api/v1/admin")
	registerCacheManagementRoutes(v1, handlers)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/cache/clear", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid request")
}

func TestRegisterCacheManagementRoutesIncludesStatsExportAndClearAudits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		CacheConfig: adminhandler.NewCacheConfigHandler(nil, nil),
		CacheStats:  adminhandler.NewCacheStatsHandler(nil),
	}}
	v1 := router.Group("/api/v1/admin")
	registerCacheManagementRoutes(v1, handlers)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/stats/export", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/clear-audits", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/semantic-audits", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRegisterCacheManagementRoutesIncludesSemanticAuditActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		CacheConfig: adminhandler.NewCacheConfigHandler(nil, nil),
		CacheStats:  adminhandler.NewCacheStatsHandler(nil),
	}}
	v1 := router.Group("/api/v1/admin")
	registerCacheManagementRoutes(v1, handlers)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/cache/semantic-audits/7/review", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/cache/semantic-audits/7/feedback", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRegisterCacheManagementRoutesIncludesAdvancedConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		CacheConfig: adminhandler.NewCacheConfigHandler(nil, nil),
		CacheStats:  adminhandler.NewCacheStatsHandler(nil),
	}}
	v1 := router.Group("/api/v1/admin")
	registerCacheManagementRoutes(v1, handlers)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/advanced-config", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/cache/advanced-config", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
