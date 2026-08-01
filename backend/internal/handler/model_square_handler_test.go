//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelSquareHandlerServiceStub struct {
	groupsResult *service.ModelSquareGroupsResult
	modelsResult *service.ModelSquareModelsResult
	err          error
	query        service.ModelSquareModelsQuery
	userID       int64
	groupID      int64
}

type modelSquareSettingsStub struct {
	enabled bool
}

func (s modelSquareSettingsStub) GetModelSquareRuntime(context.Context) service.ModelSquareRuntime {
	return service.ModelSquareRuntime{Enabled: s.enabled}
}

func TestModelSquareHandlerAllowsFeatureOnLegacySalesPricingVersion(t *testing.T) {
	stub := &modelSquareHandlerServiceStub{groupsResult: &service.ModelSquareGroupsResult{}}
	h := &ModelSquareHandler{service: stub, settings: modelSquareSettingsStub{enabled: true}}
	recorder, ctx := modelSquareTestContext(http.MethodGet, "/api/v1/model-square/groups", true)
	h.ListGroups(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), stub.userID)
}

func (s *modelSquareHandlerServiceStub) ListGroups(_ context.Context, userID int64) (*service.ModelSquareGroupsResult, error) {
	s.userID = userID
	return s.groupsResult, s.err
}

func (s *modelSquareHandlerServiceStub) ListModels(_ context.Context, userID, groupID int64, query service.ModelSquareModelsQuery) (*service.ModelSquareModelsResult, error) {
	s.userID, s.groupID, s.query = userID, groupID, query
	return s.modelsResult, s.err
}

func modelSquareTestContext(method, target string, authenticated bool) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	if authenticated {
		ctx.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	}
	return recorder, ctx
}

func TestModelSquareHandlerListGroupsContract(t *testing.T) {
	updatedAt := time.Date(2026, 7, 26, 8, 30, 0, 0, time.UTC)
	stub := &modelSquareHandlerServiceStub{groupsResult: &service.ModelSquareGroupsResult{
		Groups:           []service.ModelSquareGroupItem{{ID: 1, Name: "OpenAI", EffectiveMultiplier: "1.1000"}},
		CatalogUpdatedAt: updatedAt,
	}}
	h := &ModelSquareHandler{service: stub, settings: modelSquareSettingsStub{enabled: true}}
	recorder, ctx := modelSquareTestContext(http.MethodGet, "/api/v1/model-square/groups", true)
	h.ListGroups(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), stub.userID)
	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, float64(0), response["code"])
	data := response["data"].(map[string]any)
	require.Contains(t, data, "groups")
	require.Equal(t, updatedAt.Format(time.RFC3339), data["catalog_updated_at"])
}

func TestModelSquareHandlerListModelsParsesAndForwardsQuery(t *testing.T) {
	updatedAt := time.Date(2026, 7, 26, 8, 30, 0, 0, time.UTC)
	stub := &modelSquareHandlerServiceStub{modelsResult: &service.ModelSquareModelsResult{GroupID: 12, Items: []service.ModelSquareModelItem{}}}
	h := &ModelSquareHandler{service: stub, settings: modelSquareSettingsStub{enabled: true}}
	target := "/api/v1/model-square/groups/12/models?q=gpt&cursor=signed&page_size=100&catalog_updated_at=" + updatedAt.Format(time.RFC3339)
	recorder, ctx := modelSquareTestContext(http.MethodGet, target, true)
	ctx.Params = gin.Params{{Key: "group_id", Value: "12"}}
	h.ListModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), stub.userID)
	require.Equal(t, int64(12), stub.groupID)
	require.Equal(t, "gpt", stub.query.Search)
	require.Equal(t, "signed", stub.query.Cursor)
	require.Equal(t, 100, stub.query.PageSize)
	require.Equal(t, updatedAt, *stub.query.CatalogUpdatedAt)
}

func TestModelSquareHandlerRejectsUnauthenticatedAndInvalidQuery(t *testing.T) {
	h := &ModelSquareHandler{service: &modelSquareHandlerServiceStub{}, settings: modelSquareSettingsStub{enabled: true}}
	recorder, ctx := modelSquareTestContext(http.MethodGet, "/api/v1/model-square/groups", false)
	h.ListGroups(ctx)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	recorder, ctx = modelSquareTestContext(http.MethodGet, "/api/v1/model-square/groups/x/models?page_size=x", true)
	ctx.Params = gin.Params{{Key: "group_id", Value: "x"}}
	h.ListModels(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestModelSquareHandlerMapsApplicationErrorReason(t *testing.T) {
	stub := &modelSquareHandlerServiceStub{err: service.ErrModelSquareCatalogChanged}
	h := &ModelSquareHandler{service: stub, settings: modelSquareSettingsStub{enabled: true}}
	recorder, ctx := modelSquareTestContext(http.MethodGet, "/api/v1/model-square/groups/12/models", true)
	ctx.Params = gin.Params{{Key: "group_id", Value: "12"}}
	h.ListModels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "CATALOG_CHANGED", response["reason"])
}

func TestModelSquareHandlerRejectsRequestsWhenFeatureIsDisabled(t *testing.T) {
	stub := &modelSquareHandlerServiceStub{}
	h := &ModelSquareHandler{service: stub, settings: modelSquareSettingsStub{enabled: false}}
	recorder, ctx := modelSquareTestContext(http.MethodGet, "/api/v1/model-square/groups", true)
	h.ListGroups(ctx)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Zero(t, stub.userID)
}

func TestModelSquareHandlerFailsClosedWithoutSettingsService(t *testing.T) {
	stub := &modelSquareHandlerServiceStub{}
	h := &ModelSquareHandler{service: stub}
	recorder, ctx := modelSquareTestContext(http.MethodGet, "/api/v1/model-square/groups", true)
	h.ListGroups(ctx)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Zero(t, stub.userID)
}
