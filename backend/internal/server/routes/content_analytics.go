package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// registerContentAnalyticsRoutes wires the authenticated BI read APIs. The
// parent /admin group already applies AdminAuthMiddleware.
func registerContentAnalyticsRoutes(admin *gin.RouterGroup, h *handler.ContentAnalyticsHandler) {
	bi := admin.Group("/bi/analytics")
	bi.GET("", h.Query)
	bi.GET("/overview", h.Overview)
	bi.GET("/trends", h.Trends)
	bi.GET("/rankings", h.Rankings)
	bi.GET("/click-through-rate", h.ClickThroughRate)
	bi.GET("/export.csv", h.Export)
}
