package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLocaleForCountryCode(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"CN": "zh",
		"tw": "zh",
		"RU": "ru",
		"FR": "fr",
		"AT": "de",
		"US": "en",
		"ES": "en",
		"":   "en",
	}

	for countryCode, expected := range tests {
		countryCode := countryCode
		expected := expected
		t.Run(countryCode, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, expected, localeForCountryCode(countryCode))
		})
	}
}

func TestSettingHandlerGetDetectedLocale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &SettingHandler{}
	router.GET("/settings/locale", handler.GetDetectedLocale)

	req := httptest.NewRequest(http.MethodGet, "/settings/locale", nil)
	req.Header.Set("CF-IPCountry", "DE")
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, req)

	require.Equal(t, http.StatusOK, responseRecorder.Code)
	require.Equal(t, "private, no-store", responseRecorder.Header().Get("Cache-Control"))
	require.Contains(t, responseRecorder.Header().Get("Vary"), "CF-IPCountry")
	var payload struct {
		Code int `json:"code"`
		Data struct {
			CountryCode string `json:"country_code"`
			Locale      string `json:"locale"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(responseRecorder.Body.Bytes(), &payload))
	require.Zero(t, payload.Code)
	require.Equal(t, "DE", payload.Data.CountryCode)
	require.Equal(t, "de", payload.Data.Locale)
}

func TestSettingHandlerGetDetectedLocaleDefaultsToEnglish(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &SettingHandler{}
	router.GET("/settings/locale", handler.GetDetectedLocale)

	req := httptest.NewRequest(http.MethodGet, "/settings/locale", nil)
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, req)

	require.Contains(t, responseRecorder.Body.String(), `"locale":"en"`)
}
