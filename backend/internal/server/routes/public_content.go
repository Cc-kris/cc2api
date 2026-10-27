package routes

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// publicDefaultLocaleSegment is the language served by the legacy unprefixed
// URLs. It always wins over the galley_locale cookie so the first paint of a
// crawlable page is deterministic (R12-01).
const publicDefaultLocaleSegment = "zh"

// RegisterPublicContentRoutes registers anonymous, crawlable content APIs and HTML details.
func RegisterPublicContentRoutes(v1 *gin.RouterGroup, h *handler.Handlers, redisClients ...*redis.Client) {
	analytics := v1.POST
	if len(redisClients) > 0 && redisClients[0] != nil {
		limiter := middleware.NewRateLimiter(redisClients[0])
		// Keep the anonymous ingress bounded across all application instances.
		// The per-IP budget protects individual clients; the global budget keeps
		// synchronous event writes from exhausting the database.
		v1.POST("/public/analytics/events",
			limiter.LimitWithOptions("public-analytics-ip", 120, time.Minute, middleware.RateLimitOptions{FailureMode: middleware.RateLimitFailClose}),
			limiter.LimitGlobal("public-analytics-global", 10000, time.Minute, middleware.RateLimitOptions{FailureMode: middleware.RateLimitFailClose}),
			h.ContentAnalytics.Ingest,
		)
	} else {
		analytics("/public/analytics/events", h.ContentAnalytics.Ingest)
	}
	public := v1.Group("/public")
	public.GET("/news", h.PublicContent.ListNews)
	public.GET("/news/:locale/:slug", h.PublicContent.GetNews)
	public.GET("/articles", h.PublicContent.ListArticles)
	public.GET("/articles/featured", h.PublicContent.ListFeaturedArticles)
	public.GET("/articles/:locale/:slug", h.PublicContent.GetArticle)
}

// RegisterPublicContentPageRoutes registers the anonymous HTML entry points.
// These routes are intentionally outside /api so crawlers can render the same
// canonical pages as browsers without authentication or JavaScript execution.
//
// URL contract (technical design §3.1): every public page is language
// prefixed (`/<lang>/`, `/<lang>/news`, `/<lang>/articles`,
// `/<lang>/<kind>/<slug>`). Legacy unprefixed URLs answer 301 into the default
// language prefix and never render another language from the cookie
// (R12-01); unprefixed details 301 into `/zh/...` (R9-08).
func RegisterPublicContentPageRoutes(r *gin.Engine, h *handler.Handlers) {
	r.GET("/robots.txt", h.PublicContent.RenderRobotsTXT)
	r.GET("/sitemap.xml", h.PublicContent.RenderSitemapXML)
	r.GET("/docs", h.PublicContent.RenderDocsHTML)
	r.GET("/docs/*path", h.PublicContent.RenderDocsHTML)

	// Legacy unprefixed collections. The query string is preserved so filters
	// and pagination survive the hop.
	r.GET("/", redirectPublicPath("/"+publicDefaultLocaleSegment+"/"))
	r.GET("/news", redirectPublicPath("/"+publicDefaultLocaleSegment+"/news"))
	r.GET("/articles", redirectPublicPath("/"+publicDefaultLocaleSegment+"/articles"))

	// Legacy unprefixed details (`/news/<slug>`) 301 to `/zh/<slug>`. A tail
	// that is itself a language tag is the prefixed collection instead.
	r.GET("/news/:locale", redirectLegacyPublicDetail("/news"))
	r.GET("/articles/:locale", redirectLegacyPublicDetail("/articles"))

	// Language-prefixed SSR entries. The middleware pins the URL language, so
	// `/<lang>/...` never depends on cookie or `?locale=` (R12-02, R12-05).
	locale := r.Group("/:locale", publicLocalePrefix())
	locale.GET("/", h.PublicContent.RenderHomeHTML)
	locale.GET("/news", h.PublicContent.RenderNewsListHTML)
	locale.GET("/articles", h.PublicContent.RenderArticleListHTML)

	// Detail pages keep the established `/news|/articles/<lang>/<slug>` shape.
	r.GET("/news/:locale/:slug", h.PublicContent.RenderNewsHTML)
	r.GET("/articles/:locale/:slug", h.PublicContent.RenderArticleHTML)
}

// publicLocalePrefix validates the URL language of a prefixed public page.
// Supported spellings are pinned onto the request (and onto generated
// redirects) while stored/alias spellings such as `en-US` 301 to the short
// segment; unsupported tags answer a plain 404 instead of falling into the SPA
// login redirect (R12-05).
func publicLocalePrefix() gin.HandlerFunc {
	return func(c *gin.Context) {
		requested := c.Param("locale")
		segment, ok := service.PublicLocaleSegment(requested)
		if !ok {
			publicSSRNotFound(c)
			return
		}
		if segment != requested {
			target := replaceFirstPathSegment(c.Request.URL.Path, segment)
			if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
				target += "?" + rawQuery
			}
			c.Redirect(http.StatusMovedPermanently, target)
			c.Abort()
			return
		}
		// The shared HTML handlers read the locale from the query, so pin it to
		// the URL segment and overwrite any conflicting `?locale=`.
		query := c.Request.URL.Query()
		query.Set("locale", segment)
		c.Request.URL.RawQuery = query.Encode()
		c.Header("X-Public-Content-Version", strconv.FormatUint(service.PublicContentCacheVersion(), 10))
		c.Writer = &publicLocaleQueryWriter{ResponseWriter: c.Writer}
		c.Next()
	}
}

// publicLocaleQueryWriter hides the internally pinned `locale` query parameter
// from 3xx Location headers so prefixed URLs stay canonical.
type publicLocaleQueryWriter struct {
	gin.ResponseWriter
}

func (w *publicLocaleQueryWriter) WriteHeader(code int) {
	w.rewriteLocation(code)
	w.ResponseWriter.WriteHeader(code)
}

func (w *publicLocaleQueryWriter) WriteHeaderNow() {
	if !w.Written() {
		w.rewriteLocation(w.Status())
	}
	w.ResponseWriter.WriteHeaderNow()
}

func (w *publicLocaleQueryWriter) rewriteLocation(code int) {
	if code < http.StatusMultipleChoices || code >= http.StatusBadRequest {
		return
	}
	location := w.Header().Get("Location")
	if location == "" {
		return
	}
	w.Header().Set("Location", stripLocaleQueryParam(location))
}

func redirectPublicPath(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		location := target
		if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
			location += "?" + rawQuery
		}
		c.Redirect(http.StatusMovedPermanently, location)
		c.Abort()
	}
}

// redirectLegacyPublicDetail keeps the old `/<kind>/<slug>` detail URLs alive
// as 301s (R9-08). The slug is escaped so it cannot smuggle a path or query.
func redirectLegacyPublicDetail(prefix string) gin.HandlerFunc {
	prefix = strings.TrimRight(prefix, "/")
	return func(c *gin.Context) {
		tail := c.Param("locale")
		target := ""
		if segment, ok := service.PublicLocaleSegment(tail); ok {
			target = "/" + segment + prefix
		} else {
			target = prefix + "/" + publicDefaultLocaleSegment + "/" + url.PathEscape(tail)
		}
		if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
			target += "?" + rawQuery
		}
		c.Redirect(http.StatusMovedPermanently, target)
		c.Abort()
	}
}

func replaceFirstPathSegment(path, segment string) string {
	trimmed := strings.TrimPrefix(path, "/")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return "/" + segment + trimmed[idx:]
	}
	return "/" + segment
}

func stripLocaleQueryParam(location string) string {
	parsed, err := url.Parse(location)
	if err != nil || parsed.RawQuery == "" {
		return location
	}
	query := parsed.Query()
	if _, ok := query["locale"]; !ok {
		return location
	}
	query.Del("locale")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

const publicSSRNotFoundHTML = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">` +
	`<meta name="robots" content="noindex,nofollow"><meta name="viewport" content="width=device-width,initial-scale=1">` +
	`<title>内容未找到 | CCAI</title></head><body><h1>内容未找到</h1><p>该语言暂不支持。</p></body></html>`

// publicSSRNotFound answers an uncached, non-indexable HTML 404 for paths that
// already passed the embedded-frontend whitelist but carry an unsupported
// language segment.
func publicSSRNotFound(c *gin.Context) {
	c.Header("X-Robots-Tag", "noindex, nofollow, noarchive")
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(publicSSRNotFoundHTML))
	c.Abort()
}
