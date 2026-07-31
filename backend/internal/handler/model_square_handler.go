package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type modelSquareService interface {
	ListGroups(ctx context.Context, userID int64) (*service.ModelSquareGroupsResult, error)
	ListModels(ctx context.Context, userID, groupID int64, query service.ModelSquareModelsQuery) (*service.ModelSquareModelsResult, error)
}

type modelSquareSettings interface {
	GetModelSquareRuntime(ctx context.Context) service.ModelSquareRuntime
}

type ModelSquareHandler struct {
	service  modelSquareService
	settings modelSquareSettings
}

func NewModelSquareHandler(modelSquareService *service.ModelSquareService, settingService *service.SettingService) *ModelSquareHandler {
	return &ModelSquareHandler{service: modelSquareService, settings: settingService}
}

func (h *ModelSquareHandler) enabled(ctx context.Context) bool {
	if h.settings == nil {
		return false
	}
	runtime := h.settings.GetModelSquareRuntime(ctx)
	return runtime.Enabled && runtime.SalesPricingVersion == service.SalesPricingVersionV2
}

func (h *ModelSquareHandler) ListGroups(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	if !h.enabled(c.Request.Context()) {
		response.Forbidden(c, "model square is disabled")
		return
	}
	result, err := h.service.ListGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ModelSquareHandler) ListModels(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	if !h.enabled(c.Request.Context()) {
		response.Forbidden(c, "model square is disabled")
		return
	}
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.BadRequest(c, "invalid group_id")
		return
	}
	pageSize := 50
	if raw := c.Query("page_size"); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil {
			response.BadRequest(c, "invalid page_size")
			return
		}
	}
	var catalogUpdatedAt *time.Time
	if raw := c.Query("catalog_updated_at"); raw != "" {
		value, parseErr := time.Parse(time.RFC3339Nano, raw)
		if parseErr != nil {
			response.BadRequest(c, "invalid catalog_updated_at")
			return
		}
		catalogUpdatedAt = &value
	}
	result, err := h.service.ListModels(c.Request.Context(), subject.UserID, groupID, service.ModelSquareModelsQuery{
		Search: c.Query("q"), Cursor: c.Query("cursor"), PageSize: pageSize,
		CatalogUpdatedAt: catalogUpdatedAt,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
