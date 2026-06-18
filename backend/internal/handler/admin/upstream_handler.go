package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UpstreamHandler struct{ service *service.UpstreamService }

func NewUpstreamHandler(service *service.UpstreamService) *UpstreamHandler {
	return &UpstreamHandler{service: service}
}

func (h *UpstreamHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *UpstreamHandler) Create(c *gin.Context) {
	var req service.UpstreamInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.service.Create(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *UpstreamHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upstream id"})
		return
	}
	var req service.UpstreamInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *UpstreamHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upstream id"})
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *UpstreamHandler) SyncFromAccounts(c *gin.Context) {
	created, err := h.service.SyncFromAccounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": created})
}

func (h *UpstreamHandler) Stats(c *gin.Context) {
	start, end := parseStatsRange(c)
	resp, err := h.service.GetStats(c.Request.Context(), start, end, c.Query("granularity"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *UpstreamHandler) FinanceStats(c *gin.Context) {
	start, end := parseStatsRange(c)
	resp, err := h.service.GetFinanceStats(c.Request.Context(), start, end, c.Query("granularity"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func parseStatsRange(c *gin.Context) (time.Time, time.Time) {
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -30)
	if raw := c.Query("end_date"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			end = t.AddDate(0, 0, 1)
		}
	}
	if raw := c.Query("start_date"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			start = t
		}
	}
	return start, end
}
