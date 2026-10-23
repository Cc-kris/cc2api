package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
	req := httptest.NewRequest(http.MethodGet, "/news-html/zh-CN/hello", nil)
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
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/article-html/zh-CN/guide", nil))
	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	require.Contains(t, body, `rel="alternate" hreflang="zh-CN"`)
	require.Contains(t, body, `rel="alternate" hreflang="en-US"`)
	require.Contains(t, body, `rel="alternate" hreflang="x-default"`)
	require.Contains(t, body, "/articles/en-US/guide-en")
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

	for _, path := range []string{"/?locale=en-US", "/news-html?locale=en-US", "/articles-html?locale=en-US", "/docs?locale=en-US", "/docs/authentication?locale=en-US"} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, res.Code, path)
		require.Equal(t, "index, follow", res.Header().Get("X-Robots-Tag"), path)
		require.Contains(t, res.Body.String(), `lang="en-US"`, path)
		require.Contains(t, res.Body.String(), `application/ld+json`, path)
	}
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html?locale=en-US", nil))
	require.Contains(t, res.Body.String(), `<link rel="canonical" href="https://ccai.example/news?locale=en-US">`)

	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/en-US/release", nil))
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
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/zh-CN/missing", nil))
	require.Equal(t, http.StatusNotFound, res.Code)
	require.Equal(t, "noindex, nofollow, noarchive", res.Header().Get("X-Robots-Tag"))
	require.Contains(t, res.Header().Get("Content-Type"), "text/html")
}

func TestPublicContentHandlerHTMLGoneIsNoIndex(t *testing.T) {
	r := newPublicContentHandlerTestRouter(&publicContentHandlerRepoStub{err: service.ErrPublicContentGone})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/news-html/zh-CN/deleted", nil))
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
	require.NotContains(t, res.Body.String(), "bodyHtml")
	require.NotContains(t, res.Body.String(), "alert(1)")
}

func TestSafePublicImageURLRejectsCapturedRemoteImages(t *testing.T) {
	require.Equal(t, "/media/content/cover.png", safePublicImageURL("/media/content/cover.png"))
	require.Empty(t, safePublicImageURL("https://source.example/image.png"))
	require.Empty(t, safePublicImageURL("http://127.0.0.1/image.png"))
	require.Empty(t, safePublicImageURL(`/\\evil.example/image.png`))
}
