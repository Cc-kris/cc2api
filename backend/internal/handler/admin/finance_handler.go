package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type FinanceHandler struct {
	detailService         *service.FinanceDetailService
	reportService         *service.FinanceReportService
	alertService          *service.FinanceAlertService
	feeService            *service.FinancePaymentFeeService
	backfillService       *service.FinanceBackfillService
	reconciliationService *service.FinanceReconciliationService
	promotionService      *service.PromotionCreditReconciliationService
	exportService         *service.FinanceExportService
	settlementService     *service.AccountFinanceSettlementService
	accountProfileService *service.AccountFinanceProfileService
	initializationService *service.FinanceInitializationService
	fxRateService         *service.FinanceFXRateService
}

func NewFinanceHandler(financeService *service.FinanceDetailService, reportService *service.FinanceReportService, alertService *service.FinanceAlertService, feeService *service.FinancePaymentFeeService, backfillService *service.FinanceBackfillService, reconciliationService *service.FinanceReconciliationService, exportService *service.FinanceExportService, promotionService *service.PromotionCreditReconciliationService, settlementService *service.AccountFinanceSettlementService, accountProfileService *service.AccountFinanceProfileService, fxRateServices ...*service.FinanceFXRateService) *FinanceHandler {
	var fxRateService *service.FinanceFXRateService
	if len(fxRateServices) > 0 {
		fxRateService = fxRateServices[0]
	}
	return newFinanceHandler(financeService, reportService, alertService, feeService, backfillService, reconciliationService, exportService, promotionService, settlementService, accountProfileService, nil, fxRateService)
}

func NewFinanceHandlerWithInitialization(financeService *service.FinanceDetailService, reportService *service.FinanceReportService, alertService *service.FinanceAlertService, feeService *service.FinancePaymentFeeService, backfillService *service.FinanceBackfillService, reconciliationService *service.FinanceReconciliationService, exportService *service.FinanceExportService, promotionService *service.PromotionCreditReconciliationService, settlementService *service.AccountFinanceSettlementService, accountProfileService *service.AccountFinanceProfileService, initializationService *service.FinanceInitializationService, fxRateService *service.FinanceFXRateService) *FinanceHandler {
	return newFinanceHandler(financeService, reportService, alertService, feeService, backfillService, reconciliationService, exportService, promotionService, settlementService, accountProfileService, initializationService, fxRateService)
}

func newFinanceHandler(financeService *service.FinanceDetailService, reportService *service.FinanceReportService, alertService *service.FinanceAlertService, feeService *service.FinancePaymentFeeService, backfillService *service.FinanceBackfillService, reconciliationService *service.FinanceReconciliationService, exportService *service.FinanceExportService, promotionService *service.PromotionCreditReconciliationService, settlementService *service.AccountFinanceSettlementService, accountProfileService *service.AccountFinanceProfileService, initializationService *service.FinanceInitializationService, fxRateService *service.FinanceFXRateService) *FinanceHandler {
	return &FinanceHandler{detailService: financeService, reportService: reportService, alertService: alertService, feeService: feeService, backfillService: backfillService, reconciliationService: reconciliationService, exportService: exportService, promotionService: promotionService, settlementService: settlementService, accountProfileService: accountProfileService, initializationService: initializationService, fxRateService: fxRateService}
}

func respondFinanceServiceError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrFinanceInitializationInvalid) {
		response.BadRequest(c, err.Error())
		return
	}
	if errors.Is(err, service.ErrAccountFinanceProfileNotFound) {
		response.NotFound(c, "Account finance profile not found")
		return
	}
	if errors.Is(err, service.ErrAccountFinanceProfileConflict) {
		response.ErrorWithDetails(c, http.StatusConflict, err.Error(), "ACCOUNT_FINANCE_PROFILE_CONFLICT", nil)
		return
	}
	if errors.Is(err, service.ErrAccountFinanceProfileInvalid) {
		response.ErrorWithDetails(c, http.StatusBadRequest, err.Error(), "ACCOUNT_FINANCE_PROFILE_INVALID", nil)
		return
	}
	for _, item := range []struct {
		code   string
		status int
	}{
		{code: "IDEMPOTENCY_KEY_REUSED", status: http.StatusConflict},
		{code: "EXPORT_NOT_DOWNLOADABLE", status: http.StatusConflict},
		{code: "DOWNLOAD_TOKEN_INVALID", status: http.StatusForbidden},
	} {
		if service.IsFinanceExportError(err, item.code) {
			response.ErrorWithDetails(c, item.status, err.Error(), item.code, nil)
			return
		}
	}
	for _, item := range []struct {
		code   string
		status int
	}{
		{code: "SETTLEMENT_NOT_FOUND", status: http.StatusNotFound},
		{code: "SETTLEMENT_STATE_CONFLICT", status: http.StatusConflict},
		{code: "SETTLEMENT_INVALID", status: http.StatusBadRequest},
	} {
		if service.IsFinanceSettlementError(err, item.code) {
			response.ErrorWithDetails(c, item.status, err.Error(), item.code, nil)
			return
		}
	}
	if service.IsFinanceExportError(err, "JOB_NOT_FOUND") {
		response.ErrorWithDetails(c, http.StatusNotFound, err.Error(), "JOB_NOT_FOUND", nil)
		return
	}
	for _, item := range []struct {
		code   string
		status int
	}{
		{code: "JOB_NOT_FOUND", status: http.StatusNotFound},
		{code: "JOB_STATE_CONFLICT", status: http.StatusConflict},
		{code: "BACKFILL_PRECONDITION_FAILED", status: http.StatusConflict},
	} {
		if service.IsFinanceBackfillError(err, item.code) {
			response.ErrorWithDetails(c, item.status, err.Error(), item.code, nil)
			return
		}
	}
	if service.IsFinanceValidationError(err) {
		response.BadRequest(c, err.Error())
		return
	}
	response.ErrorFrom(c, err)
}

func (h *FinanceHandler) AccountProfile(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account id")
		return
	}
	profile, err := h.accountProfileService.Get(c.Request.Context(), accountID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *FinanceHandler) UpdateAccountProfile(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account id")
		return
	}
	var input service.AccountFinanceProfileInput
	if err = c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	input.OperatorID = subject.UserID
	profile, err := h.accountProfileService.Save(c.Request.Context(), accountID, input)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *FinanceHandler) AccountReadiness(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account id")
		return
	}
	readiness, err := h.accountProfileService.Readiness(c.Request.Context(), accountID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, readiness)
}

func (h *FinanceHandler) Settlements(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	accountID, err := optionalPositiveInt64(c.Query("account_id"))
	if err != nil {
		response.BadRequest(c, "Invalid account_id")
		return
	}
	items, total, err := h.settlementService.List(c.Request.Context(), service.FinanceSettlementListFilter{
		Status: c.Query("status"), AccountID: accountID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) SettlementDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid settlement interval id")
		return
	}
	item, err := h.settlementService.Detail(c.Request.Context(), id)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *FinanceHandler) RetrySettlement(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid settlement interval id")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	item, err := h.settlementService.Retry(c.Request.Context(), id, subject.UserID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *FinanceHandler) ReallocateSettlement(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid settlement interval id")
		return
	}
	var request struct {
		ExpectedRevision int    `json:"expected_revision" binding:"required"`
		Reason           string `json:"reason" binding:"required"`
	}
	if err = c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request payload")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	item, err := h.settlementService.Reallocate(c.Request.Context(), id, request.ExpectedRevision, request.Reason, subject.UserID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *FinanceHandler) PromotionCreditReconciliations(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.promotionService.List(c.Request.Context(), service.PromotionCreditReconciliationListRequest{Status: c.Query("status"), Page: page, PageSize: pageSize})
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *FinanceHandler) ResolvePromotionCreditReconciliation(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	var request service.ResolvePromotionCreditReconciliationRequest
	if err = c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request payload")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	item, err := h.promotionService.Resolve(c.Request.Context(), userID, request, subject.UserID)
	if errors.Is(err, service.ErrPromotionCreditReconciliationNotFound) {
		response.ErrorWithDetails(c, http.StatusNotFound, err.Error(), "RECONCILIATION_NOT_FOUND", nil)
		return
	}
	if errors.Is(err, service.ErrPromotionCreditReconciliationResolved) {
		response.ErrorWithDetails(c, http.StatusConflict, err.Error(), "RECONCILIATION_ALREADY_RESOLVED", nil)
		return
	}
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *FinanceHandler) ExportCreate(c *gin.Context) {
	var request service.FinanceExportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request payload")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	job, err := h.exportService.Create(c.Request.Context(), request, subject.UserID, c.GetHeader("Idempotency-Key"))
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Accepted(c, job)
}

func (h *FinanceHandler) ExportGet(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("job_id"), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "Invalid finance export job id")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	job, token, err := h.exportService.Get(c.Request.Context(), jobID, subject.UserID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	if token != "" {
		job.DownloadURL = fmt.Sprintf("/api/v1/admin/finance/exports/%d/download?token=%s", job.ID, url.QueryEscape(token))
	}
	response.Success(c, job)
}

func (h *FinanceHandler) ExportDownload(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("job_id"), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "Invalid finance export job id")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	download, err := h.exportService.Download(c.Request.Context(), jobID, subject.UserID, c.Query("token"))
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.FileAttachment(download.Path, download.Filename)
}

func (h *FinanceHandler) BackfillPreview(c *gin.Context) {
	var request service.FinanceBackfillRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request payload")
		return
	}
	preview, err := h.backfillService.Preview(c.Request.Context(), request)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, preview)
}

func (h *FinanceHandler) BackfillRun(c *gin.Context) {
	var request service.FinanceBackfillRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request payload")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	job, err := h.backfillService.Run(c.Request.Context(), request, subject.UserID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Accepted(c, job)
}

func (h *FinanceHandler) BackfillGet(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("job_id"), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "Invalid finance backfill job id")
		return
	}
	job, err := h.backfillService.Get(c.Request.Context(), jobID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, job)
}

func (h *FinanceHandler) BackfillPause(c *gin.Context) {
	h.backfillTransition(c, h.backfillService.Pause)
}

func (h *FinanceHandler) BackfillResume(c *gin.Context) {
	h.backfillTransition(c, h.backfillService.Resume)
}

func (h *FinanceHandler) backfillTransition(c *gin.Context, transition func(context.Context, int64) (*service.FinanceBackfillJob, error)) {
	jobID, err := strconv.ParseInt(c.Param("job_id"), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "Invalid finance backfill job id")
		return
	}
	job, err := transition(c.Request.Context(), jobID)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, job)
}

// GetUsageFinance returns the immutable sales facts, current cost projection,
// profit and per-attempt cost segments for one usage record.
func (h *FinanceHandler) GetUsageFinance(c *gin.Context) {
	usageLogID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || usageLogID <= 0 {
		response.BadRequest(c, "Invalid usage log id")
		return
	}
	detail, err := h.detailService.GetUsageDetail(c.Request.Context(), usageLogID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

func (h *FinanceHandler) Overview(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	overview, err := h.reportService.Overview(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *FinanceHandler) Trend(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	items, err := h.reportService.Trend(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *FinanceHandler) Breakdown(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.reportService.Breakdown(c.Request.Context(), filter, service.FinanceBreakdownRequest{
		Dimension: c.Query("dimension"), SortBy: c.Query("sort_by"), SortOrder: c.Query("sort_order"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) Details(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.reportService.Details(c.Request.Context(), filter, service.FinanceDetailsRequest{
		ProfitDirection: c.Query("profit_direction"), RequestID: c.Query("request_id"),
		SortBy: c.Query("sort_by"), SortOrder: c.Query("sort_order"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) DetailByUsageID(c *gin.Context) { h.GetUsageFinance(c) }

func (h *FinanceHandler) Losses(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.reportService.Losses(c.Request.Context(), filter, service.FinanceDetailsRequest{
		RequestID: c.Query("request_id"), LossStatus: c.Query("status"), SortBy: c.Query("sort_by"), SortOrder: c.Query("sort_order"), Page: page, PageSize: pageSize,
	}, c.Query("loss_reason"))
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) Funds(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	result, err := h.reportService.Funds(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FinanceHandler) DataQuality(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	page, pageSize := response.ParsePagination(c)
	result, err := h.reportService.DataQuality(c.Request.Context(), filter, c.Query("issue_type"), page, pageSize)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FinanceHandler) InitializationScan(c *gin.Context) {
	if h.initializationService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "Finance initialization is unavailable", "FINANCE_INITIALIZATION_UNAVAILABLE", nil)
		return
	}
	result, err := h.initializationService.Scan(c.Request.Context())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FinanceHandler) InitializationApply(c *gin.Context) {
	if h.initializationService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "Finance initialization is unavailable", "FINANCE_INITIALIZATION_UNAVAILABLE", nil)
		return
	}
	var input service.FinanceInitializationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid finance initialization request")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Forbidden(c, "Admin identity is required")
		return
	}
	input.OperatorID = subject.UserID
	result, err := h.initializationService.Apply(c.Request.Context(), input)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FinanceHandler) CashFlow(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	page, pageSize := response.ParsePagination(c)
	result, err := h.reportService.CashFlow(c.Request.Context(), filter, service.FinanceCashFlowRequest{
		EventType: c.Query("event_type"),
		Currency:  c.Query("currency"),
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FinanceHandler) Alerts(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.alertService.List(c.Request.Context(), filter, service.FinanceAlertListRequest{
		AlertType: c.Query("alert_type"), Severity: c.Query("severity"), Status: c.Query("status"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) UpdateAlert(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid finance alert id")
		return
	}
	var request service.FinanceAlertStatusUpdate
	if err = c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	var actorID int64
	if subject, exists := middleware2.GetAuthSubjectFromContext(c); exists && subject.UserID > 0 {
		actorID = subject.UserID
	}
	item, err := h.alertService.UpdateStatus(c.Request.Context(), id, request, actorID)
	if errors.Is(err, service.ErrFinanceAlertNotFound) {
		response.NotFound(c, "Finance alert not found")
		return
	}
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *FinanceHandler) ImportPaymentFees(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 20<<20)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "CSV file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "CSV file cannot be opened")
		return
	}
	defer func() { _ = file.Close() }()
	result, err := h.feeService.ImportCSV(c.Request.Context(), c.PostForm("provider"), c.PostForm("currency"), file)
	var validationErr *service.FinanceCSVValidationError
	if errors.As(err, &validationErr) {
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="payment-fee-import-errors.csv"`)
		c.Data(http.StatusBadRequest, "text/csv; charset=utf-8", validationErr.CSV())
		return
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FinanceHandler) PaymentFees(c *gin.Context) {
	filter, err := service.ParseFinanceReportFilter(c.Request.URL.Query())
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.feeService.List(c.Request.Context(), filter, service.FinancePaymentFeeListRequest{
		OrderNo: c.Query("order_no"), Provider: c.Query("provider"), Status: c.Query("status"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) FXRates(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.fxRateService.List(c.Request.Context(), c.Query("currency"), page, pageSize)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) CreateFXRate(c *gin.Context) {
	var input service.FinanceFXRateCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	input.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		input.OperatorID = subject.UserID
	}
	item, err := h.fxRateService.Create(c.Request.Context(), input)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Created(c, item)
}

func (h *FinanceHandler) Reconciliations(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	request := service.FinanceReconciliationListRequest{Status: c.Query("status"), Page: page, PageSize: pageSize}
	var err error
	if request.StartAt, err = optionalFinanceReconciliationTime(c.Query("start_date")); err != nil {
		response.BadRequest(c, "start_date is invalid")
		return
	}
	if request.EndBefore, err = optionalFinanceReconciliationEndTime(c.Query("end_date")); err != nil {
		response.BadRequest(c, "end_date is invalid")
		return
	}
	if request.UpstreamID, err = optionalPositiveInt64(c.Query("upstream_id")); err != nil {
		response.BadRequest(c, "upstream_id is invalid")
		return
	}
	if request.WalletID, err = optionalPositiveInt64(c.Query("wallet_id")); err != nil {
		response.BadRequest(c, "wallet_id is invalid")
		return
	}
	items, total, err := h.reconciliationService.List(c.Request.Context(), request)
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *FinanceHandler) ImportReconciliation(c *gin.Context) {
	// Multipart boundaries and form fields add a small amount of overhead; the
	// service independently enforces the exact 20 MB file-content limit.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	walletID, err := strconv.ParseInt(strings.TrimSpace(c.PostForm("wallet_id")), 10, 64)
	if err != nil || walletID <= 0 {
		response.BadRequest(c, "wallet_id is invalid")
		return
	}
	periodStart, err := parseFinanceReconciliationTime(c.PostForm("period_start"))
	if err != nil {
		response.BadRequest(c, "period_start is invalid")
		return
	}
	periodEnd, err := parseFinanceReconciliationTime(c.PostForm("period_end"))
	if err != nil {
		response.BadRequest(c, "period_end is invalid")
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "CSV file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "CSV file cannot be opened")
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "CSV file cannot be read")
		return
	}
	var actorID int64
	if subject, exists := middleware2.GetAuthSubjectFromContext(c); exists && subject.UserID > 0 {
		actorID = subject.UserID
	}
	result, err := h.reconciliationService.ImportCSV(
		c.Request.Context(), walletID, periodStart, periodEnd, c.PostForm("currency"),
		c.PostForm("source_reference"), fileHeader.Filename, content, actorID,
	)
	var validationErr *service.FinanceCSVValidationError
	if errors.As(err, &validationErr) {
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="reconciliation-import-errors.csv"`)
		c.Data(http.StatusBadRequest, "text/csv; charset=utf-8", validationErr.CSV())
		return
	}
	if errors.Is(err, service.ErrUpstreamWalletNotFound) {
		response.NotFound(c, "Upstream wallet not found")
		return
	}
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FinanceHandler) UpdateReconciliation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid finance reconciliation id")
		return
	}
	var request service.FinanceReconciliationStatusUpdate
	if err = c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	var actorID int64
	if subject, exists := middleware2.GetAuthSubjectFromContext(c); exists && subject.UserID > 0 {
		actorID = subject.UserID
	}
	item, err := h.reconciliationService.UpdateStatus(c.Request.Context(), id, request, actorID)
	if errors.Is(err, service.ErrFinanceReconciliationNotFound) {
		response.NotFound(c, "Finance reconciliation not found")
		return
	}
	if err != nil {
		respondFinanceServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func parseFinanceReconciliationTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if value, err := time.Parse(time.RFC3339, raw); err == nil {
		return value, nil
	}
	return time.Parse("2006-01-02", raw)
}

func optionalFinanceReconciliationTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := parseFinanceReconciliationTime(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func optionalFinanceReconciliationEndTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := parseFinanceReconciliationTime(raw)
	if err != nil {
		return nil, err
	}
	if len(raw) == len("2006-01-02") {
		value = value.AddDate(0, 0, 1)
	}
	return &value, nil
}

func optionalPositiveInt64(raw string) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, errors.New("value must be a positive integer")
	}
	return &value, nil
}
