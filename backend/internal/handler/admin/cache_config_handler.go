package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type localResponseCacheClearService interface {
	ClearLocalResponseCache(ctx context.Context, req service.LocalResponseCacheClearRequest) (*service.LocalResponseCacheClearResult, error)
}

type CacheConfigHandler struct {
	settingService       *service.SettingService
	openAIGatewayService localResponseCacheClearService
}

func NewCacheConfigHandler(settingService *service.SettingService, openAIGatewayService localResponseCacheClearService) *CacheConfigHandler {
	return &CacheConfigHandler{settingService: settingService, openAIGatewayService: openAIGatewayService}
}

func (h *CacheConfigHandler) GetConfig(c *gin.Context) {
	cfg, err := h.settingService.GetCacheManagementConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *CacheConfigHandler) UpdateConfig(c *gin.Context) {
	var req service.CacheManagementConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.settingService.UpdateCacheManagementConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *CacheConfigHandler) Clear(c *gin.Context) {
	var req service.LocalResponseCacheClearRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		req.OperatorUserID = &subject.UserID
	}
	if h.openAIGatewayService == nil {
		response.Error(c, http.StatusInternalServerError, "Local response cache clear unavailable")
		return
	}
	result, err := h.openAIGatewayService.ClearLocalResponseCache(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidLocalResponseCacheClear) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrLocalResponseCacheClearUnavailable) {
			response.Error(c, http.StatusInternalServerError, "Local response cache clear unavailable")
			return
		}
		if result != nil {
			response.Success(c, result)
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to clear local response cache")
		return
	}
	response.Success(c, result)
}
