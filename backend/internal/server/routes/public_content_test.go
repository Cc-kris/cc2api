package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type publicRouteRepoStub struct {
	newsTotal  int
	lastLocale string
	lastLimit  int
	lastOffset int
}

func (s *publicRouteRepoStub) ListPublishedNews(_ context.Context, locale string, limit, offset int) (service.PublicNewsPage, error) {
	s.lastLocale, s.lastLimit, s.lastOffset = locale, limit, offset
	return service.PublicNewsPage{Total: s.newsTotal}, nil
}
func (s *publicRouteRepoStub) GetPublishedNews(context.Context, string, string) (*service.PublicNews, error) {
	return nil, service.ErrPublicContentNotFound
}
func (s *publicRouteRepoStub) ListPublishedArticles(_ context.Context, locale string, limit, offset int) (service.PublicArticlePage, error) {
	s.lastLocale, s.lastLimit, s.lastOffset = locale, limit, offset
	return service.PublicArticlePage{}, nil
}
func (s *publicRouteRepoStub) GetPublishedArticle(context.Context, string, string) (*service.PublicArticle, error) {
	return nil, service.ErrPublicContentNotFound
}
func (s *publicRouteRepoStub) ListPublishedNewsForSitemap(context.Context) ([]service.PublicNewsListItem, error) {
	return []service.PublicNewsListItem{{Locale: "en-US", Slug: "release", Title: "Release", Summary: "Summary", SourceName: "CCAI", PublishedAt: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), Status: "published"}}, nil
}
func (s *publicRouteRepoStub) ListPublishedArticlesForSitemap(context.Context) ([]service.PublicArticleListItem, error) {
	return []service.PublicArticleListItem{{Locale: "en-US", Slug: "guide", Title: "Guide", Excerpt: "Excerpt", PublishedAt: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC), Status: "published"}}, nil
}

func newPublicContentTestEngine(t *testing.T, stub *publicRouteRepoStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &handler.Handlers{PublicContent: handler.NewPublicContentHandler(
		service.NewPublicContentService(stub),
		&config.Config{Server: config.ServerConfig{FrontendURL: "https://ccai.example"}},
	)}
	RegisterPublicContentRoutes(r.Group("/api/v1"), h)
	RegisterPublicContentPageRoutes(r, h)
	return r
}

func getPublicPath(r *gin.Engine, path string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if withCookie {
		// URL language must win over any remembered language (R12-01/R12-02).
		req.AddCookie(&http.Cookie{Name: "locale", Value: "en"})
	}
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}

type analyticsRouteRepoStub struct {
	events []*service.ContentAnalyticsEvent
}

func (s *analyticsRouteRepoStub) CreateEvent(_ context.Context, e *service.ContentAnalyticsEvent) error {
	s.events = append(s.events, e)
	return nil
}
func (s *analyticsRouteRepoStub) ListDaily(context.Context, service.ContentAnalyticsFilters) ([]service.ContentAnalyticsDaily, error) {
	return nil, nil
}
func (s *analyticsRouteRepoStub) AggregateDaily(context.Context, time.Time, string) error { return nil }

// SEC-01: raw referrer URL (may carry tokens) is rejected and never echoed; only the host is stored.
func TestPublicAnalyticsIngestStoresReferrerHostOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &analyticsRouteRepoStub{}
	r := gin.New()
	RegisterPublicContentRoutes(r.Group("/api/v1"), &handler.Handlers{
		ContentAnalytics: handler.NewContentAnalyticsHandler(service.NewContentAnalyticsService(repo, "secret")),
	})

	raw := `{"eventId":"e1","eventName":"page_view","pagePath":"/","payload":{"referrer":"https://evil.example/s?token=SECRET123"}}`
	bad := httptest.NewRecorder()
	r.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v1/public/analytics/events", strings.NewReader(raw)))
	require.Equal(t, http.StatusBadRequest, bad.Code, bad.Body.String())
	require.NotContains(t, bad.Body.String(), "SECRET123")

	ok := httptest.NewRecorder()
	r.ServeHTTP(ok, httptest.NewRequest(http.MethodPost, "/api/v1/public/analytics/events", strings.NewReader(
		`{"eventId":"e2","eventName":"page_view","pagePath":"/","referrerHost":"evil.example","payload":{"view_id":"v2"}}`)))
	require.Equal(t, http.StatusAccepted, ok.Code, ok.Body.String())
	require.Eventually(t, func() bool { return len(repo.events) == 1 }, 2*time.Second, 10*time.Millisecond)
	require.NotNil(t, repo.events[0].ReferrerHost)
	require.Equal(t, "evil.example", *repo.events[0].ReferrerHost)
}

func TestRegisterPublicContentRoutes(t *testing.T) {
	stub := &publicRouteRepoStub{newsTotal: 41}
	r := newPublicContentTestEngine(t, stub)

	res := getPublicPath(r, "/api/v1/public/news", false)
	require.Equal(t, http.StatusOK, res.Code)
	// R8-01:省略 page_size 默认 20，total/total_pages 真实。
	require.Contains(t, res.Body.String(), `"pageSize":20`)
	require.Contains(t, res.Body.String(), `"total":41`)
	require.Contains(t, res.Body.String(), `"totalPages":3`)
	require.Equal(t, 20, stub.lastLimit)
	require.Equal(t, 0, stub.lastOffset)

	res = getPublicPath(r, "/api/v1/public/articles", false)
	require.Equal(t, http.StatusOK, res.Code)

	res = getPublicPath(r, "/api/v1/public/news/zh-CN/missing", false)
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestRegisterPublicContentPageRoutesAreAnonymousHTML(t *testing.T) {
	r := newPublicContentTestEngine(t, &publicRouteRepoStub{})

	for _, path := range []string{"/docs", "/docs/authentication", "/robots.txt", "/sitemap.xml"} {
		res := getPublicPath(r, path, false)
		require.Equal(t, http.StatusOK, res.Code, path)
		if path == "/robots.txt" {
			require.Contains(t, res.Body.String(), "Sitemap: https://ccai.example/sitemap.xml")
			require.Contains(t, res.Body.String(), "Disallow: /model-square")
			require.Contains(t, res.Body.String(), "Disallow: /api/v1/admin/")
			continue
		}
		if path == "/sitemap.xml" {
			require.Contains(t, res.Header().Get("Content-Type"), "application/xml", path)
			require.Contains(t, res.Body.String(), "https://ccai.example/news/en/release", path)
			require.NotContains(t, res.Body.String(), "draft", path)
			require.NotContains(t, res.Body.String(), "/model-square", path)
			require.Equal(t, 1, strings.Count(res.Body.String(), "<loc>https://ccai.example/news</loc>"))
			continue
		}
		require.Contains(t, res.Header().Get("Content-Type"), "text/html", path)
		require.Contains(t, res.Body.String(), "CCAI", path)
	}
}

// R12-01/R12-02: 前缀路由 SSR 200，语言取自 URL 而非 cookie/query。
func TestPublicContentLanguagePrefixRendersSSR(t *testing.T) {
	for _, tc := range []struct{ path, segment, canonical string }{
		{"/zh/", "zh", "zh-CN"},
		{"/zh/news", "zh", "zh-CN"},
		{"/zh/articles", "zh", "zh-CN"},
		{"/en/news", "en", "en-US"},
		{"/ru/articles", "ru", "ru-RU"},
	} {
		stub := &publicRouteRepoStub{}
		r := newPublicContentTestEngine(t, stub)
		res := getPublicPath(r, tc.path, true)
		require.Equal(t, http.StatusOK, res.Code, tc.path)
		require.Contains(t, res.Header().Get("Content-Type"), "text/html", tc.path)
		// URL 语言决定 SSR 语言（renderer 对列表可能回显短码）。
		require.Contains(t, res.Body.String(), `<html lang="`+tc.segment, tc.path)
		require.Equal(t, tc.canonical, stub.lastLocale, tc.path)
		// 缓存响应带内容版本，便于核对旧版本是否已失效（R9-10）。
		require.NotEmpty(t, res.Header().Get("X-Public-Content-Version"), tc.path)
	}

	// 前缀 URL 的显式 ?locale= 不得覆盖 URL 语言。
	stub := &publicRouteRepoStub{}
	res := getPublicPath(newPublicContentTestEngine(t, stub), "/en/news?locale=zh", false)
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "en-US", stub.lastLocale)
}

// R12-01: 旧无前缀 URL 全部 301 到默认语言前缀，保留查询串。
func TestPublicContentLegacyUnprefixedURLsRedirect(t *testing.T) {
	r := newPublicContentTestEngine(t, &publicRouteRepoStub{})
	for _, tc := range []struct{ from, to string }{
		{"/", "/zh/"},
		{"/news", "/zh/news"},
		{"/news?page=2", "/zh/news?page=2"},
		{"/articles", "/zh/articles"},
		{"/news/hello-world", "/news/zh/hello-world"},
		{"/articles/guide?v=1", "/articles/zh/guide?v=1"},
		// R12-05: 详情里出现语言段时跳到对应的前缀集合，而不是拼成 slug。
		{"/news/en", "/en/news"},
	} {
		res := getPublicPath(r, tc.from, true)
		require.Equal(t, http.StatusMovedPermanently, res.Code, tc.from)
		require.Equal(t, tc.to, res.Header().Get("Location"), tc.from)
	}

	// 缺少结尾斜杠的首页 301 到标准形态（`/<lang>/`）。
	res := getPublicPath(r, "/zh", false)
	require.Equal(t, http.StatusMovedPermanently, res.Code)
	require.Equal(t, "/zh/", res.Header().Get("Location"))
}

// R12-05: 合法长码 301 短码；非法 locale 404（不跳登录）。
func TestPublicContentLocaleAliasesAndUnknownLocale(t *testing.T) {
	r := newPublicContentTestEngine(t, &publicRouteRepoStub{})
	for _, tc := range []struct{ from, to string }{
		{"/en-US/news", "/en/news"},
		{"/zh-CN/articles?page=2", "/zh/articles?page=2"},
		{"/de-DE/", "/de/"},
	} {
		res := getPublicPath(r, tc.from, false)
		require.Equal(t, http.StatusMovedPermanently, res.Code, tc.from)
		require.Equal(t, tc.to, res.Header().Get("Location"), tc.from)
	}

	for _, path := range []string{"/xx/news", "/xx/articles", "/xx/", "/zh-CN-CN/news"} {
		res := getPublicPath(r, path, false)
		require.Equal(t, http.StatusNotFound, res.Code, path)
		require.Contains(t, res.Header().Get("X-Robots-Tag"), "noindex", path)
		require.Equal(t, "no-store", res.Header().Get("Cache-Control"), path)
		require.Contains(t, res.Body.String(), "内容未找到", path)
	}
}

// R8-04: 前缀列表越界 301 到最后一页，Location 不残留内部注入的 locale。
func TestPublicContentPrefixedListClampsPageWithoutLocaleLeak(t *testing.T) {
	r := newPublicContentTestEngine(t, &publicRouteRepoStub{newsTotal: 41})
	res := getPublicPath(r, "/zh/news?page=9", false)
	require.Equal(t, http.StatusMovedPermanently, res.Code)
	require.Equal(t, "/zh/news?page=3", res.Header().Get("Location"))

	res = getPublicPath(r, "/en/articles?page=99&page_size=10", false)
	require.Equal(t, http.StatusOK, res.Code)
}

// R9-10: 内容变更后版本号推进，旧缓存条目不可能再命中。
func TestPublicContentCacheVersionAdvancesOnInvalidation(t *testing.T) {
	r := newPublicContentTestEngine(t, &publicRouteRepoStub{})
	before := getPublicPath(r, "/zh/news", false)
	require.Equal(t, http.StatusOK, before.Code)
	oldVersion := before.Header().Get("X-Public-Content-Version")
	require.NotEmpty(t, oldVersion)

	oldNumber, err := strconv.ParseUint(oldVersion, 10, 64)
	require.NoError(t, err)
	require.Equal(t, oldNumber, service.PublicContentCacheVersion())

	service.InvalidatePublicContentCache()

	after := getPublicPath(r, "/zh/news", false)
	require.Equal(t, http.StatusOK, after.Code)
	newNumber, err := strconv.ParseUint(after.Header().Get("X-Public-Content-Version"), 10, 64)
	require.NoError(t, err)
	require.Greater(t, newNumber, oldNumber)
	require.Equal(t, service.PublicContentCacheVersion(), newNumber)
	require.Contains(t, service.PublicContentCacheKey("/news", "en"), "v="+strconv.FormatUint(newNumber, 10))
}

// R8-04: 空列表第 1 页返回 200（不是 404/301），并保持默认每页 20。
func TestPublicContentPrefixedEmptyListReturns200(t *testing.T) {
	stub := &publicRouteRepoStub{newsTotal: 0}
	r := newPublicContentTestEngine(t, stub)
	for _, path := range []string{"/zh/news", "/en/articles"} {
		res := getPublicPath(r, path, false)
		require.Equal(t, http.StatusOK, res.Code, path)
		require.Equal(t, 20, stub.lastLimit, path)
		require.Equal(t, 0, stub.lastOffset, path)
	}
}

// 2026-11-05：只注册 `/:locale/` 时 gin 的 RedirectTrailingSlash 会把 `/login`、`/dashboard`
// 这类一段路径 301 到 `/login/`。显式注册 `/:locale` 后，只有真语言标签才跳 `/<lang>/`。
func TestPublicContentBareSegmentIsNotTrailingSlashRedirected(t *testing.T) {
	r := newPublicContentTestEngine(t, &publicRouteRepoStub{})
	for _, path := range []string{"/login", "/dashboard", "/register", "/keys"} {
		res := getPublicPath(r, path, false)
		require.NotEqual(t, http.StatusMovedPermanently, res.Code, path)
		require.NotEqual(t, path+"/", res.Header().Get("Location"), path)
	}
	for _, tc := range []struct{ from, to string }{{"/en", "/en/"}, {"/de-DE", "/de/"}, {"/zh?x=1", "/zh/?x=1"}} {
		res := getPublicPath(r, tc.from, false)
		require.Equal(t, http.StatusMovedPermanently, res.Code, tc.from)
		require.Equal(t, tc.to, res.Header().Get("Location"), tc.from)
	}
	res := getPublicPath(r, "/xx", false)
	require.Equal(t, http.StatusNotFound, res.Code)
	require.Contains(t, res.Body.String(), "内容未找到")
}
