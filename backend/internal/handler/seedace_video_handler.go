package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SeedaceVideoHandler handles Seedace-compatible video generation endpoints.
type SeedaceVideoHandler struct {
	service *service.SeedaceVideoService
}

func NewSeedaceVideoHandler(service *service.SeedaceVideoService) *SeedaceVideoHandler {
	return &SeedaceVideoHandler{service: service}
}

func (h *SeedaceVideoHandler) Create(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "service_unavailable", "message": "Seedace video service is unavailable"}})
		return
	}
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": "unauthorized", "message": "API key context is missing"}})
		return
	}
	subscription, _ := middleware.GetSubscriptionFromContext(c)
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Failed to read request body"}})
		return
	}
	result, err := h.service.Create(c.Request.Context(), service.SeedaceVideoCreateInput{
		APIKey:       apiKey,
		Subscription: subscription,
		Body:         body,
		Headers:      c.Request.Header,
		UserAgent:    c.Request.UserAgent(),
		IPAddress:    c.ClientIP(),
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "upstream_error", "message": err.Error()}})
		return
	}
	writeSeedaceVideoResult(c, result)
}

func (h *SeedaceVideoHandler) Poll(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "service_unavailable", "message": "Seedace video service is unavailable"}})
		return
	}
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": "unauthorized", "message": "API key context is missing"}})
		return
	}
	result, err := h.service.Poll(c.Request.Context(), service.SeedaceVideoPollInput{
		APIKey:    apiKey,
		TaskID:    c.Param("task_id"),
		Headers:   c.Request.Header,
		UserAgent: c.Request.UserAgent(),
		IPAddress: c.ClientIP(),
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "upstream_error", "message": err.Error()}})
		return
	}
	writeSeedaceVideoResult(c, result)
}

func writeSeedaceVideoResult(c *gin.Context, result *service.SeedaceVideoResult) {
	if result == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "upstream_error", "message": "empty upstream response"}})
		return
	}
	for key, values := range result.Header {
		if shouldSkipSeedaceResponseHeader(key) {
			continue
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	contentType := result.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(result.StatusCode, contentType, result.Body)
}

func shouldSkipSeedaceResponseHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Content-Length", "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}
