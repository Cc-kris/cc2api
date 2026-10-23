package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"time"
)

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
	public.GET("/articles/:locale/:slug", h.PublicContent.GetArticle)
}

// RegisterPublicContentPageRoutes registers the anonymous HTML entry points.
// These routes are intentionally outside /api so crawlers can render the same
// canonical pages as browsers without authentication or JavaScript execution.
func RegisterPublicContentPageRoutes(r *gin.Engine, h *handler.Handlers) {
	r.GET("/robots.txt", h.PublicContent.RenderRobotsTXT)
	r.GET("/sitemap.xml", h.PublicContent.RenderSitemapXML)
	r.GET("/", h.PublicContent.RenderHomeHTML)
	r.GET("/news", h.PublicContent.RenderNewsListHTML)
	r.GET("/news/:locale/:slug", h.PublicContent.RenderNewsHTML)
	r.GET("/articles", h.PublicContent.RenderArticleListHTML)
	r.GET("/articles/:locale/:slug", h.PublicContent.RenderArticleHTML)
	r.GET("/docs", h.PublicContent.RenderDocsHTML)
	r.GET("/docs/*path", h.PublicContent.RenderDocsHTML)
}
