package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCacheConfigHandlerForTest(values map[string]string) (*CacheConfigHandler, *cacheManagementHandlerRepoStub) {
	repo := &cacheManagementHandlerRepoStub{values: values}
	svc := service.NewSettingService(repo, &config.Config{})
	return NewCacheConfigHandler(svc), repo
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
