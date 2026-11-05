package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type publicContentHandlerRepoStub struct {
	news          service.PublicNewsPage
	article       service.PublicArticlePage
	newsDetail    *service.PublicNews
	articleDetail *service.PublicArticle
	err           error
}

func (s *publicContentHandlerRepoStub) ListPublishedNews(context.Context, string, int, int) (service.PublicNewsPage, error) {
	return s.news, s.err
}
func (s *publicContentHandlerRepoStub) GetPublishedNews(context.Context, string, string) (*service.PublicNews, error) {
	return s.newsDetail, s.err
}
func (s *publicContentHandlerRepoStub) ListPublishedArticles(context.Context, string, int, int) (service.PublicArticlePage, error) {
	return s.article, s.err
}
func (s *publicContentHandlerRepoStub) GetPublishedArticle(context.Context, string, string) (*service.PublicArticle, error) {
	return s.articleDetail, s.err
}

func newPublicContentHandlerTestRouter(repo *publicContentHandlerRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewPublicContentHandler(service.NewPublicContentService(repo), &config.Config{Server: config.ServerConfig{FrontendURL: "https://ccai.example"}})
	r.GET("/", h.RenderHomeHTML)
	r.GET("/news", h.ListNews)
	r.GET("/news/:locale/:slug", h.GetNews)
	r.GET("/news-html", h.RenderNewsListHTML)
	r.GET("/articles", h.ListArticles)
	r.GET("/articles/:locale/:slug", h.GetArticle)
	r.GET("/news-html/:locale/:slug", h.RenderNewsHTML)
	r.GET("/articles-html", h.RenderArticleListHTML)
	r.GET("/article-html/:locale/:slug", h.RenderArticleHTML)
	r.GET("/docs", h.RenderDocsHTML)
	r.GET("/docs/*path", h.RenderDocsHTML)
	return r
}

func TestPublicContentHandlerListNewsEnvelope(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{news: service.PublicNewsPage{
		Items: []service.PublicNewsListItem{{Locale: "en-US", Slug: "release", Title: "Release", Summary: "Summary", SourceName: "CCAI", PublishedAt: time.Now(), Status: "published"}},
		Total: 3,
	}})
	req := httptest.NewRequest(http.MethodGet, "/news?locale=en-US&page=2&page_size=1", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
			Total    int `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, 2, envelope.Data.Page)
	require.Equal(t, 1, envelope.Data.PageSize)
	require.Equal(t, 3, envelope.Data.Total)
}

func TestPublicHTMLCacheKeyCanonicalizesQueries(t *testing.T) {
	u, err := url.Parse("/news?utm_source=bot&locale=zh-CN&page=2&page_size=12&random=1")
	require.NoError(t, err)
	require.Equal(t, "/news?locale=zh-CN&page=2&page_size=12", publicHTMLCacheKey(u))
}

func TestPublicContentHandlerRejectsInvalidPagination(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{})
	req := httptest.NewRequest(http.MethodGet, "/news?page_size=101", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestPublicContentHandlerDetailNotFound(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{err: service.ErrPublicContentNotFound})
	req := httptest.NewRequest(http.MethodGet, "/news/zh-CN/missing", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusNotFound, res.Code)
	require.NotContains(t, res.Body.String(), "sourceUrl")
}

func TestPublicContentHandlerDetailGone(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{err: service.ErrPublicContentGone})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news/zh-CN/deleted", nil))
	require.Equal(t, http.StatusGone, res.Code)
}

func TestPublicContentHandlerHTMLIsPublicAndRedactsSourceLink(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{newsDetail: &service.PublicNews{
		Locale: "zh-CN", Slug: "hello", Title: "<Hello>", Summary: "摘要", BodyHTML: "<p>正文</p>", SourceName: "CCAI", PublishedAt: time.Date(2026, 9, 9, 1, 2, 3, 0, time.UTC), Status: "published",
	}})
	req := httptest.NewRequest(http.MethodGet, "/news-html/zh/hello", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Header().Get("Content-Type"), "text/html")
	require.Equal(t, "index, follow", res.Header().Get("X-Robots-Tag"))
	body := res.Body.String()
	require.Contains(t, body, "&lt;Hello&gt;")
	require.Contains(t, body, "<p>正文</p>")
	require.NotContains(t, body, "阅读原文")
	require.NotContains(t, body, "sourceUrl")
	require.NotContains(t, body, "原文链接")
}

func TestPublicContentHandlerArticleHTMLIncludesPublishedAlternates(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{articleDetail: &service.PublicArticle{
		Locale: "zh-CN", Slug: "guide", Title: "Guide", Excerpt: "Excerpt", BodyHTML: "<p>body</p>", PublishedAt: time.Now(), Status: "published",
		Alternates: []service.PublicAlternate{{Locale: "en-US", Slug: "guide-en"}},
	}})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/article-html/zh/guide", nil))
	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	require.Contains(t, body, `rel="alternate" hreflang="zh"`)
	require.Contains(t, body, `rel="alternate" hreflang="en"`)
	require.Contains(t, body, `rel="alternate" hreflang="x-default"`)
	require.Contains(t, body, "/articles/en/guide-en")
	require.Contains(t, body, `rel="canonical" href="https://ccai.example/articles/zh/guide"`)
}

func TestPublicContentHandlerInternalErrorDoesNotLeak(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{err: errors.New("database password")})
	req := httptest.NewRequest(http.MethodGet, "/articles", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusInternalServerError, res.Code)
	require.NotContains(t, strings.ToLower(res.Body.String()), "database password")
}

func TestPublicContentHandlerRendersCollectionsDocsAndSanitizedDetails(t *testing.T) {
	now := time.Date(2026, 9, 9, 1, 2, 3, 0, time.UTC)
	repo := &publicContentHandlerRepoStub{
		news: service.PublicNewsPage{Items: []service.PublicNewsListItem{{
			Locale: "en-US", Slug: "release", Title: "Release", Summary: "Summary", SourceName: "CCAI", PublishedAt: now, Status: "published",
		}}},
		article: service.PublicArticlePage{Items: []service.PublicArticleListItem{{
			Locale: "en-US", Slug: "guide", Title: "Guide", Excerpt: "Excerpt", PublishedAt: now, Status: "published",
		}}},
		newsDetail: &service.PublicNews{
			Locale: "en-US", Slug: "release", Title: "Release", Summary: "Summary", SourceName: "CCAI", PublishedAt: now, Status: "published",
			BodyHTML: `<p class="lead" onclick="alert(1)">Safe <strong>body</strong></p><script>alert(1)</script><iframe src="https://evil.test"></iframe><a href="https://source.test">Read source</a><img src="javascript:alert(1)" onerror="alert(1)">`,
		},
		articleDetail: &service.PublicArticle{
			Locale: "en-US", Slug: "guide", Title: "Guide", Excerpt: "Excerpt", PublishedAt: now, Status: "published",
			BodyHTML: `<h2>Heading</h2><p>Article body</p>`,
		},
	}
	r := newPublicContentHandlerTestRouter(repo)

	// Collection pages keep the stored BCP-47 tag in <html lang>; paths carry
	// the short segment in canonical URLs and internal links (R12-05).
	for path, wantLang := range map[string]string{
		"/?locale=en-US":                    "en-US",
		"/news-html?locale=en-US":           "en-US",
		"/articles-html?locale=en-US":       "en-US",
		"/docs?locale=en-US":                "en",
		"/docs/authentication?locale=en-US": "en",
	} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, res.Code, path)
		require.Equal(t, "index, follow", res.Header().Get("X-Robots-Tag"), path)
		require.Contains(t, res.Body.String(), `lang="`+wantLang+`"`, path)
		require.Contains(t, res.Body.String(), `application/ld+json`, path)
	}
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html?locale=en-US", nil))
	require.Contains(t, res.Body.String(), `<link rel="canonical" href="https://ccai.example/en/news">`)
	require.Contains(t, res.Body.String(), `href="/news/en/release"`)

	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/en/release", nil))
	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	require.Contains(t, body, `<p class="lead">Safe <strong>body</strong></p>`)
	require.Contains(t, body, "Read source")
	require.Contains(t, body, `nonce="`)
	require.NotContains(t, body, "onclick")
	require.NotContains(t, body, "alert(1)")
	require.NotContains(t, body, "<iframe")
	require.NotContains(t, body, "source.test")
	require.NotContains(t, body, "javascript:")
	// Captured third-party media must be copied to controlled CCAI storage
	// before publication; the SSR sanitizer never emits arbitrary source URLs.
	require.NotContains(t, body, "https://evil.test/image.png")
	require.NotContains(t, body, "阅读原文")
}

func TestSafePublicMediaURLOnlyAllowsControlledPaths(t *testing.T) {
	require.Equal(t, "/media/content/news-1.png", safePublicMediaURL("/media/content/news-1.png"))
	require.Equal(t, "/public/media/news-1.mp4", safePublicMediaURL("/public/media/news-1.mp4"))
	require.Empty(t, safePublicMediaURL("https://cdn.example/news-1.png"))
	require.Empty(t, safePublicMediaURL("http://127.0.0.1/news-1.png"))
}

func TestPublicContentHandlerHTMLNotFoundIsNoIndex(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{err: service.ErrPublicContentNotFound})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/zh/missing", nil))
	require.Equal(t, http.StatusNotFound, res.Code)
	require.Equal(t, "noindex, nofollow, noarchive", res.Header().Get("X-Robots-Tag"))
	require.Contains(t, res.Header().Get("Content-Type"), "text/html")
}

func TestPublicContentHandlerHTMLGoneIsNoIndex(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{err: service.ErrPublicContentGone})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/zh/deleted", nil))
	require.Equal(t, http.StatusGone, res.Code)
	require.Equal(t, "noindex, nofollow, noarchive", res.Header().Get("X-Robots-Tag"))
	require.NotContains(t, res.Body.String(), "deleted")
}

func TestPublicContentHandlerHTMLInvalidPaginationIsNoIndex(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html?page_size=101", nil))
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Equal(t, "noindex, nofollow, noarchive", res.Header().Get("X-Robots-Tag"))
	require.Contains(t, res.Header().Get("Content-Type"), "text/html")
	require.NotContains(t, res.Body.String(), `"code"`)
}

func TestPublicContentHandlerJSONDoesNotExposeRawHTML(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{newsDetail: &service.PublicNews{
		Locale: "zh-CN", Slug: "safe", Title: "Safe", Summary: "Summary", SourceName: "CCAI",
		BodyHTML: `<p onclick="alert(1)">unsafe</p><script>alert(1)</script>`, PublishedAt: time.Now(), Status: "published",
	}})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news/zh-CN/safe", nil))
	require.Equal(t, http.StatusOK, res.Code)
	// NE-19: bodyHtml 对外可见，但只允许清洗后的白名单内容。
	require.Contains(t, res.Body.String(), `"bodyHtml"`)
	require.Contains(t, res.Body.String(), "unsafe")
	require.NotContains(t, res.Body.String(), "alert(1)")
	require.NotContains(t, res.Body.String(), "<script")
}

func TestSafePublicImageURLRejectsCapturedRemoteImages(t *testing.T) {
	require.Equal(t, "/media/content/cover.png", safePublicImageURL("/media/content/cover.png"))
	require.Empty(t, safePublicImageURL("https://source.example/image.png"))
	require.Empty(t, safePublicImageURL("http://127.0.0.1/image.png"))
	require.Empty(t, safePublicImageURL(`/\\evil.example/image.png`))
}

func TestPublicContentHandlerRedirectsStoredLocaleToShortSegment(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/en-US/hello?page=2", nil))
	require.Equal(t, http.StatusMovedPermanently, res.Code)
	require.Equal(t, "/news/en/hello?page=2", res.Header().Get("Location"))

	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/article-html/zh-CN/guide", nil))
	require.Equal(t, http.StatusMovedPermanently, res.Code)
	require.Equal(t, "/articles/zh/guide", res.Header().Get("Location"))

	// Short segments and unsupported tags are not redirected.
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/zh/hello", nil))
	require.NotEqual(t, http.StatusMovedPermanently, res.Code)
}

func TestPublicContentHandlerFallbackPageIsNoIndex(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{newsDetail: &service.PublicNews{
		Locale: "zh-CN", RequestedLocale: "de-DE", Fallback: "zh-CN", Slug: "hello",
		Title: "中文标题", Summary: "摘要", BodyHTML: "<p>中文正文</p>", SourceName: "CCAI",
		PublishedAt: time.Now(), Status: "published",
		Alternates: []service.PublicAlternate{{Locale: "en-US", Slug: "hello-en"}},
	}})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/de/hello", nil))
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "noindex, follow", res.Header().Get("X-Robots-Tag"))
	body := res.Body.String()
	require.Contains(t, body, `<meta name="robots" content="noindex, follow">`)
	require.Contains(t, body, `<html lang="de-DE">`)
	require.Contains(t, body, "中文正文")
	// Canonical points at the Chinese master, never at the untranslated URL.
	require.Contains(t, body, `rel="canonical" href="https://ccai.example/news/zh/hello"`)
	// hreflang only advertises Chinese and the published English translation.
	require.Contains(t, body, `rel="alternate" hreflang="zh" href="https://ccai.example/news/zh/hello"`)
	require.Contains(t, body, `rel="alternate" hreflang="en" href="https://ccai.example/news/en/hello-en"`)
	require.Contains(t, body, `hreflang="x-default" href="https://ccai.example/news/zh/hello"`)
	require.NotContains(t, body, `hreflang="de"`)
}

func TestPublicContentHandlerTranslatedPageStaysIndexable(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{articleDetail: &service.PublicArticle{
		Locale: "en-US", RequestedLocale: "en-US", Slug: "guide-en",
		Title: "Guide", Excerpt: "Excerpt", BodyHTML: "<p>body</p>", PublishedAt: time.Now(), Status: "published",
		Alternates: []service.PublicAlternate{{Locale: "zh-CN", Slug: "guide"}},
	}})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/article-html/en/guide-en", nil))
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "index, follow", res.Header().Get("X-Robots-Tag"))
	body := res.Body.String()
	require.Contains(t, body, `<meta name="robots" content="index, follow">`)
	require.Contains(t, body, `rel="canonical" href="https://ccai.example/articles/en/guide-en"`)
	require.Contains(t, body, `hreflang="zh" href="https://ccai.example/articles/zh/guide"`)
	require.Contains(t, body, `hreflang="x-default" href="https://ccai.example/articles/zh/guide"`)
}

type publicSitemapRepoStub struct {
	publicContentHandlerRepoStub
}

func (s *publicSitemapRepoStub) ListPublishedNewsForSitemap(context.Context) ([]service.PublicNewsListItem, error) {
	return []service.PublicNewsListItem{{
		Locale: "en-US", Slug: "release", Title: "Release", Summary: "Summary",
		SourceName: "CCAI", PublishedAt: time.Now(), Status: "published",
	}}, nil
}

func (s *publicSitemapRepoStub) ListPublishedArticlesForSitemap(context.Context) ([]service.PublicArticleListItem, error) {
	return nil, nil
}

func TestPublicContentSitemapUsesShortLocaleSegments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewPublicContentHandler(service.NewPublicContentService(&publicSitemapRepoStub{}), &config.Config{Server: config.ServerConfig{FrontendURL: "https://ccai.example"}})
	r.GET("/sitemap.xml", h.RenderSitemapXML)

	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	require.Contains(t, body, "<loc>https://ccai.example/news/en/release</loc>")
	require.NotContains(t, body, "/news/en-US/")
}

func newPublicContentCachingRouter(repo *publicContentHandlerRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CSPNonceKey, "test-nonce")
		c.Next()
	})
	h := NewPublicContentHandler(service.NewPublicContentService(repo), &config.Config{Server: config.ServerConfig{FrontendURL: "https://ccai.example"}})
	r.GET("/article-html/:locale/:slug", h.RenderArticleHTML)
	r.GET("/news-html", h.RenderNewsListHTML)
	return r
}

func TestPublicContentTranslationInvalidationClearsRenderedCache(t *testing.T) {
	// R9-10: 译文发布/下线后，已渲染的 SSR 文档不能继续服务旧内容。
	repo := &publicContentHandlerRepoStub{articleDetail: &service.PublicArticle{
		Locale: "en-US", RequestedLocale: "en-US", Slug: "guide-en",
		Title: "Before publish", Excerpt: "Excerpt", BodyHTML: "<p>old</p>",
		PublishedAt: time.Now(), Status: "published",
	}}
	r := newPublicContentCachingRouter(repo)

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/article-html/en/guide-en", nil))
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), "Before publish")

	// 同一 URL 的第二次请求命中进程内渲染缓存（证明缓存确实生效）。
	repo.articleDetail.Title = "Ignored while cached"
	cached := httptest.NewRecorder()
	r.ServeHTTP(cached, httptest.NewRequest(http.MethodGet, "/article-html/en/guide-en", nil))
	require.Contains(t, cached.Body.String(), "Before publish")

	versionBefore := service.PublicContentCacheVersion()
	service.InvalidatePublicContentCache()

	repo.articleDetail.Title = "After publish"
	after := httptest.NewRecorder()
	r.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/article-html/en/guide-en", nil))
	require.Equal(t, http.StatusOK, after.Code)
	require.Contains(t, after.Body.String(), "After publish")
	require.NotContains(t, after.Body.String(), "Before publish")
	require.Greater(t, service.PublicContentCacheVersion(), versionBefore)
	require.Contains(t, service.PublicContentCacheKey("/articles", "en-US"), "v="+strconv.FormatUint(service.PublicContentCacheVersion(), 10))
}

// publicPagedRepoStub honours limit/offset like the real repository (which
// clamps the window, see repository.publicPageBounds) so tests can observe
// which page the handler actually served.
type publicPagedRepoStub struct {
	publicContentHandlerRepoStub

	newsTotal     int
	articleTotal  int
	lastNewsLimit int
	lastNewsOff   int
}

func publicPagedBounds(total, limit, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return offset, end
}

func (s *publicPagedRepoStub) ListPublishedNews(_ context.Context, _ string, limit, offset int) (service.PublicNewsPage, error) {
	s.lastNewsLimit, s.lastNewsOff = limit, offset
	start, end := publicPagedBounds(s.newsTotal, limit, offset)
	items := make([]service.PublicNewsListItem, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, service.PublicNewsListItem{
			ID: int64(i + 1), Locale: "zh-CN", Slug: "news-" + strconv.Itoa(i+1),
			Title: "News " + strconv.Itoa(i+1), Summary: "Summary", SourceName: "CCAI",
			PublishedAt: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), Status: "published",
		})
	}
	return service.PublicNewsPage{Items: items, Total: s.newsTotal}, nil
}

func (s *publicPagedRepoStub) ListPublishedArticles(_ context.Context, _ string, limit, offset int) (service.PublicArticlePage, error) {
	start, end := publicPagedBounds(s.articleTotal, limit, offset)
	items := make([]service.PublicArticleListItem, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, service.PublicArticleListItem{
			ID: int64(i + 1), Locale: "zh-CN", Slug: "article-" + strconv.Itoa(i+1),
			Title: "Article " + strconv.Itoa(i+1), Excerpt: "Excerpt",
			PublishedAt: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), Status: "published",
		})
	}
	return service.PublicArticlePage{Items: items, Total: s.articleTotal}, nil
}

type publicPageEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
		Total      int `json:"total"`
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		TotalPages int `json:"totalPages"`
	} `json:"data"`
}

func decodePublicPage(t *testing.T, res *httptest.ResponseRecorder) publicPageEnvelope {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.Contains(t, res.Header().Get("Content-Type"), "application/json")
	var envelope publicPageEnvelope
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope), res.Body.String())
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, "success", envelope.Message)
	return envelope
}

// The JSON APIs must clamp a past-the-end page to the last page and answer
// 200 JSON; the 301 they used to emit is unreadable for non-browser clients.
func TestPublicContentHandlerJSONClampsPagePastEnd(t *testing.T) {
	repo := &publicPagedRepoStub{newsTotal: 45, articleTotal: 45}
	r := newPublicContentHandlerPagedRouter(repo)

	for _, tc := range []struct {
		target      string
		wantFirst   string
		wantItemLen int
	}{
		{"/articles?locale=zh-CN&page=999&page_size=20", "article-41", 5},
		{"/news?locale=zh-CN&page=999&page_size=20", "news-41", 5},
	} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, tc.target, nil))
		envelope := decodePublicPage(t, res)
		require.Empty(t, res.Header().Get("Location"), tc.target)
		require.Equal(t, 3, envelope.Data.TotalPages, tc.target)
		require.Equal(t, 3, envelope.Data.Page, tc.target)
		require.Equal(t, 45, envelope.Data.Total, tc.target)
		require.Len(t, envelope.Data.Items, tc.wantItemLen, tc.target)
		require.Equal(t, tc.wantFirst, envelope.Data.Items[0].Slug, tc.target)
	}

	// The clamped page really was re-queried from the service (page 3 => offset 40).
	require.Equal(t, 20, repo.lastNewsLimit)
	require.Equal(t, 40, repo.lastNewsOff)
}

func TestPublicContentHandlerJSONKeepsInRangePage(t *testing.T) {
	repo := &publicPagedRepoStub{newsTotal: 45, articleTotal: 45}
	r := newPublicContentHandlerPagedRouter(repo)

	for _, target := range []string{
		"/articles?locale=zh-CN&page=2&page_size=20",
		"/news?locale=zh-CN&page=2&page_size=20",
	} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
		envelope := decodePublicPage(t, res)
		require.Equal(t, 2, envelope.Data.Page, target)
		require.Equal(t, 3, envelope.Data.TotalPages, target)
		require.Len(t, envelope.Data.Items, 20, target)
	}

	// An empty collection keeps the page echo and never redirects.
	empty := &publicPagedRepoStub{}
	emptyRes := httptest.NewRecorder()
	newPublicContentHandlerPagedRouter(empty).ServeHTTP(emptyRes, httptest.NewRequest(http.MethodGet, "/articles?page=5", nil))
	emptyEnvelope := decodePublicPage(t, emptyRes)
	require.Equal(t, 5, emptyEnvelope.Data.Page)
	require.Equal(t, 0, emptyEnvelope.Data.TotalPages)
	require.Empty(t, emptyEnvelope.Data.Items)
}

func TestPublicContentHandlerJSONDegradesInvalidPageToFirst(t *testing.T) {
	repo := &publicPagedRepoStub{newsTotal: 45, articleTotal: 45}
	r := newPublicContentHandlerPagedRouter(repo)

	for _, target := range []string{
		"/articles?locale=zh-CN&page=0&page_size=20",
		"/articles?locale=zh-CN&page=abc&page_size=20",
		"/articles?locale=zh-CN&page=-5&page_size=20",
		"/news?locale=zh-CN&page=0&page_size=20",
		"/news?locale=zh-CN&page=abc&page_size=20",
	} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
		envelope := decodePublicPage(t, res)
		require.Equal(t, 1, envelope.Data.Page, target)
		require.Equal(t, 20, envelope.Data.PageSize, target)
		require.Len(t, envelope.Data.Items, 20, target)
	}
}

// HTML collection pages keep canonicalising past-the-end pages with a 301.
func TestPublicContentHandlerHTMLStillRedirectsPastEnd(t *testing.T) {
	repo := &publicPagedRepoStub{newsTotal: 45, articleTotal: 45}
	r := newPublicContentHandlerPagedRouter(repo)

	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/articles-html?locale=zh-CN&page=999&page_size=20", nil))
	require.Equal(t, http.StatusMovedPermanently, res.Code)
	require.Equal(t, "/articles-html?locale=zh-CN&page=3&page_size=20", res.Header().Get("Location"))

	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html?locale=zh-CN&page=999&page_size=20", nil))
	require.Equal(t, http.StatusMovedPermanently, res.Code)
	require.Equal(t, "/news-html?locale=zh-CN&page=3&page_size=20", res.Header().Get("Location"))
}

func newPublicContentHandlerPagedRouter(repo *publicPagedRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewPublicContentHandler(service.NewPublicContentService(repo), &config.Config{Server: config.ServerConfig{FrontendURL: "https://ccai.example"}})
	r.GET("/news", h.ListNews)
	r.GET("/articles", h.ListArticles)
	r.GET("/news-html", h.RenderNewsListHTML)
	r.GET("/articles-html", h.RenderArticleListHTML)
	return r
}

func TestPublicContentListCacheUsesVersionedKey(t *testing.T) {
	// 版本号参与缓存键：同一路径在同一版本下复用，版本推进后换键。
	repo := &publicContentHandlerRepoStub{news: service.PublicNewsPage{Total: 1, Locale: "zh-CN"}}
	r := newPublicContentCachingRouter(repo)
	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/news-html", nil))
	require.Equal(t, http.StatusOK, first.Code)

	key := service.PublicContentCacheKey("/news", "zh-CN")
	version := service.PublicContentCacheVersion()
	require.Contains(t, key, "?locale=zh-CN&v="+strconv.FormatUint(version, 10))
	require.NotEqual(t, key, service.PublicContentCacheKey("/news", "en-US"))
}
