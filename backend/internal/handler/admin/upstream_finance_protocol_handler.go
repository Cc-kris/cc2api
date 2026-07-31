package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UpstreamFinanceProtocolHandler struct {
	service  *service.UpstreamFinanceProtocolService
	accounts *service.AccountService
}

func NewUpstreamFinanceProtocolHandler(protocolService *service.UpstreamFinanceProtocolService, accounts *service.AccountService) *UpstreamFinanceProtocolHandler {
	return &UpstreamFinanceProtocolHandler{service: protocolService, accounts: accounts}
}

func (h *UpstreamFinanceProtocolHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.List(c.Request.Context(), service.FinanceProtocolListFilter{Status: c.Query("status"), ProtocolType: c.Query("protocol_type"), Page: page, PageSize: pageSize})
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}
func (h *UpstreamFinanceProtocolHandler) Create(c *gin.Context) {
	var input service.FinanceProtocolCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.OperatorID = financeProtocolOperator(c)
	item, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Created(c, item)
}
func (h *UpstreamFinanceProtocolHandler) Get(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, item)
}
func (h *UpstreamFinanceProtocolHandler) UpdateDraft(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	var input service.FinanceProtocolDraftInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	input.OperatorID = financeProtocolOperator(c)
	item, err := h.service.UpdateDraft(c.Request.Context(), id, input)
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, item)
}
func (h *UpstreamFinanceProtocolHandler) Test(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	var input service.FinanceProtocolTestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	result, err := h.service.Test(c.Request.Context(), id, input)
	if err != nil {
		if result != nil {
			response.Error(c, http.StatusUnprocessableEntity, result.ErrorSummary)
			return
		}
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, result)
}
func (h *UpstreamFinanceProtocolHandler) Publish(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	var input struct {
		VersionID int64 `json:"version_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.VersionID <= 0 {
		response.BadRequest(c, "version_id is required")
		return
	}
	if err := h.service.Publish(c.Request.Context(), id, input.VersionID, financeProtocolOperator(c)); err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, gin.H{"published": true, "version_id": input.VersionID})
}
func (h *UpstreamFinanceProtocolHandler) Disable(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	if err := h.service.Disable(c.Request.Context(), id, financeProtocolOperator(c)); err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, gin.H{"disabled": true})
}
func (h *UpstreamFinanceProtocolHandler) Copy(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	var input service.FinanceProtocolCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	input.OperatorID = financeProtocolOperator(c)
	item, err := h.service.Copy(c.Request.Context(), id, input)
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Created(c, item)
}
func (h *UpstreamFinanceProtocolHandler) DeleteDraft(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteDraft(c.Request.Context(), id); err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
func (h *UpstreamFinanceProtocolHandler) Versions(c *gin.Context) {
	id, ok := parseFinanceProtocolID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.Versions(c.Request.Context(), id)
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *UpstreamFinanceProtocolHandler) DetectAccount(c *gin.Context) {
	accountID, ok := parseFinanceProtocolID(c, "account_id")
	if !ok {
		return
	}
	if h.accounts == nil {
		response.Error(c, http.StatusServiceUnavailable, "account finance detection is unavailable")
		return
	}
	account, err := h.accounts.GetByID(c.Request.Context(), accountID)
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	result, err := h.service.DetectAccount(c.Request.Context(), account, financeProtocolOperator(c))
	if err != nil {
		writeFinanceProtocolError(c, err)
		return
	}
	response.Success(c, result)
}

func parseFinanceProtocolID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, name+" must be a positive integer")
		return 0, false
	}
	return id, true
}
func financeProtocolOperator(c *gin.Context) *int64 {
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		id := subject.UserID
		return &id
	}
	return nil
}
func writeFinanceProtocolError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUpstreamFinanceProtocolNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrUpstreamFinanceProtocolConflict), errors.Is(err, service.ErrUpstreamFinanceProtocolInvalidState):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrUpstreamFinanceProtocolUnsafe), service.IsFinanceValidationError(err):
		response.BadRequest(c, err.Error())
	default:
		response.ErrorFrom(c, err)
	}
}
