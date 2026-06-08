package admin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cacheStatsHandlerRepoStub struct {
	filter *service.CacheStatsFilter
	rows   []*service.CacheStatsRawRow
}

func (r *cacheStatsHandlerRepoStub) ListCacheStatsRows(_ context.Context, filter *service.CacheStatsFilter) ([]*service.CacheStatsRawRow, error) {
	r.filter = filter
	return r.rows, nil
}

func TestCacheStatsHandlerGetStatsReturnsContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &cacheStatsHandlerRepoStub{rows: []*service.CacheStatsRawRow{{
		Platform: "openai", Model: "gpt-5.5", TotalRequests: 4, CandidateRequests: 3, HitRequests: 2, BypassRequests: 1,
		InputTokens: 100, OutputTokens: 20, HitTokens: 60, CandidateTokens: 120, AllRequestTokens: 120,
		BypassReasonsJSON: `{"disabled":1}`, EstimatedSavedAmount: "2.00000000",
	}}}
	handler := NewCacheStatsHandler(service.NewCacheStatsService(repo))
	start := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	url := "/api/v1/admin/cache/stats?start_time=" + start.Format(time.RFC3339) + "&end_time=" + end.Format(time.RFC3339) + "&platform=claude&model=gpt-5.5&api_key_id=12&group_id=34"
	c.Request = httptest.NewRequest(http.MethodGet, url, nil)

	handler.GetStats(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "claude", repo.filter.Platform)
	require.Equal(t, "gpt-5.5", repo.filter.Model)
	require.Equal(t, int64(12), *repo.filter.APIKeyID)
	require.Equal(t, int64(34), *repo.filter.GroupID)
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data := body.Data.(map[string]any)
	require.Contains(t, data, "summary")
	require.Contains(t, data, "model_rows")
	require.Contains(t, data, "bypass_reasons")
	require.Contains(t, data, "store_skip_reasons")
	summary := data["summary"].(map[string]any)
	require.Equal(t, float64(4), summary["total_requests"])
	require.Equal(t, 66.67, summary["request_hit_rate"])
	require.Equal(t, 50.0, summary["tokens_hit_rate"])
}

func TestCacheStatsHandlerRejectsInvalidRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewCacheStatsHandler(service.NewCacheStatsService(&cacheStatsHandlerRepoStub{}))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/stats?time_range=32d", nil)

	handler.GetStats(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCacheStatsHandlerExportRejectsInvalidRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewCacheStatsHandler(service.NewCacheStatsService(&cacheStatsHandlerRepoStub{}))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/stats/export?time_range=32d", nil)

	handler.Export(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCacheStatsHandlerExportReturnsCSV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &cacheStatsHandlerRepoStub{rows: []*service.CacheStatsRawRow{{
		Platform: "openai", Model: "gpt-5.5", TotalRequests: 2, CandidateRequests: 2, HitRequests: 1,
		InputTokens: 100, OutputTokens: 10, HitTokens: 50, CandidateTokens: 100,
	}}}
	handler := NewCacheStatsHandler(service.NewCacheStatsService(repo))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/stats/export?time_range=1d&platform=openai", nil)

	handler.Export(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/csv")
	require.Contains(t, rec.Header().Get("Content-Disposition"), "cache-stats.csv")
	require.Equal(t, "openai", repo.filter.Platform)
	records, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	require.NoError(t, err)
	require.Equal(t, "平台", records[0][0])
	require.Equal(t, []string{"openai", "gpt-5.5", "2", "2", "1", "1", "0", "100", "10", "50", "50.00%", "50.00%", ""}, records[1])
}

func TestCacheStatsHandlerExportUsesFilteredRowsAndDoesNotLeakSensitiveScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &cacheStatsHandlerRepoStub{rows: []*service.CacheStatsRawRow{{
		Platform: "anthropic", Model: "claude-sonnet-4-5", TotalRequests: 10, CandidateRequests: 10, HitRequests: 9,
		InputTokens: 1000, OutputTokens: 500, HitTokens: 1350, CandidateTokens: 1500,
		BypassReasonsJSON: `{"authorization: Bearer secret-token":1}`,
	}}}
	handler := NewCacheStatsHandler(service.NewCacheStatsService(repo))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/stats/export?time_range=1d&platform=claude&api_key_id=123&group_id=456", nil)

	handler.Export(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "claude", repo.filter.Platform)
	require.Equal(t, int64(123), *repo.filter.APIKeyID)
	require.Equal(t, int64(456), *repo.filter.GroupID)
	require.NotContains(t, rec.Body.String(), "secret-token")
	require.NotContains(t, rec.Body.String(), "api_key_id")
	require.NotContains(t, rec.Body.String(), "group_id")
}

func TestCacheStatsHandlerExportRejectsEmptyRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewCacheStatsHandler(service.NewCacheStatsService(&cacheStatsHandlerRepoStub{}))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/stats/export?time_range=1d", nil)

	handler.Export(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, rec.Header().Get("Content-Disposition"))
}
