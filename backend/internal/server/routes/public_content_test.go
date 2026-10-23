package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type publicRouteRepoStub struct{}

func (publicRouteRepoStub) ListPublishedNews(context.Context, string, int, int) (service.PublicNewsPage, error) {
	return service.PublicNewsPage{}, nil
}
func (publicRouteRepoStub) GetPublishedNews(context.Context, string, string) (*service.PublicNews, error) {
	return nil, service.ErrPublicContentNotFound
}
func (publicRouteRepoStub) ListPublishedArticles(context.Context, string, int, int) (service.PublicArticlePage, error) {
	return service.PublicArticlePage{}, nil
}
func (publicRouteRepoStub) GetPublishedArticle(context.Context, string, string) (*service.PublicArticle, error) {
	return nil, service.ErrPublicContentNotFound
}
func (publicRouteRepoStub) ListPublishedNewsForSitemap(context.Context) ([]service.PublicNewsListItem, error) {
	return []service.PublicNewsListItem{{Locale: "en-US", Slug: "release", Title: "Release", Summary: "Summary", SourceName: "CCAI", PublishedAt: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), Status: "published"}}, nil
}
func (publicRouteRepoStub) ListPublishedArticlesForSitemap(context.Context) ([]service.PublicArticleListItem, error) {
	return []service.PublicArticleListItem{{Locale: "en-US", Slug: "guide", Title: "Guide", Excerpt: "Excerpt", PublishedAt: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC), Status: "published"}}, nil
}

func TestRegisterPublicContentRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &handler.Handlers{PublicContent: handler.NewPublicContentHandler(service.NewPublicContentService(publicRouteRepoStub{}), &config.Config{Server: config.ServerConfig{FrontendURL: "https://ccai.example"}})}
	RegisterPublicContentRoutes(r.Group("/api/v1"), h)
	RegisterPublicContentPageRoutes(r, h)

	for _, path := range []string{"/api/v1/public/news", "/api/v1/public/articles"} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, res.Code, path)
	}
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/public/news/zh-CN/missing", nil))
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestRegisterPublicContentPageRoutesAreAnonymousHTML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &handler.Handlers{PublicContent: handler.NewPublicContentHandler(service.NewPublicContentService(publicRouteRepoStub{}), &config.Config{Server: config.ServerConfig{FrontendURL: "https://ccai.example"}})}
	RegisterPublicContentPageRoutes(r, h)

	for _, path := range []string{"/", "/news", "/articles", "/docs", "/docs/authentication", "/robots.txt", "/sitemap.xml"} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, res.Code, path)
		if path == "/robots.txt" {
			require.Contains(t, res.Body.String(), "Sitemap: https://ccai.example/sitemap.xml")
			require.Contains(t, res.Body.String(), "Disallow: /model-square")
			require.Contains(t, res.Body.String(), "Disallow: /api/v1/admin/")
			continue
		}
		if path == "/sitemap.xml" {
			require.Contains(t, res.Header().Get("Content-Type"), "application/xml", path)
			require.Contains(t, res.Body.String(), "https://ccai.example/news/en-US/release", path)
			require.NotContains(t, res.Body.String(), "draft", path)
			require.NotContains(t, res.Body.String(), "/model-square", path)
			require.Equal(t, 1, strings.Count(res.Body.String(), "<loc>https://ccai.example/news</loc>"))
			continue
		}
		require.Contains(t, res.Header().Get("Content-Type"), "text/html", path)
		require.Contains(t, res.Body.String(), "CCAI", path)
	}
}
