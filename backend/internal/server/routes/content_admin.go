package routes

import (
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
)

// RegisterContentAdminRoutes registers the content-management endpoints under
// an already-authenticated /admin group. The caller applies the existing
// super-admin middleware, so this helper does not introduce a second auth path.
func RegisterContentAdminRoutes(admin *gin.RouterGroup, h *adminhandler.ContentNewsHandler) {
	content := admin.Group("/content")
	{
		sources := content.Group("/news-sources")
		sources.GET("", h.ListNewsSources)
		sources.POST("", h.CreateNewsSource)
		sources.GET("/:id", h.GetNewsSource)
		sources.PUT("/:id", h.UpdateNewsSource)
		sources.DELETE("/:id", h.DeleteNewsSource)
		sources.POST("/:id/enable", h.EnableNewsSource)
		sources.POST("/:id/disable", h.DisableNewsSource)
		sources.POST("/:id/fetch", h.FetchNewsSource)
		sources.POST("/:id/restore", h.RestoreNewsSource)

		news := content.Group("/news")
		news.GET("", h.ListNews)
		news.POST("/bulk", h.BulkNews)
		news.POST("", h.CreateNews)
		news.GET("/:id", h.GetNews)
		news.GET("/:id/translation-tasks", h.ListNewsTranslationTasks)
		news.GET("/:id/translation-tasks/:taskId", h.GetNewsTranslationTask)
		news.PUT("/:id", h.UpdateNews)
		news.DELETE("/:id", h.DeleteNews)
		news.POST("/:id/publish", h.PublishNews)
		news.POST("/:id/seo-autofill", h.AutoFillSEO)
		news.POST("/:id/unpublish", h.UnpublishNews)
		news.POST("/:id/restore", h.RestoreNews)

		runs := content.Group("/fetch-runs")
		runs.GET("", h.ListFetchRuns)
		runs.GET("/:id", h.GetFetchRun)
		runs.POST("/:id/retry", h.RetryFetchRun)

		automation := content.Group("/automation-settings")
		automation.GET("", h.GetAutomationSettings)
		automation.PUT("", h.UpdateAutomationSettings)
		automation.POST("/pause", h.PauseAutomation)
		automation.POST("/resume", h.ResumeAutomation)
	}
}

func registerArticleAdminRoutes(admin *gin.RouterGroup, h *adminhandler.ContentArticleHandler) {
	content := admin.Group("/content/articles")
	content.GET("", h.List)
	content.POST("/bulk", h.Bulk)
	content.POST("", h.Create)
	content.GET("/models", h.Models)
	content.GET("/ai-config", h.GetAIConfig)
	content.GET("/ai-configs", h.ListAIConfigs)
	content.POST("/ai-configs/:action", h.MutateAIConfig)
	content.POST("/ai-config-test", h.TestAIConfig)
	content.PUT("/ai-config", h.UpdateAIConfig)
	content.POST("/ai-config/fetch-models", h.FetchAIModels)
	content.GET("/taxonomy", h.GetTaxonomy)
	content.POST("/taxonomy/mutate", h.MutateTaxonomy)
	content.POST("/writing-tasks", h.CreateWritingTask)
	content.GET("/writing-tasks", h.ListWritingTasks)
	content.GET("/writing-tasks/:task_id", h.GetWritingTask)
	content.POST("/writing-tasks/:task_id/cancel", h.CancelWritingTask)
	content.DELETE("/writing-tasks/:task_id", h.DeleteWritingTask)
	content.GET("/translations/:translation_id", h.GetTranslation)
	content.PUT("/translations/:translation_id", h.UpdateTranslation)
	content.POST("/translations/:translation_id/publish", h.PublishTranslation)
	content.POST("/translations/:translation_id/unpublish", h.UnpublishTranslation)
	content.DELETE("/translations/:translation_id", h.DeleteTranslation)
	content.GET("/:id", h.Get)
	content.POST("/:id/seo-autofill", h.AutoFillSEO)
	content.PUT("/:id", h.Update)
	content.DELETE("/:id", h.Delete)
	content.POST("/:id/publish", h.Publish)
	content.POST("/:id/unpublish", h.Unpublish)
	content.POST("/:id/restore", h.Restore)
	content.GET("/:id/translations", h.ListTranslations)
	content.POST("/:id/translations", h.CreateTranslation)
	content.POST("/:id/translations/tasks", h.CreateTranslationTask)
}
