//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type promotionCreditReconciliationHandlerRepo struct {
	listRequest service.PromotionCreditReconciliationListRequest
	userID      int64
	amount      decimal.Decimal
	note        string
	operatorID  int64
}

func (r *promotionCreditReconciliationHandlerRepo) ListPromotionCreditReconciliations(_ context.Context, request service.PromotionCreditReconciliationListRequest) ([]service.PromotionCreditReconciliation, int64, error) {
	r.listRequest = request
	return []service.PromotionCreditReconciliation{{UserID: 8, Status: service.PromotionCreditReconciliationRequired}}, 1, nil
}

func (r *promotionCreditReconciliationHandlerRepo) ResolvePromotionCreditReconciliation(_ context.Context, userID int64, amount decimal.Decimal, note string, operatorID int64, _ time.Time) (*service.PromotionCreditReconciliation, error) {
	r.userID, r.amount, r.note, r.operatorID = userID, amount, note, operatorID
	value := amount.StringFixed(10)
	return &service.PromotionCreditReconciliation{UserID: userID, Status: service.PromotionCreditReconciliationResolved, ConfirmedRemainingAmount: &value}, nil
}

func TestPromotionCreditReconciliationHandlersListAndResolve(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &promotionCreditReconciliationHandlerRepo{}
	promotionService := service.NewPromotionCreditReconciliationService(repo)
	handler := NewFinanceHandler(nil, nil, nil, nil, nil, nil, nil, promotionService, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 81})
		c.Next()
	})
	router.GET("/api/v1/admin/finance/promotion-credit-reconciliations", handler.PromotionCreditReconciliations)
	router.POST("/api/v1/admin/finance/promotion-credit-reconciliations/:user_id/resolve", handler.ResolvePromotionCreditReconciliation)

	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/finance/promotion-credit-reconciliations?status=requires_reconciliation&page=2&page_size=10", nil))
	require.Equal(t, http.StatusOK, listRecorder.Code)
	require.Contains(t, listRecorder.Body.String(), `"user_id":8`)
	require.Equal(t, 2, repo.listRequest.Page)
	require.Equal(t, 10, repo.listRequest.PageSize)

	resolveRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/finance/promotion-credit-reconciliations/8/resolve", strings.NewReader(`{"confirmed_remaining_amount":"0","note":"客户已使用完毕"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resolveRecorder, request)
	require.Equal(t, http.StatusOK, resolveRecorder.Code)
	require.Equal(t, int64(8), repo.userID)
	require.True(t, repo.amount.IsZero())
	require.Equal(t, "客户已使用完毕", repo.note)
	require.Equal(t, int64(81), repo.operatorID)
}
