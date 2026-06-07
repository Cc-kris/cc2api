package admin

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseOpsIncidentTimeRangeAllowedDurations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tr := range []string{"1m", "5m", "30m", "1h", "6h", "24h"} {
		t.Run(tr, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?time_range="+tr, nil)
			start, end, err := parseOpsIncidentTimeRange(c)
			require.NoError(t, err)
			require.True(t, end.After(start))
		})
	}
}

func TestParseOpsIncidentTimeRangeRejectsUnsupportedDurations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tr := range []string{"7d", "30d", "2m", "bad"} {
		t.Run(tr, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?time_range="+tr, nil)
			_, _, err := parseOpsIncidentTimeRange(c)
			require.Error(t, err)
		})
	}
}

func TestParseOpsIncidentTimeRangeCustomRequiresEndAfterStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	start := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?time_range=custom&start_time="+start+"&end_time="+start, nil)
	_, _, err := parseOpsIncidentTimeRange(c)
	require.Error(t, err)
	require.Contains(t, err.Error(), "end_time must be after start_time")
}

func TestParseOpsIncidentTimeRangeCustomRequiresBothBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?time_range=custom&start_time=2026-06-07T12:00:00Z", nil)
	_, _, err := parseOpsIncidentTimeRange(c)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires start_time and end_time")
}
