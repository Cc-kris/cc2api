package admin

import (
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

const cacheStatsMaxWindow = 31 * 24 * time.Hour

type CacheStatsHandler struct {
	cacheStatsService *service.CacheStatsService
}

func NewCacheStatsHandler(cacheStatsService *service.CacheStatsService) *CacheStatsHandler {
	return &CacheStatsHandler{cacheStatsService: cacheStatsService}
}

func (h *CacheStatsHandler) GetStats(c *gin.Context) {
	if h == nil || h.cacheStatsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Cache stats service is unavailable")
		return
	}
	filter, err := parseCacheStatsFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	stats, err := h.cacheStatsService.GetStats(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get cache stats")
		return
	}
	response.Success(c, stats)
}

func (h *CacheStatsHandler) Export(c *gin.Context) {
	if h == nil || h.cacheStatsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Cache stats service is unavailable")
		return
	}
	filter, err := parseCacheStatsFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	data, err := h.cacheStatsService.ExportCSV(c.Request.Context(), filter)
	if err != nil {
		if errors.Is(err, service.ErrCacheStatsExportTooLarge) {
			response.BadRequest(c, "cache stats export row count exceeds limit")
			return
		}
		if errors.Is(err, service.ErrCacheStatsExportEmpty) {
			response.BadRequest(c, "current filter has no cache stats to export")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to export cache stats")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="cache-stats.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}

func parseCacheStatsFilter(c *gin.Context) (*service.CacheStatsFilter, error) {
	start, end, err := parseCacheStatsTimeRange(c)
	if err != nil {
		return nil, err
	}
	apiKeyID, err := parseOptionalPositiveInt64(c.Query("api_key_id"), "api_key_id")
	if err != nil {
		return nil, err
	}
	groupID, err := parseOptionalPositiveInt64(c.Query("group_id"), "group_id")
	if err != nil {
		return nil, err
	}
	return &service.CacheStatsFilter{
		StartTime:  start,
		EndTime:    end,
		Platform:   strings.TrimSpace(c.Query("platform")),
		Model:      strings.TrimSpace(c.Query("model")),
		APIKeyID:   apiKeyID,
		GroupID:    groupID,
		ViewerRole: cacheStatsViewerRole(c),
	}, nil
}

func cacheStatsViewerRole(c *gin.Context) string {
	role, _ := middleware.GetUserRoleFromContext(c)
	return role
}

func parseCacheStatsTimeRange(c *gin.Context) (time.Time, time.Time, error) {
	startStr := strings.TrimSpace(c.Query("start_time"))
	endStr := strings.TrimSpace(c.Query("end_time"))
	parseTS := func(raw string) (time.Time, error) {
		if raw == "" {
			return time.Time{}, nil
		}
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return t, nil
		}
		return time.Parse(time.RFC3339, raw)
	}
	start, err := parseTS(startStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_time")
	}
	end, err := parseTS(endStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end_time")
	}
	if startStr != "" || endStr != "" {
		if end.IsZero() {
			end = time.Now()
		}
		if start.IsZero() {
			start = end.Add(-24 * time.Hour)
		}
		return validateCacheStatsTimeRange(start, end)
	}
	dur := cacheStatsDuration(strings.TrimSpace(c.Query("time_range")))
	if dur <= 0 {
		dur = 24 * time.Hour
	}
	end = time.Now()
	start = end.Add(-dur)
	return validateCacheStatsTimeRange(start, end)
}

func validateCacheStatsTimeRange(start, end time.Time) (time.Time, time.Time, error) {
	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid time range: start_time must be <= end_time")
	}
	if end.Sub(start) > cacheStatsMaxWindow {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid time range: max window is 31 days")
	}
	return start, end, nil
}

func cacheStatsDuration(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	unit := raw[len(raw)-1:]
	value, err := strconv.Atoi(raw[:len(raw)-1])
	if err != nil || value <= 0 {
		return 0
	}
	switch unit {
	case "m":
		return time.Duration(value) * time.Minute
	case "h":
		return time.Duration(value) * time.Hour
	case "d":
		return time.Duration(value) * 24 * time.Hour
	default:
		return 0
	}
}

func parseOptionalPositiveInt64(raw, field string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, fmt.Errorf("invalid %s", field)
	}
	return &value, nil
}
