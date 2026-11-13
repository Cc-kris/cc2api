//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrontendLoginRedirectPreservesDeepLink(t *testing.T) {
	server, err := NewFrontendServer(&mockSettingsProvider{settings: map[string]string{}})
	require.NoError(t, err)
	for name, middleware := range map[string]gin.HandlerFunc{
		"injected": server.Middleware(),
		"legacy":   ServeEmbeddedFrontend(),
	} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware)
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				for _, path := range []string{
					"/admin/settings",
					"/admin/settings?tab=gateway&search=a%26b&redirect=%2Fadmin%2Faccounts",
					"/admin/accounts?page=2",
					"/keys?name=%E6%B5%8B%E8%AF%95",
				} {
					t.Run(method+path, func(t *testing.T) {
						w := httptest.NewRecorder()
						router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
						require.Equal(t, http.StatusFound, w.Code)
						location, err := w.Result().Location()
						require.NoError(t, err)
						assert.Empty(t, location.Host)
						assert.Equal(t, "/login", location.Path)
						assert.Equal(t, path, location.Query().Get("redirect"))
						assert.Len(t, location.Query(), 1)
						assert.Equal(t, robotsNoIndexHeader, w.Header().Get("X-Robots-Tag"))
						assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
					})
				}
			}
		})
	}
}
