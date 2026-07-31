package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceProtocolHandlerCreateValidDraft(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &financeProtocolHandlerRepoStub{}
	handler := NewUpstreamFinanceProtocolHandler(service.NewUpstreamFinanceProtocolService(repo, service.NewUpstreamFinanceHTTPExecutor()), nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/upstream-finance-protocols", bytes.NewBufferString(`{
		"code":"vendor_usage","name":"Vendor usage","protocol_type":"http_json",
		"config":{"capabilities":["account_usage"],"authentication":{"type":"bearer","credential_source":"account_api_key"},"operations":{"account_usage":{"method":"GET","path":"/v1/usage","mapping":{"list_cost":"$.list_cost","actual_cost":"$.actual_cost"}}},"cost_mode":"cumulative_list_and_actual","unit_semantics":"platform_credit","counter_scope":"account"}
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler.Create(ctx)
	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Equal(t, "vendor_usage", repo.created.Code)
}

func TestUpstreamFinanceProtocolHandlerRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUpstreamFinanceProtocolHandler(service.NewUpstreamFinanceProtocolService(&financeProtocolHandlerRepoStub{}, service.NewUpstreamFinanceHTTPExecutor()), nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler.Get(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

type financeProtocolHandlerRepoStub struct {
	created service.FinanceProtocolCreateInput
}

func (s *financeProtocolHandlerRepoStub) ListProtocols(context.Context, service.FinanceProtocolListFilter) ([]service.UpstreamFinanceProtocol, int64, error) {
	return nil, 0, nil
}
func (s *financeProtocolHandlerRepoStub) GetProtocol(context.Context, int64) (*service.UpstreamFinanceProtocol, error) {
	return nil, service.ErrUpstreamFinanceProtocolNotFound
}
func (s *financeProtocolHandlerRepoStub) GetVersion(context.Context, int64) (*service.UpstreamFinanceProtocolVersion, error) {
	return nil, service.ErrUpstreamFinanceProtocolNotFound
}
func (s *financeProtocolHandlerRepoStub) ListVersions(context.Context, int64) ([]service.UpstreamFinanceProtocolVersion, error) {
	return nil, nil
}
func (s *financeProtocolHandlerRepoStub) CreateProtocol(_ context.Context, input service.FinanceProtocolCreateInput, validation service.FinanceProtocolValidationResult) (*service.UpstreamFinanceProtocol, error) {
	s.created = input
	return &service.UpstreamFinanceProtocol{ID: 1, Code: input.Code, Name: input.Name, ProtocolType: input.ProtocolType, Status: service.FinanceProtocolStatusDraft, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}
func (s *financeProtocolHandlerRepoStub) CreateDraftVersion(context.Context, int64, service.FinanceProtocolDraftInput, service.FinanceProtocolValidationResult) (*service.UpstreamFinanceProtocolVersion, error) {
	return nil, nil
}
func (s *financeProtocolHandlerRepoStub) PublishVersion(context.Context, int64, int64, *int64) error {
	return nil
}
func (s *financeProtocolHandlerRepoStub) DisableProtocol(context.Context, int64, *int64) error {
	return nil
}
func (s *financeProtocolHandlerRepoStub) DeleteDraft(context.Context, int64) error { return nil }
func (s *financeProtocolHandlerRepoStub) CreateDetectionAudit(context.Context, service.FinanceProtocolDetectionAudit) error {
	return nil
}
