package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetIncidentOverview returns the vNext ops incident overview.
// GET /api/v1/admin/ops/incidents/overview
func (h *OpsHandler) GetIncidentOverview(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}

	startTime, endTime, err := parseOpsIncidentTimeRange(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	platform := strings.TrimSpace(strings.ToLower(c.Query("platform")))
	if platform != "" && platform != "openai" && platform != "claude" && platform != "gemini" {
		response.BadRequest(c, "Invalid platform")
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  platform,
		Model:     strings.TrimSpace(c.Query("model")),
		QueryMode: parseOpsQueryMode(c),
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	data, err := h.opsService.GetIncidentOverview(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

func parseOpsIncidentTimeRange(c *gin.Context) (time.Time, time.Time, error) {
	rangeValue := strings.TrimSpace(c.Query("time_range"))
	if rangeValue == "" {
		rangeValue = "1m"
	}
	if rangeValue == "custom" {
		startStr := strings.TrimSpace(c.Query("start_time"))
		endStr := strings.TrimSpace(c.Query("end_time"))
		if startStr == "" || endStr == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("custom time_range requires start_time and end_time")
		}
		start, err := parseOpsIncidentTimestamp(startStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end, err := parseOpsIncidentTimestamp(endStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if !end.After(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid time range: end_time must be after start_time")
		}
		if end.Sub(start) > 30*24*time.Hour {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid time range: max window is 30 days")
		}
		return start, end, nil
	}

	dur, ok := parseOpsIncidentDuration(rangeValue)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid time_range")
	}
	end := time.Now()
	return end.Add(-dur), end, nil
}

func parseOpsIncidentTimestamp(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, v)
}

func parseOpsIncidentDuration(v string) (time.Duration, bool) {
	switch strings.TrimSpace(v) {
	case "1m":
		return time.Minute, true
	case "5m":
		return 5 * time.Minute, true
	case "30m":
		return 30 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	default:
		return 0, false
	}
}
