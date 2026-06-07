package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type opsAIHandlerEncryptorStub struct{}

func (opsAIHandlerEncryptorStub) Encrypt(plaintext string) (string, error) {
	runes := []rune(plaintext)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return "enc:" + string(runes), nil
}

func (opsAIHandlerEncryptorStub) Decrypt(ciphertext string) (string, error) {
	plain := []rune(strings.TrimPrefix(ciphertext, "enc:"))
	for i, j := 0, len(plain)-1; i < j; i, j = i+1, j-1 {
		plain[i], plain[j] = plain[j], plain[i]
	}
	return string(plain), nil
}

func newOpsAIAnalysisConfigRouter(handler *OpsHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Next()
	})
	r.GET("/ai-analysis/config", handler.GetAIAnalysisConfig)
	r.PUT("/ai-analysis/config", handler.UpdateAIAnalysisConfig)
	return r
}

func newOpsAIAnalysisConfigHandler(repo *testSettingRepo) *OpsHandler {
	if repo == nil {
		repo = newTestSettingRepo()
	}
	svc := service.NewOpsService(nil, repo, &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.SetSecretEncryptor(opsAIHandlerEncryptorStub{})
	return NewOpsHandler(svc)
}

func decodeOpsAIResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return envelope.Data
}

func TestOpsAIAnalysisConfigHandler_UpdateAndGetMasksAPIKey(t *testing.T) {
	repo := newTestSettingRepo()
	h := newOpsAIAnalysisConfigHandler(repo)
	r := newOpsAIAnalysisConfigRouter(h)

	body := `{"enabled":true,"base_url":"https://ai.example.com/v1","api_key":"sk-handler-secret","model":"gpt-5.5","interface_type":"responses","timeout_seconds":60,"max_samples":50,"auto_dedup_minutes":10,"global_rate_limit_per_minute":10,"auto_levels":["P0","P1"],"manual_enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/ai-analysis/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeOpsAIResponse(t, w)
	if data["api_key_masked"] != "****cret" {
		t.Fatalf("api_key_masked = %v", data["api_key_masked"])
	}
	if _, ok := data["api_key"]; ok {
		t.Fatalf("response leaked api_key: %s", w.Body.String())
	}
	if _, ok := data["api_key_encrypted"]; ok {
		t.Fatalf("response leaked api_key_encrypted: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sk-handler-secret") {
		t.Fatalf("response leaked plaintext API key: %s", w.Body.String())
	}
	if strings.Contains(repo.values[service.SettingKeyOpsAIAnalysisConfig], "sk-handler-secret") {
		t.Fatalf("settings storage leaked plaintext API key: %s", repo.values[service.SettingKeyOpsAIAnalysisConfig])
	}

	req = httptest.NewRequest(http.MethodGet, "/ai-analysis/config", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", w.Code, w.Body.String())
	}
	data = decodeOpsAIResponse(t, w)
	if data["api_key_masked"] != "****cret" {
		t.Fatalf("GET api_key_masked = %v", data["api_key_masked"])
	}
	if strings.Contains(w.Body.String(), "sk-handler-secret") {
		t.Fatalf("GET response leaked plaintext API key: %s", w.Body.String())
	}
}

func TestOpsAIAnalysisConfigHandler_InvalidPayloadsReturnBadRequest(t *testing.T) {
	h := newOpsAIAnalysisConfigHandler(newTestSettingRepo())
	r := newOpsAIAnalysisConfigRouter(h)

	for _, body := range []string{
		`{`,
		`{"enabled":true,"base_url":"notaurl","api_key":"sk-test","model":"gpt-5.5","interface_type":"responses","timeout_seconds":60,"max_samples":50,"auto_dedup_minutes":10,"global_rate_limit_per_minute":10,"auto_levels":["P0"],"manual_enabled":true}`,
		`{"enabled":true,"base_url":"https://ai.example.com/v1","api_key":"sk-test","model":"gpt-5.5","interface_type":"responses","timeout_seconds":4,"max_samples":50,"auto_dedup_minutes":10,"global_rate_limit_per_minute":10,"auto_levels":["P0"],"manual_enabled":true}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/ai-analysis/config", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("PUT invalid status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	}
}

func TestOpsAIAnalysisConfigHandler_MonitoringDisabled(t *testing.T) {
	repo := newTestSettingRepo()
	repo.values[service.SettingKeyOpsMonitoringEnabled] = "false"
	h := newOpsAIAnalysisConfigHandler(repo)
	r := newOpsAIAnalysisConfigRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/ai-analysis/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET disabled status = %d, body=%s", w.Code, w.Body.String())
	}
}
