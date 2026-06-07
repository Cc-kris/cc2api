package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	return newOpsAIAnalysisConfigRouterWithRole(handler, service.RoleAdmin)
}

func newOpsAIAnalysisConfigRouterWithRole(handler *OpsHandler, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Next()
	})
	r.GET("/ai-analysis/config", handler.GetAIAnalysisConfig)
	r.PUT("/ai-analysis/config", handler.UpdateAIAnalysisConfig)
	r.POST("/ai-analysis/test", handler.TestAIAnalysisConnection)
	r.POST("/ai-analysis/tasks", handler.CreateAIAnalysisTask)
	r.GET("/ai-analysis/tasks/:id", handler.GetAIAnalysisTask)
	r.POST("/ai-analysis/tasks/:id/feedback", handler.UpdateAIAnalysisReportFeedback)
	return r
}

func newOpsAIAnalysisConfigHandler(repo *testSettingRepo) *OpsHandler {
	return newOpsAIAnalysisConfigHandlerWithOpsRepo(repo, nil)
}

func newOpsAIAnalysisConfigHandlerWithOpsRepo(repo *testSettingRepo, opsRepo service.OpsRepository) *OpsHandler {
	if repo == nil {
		repo = newTestSettingRepo()
	}
	svc := service.NewOpsService(opsRepo, repo, &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
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

func TestOpsAIAnalysisConfigHandler_TestConnection(t *testing.T) {
	var gotAuth string

	repo := newTestSettingRepo()
	h := newOpsAIAnalysisConfigHandler(repo)
	h.opsService.SetAIAnalysisHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"id":"resp_1"}`)), Header: http.Header{}}, nil
	})})
	r := newOpsAIAnalysisConfigRouter(h)

	body := `{"enabled":true,"base_url":"https://93.184.216.34/v1","api_key":"sk-handler-secret","model":"gpt-5.5","interface_type":"responses","timeout_seconds":60,"max_samples":50,"auto_dedup_minutes":10,"global_rate_limit_per_minute":10,"auto_levels":["P0","P1"],"manual_enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/ai-analysis/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/ai-analysis/test", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST test status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeOpsAIResponse(t, w)
	if data["status"] != service.OpsAIAnalysisConnectionStatusSuccess || data["success"] != true {
		t.Fatalf("unexpected data: %+v", data)
	}
	if gotAuth != "Bearer sk-handler-secret" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if strings.Contains(w.Body.String(), "sk-handler-secret") {
		t.Fatalf("test response leaked plaintext API key: %s", w.Body.String())
	}
}

func TestOpsAIAnalysisConfigHandler_TestConnectionConfigMissing(t *testing.T) {
	h := newOpsAIAnalysisConfigHandler(newTestSettingRepo())
	r := newOpsAIAnalysisConfigRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/ai-analysis/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST test status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeOpsAIResponse(t, w)
	if data["status"] != service.OpsAIAnalysisConnectionStatusConfigError || data["success"] != false {
		t.Fatalf("unexpected data: %+v", data)
	}
	if !strings.Contains(w.Body.String(), "请先配置 AI 分析服务") {
		t.Fatalf("missing clear config message: %s", w.Body.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestOpsAIAnalysisConfigHandler_CreateAIAnalysisTaskInvalidBody(t *testing.T) {
	h := newOpsAIAnalysisConfigHandler(newTestSettingRepo())
	r := newOpsAIAnalysisConfigRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/ai-analysis/tasks", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestOpsAIAnalysisConfigHandler_GetAIAnalysisTaskInvalidID(t *testing.T) {
	h := newOpsAIAnalysisConfigHandler(newTestSettingRepo())
	r := newOpsAIAnalysisConfigRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/ai-analysis/tasks/bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("GET invalid status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestOpsAIAnalysisConfigHandler_UpdateAIAnalysisReportFeedback(t *testing.T) {
	h := newOpsAIAnalysisConfigHandlerWithOpsRepo(newTestSettingRepo(), &opsAIHandlerRepoStub{})
	r := newOpsAIAnalysisConfigRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/ai-analysis/tasks/77/feedback", bytes.NewBufferString(`{"feedback_status":"useful","feedback_note":"判断准确"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST feedback status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeOpsAIResponse(t, w)
	if data["task_id"] != float64(77) || data["feedback_status"] != service.OpsAIAnalysisFeedbackUseful || data["feedback_note"] != "判断准确" || data["feedback_user_id"] != float64(7) {
		t.Fatalf("unexpected feedback response: %+v; body=%s", data, w.Body.String())
	}
}

func TestOpsAIAnalysisConfigHandler_UpdateAIAnalysisReportFeedbackSupportRole(t *testing.T) {
	h := newOpsAIAnalysisConfigHandlerWithOpsRepo(newTestSettingRepo(), &opsAIHandlerRepoStub{})
	r := newOpsAIAnalysisConfigRouterWithRole(h, "customer_service")

	req := httptest.NewRequest(http.MethodPost, "/ai-analysis/tasks/77/feedback", bytes.NewBufferString(`{"feedback_status":"wrong_category","feedback_note":"主因不准确"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST feedback support role status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeOpsAIResponse(t, w)
	if data["feedback_status"] != service.OpsAIAnalysisFeedbackWrongCategory || data["feedback_note"] != "主因不准确" {
		t.Fatalf("unexpected support feedback response: %+v; body=%s", data, w.Body.String())
	}
}

func TestOpsAIAnalysisConfigHandler_UpdateAIAnalysisReportFeedbackInvalidPayloads(t *testing.T) {
	h := newOpsAIAnalysisConfigHandlerWithOpsRepo(newTestSettingRepo(), &opsAIHandlerRepoStub{})
	r := newOpsAIAnalysisConfigRouter(h)

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/ai-analysis/tasks/bad/feedback", body: `{"feedback_status":"useful"}`},
		{path: "/ai-analysis/tasks/77/feedback", body: `{`},
		{path: "/ai-analysis/tasks/77/feedback", body: `{"feedback_status":"bad"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("POST %s status = %d, body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}

type opsAIHandlerRepoStub struct {
	service.OpsRepository
}

func (opsAIHandlerRepoStub) UpdateAIAnalysisReportFeedback(ctx context.Context, input *service.OpsAIAnalysisFeedbackInput) (*service.OpsAIAnalysisReport, error) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	return &service.OpsAIAnalysisReport{TaskID: input.TaskID, FeedbackStatus: input.FeedbackStatus, FeedbackNote: input.FeedbackNote, FeedbackUserID: &input.FeedbackUserID, FeedbackAt: &now, CreatedAt: now, UpdatedAt: now}, nil
}
