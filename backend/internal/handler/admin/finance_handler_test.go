//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFinanceHandlerGetUsageFinance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	financeService := service.NewFinanceDetailService(&financeHandlerRepositoryStub{facts: &service.FinanceUsageDetailFacts{
		Usage: &service.UsageLog{ID: 88, RequestID: "req", Model: "model-a", CreatedAt: time.Now()},
	}})
	handler := NewFinanceHandler(financeService, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/usage/:id/finance", handler.GetUsageFinance)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/88/finance", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"usage_log_id":88`)
	require.Contains(t, recorder.Body.String(), `"finance_calculation_pending"`)
}

func TestFinanceHandlerGetUsageFinanceInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewFinanceHandler(service.NewFinanceDetailService(&financeHandlerRepositoryStub{}), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/usage/:id/finance", handler.GetUsageFinance)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/bad/finance", nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

type financeHandlerRepositoryStub struct {
	facts *service.FinanceUsageDetailFacts
}

func (s *financeHandlerRepositoryStub) GetUsageFinanceDetailFacts(context.Context, int64) (*service.FinanceUsageDetailFacts, error) {
	return s.facts, nil
}
