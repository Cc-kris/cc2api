package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiModelVisibility_ExplicitListMatchesOpenAIList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(25)
	mapping := map[string]any{
		"gemini-3.1-pro-preview": "private-upstream-pro",
		"gemini-3.1-flash-image": "gemini-3.1-flash-image",
		"gemini-3.5-flash":       "gemini-3.5-flash",
		"gemini-3.6-flash":       "gemini-3.6-flash",
		"gemini-3.7-flash":       "gemini-3.7-flash",
		"gemini-3.8-flash":       "gemini-3.8-flash",
		"public-alias":           "private-upstream-alias",
	}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		groupID: {{ID: 1, Platform: service.PlatformGemini, Credentials: map[string]any{"model_mapping": mapping}}},
		26:      {{ID: 2, Platform: service.PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{"other-group-secret": "other-group-secret"}}}},
	}})
	request := func(path string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformGemini}})
		handler(c)
		return rec
	}
	openAI := request("/v1/models", h.Models)
	native := request("/v1beta/models", h.GeminiV1BetaListModels)
	require.Equal(t, http.StatusOK, openAI.Code)
	require.Equal(t, http.StatusOK, native.Code)
	var want gatewayModelsResponseForTest
	var got gemini.ModelsListResponse
	require.NoError(t, json.Unmarshal(openAI.Body.Bytes(), &want))
	require.NoError(t, json.Unmarshal(native.Body.Bytes(), &got))
	ids := make([]string, 0, len(got.Models))
	for _, model := range got.Models {
		ids = append(ids, strings.TrimPrefix(model.Name, "models/"))
	}
	require.Len(t, ids, 7)
	require.ElementsMatch(t, modelIDsForTest(want.Data), ids)
	for _, forbidden := range []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-3-flash-preview", "private-upstream-pro", "private-upstream-alias", "other-group-secret"} {
		require.NotContains(t, ids, forbidden)
	}
	// The compat service is deliberately nil: discovery of a finite public
	// whitelist must not contact the upstream or depend on its catalogue.
}

func TestGeminiModelVisibility_GetModelHonorsWhitelistAndAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(25)
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		groupID: {{ID: 1, Platform: service.PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{"public-alias": "private-model", "gemini-3.*": "gemini-3.*"}}}},
	}})
	for _, tt := range []struct {
		model  string
		status int
	}{
		{"public-alias", http.StatusOK},
		{"gemini-2.5-pro", http.StatusNotFound},
		{"private-model", http.StatusNotFound},
		{"../models", http.StatusBadRequest},
	} {
		t.Run(tt.model, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/"+tt.model, nil)
			c.Params = gin.Params{{Key: "model", Value: tt.model}}
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformGemini}})
			h.GeminiV1BetaGetModel(c)
			require.Equal(t, tt.status, rec.Code)
			if tt.status == http.StatusOK {
				require.Contains(t, rec.Body.String(), "models/public-alias")
			}
		})
	}
}

func TestGeminiModelVisibility_ForcedPlatformIsScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(25)
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		groupID: {
			{ID: 1, Platform: service.PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-only": "gemini-only"}}},
			{ID: 2, Platform: service.PlatformAntigravity, Credentials: map[string]any{"model_mapping": map[string]any{"antigravity-only": "gemini-3.1-pro-preview"}}},
		},
	}})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/antigravity/v1beta/models", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformGemini}})
	c.Set(string(middleware.ContextKeyForcePlatform), service.PlatformAntigravity)
	h.GeminiV1BetaListModels(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "models/antigravity-only")
	require.NotContains(t, rec.Body.String(), "models/gemini-only")
	require.NotContains(t, rec.Body.String(), "models/gemini-2.5-pro")
}

func TestGeminiModelVisibility_WildcardsPreserveMetadataAndFilterFallback(t *testing.T) {
	v := newGeminiModelVisibility([]string{"gemini-3.*", "public-alias"})
	require.True(t, v.hasWildcard())
	body, err := v.filterList([]byte(`{"models":[{"name":"models/gemini-2.5-pro"},{"name":"models/gemini-3.7-flash","inputTokenLimit":12345},{"name":"models/gemini-3.7-flash"}],"nextPageToken":"next"}`))
	require.NoError(t, err)
	var got struct {
		Models []struct {
			Name  string `json:"name"`
			Limit int    `json:"inputTokenLimit"`
		} `json:"models"`
		Token string `json:"nextPageToken"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	require.Len(t, got.Models, 2)
	require.Equal(t, "models/gemini-3.7-flash", got.Models[0].Name)
	require.Equal(t, 12345, got.Models[0].Limit)
	require.Equal(t, "models/public-alias", got.Models[1].Name)
	require.Equal(t, "next", got.Token)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writeVisibleGeminiFallback(c, v, gemini.FallbackModelsList())
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "gemini-2.5-pro")
	require.Contains(t, rec.Body.String(), "models/public-alias")
}

func TestGeminiModelVisibility_EmptyDoesNotReviveDefaults(t *testing.T) {
	v := newGeminiModelVisibility([]string{})
	require.True(t, v.configured)
	require.False(t, v.allows("gemini-2.5-pro"))
	body, err := v.filterList([]byte(`{"models":[{"name":"models/gemini-2.5-pro"}]}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[]}`, string(body))
	unrestricted := newGeminiModelVisibility(nil)
	require.False(t, unrestricted.configured)
	require.True(t, unrestricted.allows("gemini-2.5-pro"))
}

func TestGeminiModelVisibility_MalformedUpstreamFailsClosed(t *testing.T) {
	v := newGeminiModelVisibility([]string{"gemini-3.*"})
	for _, body := range []string{"not json", "null", `{}`, `{"models":{}}`} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		writeVisibleGeminiUpstream(c, v, &service.UpstreamHTTPResult{StatusCode: http.StatusOK, Body: []byte(body)})
		require.Equal(t, http.StatusBadGateway, rec.Code, body)
	}
}
