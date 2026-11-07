package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandlerUpdateSettingsRejectsInvalidCodexClientVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyRegistrationEnabled: "true",
	}}
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)
	body, err := json.Marshal(map[string]any{
		"registration_enabled":        true,
		"openai_codex_client_version": "0.150.0\r\nX-Evil: 1",
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.UpdateSettings(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, repo.lastUpdates)
}

func TestSettingHandlerUpdateSettingsPersistsCodexVersionAndPreservesSyncedValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyRegistrationEnabled:               "true",
		service.SettingKeyOpenAICodexClientVersionSynced:    "0.151.0",
		service.SettingKeyOpenAICodexVersionAutoSyncEnabled: "true",
	}}
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)
	body, err := json.Marshal(map[string]any{
		"registration_enabled":                   true,
		"openai_codex_client_version":            " 0.150.0 ",
		"openai_codex_client_version_synced":     "0.999.0",
		"openai_codex_version_auto_sync_enabled": false,
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "0.150.0", repo.lastUpdates[service.SettingKeyOpenAICodexClientVersion])
	require.Equal(t, "false", repo.lastUpdates[service.SettingKeyOpenAICodexVersionAutoSyncEnabled])
	require.NotContains(t, repo.lastUpdates, service.SettingKeyOpenAICodexClientVersionSynced)
	require.Equal(t, "0.151.0", repo.values[service.SettingKeyOpenAICodexClientVersionSynced])
}
