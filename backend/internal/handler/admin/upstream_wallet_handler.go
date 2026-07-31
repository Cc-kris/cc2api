package admin

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UpstreamWalletHandler struct {
	walletService *service.UpstreamWalletService
	syncService   *service.UpstreamFinanceSyncService
	fundService   *service.UpstreamFundService
}

func NewUpstreamWalletHandler(walletService *service.UpstreamWalletService, syncService *service.UpstreamFinanceSyncService, fundService *service.UpstreamFundService) *UpstreamWalletHandler {
	return &UpstreamWalletHandler{walletService: walletService, syncService: syncService, fundService: fundService}
}

func (h *UpstreamWalletHandler) List(c *gin.Context) {
	upstreamID, ok := parseWalletID(c, "upstream_id")
	if !ok {
		return
	}
	includeDeleted, err := strconv.ParseBool(defaultString(c.Query("include_deleted"), "false"))
	if err != nil {
		response.BadRequest(c, "include_deleted must be true or false")
		return
	}
	items, err := h.walletService.List(c.Request.Context(), upstreamID, includeDeleted)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *UpstreamWalletHandler) Create(c *gin.Context) {
	upstreamID, ok := parseWalletID(c, "upstream_id")
	if !ok {
		return
	}
	var input service.UpstreamWalletInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	wallet, err := h.walletService.Create(c.Request.Context(), upstreamID, input)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Created(c, wallet)
}

func (h *UpstreamWalletHandler) Update(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	var input service.UpstreamWalletInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	wallet, err := h.walletService.Update(c.Request.Context(), walletID, input)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Success(c, wallet)
}

func (h *UpstreamWalletHandler) Delete(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	if err := h.walletService.Delete(c.Request.Context(), walletID); err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *UpstreamWalletHandler) AssignAccounts(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	var input service.UpstreamWalletAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if subject, exists := middleware2.GetAuthSubjectFromContext(c); exists && subject.UserID > 0 {
		input.OperatorID = &subject.UserID
	}
	if err := h.walletService.AssignAccounts(c.Request.Context(), walletID, input); err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Success(c, gin.H{"assigned": len(input.AccountIDs)})
}

func (h *UpstreamWalletHandler) Probe(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	probe, err := h.syncService.Probe(c.Request.Context(), walletID)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Success(c, probe)
}

func (h *UpstreamWalletHandler) SyncPricing(c *gin.Context) {
	h.enqueueSync(c, service.UpstreamFinanceSyncPricing)
}
func (h *UpstreamWalletHandler) SyncBalance(c *gin.Context) {
	h.enqueueSync(c, service.UpstreamFinanceSyncBalance)
}
func (h *UpstreamWalletHandler) SyncQuota(c *gin.Context) {
	h.enqueueSync(c, service.UpstreamFinanceSyncQuota)
}

func (h *UpstreamWalletHandler) SyncFunding(c *gin.Context) {
	h.enqueueSync(c, service.UpstreamFinanceSyncFunding)
}

func (h *UpstreamWalletHandler) SyncAccountUsage(c *gin.Context) {
	h.enqueueSync(c, service.UpstreamFinanceSyncAccountUsage)
}

func (h *UpstreamWalletHandler) enqueueSync(c *gin.Context, syncType string) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	var operatorID *int64
	if subject, exists := middleware2.GetAuthSubjectFromContext(c); exists && subject.UserID > 0 {
		operatorID = &subject.UserID
	}
	job, created, err := h.syncService.Enqueue(c.Request.Context(), walletID, syncType, operatorID)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Accepted(c, gin.H{"job": job, "created": created})
}

func (h *UpstreamWalletHandler) ListPrices(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	filter := service.UpstreamFinancePriceListFilter{Model: c.Query("model"), Page: page, PageSize: pageSize}
	if raw := c.Query("effective_at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "effective_at must be RFC3339")
			return
		}
		filter.EffectiveAt = &parsed
	}
	items, total, err := h.syncService.ListPrices(c.Request.Context(), walletID, filter)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

type upstreamPriceImportRequest struct {
	EffectiveAt time.Time                      `json:"effective_at"`
	Prices      []service.UpstreamFinancePrice `json:"prices"`
}

func (h *UpstreamWalletHandler) ImportPrices(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	var request upstreamPriceImportRequest
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "text/csv") || strings.HasPrefix(contentType, "application/csv") {
		prices, err := service.ParseUpstreamPriceCSV(io.LimitReader(c.Request.Body, 5<<20))
		if err != nil {
			var validationErr *service.UpstreamPriceCSVValidationError
			if errors.As(err, &validationErr) {
				writeUpstreamPriceCSVErrors(c, validationErr.Rows)
				return
			}
			response.BadRequest(c, err.Error())
			return
		}
		request.Prices = prices
	} else if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	created, skipped, err := h.syncService.ImportPrices(c.Request.Context(), walletID, request.Prices, request.EffectiveAt)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Success(c, gin.H{"created_count": created, "skipped_count": skipped})
}

func writeUpstreamPriceCSVErrors(c *gin.Context, rows []service.UpstreamPriceCSVError) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="upstream-price-import-errors.csv"`)
	c.Status(http.StatusBadRequest)
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"row", "field", "message"})
	for _, row := range rows {
		_ = writer.Write([]string{strconv.Itoa(row.Row), row.Field, row.Message})
	}
	writer.Flush()
}

func (h *UpstreamWalletHandler) SyncHistory(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.syncService.ListHistory(c.Request.Context(), walletID, service.UpstreamFinanceSyncHistoryFilter{
		SyncType: c.Query("sync_type"), Status: c.Query("status"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *UpstreamWalletHandler) CreateFundEvent(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	var input service.UpstreamFundEventInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	input.IdempotencyKey = c.GetHeader("Idempotency-Key")
	if subject, exists := middleware2.GetAuthSubjectFromContext(c); exists && subject.UserID > 0 {
		input.OperatorID = &subject.UserID
	}
	event, created, err := h.fundService.Create(c.Request.Context(), walletID, input)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	if created {
		response.Created(c, event)
		return
	}
	response.Success(c, event)
}

func (h *UpstreamWalletHandler) ListFundEvents(c *gin.Context) {
	walletID, ok := parseWalletID(c, "id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.fundService.List(c.Request.Context(), walletID, page, pageSize)
	if err != nil {
		writeUpstreamWalletError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func parseWalletID(c *gin.Context, key string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid "+key)
		return 0, false
	}
	return id, true
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func writeUpstreamWalletError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUpstreamWalletNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrUpstreamFundEventNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrUpstreamWalletAssignmentConflict), errors.Is(err, service.ErrUpstreamWalletAssignmentTooEarly), errors.Is(err, service.ErrUpstreamWalletAccountMismatch):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrUpstreamFundIdempotencyConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrUpstreamFundDuplicateReference):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrUpstreamWalletDisabled):
		response.Error(c, http.StatusConflict, err.Error())
	case service.IsFinanceValidationError(err):
		response.BadRequest(c, err.Error())
	default:
		response.ErrorFrom(c, err)
	}
}
