package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ContentAnalyticsHandler struct {
	svc          *service.ContentAnalyticsService
	limitMu      sync.Mutex
	lastIngest   map[string]time.Time
	ingestSem    chan struct{}
	ingestQueue  chan *service.ContentAnalyticsEvent
	stopCh       chan struct{}
	stopOnce     sync.Once
	workerDone   chan struct{}
	queueMu      sync.Mutex
	workerCtx    context.Context
	workerCancel context.CancelFunc
}

func NewContentAnalyticsHandler(s *service.ContentAnalyticsService) *ContentAnalyticsHandler {
	workerCtx, cancel := context.WithCancel(context.Background())
	h := &ContentAnalyticsHandler{svc: s, lastIngest: make(map[string]time.Time), ingestSem: make(chan struct{}, 64), ingestQueue: make(chan *service.ContentAnalyticsEvent, 64), stopCh: make(chan struct{}), workerDone: make(chan struct{}), workerCtx: workerCtx, workerCancel: cancel}
	go h.runIngestWorker()
	return h
}
func (h *ContentAnalyticsHandler) runIngestWorker() {
	defer close(h.workerDone)
	for {
		var e *service.ContentAnalyticsEvent
		select {
		case e = <-h.ingestQueue:
		case <-h.stopCh:
			for {
				select {
				case e = <-h.ingestQueue:
					if h.workerCtx.Err() != nil {
						<-h.ingestSem
						continue
					}
				default:
					return
				}
				h.persistEventWithRetry(e)
			}
		}
		h.persistEventWithRetry(e)
	}
}
func (h *ContentAnalyticsHandler) persistEventWithRetry(e *service.ContentAnalyticsEvent) {
	ctx := h.workerCtx
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = h.svc.PersistEvent(ctx, e)
		if err == nil {
			break
		}
		if attempt < 2 && !waitAnalyticsRetry(ctx, attempt) {
			break
		}
	}
	if err != nil {
		slog.Error("content analytics async ingest failed", "error", err)
	}
	<-h.ingestSem
}

// Stop prevents new events, drains queued events, and waits for persistence
// work to finish. It is safe to call more than once.
func (h *ContentAnalyticsHandler) Stop(ctx context.Context) error {
	h.stopOnce.Do(func() { h.queueMu.Lock(); close(h.stopCh); h.queueMu.Unlock() })
	select {
	case <-h.workerDone:
		return nil
	case <-ctx.Done():
		h.workerCancel()
		return ctx.Err()
	}
}

func waitAnalyticsRetry(ctx context.Context, attempt int) bool {
	t := time.NewTimer(time.Duration(100*(1<<attempt)) * time.Millisecond)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
func (h *ContentAnalyticsHandler) Ingest(c *gin.Context) {
	queued := false
	select {
	case h.ingestSem <- struct{}{}:
		defer func() {
			if !queued {
				<-h.ingestSem
			}
		}()
	default:
		c.Status(http.StatusTooManyRequests)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var e service.ContentAnalyticsEvent
	if c.ShouldBindJSON(&e) != nil {
		response.BadRequest(c, "invalid event")
		return
	}
	if err := h.svc.PrepareEvent(&e, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		if errors.Is(err, service.ErrContentAnalyticsInvalid) {
			response.BadRequest(c, err.Error())
			return
		}
		slog.ErrorContext(c.Request.Context(), "content analytics ingest failed", "error", err)
		response.ErrorFrom(c, err)
		return
	}
	key := c.ClientIP()
	h.limitMu.Lock()
	previous := h.lastIngest[key]
	now := time.Now()
	if !previous.IsZero() && now.Sub(previous) < 100*time.Millisecond {
		h.limitMu.Unlock()
		c.Status(http.StatusTooManyRequests)
		return
	}
	h.lastIngest[key] = now
	if len(h.lastIngest) > 10000 {
		cutoff := now.Add(-10 * time.Minute)
		for ip, seen := range h.lastIngest {
			if seen.Before(cutoff) {
				delete(h.lastIngest, ip)
			}
		}
	}
	h.limitMu.Unlock()
	h.queueMu.Lock()
	defer h.queueMu.Unlock()
	select {
	case <-h.stopCh:
		c.Status(http.StatusServiceUnavailable)
		return
	default:
	}
	select {
	case h.ingestQueue <- &e:
		queued = true
	case <-h.stopCh:
		c.Status(http.StatusServiceUnavailable)
		return
	case <-time.After(10 * time.Millisecond):
		c.Status(http.StatusTooManyRequests)
		return
	}
	c.Status(http.StatusAccepted)
}
func (h *ContentAnalyticsHandler) Query(c *gin.Context) {
	f, ok := analyticsFilters(c)
	if !ok {
		return
	}
	out, err := h.svc.Query(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentAnalyticsHandler) Distribution(c *gin.Context) {
	f, ok := analyticsFilters(c)
	if !ok {
		return
	}
	out, err := h.svc.Distribution(c.Request.Context(), c.Query("dim"), f)
	if err != nil {
		if strings.Contains(err.Error(), "invalid analytics distribution dimension") {
			response.BadRequest(c, "invalid dim")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentAnalyticsHandler) Overview(c *gin.Context) {
	f, ok := analyticsFilters(c)
	if !ok {
		return
	}
	out, err := h.svc.Overview(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentAnalyticsHandler) Trends(c *gin.Context) {
	f, ok := analyticsFilters(c)
	if !ok {
		return
	}
	out, err := h.svc.Trends(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentAnalyticsHandler) Rankings(c *gin.Context) {
	f, ok := analyticsFilters(c)
	if !ok {
		return
	}
	if v := c.Query("page"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &f.Page); err != nil {
			response.BadRequest(c, "invalid page")
			return
		}
	}
	if v := c.Query("page_size"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &f.PageSize); err != nil {
			response.BadRequest(c, "invalid page_size")
			return
		}
	}
	f.Sort = strings.TrimSpace(c.Query("sort"))
	out, err := h.svc.Rankings(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentAnalyticsHandler) ClickThroughRate(c *gin.Context) {
	f, ok := analyticsFilters(c)
	if !ok {
		return
	}
	out, err := h.svc.CTR(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentAnalyticsHandler) Export(c *gin.Context) {
	f, ok := analyticsFilters(c)
	if !ok {
		return
	}
	out, err := h.svc.ExportCSV(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=content-analytics.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", out)
}

func analyticsFilters(c *gin.Context) (service.ContentAnalyticsFilters, bool) {
	f := service.ContentAnalyticsFilters{Locale: strings.TrimSpace(c.Query("locale")), ContentType: strings.TrimSpace(c.Query("content_type")), Placement: strings.TrimSpace(c.Query("placement"))}
	if value := strings.TrimSpace(c.Query("source_id")); value != "" {
		var sourceID int64
		if _, err := fmt.Sscan(value, &sourceID); err != nil || sourceID <= 0 {
			response.BadRequest(c, "invalid source_id")
			return f, false
		}
		f.SourceID = &sourceID
	}
	for key, target := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := strings.TrimSpace(c.Query(key)); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				response.BadRequest(c, "invalid "+key+" date")
				return f, false
			}
			*target = &t
		}
	}
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		response.BadRequest(c, "from must not be after to")
		return f, false
	}
	return f, true
}
