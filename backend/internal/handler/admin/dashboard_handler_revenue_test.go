package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardRevenueRepoStub struct {
	service.UsageLogRepository
	overview     *service.DashboardRevenueOverview
	distribution *service.DashboardRepurchaseDistribution
}

func (s *dashboardRevenueRepoStub) GetDashboardRevenueOverview(context.Context) (*service.DashboardRevenueOverview, error) {
	return s.overview, nil
}

func (s *dashboardRevenueRepoStub) GetDashboardRepurchaseDistribution(context.Context) (*service.DashboardRepurchaseDistribution, error) {
	return s.distribution, nil
}

func newDashboardRevenueTestRouter(repo *dashboardRevenueRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/revenue-overview", handler.GetRevenueOverview)
	router.GET("/admin/dashboard/repurchase-distribution", handler.GetRepurchaseDistribution)
	return router
}

func TestDashboardHandler_GetRevenueOverview(t *testing.T) {
	router := newDashboardRevenueTestRouter(&dashboardRevenueRepoStub{
		overview: &service.DashboardRevenueOverview{
			TotalCreditAmount: "125.50",
			UsedAmount:        "45.25",
			UnusedAmount:      "80.25",
			NonAdminUserCount: 4,
			CreditedUserCount: 2,
			IsEstimated:       true,
			UpdatedAt:         "2026-06-07T14:00:00Z",
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/revenue-overview", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	require.Equal(t, "125.50", data["total_credit_amount"])
	require.Equal(t, true, data["is_estimated"])
	require.Equal(t, float64(4), data["non_admin_user_count"])
}

func TestDashboardHandler_GetRepurchaseDistribution(t *testing.T) {
	router := newDashboardRevenueTestRouter(&dashboardRevenueRepoStub{
		distribution: &service.DashboardRepurchaseDistribution{
			Buckets: []service.DashboardRepurchaseBucket{
				{Bucket: "zero", Label: "零购", UserCount: 4, Ratio: 40},
				{Bucket: "one", Label: "一购", UserCount: 3, Ratio: 30},
			},
			UpdatedAt: "2026-06-07T14:00:00Z",
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/repurchase-distribution", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	buckets := data["buckets"].([]any)
	require.Len(t, buckets, 2)
	first := buckets[0].(map[string]any)
	require.Equal(t, "zero", first["bucket"])
	require.Equal(t, "零购", first["label"])
	require.Equal(t, float64(4), first["user_count"])
	require.Equal(t, float64(40), first["ratio"])
}
