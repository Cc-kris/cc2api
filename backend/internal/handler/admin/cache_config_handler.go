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
	ListSemanticCacheAudits(ctx context.Context, filter service.SemanticCacheAuditListFilter) (*service.SemanticCacheAuditListPage, error)
	ReviewSemanticCacheAudit(ctx context.Context, auditID int64, req service.SemanticCacheAuditReviewRequest, operatorUserID int64, viewerRole string) (*service.SemanticCacheAuditListRecord, error)
	FeedbackSemanticCacheAudit(ctx context.Context, auditID int64, req service.SemanticCacheAuditFeedbackRequest, operatorUserID int64, viewerRole string) (*service.SemanticCacheAuditListRecord, error)
}

type CacheConfigHandler struct {
	settingService       *service.SettingService
	openAIGatewayService localResponseCacheClearService
}

func NewCacheConfigHandler(settingService *service.SettingService, openAIGatewayService localResponseCacheClearService) *CacheConfigHandler {
	return &CacheConfigHandler{settingService: settingService, openAIGatewayService: openAIGatewayService}
}

func (h *CacheConfigHandler) GetConfig(c *gin.Context) {
	role, _ := middleware.GetUserRoleFromContext(c)
	cfg, err := h.settingService.GetCacheManagementConfigForRole(c.Request.Context(), role)
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
	role, _ := middleware.GetUserRoleFromContext(c)
	cfg, err := h.settingService.UpdateCacheManagementConfigForRole(c.Request.Context(), req, role)
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
	role, _ := middleware.GetUserRoleFromContext(c)
	cfg, err := h.settingService.GetAdvancedCacheConfigForRole(c.Request.Context(), role)
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
	role, _ := middleware.GetUserRoleFromContext(c)
	cfg, err := h.settingService.UpdateAdvancedCacheConfigForRole(c.Request.Context(), req, role)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *CacheConfigHandler) GetSemanticConfig(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Semantic cache config service is unavailable")
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	cfg, err := h.settingService.GetSemanticCacheConfigForRole(c.Request.Context(), role)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *CacheConfigHandler) UpdateSemanticConfig(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Semantic cache config service is unavailable")
		return
	}
	var req service.SemanticCacheConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	cfg, err := h.settingService.UpdateSemanticCacheConfigForRole(c.Request.Context(), req, role)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *CacheConfigHandler) TestSemanticConfig(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Semantic cache config service is unavailable")
		return
	}
	var req service.SemanticCacheConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	if !strings.EqualFold(strings.TrimSpace(role), "admin") {
		response.Forbidden(c, "无权限测试语义缓存配置")
		return
	}
	result, err := h.settingService.TestSemanticCacheConnection(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
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
	req.ViewerRole, _ = middleware.GetUserRoleFromContext(c)
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
	filter.ViewerRole, _ = middleware.GetUserRoleFromContext(c)
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

func (h *CacheConfigHandler) ListSemanticAudits(c *gin.Context) {
	if h.openAIGatewayService == nil {
		response.Error(c, http.StatusInternalServerError, "Semantic cache audit unavailable")
		return
	}
	filter, err := parseSemanticCacheAuditFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter.ViewerRole, _ = middleware.GetUserRoleFromContext(c)
	page, err := h.openAIGatewayService.ListSemanticCacheAudits(c.Request.Context(), filter)
	if err != nil {
		if errors.Is(err, service.ErrInvalidSemanticCacheAuditList) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrSemanticCacheAuditUnavailable) {
			response.Error(c, http.StatusInternalServerError, "Semantic cache audit unavailable")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, page)
}

func (h *CacheConfigHandler) ReviewSemanticAudit(c *gin.Context) {
	if h.openAIGatewayService == nil {
		response.Error(c, http.StatusInternalServerError, "Semantic cache audit unavailable")
		return
	}
	auditID, err := parsePositiveInt64Param(c.Param("id"), "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req service.SemanticCacheAuditReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Forbidden(c, "无权限审核语义审计")
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	record, err := h.openAIGatewayService.ReviewSemanticCacheAudit(c.Request.Context(), auditID, req, subject.UserID, role)
	if err != nil {
		if errors.Is(err, service.ErrInvalidSemanticCacheAuditReview) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrSemanticCacheAuditUnavailable) {
			response.Error(c, http.StatusInternalServerError, "Semantic cache audit unavailable")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, record)
}

func (h *CacheConfigHandler) FeedbackSemanticAudit(c *gin.Context) {
	if h.openAIGatewayService == nil {
		response.Error(c, http.StatusInternalServerError, "Semantic cache audit unavailable")
		return
	}
	auditID, err := parsePositiveInt64Param(c.Param("id"), "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req service.SemanticCacheAuditFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Forbidden(c, "无权限反馈语义审计")
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	record, err := h.openAIGatewayService.FeedbackSemanticCacheAudit(c.Request.Context(), auditID, req, subject.UserID, role)
	if err != nil {
		if errors.Is(err, service.ErrInvalidSemanticCacheAuditFeedback) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrSemanticCacheAuditUnavailable) {
			response.Error(c, http.StatusInternalServerError, "Semantic cache audit unavailable")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, record)
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

func parseSemanticCacheAuditFilter(c *gin.Context) (service.SemanticCacheAuditListFilter, error) {
	page, err := parseOptionalPositiveInt(c.Query("page"), "page")
	if err != nil {
		return service.SemanticCacheAuditListFilter{}, err
	}
	pageSize, err := parseOptionalPositiveInt(c.Query("page_size"), "page_size")
	if err != nil {
		return service.SemanticCacheAuditListFilter{}, err
	}
	start, err := parseOptionalRFC3339(c.Query("start_time"), "start_time")
	if err != nil {
		return service.SemanticCacheAuditListFilter{}, err
	}
	end, err := parseOptionalRFC3339(c.Query("end_time"), "end_time")
	if err != nil {
		return service.SemanticCacheAuditListFilter{}, err
	}
	apiKeyID, err := parseOptionalPositiveInt64(c.Query("api_key_id"), "api_key_id")
	if err != nil {
		return service.SemanticCacheAuditListFilter{}, err
	}
	minSimilarity, err := parseOptionalUnitFloat(c.Query("min_similarity"), "min_similarity")
	if err != nil {
		return service.SemanticCacheAuditListFilter{}, err
	}
	maxSimilarity, err := parseOptionalUnitFloat(c.Query("max_similarity"), "max_similarity")
	if err != nil {
		return service.SemanticCacheAuditListFilter{}, err
	}
	return service.SemanticCacheAuditListFilter{
		Page:          page,
		PageSize:      pageSize,
		StartTime:     start,
		EndTime:       end,
		Platform:      strings.TrimSpace(c.Query("platform")),
		Model:         strings.TrimSpace(c.Query("model")),
		APIKeyID:      apiKeyID,
		ReviewStatus:  strings.TrimSpace(c.Query("review_status")),
		Decision:      strings.TrimSpace(c.Query("decision")),
		MinSimilarity: minSimilarity,
		MaxSimilarity: maxSimilarity,
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

func parsePositiveInt64Param(raw, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return value, nil
}

func parseOptionalUnitFloat(raw, field string) (*float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || value > 1 {
		return nil, fmt.Errorf("invalid %s", field)
	}
	return &value, nil
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
