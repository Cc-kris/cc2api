package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type analyticsHandlerRepoStub struct{}

func (analyticsHandlerRepoStub) CreateEvent(context.Context, *service.ContentAnalyticsEvent) error {
	return nil
}
func (analyticsHandlerRepoStub) ListDaily(context.Context, service.ContentAnalyticsFilters) ([]service.ContentAnalyticsDaily, error) {
	return []service.ContentAnalyticsDaily{{PagePath: "/", Impressions: 2, Clicks: 1}}, nil
}
func (analyticsHandlerRepoStub) AggregateDaily(context.Context, time.Time, string) error { return nil }

type blockingAnalyticsRepo struct{ started, release chan struct{} }

func (r blockingAnalyticsRepo) CreateEvent(context.Context, *service.ContentAnalyticsEvent) error {
	close(r.started)
	<-r.release
	return nil
}
func (blockingAnalyticsRepo) ListDaily(context.Context, service.ContentAnalyticsFilters) ([]service.ContentAnalyticsDaily, error) {
	return nil, nil
}
func (blockingAnalyticsRepo) AggregateDaily(context.Context, time.Time, string) error { return nil }

type retryAnalyticsRepo struct{ calls atomic.Int32 }

func (r *retryAnalyticsRepo) CreateEvent(context.Context, *service.ContentAnalyticsEvent) error {
	if r.calls.Add(1) < 3 {
		return errors.New("temporary")
	}
	return nil
}
func (*retryAnalyticsRepo) ListDaily(context.Context, service.ContentAnalyticsFilters) ([]service.ContentAnalyticsDaily, error) {
	return nil, nil
}
func (*retryAnalyticsRepo) AggregateDaily(context.Context, time.Time, string) error { return nil }

func TestContentAnalyticsIngestReturnsBeforePersistenceCompletes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := blockingAnalyticsRepo{started: make(chan struct{}), release: make(chan struct{})}
	r := gin.New()
	r.POST("/events", NewContentAnalyticsHandler(service.NewContentAnalyticsService(repo, "secret")).Ingest)
	payload, _ := json.Marshal(map[string]any{"eventId": "evt-async-1", "eventName": "page_view", "pagePath": "/", "payload": map[string]any{"view_id": "view-async-1"}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(string(payload))))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rec.Code)
	}
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not persist")
	}
	close(repo.release)
}

func TestContentAnalyticsIngestRetriesPersistenceFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &retryAnalyticsRepo{}
	r := gin.New()
	r.POST("/events", NewContentAnalyticsHandler(service.NewContentAnalyticsService(repo, "secret")).Ingest)
	payload := `{"eventId":"evt-retry-1","eventName":"page_view","pagePath":"/","payload":{"view_id":"view-retry-1"}}`
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(payload)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rec.Code)
	}
	deadline := time.Now().Add(time.Second)
	for repo.calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if repo.calls.Load() != 3 {
		t.Fatalf("calls=%d", repo.calls.Load())
	}
}

func TestContentAnalyticsInvalidEventDoesNotConsumeRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/events", NewContentAnalyticsHandler(service.NewContentAnalyticsService(analyticsHandlerRepoStub{}, "secret")).Ingest)
	bad := httptest.NewRecorder()
	r.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"eventId":"bad","eventName":"unknown","pagePath":"/"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d", bad.Code)
	}
	good := httptest.NewRecorder()
	r.ServeHTTP(good, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"eventId":"good","eventName":"page_view","pagePath":"/","payload":{"view_id":"view-good-1"}}`)))
	if good.Code != http.StatusAccepted {
		t.Fatalf("good status=%d", good.Code)
	}
}

func TestContentAnalyticsHandlerStopDrainsAndExits(t *testing.T) {
	h := NewContentAnalyticsHandler(service.NewContentAnalyticsService(analyticsHandlerRepoStub{}, "secret"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

func TestContentAnalyticsHandlerRejectsIngestAfterStop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewContentAnalyticsHandler(service.NewContentAnalyticsService(analyticsHandlerRepoStub{}, "secret"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	r := gin.New()
	r.POST("/events", h.Ingest)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"eventId":"after-stop","eventName":"page_view","pagePath":"/","payload":{"view_id":"view-after-stop-1"}}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestContentAnalyticsHandlerRejectsInvalidDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewContentAnalyticsHandler(service.NewContentAnalyticsService(analyticsHandlerRepoStub{}, "secret"))
	r.GET("/bi", h.Overview)
	req := httptest.NewRequest("GET", "/bi?from=2026-99-99", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
