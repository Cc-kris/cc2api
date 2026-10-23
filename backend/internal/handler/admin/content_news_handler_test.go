package admin

import (
	"net/http"
	"net/http/httptest"
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
