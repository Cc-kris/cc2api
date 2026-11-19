package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type legacyCredentialOpsRepo struct {
	service.OpsRepository
	message string
	body    string
	events  string
}

func (r *legacyCredentialOpsRepo) GetErrorLogByID(context.Context, int64) (*service.OpsErrorLogDetail, error) {
	return &service.OpsErrorLogDetail{
		OpsErrorLog:          service.OpsErrorLog{ID: 25, StatusCode: 400, Message: r.message},
		ErrorBody:            r.body,
		UpstreamErrorMessage: r.message,
		UpstreamErrorDetail:  r.body,
		UpstreamErrors:       r.events,
	}, nil
}

func TestGetUpstreamError_RedactsLegacyStoredCredentialInHTTPResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	credential := "AQ." + "Ab" + strings.Repeat("Legacy_W6-", 5) + "Z"
	message := "provider rejected credential [" + credential + "]"
	body := `{"error":{"code":400,"message":"` + message + `","status":"INVALID_ARGUMENT"}}`
	repo := &legacyCredentialOpsRepo{message: message, body: body, events: `[{"status":400,"detail":` + quotedJSONString(body) + `}]`}
	svc := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := gin.New()
	r.GET("/ops/upstream-errors/:id", NewOpsHandler(svc).GetUpstreamError)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ops/upstream-errors/25", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), credential) {
		t.Fatal("legacy upstream-error HTTP response contains synthetic credential")
	}
	for _, want := range []string{"provider rejected", "INVALID_ARGUMENT", `"status_code":400`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("legacy response lost diagnostic %q", want)
		}
	}
}

func quotedJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
