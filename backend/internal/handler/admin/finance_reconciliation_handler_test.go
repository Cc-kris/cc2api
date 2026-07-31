//go:build unit

package admin

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type financeReconciliationHandlerRepo struct {
	input       service.FinanceReconciliationImportInput
	listRequest service.FinanceReconciliationListRequest
	statusID    int64
	status      string
	note        string
	actorID     int64
}

func (r *financeReconciliationHandlerRepo) ImportFinanceReconciliation(_ context.Context, input service.FinanceReconciliationImportInput, _ time.Time) (*service.FinanceReconciliationImportResult, error) {
	r.input = input
	return &service.FinanceReconciliationImportResult{Reconciliation: service.FinanceReconciliation{ID: 91}}, nil
}

func (r *financeReconciliationHandlerRepo) ListFinanceReconciliations(_ context.Context, request service.FinanceReconciliationListRequest) ([]service.FinanceReconciliation, int64, error) {
	r.listRequest = request
	return []service.FinanceReconciliation{{ID: 92, Status: service.FinanceReconciliationDifference}}, 1, nil
}

func (r *financeReconciliationHandlerRepo) UpdateFinanceReconciliationStatus(_ context.Context, id int64, status, note string, actorID int64, _ time.Time) (*service.FinanceReconciliation, error) {
	r.statusID = id
	r.status = status
	r.note = note
	r.actorID = actorID
	return &service.FinanceReconciliation{ID: id, Status: status, HandledNote: note}, nil
}

func TestFinanceReconciliationImportHandlerAcceptsMultipartCSV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &financeReconciliationHandlerRepo{}
	handler := NewFinanceHandler(nil, nil, nil, nil, nil, service.NewFinanceReconciliationService(repo), nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/finance/reconciliations/import", handler.ImportReconciliation)

	request := financeReconciliationMultipartRequest(t, "amount\n3.25\n6.75\n")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"id":91`)
	require.Equal(t, "10", repo.input.UpstreamBillAmount.String())
	require.Equal(t, int64(7), repo.input.WalletID)
}

func TestFinanceReconciliationImportHandlerReturnsErrorCSVWithoutWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &financeReconciliationHandlerRepo{}
	handler := NewFinanceHandler(nil, nil, nil, nil, nil, service.NewFinanceReconciliationService(repo), nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/finance/reconciliations/import", handler.ImportReconciliation)

	request := financeReconciliationMultipartRequest(t, "amount\ninvalid\n")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/csv")
	require.Contains(t, recorder.Header().Get("Content-Disposition"), "reconciliation-import-errors.csv")
	require.Contains(t, recorder.Body.String(), "upstream_bill_amount")
	require.Zero(t, repo.input.WalletID)
}

func TestFinanceReconciliationListHandlerParsesFiltersAndInclusiveEndDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &financeReconciliationHandlerRepo{}
	handler := NewFinanceHandler(nil, nil, nil, nil, nil, service.NewFinanceReconciliationService(repo), nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/finance/reconciliations", handler.Reconciliations)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/finance/reconciliations?start_date=2026-07-01&end_date=2026-07-31&upstream_id=3&wallet_id=7&status=difference&page=2&page_size=10", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"id":92`)
	require.Equal(t, 2, repo.listRequest.Page)
	require.Equal(t, 10, repo.listRequest.PageSize)
	require.Equal(t, int64(3), *repo.listRequest.UpstreamID)
	require.Equal(t, int64(7), *repo.listRequest.WalletID)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), *repo.listRequest.EndBefore)
}

func TestFinanceReconciliationUpdateHandlerRecordsActorAndNote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &financeReconciliationHandlerRepo{}
	handler := NewFinanceHandler(nil, nil, nil, nil, nil, service.NewFinanceReconciliationService(repo), nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 81})
		c.Next()
	})
	router.PUT("/api/v1/admin/finance/reconciliations/:id", handler.UpdateReconciliation)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/finance/reconciliations/92", strings.NewReader(`{"status":"confirmed","note":"verified"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(92), repo.statusID)
	require.Equal(t, service.FinanceReconciliationConfirmed, repo.status)
	require.Equal(t, "verified", repo.note)
	require.Equal(t, int64(81), repo.actorID)
}

func financeReconciliationMultipartRequest(t *testing.T, csvContent string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("wallet_id", "7"))
	require.NoError(t, writer.WriteField("period_start", "2026-07-01T00:00:00Z"))
	require.NoError(t, writer.WriteField("period_end", "2026-08-01T00:00:00Z"))
	require.NoError(t, writer.WriteField("currency", "USD"))
	require.NoError(t, writer.WriteField("source_reference", "bill-202607"))
	part, err := writer.CreateFormFile("file", "bill.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte(csvContent))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/finance/reconciliations/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
