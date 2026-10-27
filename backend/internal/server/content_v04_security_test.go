//go:build unit

package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	mw "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// sensitiveContentPaths are the AI config, upload, translation and BI endpoints that SEC-01 keeps behind the admin gate.
var sensitiveContentPaths = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/admin/content/articles/ai-config"},
	{http.MethodGet, "/api/v1/admin/content/articles/ai-configs"},
	{http.MethodPost, "/api/v1/admin/content/upload"},
	{http.MethodGet, "/api/v1/admin/content/news/7/translation-tasks"},
	{http.MethodGet, "/api/v1/admin/content/articles/translations/7"},
	{http.MethodGet, "/api/v1/admin/bi/analytics/distribution"},
}

func TestSensitiveContentEndpointsRejectAnonymousAndNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "task-032-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	users := map[int64]*service.User{
		1: {ID: 1, Email: "admin@example.com", Role: service.RoleAdmin, Status: service.StatusActive, TokenVersion: 1},
		2: {ID: 2, Email: "user@example.com", Role: service.RoleUser, Status: service.StatusActive, TokenVersion: 1},
	}
	h := &handler.Handlers{Admin: &handler.AdminHandlers{
		ContentNews:    adminhandler.NewContentNewsHandler(service.NewContentNewsAdminService(nil)),
		ContentArticle: adminhandler.NewContentArticleHandler(service.NewContentArticleAdminService(nil, nil, nil)),
		ContentUpload:  adminhandler.NewContentUploadHandler(nil),
	}}
	router := gin.New()
	routes.RegisterAdminRoutes(router.Group("/api/v1"), h, mw.NewAdminAuthMiddleware(authService, service.NewUserService(&stubUserRepo{users: users}, nil, nil, nil), nil))

	token, err := authService.GenerateToken(users[2])
	require.NoError(t, err)

	for _, tc := range sensitiveContentPaths {
		t.Run("anonymous_"+tc.path, func(t *testing.T) {
			status, body := doRequest(t, router, tc.method, tc.path, "", nil)
			require.Equal(t, http.StatusUnauthorized, status, body)
			requireNoSensitiveLeak(t, body)
		})
		t.Run("non_admin_"+tc.path, func(t *testing.T) {
			status, body := doRequest(t, router, tc.method, tc.path, "", map[string]string{"Authorization": "Bearer " + token})
			require.Equal(t, http.StatusForbidden, status, body)
			requireNoSensitiveLeak(t, body)
		})
	}
}

func requireNoSensitiveLeak(t *testing.T, body string) {
	t.Helper()
	for _, needle := range []string{"apiKeyEncrypted", `"apiKey"`, "sk-", "referrer", "referrerUrl"} {
		require.NotContains(t, strings.ToLower(body), strings.ToLower(needle))
	}
}
