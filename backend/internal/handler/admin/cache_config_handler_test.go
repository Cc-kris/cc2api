package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCacheConfigHandlerForTest(values map[string]string) (*CacheConfigHandler, *cacheManagementHandlerRepoStub) {
	repo := &cacheManagementHandlerRepoStub{values: values}
	svc := service.NewSettingService(repo, &config.Config{})
	return NewCacheConfigHandler(svc, nil), repo
}

type cacheManagementHandlerRepoStub struct {
	values map[string]string
	sets   map[string]string
}

func (r *cacheManagementHandlerRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	if value, ok := r.values[key]; ok {
		return &service.Setting{Key: key, Value: value}, nil
	}
	return nil, service.ErrSettingNotFound
}

func (r *cacheManagementHandlerRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *cacheManagementHandlerRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	if r.sets == nil {
		r.sets = map[string]string{}
	}
	r.values[key] = value
	r.sets[key] = value
	return nil
}

func (r *cacheManagementHandlerRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *cacheManagementHandlerRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *cacheManagementHandlerRepoStub) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *cacheManagementHandlerRepoStub) Delete(context.Context, string) error { return nil }

func TestCacheConfigHandlerGetConfigReturnsDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newCacheConfigHandlerForTest(nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/config", nil)

	handler.GetConfig(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data, ok := body.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["global_enabled"])
	require.Equal(t, float64(600), data["ttl_seconds"])
	require.Equal(t, float64(256*1024), data["max_request_bytes"])
	require.Equal(t, float64(512*1024), data["max_response_bytes"])
	require.Equal(t, 0.3, data["max_temperature"])
	header := data["bypass_header"].(map[string]any)
	require.Equal(t, "X-Sub2API-Cache-Control", header["name"])
	require.Equal(t, "bypass", header["value"])
}

func TestCacheConfigHandlerUpdateConfigPersistsFixedBypassHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newCacheConfigHandlerForTest(nil)
	reqBody := []byte(`{
		"global_enabled": true,
		"platforms": {"openai": {"enabled": true}, "claude": {"enabled": false}, "gemini": {"enabled": true}},
		"ttl_seconds": 600,
		"max_request_bytes": 1024,
		"max_response_bytes": 1048576,
		"max_temperature": 0.3,
		"model_allowlist": ["gpt-4o", "gpt-4o", ""],
		"model_blocklist": ["claude-3"],
		"bypass_header": {"name": "client", "value": "override"}
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/cache/config", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateConfig(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, repo.sets, service.SettingKeyCacheManagementConfig)
	var stored service.CacheManagementConfig
	require.NoError(t, json.Unmarshal([]byte(repo.sets[service.SettingKeyCacheManagementConfig]), &stored))
	require.True(t, stored.GlobalEnabled)
	require.True(t, stored.Platforms.OpenAI.Enabled)
	require.True(t, stored.Platforms.Gemini.Enabled)
	require.Equal(t, []string{"gpt-4o"}, stored.ModelAllowlist)
	require.Equal(t, "X-Sub2API-Cache-Control", stored.BypassHeader.Name)
	require.Equal(t, "bypass", stored.BypassHeader.Value)
}

func TestCacheConfigHandlerUpdateConfigRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newCacheConfigHandlerForTest(nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/cache/config", bytes.NewReader([]byte(`{
		"ttl_seconds": 59,
		"max_request_bytes": 1024,
		"max_response_bytes": 1048576,
		"max_temperature": 0.3
	}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateConfig(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, repo.sets)
}

func TestCacheConfigHandlerGetAdvancedConfigReturnsDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newCacheConfigHandlerForTest(nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/advanced-config", nil)

	handler.GetAdvancedConfig(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data := body.Data.(map[string]any)
	require.Equal(t, false, data["advanced_cache_enabled"])
	require.Equal(t, float64(512), data["redis_capacity_mb"])
	require.Equal(t, float64(2048), data["memory_safe_limit_mb"])
	require.Equal(t, true, data["compression_enabled"])
	require.Equal(t, float64(64), data["compression_threshold_kb"])
	require.Equal(t, "LRU", data["eviction_policy"])
	require.Equal(t, "1h", data["hot_window"])
	require.Equal(t, float64(5), data["hot_threshold"])
	require.Equal(t, true, data["cost_saving_enabled"])
	require.Equal(t, true, data["upstream_prompt_cache_enabled"])
	grayScope := data["gray_scope"].(map[string]any)
	require.Empty(t, grayScope["api_key_ids"])
	require.Empty(t, grayScope["group_ids"])
	require.Empty(t, grayScope["models"])
}

func TestCacheConfigHandlerUpdateAdvancedConfigPersistsNormalizedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newCacheConfigHandlerForTest(nil)
	reqBody := []byte(`{
		"advanced_cache_enabled": true,
		"gray_scope": {"api_key_ids": [3, 1, 3], "group_ids": [8, 2, 8], "models": [" gpt-5.5 ", "GPT-5.5", ""]},
		"redis_capacity_mb": 768,
		"memory_safe_limit_mb": 64,
		"compression_enabled": true,
		"compression_threshold_kb": 128,
		"eviction_policy": " LFU ",
		"hot_window": " 6h ",
		"hot_threshold": 10,
		"cost_saving_enabled": true,
		"upstream_prompt_cache_enabled": true
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/cache/advanced-config", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAdvancedConfig(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, repo.sets, service.SettingKeyAdvancedCacheConfig)
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(repo.sets[service.SettingKeyAdvancedCacheConfig]), &stored))
	require.NotContains(t, stored, "memory_safe_limit_mb")
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data := body.Data.(map[string]any)
	require.Equal(t, "LFU", data["eviction_policy"])
	require.Equal(t, "6h", data["hot_window"])
	grayScope := data["gray_scope"].(map[string]any)
	require.Equal(t, []any{float64(3), float64(1)}, grayScope["api_key_ids"])
	require.Equal(t, []any{float64(8), float64(2)}, grayScope["group_ids"])
	require.Equal(t, []any{"gpt-5.5"}, grayScope["models"])
}

func TestCacheConfigHandlerUpdateAdvancedConfigRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newCacheConfigHandlerForTest(nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/cache/advanced-config", bytes.NewReader([]byte(`{
		"advanced_cache_enabled": false,
		"gray_scope": {"api_key_ids": [-1]},
		"redis_capacity_mb": 512,
		"compression_enabled": true,
		"compression_threshold_kb": 64,
		"eviction_policy": "LRU",
		"hot_window": "1h",
		"hot_threshold": 5,
		"cost_saving_enabled": true,
		"upstream_prompt_cache_enabled": true
	}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAdvancedConfig(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, repo.sets)
}

type cacheClearHandlerServiceStub struct {
	got service.LocalResponseCacheClearRequest
	res *service.LocalResponseCacheClearResult
	err error

	filter service.LocalResponseCacheClearAuditFilter
	page   *service.LocalResponseCacheClearAuditPage
}

func (s *cacheClearHandlerServiceStub) ClearLocalResponseCache(_ context.Context, req service.LocalResponseCacheClearRequest) (*service.LocalResponseCacheClearResult, error) {
	s.got = req
	return s.res, s.err
}
func (s *cacheClearHandlerServiceStub) ListLocalResponseCacheClearAudits(_ context.Context, filter service.LocalResponseCacheClearAuditFilter) (*service.LocalResponseCacheClearAuditPage, error) {
	s.filter = filter
	return s.page, s.err
}

func TestCacheConfigHandlerClearPassesOperatorAndScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &cacheClearHandlerServiceStub{res: &service.LocalResponseCacheClearResult{ClearType: service.LocalResponseCacheClearTypeByModel, MatchedKeys: 2, DeletedKeys: 2, Status: service.LocalResponseCacheClearStatusSuccess}}
	handler := NewCacheConfigHandler(nil, stub)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("user", middleware.AuthSubject{UserID: 9})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/cache/clear", bytes.NewReader([]byte(`{"clear_type":"by_model","scope":{"models":["gpt-5.5"]}}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Clear(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.LocalResponseCacheClearTypeByModel, stub.got.ClearType)
	require.Equal(t, []string{"gpt-5.5"}, stub.got.Scope.Models)
	require.NotNil(t, stub.got.OperatorUserID)
	require.Equal(t, int64(9), *stub.got.OperatorUserID)
}

func TestCacheConfigHandlerClearRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &cacheClearHandlerServiceStub{err: service.ErrInvalidLocalResponseCacheClear}
	handler := NewCacheConfigHandler(nil, stub)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/cache/clear", bytes.NewReader([]byte(`{"clear_type":"all"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Clear(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCacheConfigHandlerListClearAuditsParsesFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, 6, 8, 1, 0, 0, 0, time.UTC)
	operatorID := int64(9)
	stub := &cacheClearHandlerServiceStub{page: &service.LocalResponseCacheClearAuditPage{Items: []service.LocalResponseCacheClearAuditRecord{{
		ID:             1,
		OperatorUserID: &operatorID,
		ClearType:      service.LocalResponseCacheClearTypeByModel,
		Scope:          service.LocalResponseCacheClearScope{Models: []string{"gpt-5.5"}},
		MatchedKeys:    2,
		DeletedKeys:    2,
		Status:         service.LocalResponseCacheClearStatusSuccess,
		CreatedAt:      createdAt,
	}}, Total: 1, Page: 2, PageSize: 10}}
	handler := NewCacheConfigHandler(nil, stub)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	start := createdAt.Add(-time.Hour).Format(time.RFC3339)
	end := createdAt.Format(time.RFC3339)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/clear-audits?page=2&page_size=10&start_time="+start+"&end_time="+end+"&operator_user_id=9&clear_type=by_model&status=success", nil)

	handler.ListClearAudits(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 2, stub.filter.Page)
	require.Equal(t, 10, stub.filter.PageSize)
	require.Equal(t, int64(9), *stub.filter.OperatorUserID)
	require.Equal(t, service.LocalResponseCacheClearTypeByModel, stub.filter.ClearType)
	require.Equal(t, service.LocalResponseCacheClearStatusSuccess, stub.filter.Status)
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data := body.Data.(map[string]any)
	require.Equal(t, float64(1), data["total"])
}

func TestCacheConfigHandlerListClearAuditsRejectsInvalidQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewCacheConfigHandler(nil, &cacheClearHandlerServiceStub{page: &service.LocalResponseCacheClearAuditPage{}})
	cases := []string{
		"/api/v1/admin/cache/clear-audits?page=0",
		"/api/v1/admin/cache/clear-audits?page_size=-1",
		"/api/v1/admin/cache/clear-audits?start_time=nope",
		"/api/v1/admin/cache/clear-audits?operator_user_id=0",
	}
	for _, url := range cases {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, url, nil)

		handler.ListClearAudits(c)

		require.Equal(t, http.StatusBadRequest, rec.Code, url)
	}
}

func TestCacheConfigHandlerListClearAuditsRejectsInvalidServiceFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewCacheConfigHandler(nil, &cacheClearHandlerServiceStub{err: service.ErrInvalidLocalResponseCacheAuditList})
	cases := []string{
		"/api/v1/admin/cache/clear-audits?clear_type=bad",
		"/api/v1/admin/cache/clear-audits?status=done",
		"/api/v1/admin/cache/clear-audits?start_time=2026-06-08T01:00:00Z&end_time=2026-06-08T00:00:00Z",
	}
	for _, url := range cases {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, url, nil)

		handler.ListClearAudits(c)

		require.Equal(t, http.StatusBadRequest, rec.Code, url)
	}
}
