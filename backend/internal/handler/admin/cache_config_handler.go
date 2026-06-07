package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CacheConfigHandler struct {
	settingService *service.SettingService
}

func NewCacheConfigHandler(settingService *service.SettingService) *CacheConfigHandler {
	return &CacheConfigHandler{settingService: settingService}
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
