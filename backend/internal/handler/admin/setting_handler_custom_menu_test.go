package admin

import (
	"bytes"
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

func newCustomMenuSettingsHandler() (*SettingHandler, *settingHandlerRepoStub) {
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	return handler, repo
}

func TestSettingsPUT_CustomMenuAllowsSiteRelativeHTMLPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newCustomMenuSettingsHandler()

	body := map[string]any{
		"custom_menu_items": []map[string]any{
			{
				"id":         "seedance_video_guide",
				"label":      "seedace视频调用说明",
				"icon_svg":   "",
				"url":        "/seedance-video-guide.html",
				"visibility": "user",
				"sort_order": 1,
			},
		},
	}

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Contains(t, repo.values[service.SettingKeyCustomMenuItems], `"/seedance-video-guide.html"`)
}

func TestSettingsPUT_CustomMenuRejectsUnsafeSiteRelativePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newCustomMenuSettingsHandler()

	body := map[string]any{
		"custom_menu_items": []map[string]any{
			{
				"id":         "bad_path",
				"label":      "Bad Path",
				"icon_svg":   "",
				"url":        "/../admin",
				"visibility": "user",
				"sort_order": 1,
			},
		},
	}

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
