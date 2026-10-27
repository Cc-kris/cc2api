package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestContentNewsHandlerRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewContentNewsHandler(service.NewContentNewsAdminService(nil))
	r.GET("/content/news-sources/:id", h.GetNewsSource)
	req := httptest.NewRequest(http.MethodGet, "/content/news-sources/not-an-id", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.Code)
	}
}

// R10-05/R10-06: masked-only responses, encrypted disk storage, fail-closed writes, and empty/masked edits keep the old key.
func TestContentAIConfigNeverExposesOrStoresPlaintextKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Chdir(t.TempDir())
	svc := service.NewContentArticleAdminService(nil, nil, nil)
	r := gin.New()
	h := NewContentArticleHandler(svc)
	r.GET("/ai-config", h.GetAIConfig)
	r.PUT("/ai-config", h.UpdateAIConfig)
	const plaintext, mask = "sk-live-SECRET-1234567890", "sk-l****7890"
	stored := filepath.Join("data", "content_ai_provider.json")
	do := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, "/ai-config", strings.NewReader(body)))
		return w
	}
	aiBody := func(key string) string { return `{"baseUrl":"https://api.example.com","apiKey":"` + key + `"}` }
	leaks := func(b string) bool {
		return strings.Contains(b, "SECRET") || strings.Contains(b, `"apiKey":`) || strings.Contains(b, "encryptedKey")
	}
	_, statErr := os.Stat(stored)
	w := do(http.MethodPut, aiBody(plaintext))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "encryption_not_configured") || leaks(w.Body.String()) || !os.IsNotExist(statErr) {
		t.Fatalf("PUT without encryption key: status=%d body=%s err=%v", w.Code, w.Body.String(), statErr)
	}
	t.Setenv("TOTP_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	check := func(name string, putRes *httptest.ResponseRecorder) string {
		getRes := do(http.MethodGet, "")
		for _, out := range []string{putRes.Body.String(), getRes.Body.String()} {
			if putRes.Code != http.StatusOK || getRes.Code != http.StatusOK || leaks(out) || !strings.Contains(out, mask) || !strings.Contains(out, `"hasApiKey":true`) {
				t.Fatalf("%s: put=%d get=%d key must stay masked: %s", name, putRes.Code, getRes.Code, out)
			}
		}
		cfg, err := svc.GetAIProviderConfig(context.Background())
		if raw, rerr := os.ReadFile(stored); err != nil || cfg.APIKey != plaintext || rerr != nil || strings.Contains(string(raw), plaintext) {
			t.Fatalf("%s must keep the original key encrypted: apiKey=%q err=%v rerr=%v raw=%s", name, cfg.APIKey, err, rerr, raw)
		}
		return getRes.Body.String()
	}
	initial := check("initial", do(http.MethodPut, aiBody(plaintext)))
	_, rest, _ := strings.Cut(initial, `"apiKeyMasked":"`)
	gotMask, _, _ := strings.Cut(rest, `"`)
	if gotMask != mask {
		t.Fatalf("GET must return the original key mask %q, got %q", mask, gotMask)
	}
	for _, tc := range []struct{ name, apiKey string }{{"empty_update", ""}, {"masked_update", gotMask}} {
		check(tc.name, do(http.MethodPut, aiBody(tc.apiKey)))
	}
}
