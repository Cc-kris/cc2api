package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type localResponseCacheClearService interface {
	ClearLocalResponseCache(ctx context.Context, req service.LocalResponseCacheClearRequest) (*service.LocalResponseCacheClearResult, error)
	ListLocalResponseCacheClearAudits(ctx context.Context, filter service.LocalResponseCacheClearAuditFilter) (*service.LocalResponseCacheClearAuditPage, error)
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

func (h *CacheConfigHandler) GetAdvancedConfig(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Advanced cache config service is unavailable")
		return
	}
	cfg, err := h.settingService.GetAdvancedCacheConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *CacheConfigHandler) UpdateAdvancedConfig(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Advanced cache config service is unavailable")
		return
	}
	var req service.AdvancedCacheConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.settingService.UpdateAdvancedCacheConfig(c.Request.Context(), req)
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

func (h *CacheConfigHandler) ListClearAudits(c *gin.Context) {
	if h.openAIGatewayService == nil {
		response.Error(c, http.StatusInternalServerError, "Local response cache clear audit unavailable")
		return
	}
	filter, err := parseCacheClearAuditFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	page, err := h.openAIGatewayService.ListLocalResponseCacheClearAudits(c.Request.Context(), filter)
	if err != nil {
		if errors.Is(err, service.ErrInvalidLocalResponseCacheAuditList) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrLocalResponseCacheClearUnavailable) {
			response.Error(c, http.StatusInternalServerError, "Local response cache clear audit unavailable")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to list local response cache clear audits")
		return
	}
	response.Success(c, page)
}

func parseCacheClearAuditFilter(c *gin.Context) (service.LocalResponseCacheClearAuditFilter, error) {
	page, err := parseOptionalPositiveInt(c.Query("page"), "page")
	if err != nil {
		return service.LocalResponseCacheClearAuditFilter{}, err
	}
	pageSize, err := parseOptionalPositiveInt(c.Query("page_size"), "page_size")
	if err != nil {
		return service.LocalResponseCacheClearAuditFilter{}, err
	}
	start, err := parseOptionalRFC3339(c.Query("start_time"), "start_time")
	if err != nil {
		return service.LocalResponseCacheClearAuditFilter{}, err
	}
	end, err := parseOptionalRFC3339(c.Query("end_time"), "end_time")
	if err != nil {
		return service.LocalResponseCacheClearAuditFilter{}, err
	}
	operatorUserID, err := parseOptionalPositiveInt64(c.Query("operator_user_id"), "operator_user_id")
	if err != nil {
		return service.LocalResponseCacheClearAuditFilter{}, err
	}
	return service.LocalResponseCacheClearAuditFilter{
		Page:           page,
		PageSize:       pageSize,
		StartTime:      start,
		EndTime:        end,
		OperatorUserID: operatorUserID,
		ClearType:      strings.TrimSpace(c.Query("clear_type")),
		Status:         strings.TrimSpace(c.Query("status")),
	}, nil
}

func parseOptionalPositiveInt(raw, field string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return value, nil
}

func parseOptionalRFC3339(raw, field string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", field)
	}
	return &t, nil
}
