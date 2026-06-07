package handler

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type localResponseCacheCaptureWriter struct {
	gin.ResponseWriter
	status      int
	body        bytes.Buffer
	maxBodySize int
	overLimit   bool
	writeErr    error
}

func newLocalResponseCacheCaptureWriter(w gin.ResponseWriter, maxBodySize int) *localResponseCacheCaptureWriter {
	return &localResponseCacheCaptureWriter{ResponseWriter: w, maxBodySize: maxBodySize}
}

func (w *localResponseCacheCaptureWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *localResponseCacheCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		w.writeErr = err
		return n, err
	}
	if n < len(data) {
		w.writeErr = http.ErrShortBody
		return n, nil
	}
	if w.maxBodySize <= 0 || w.body.Len()+len(data) <= w.maxBodySize {
		_, _ = w.body.Write(data)
	} else {
		w.overLimit = true
	}
	return n, nil
}

func (w *localResponseCacheCaptureWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *localResponseCacheCaptureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *localResponseCacheCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *localResponseCacheCaptureWriter) StatusCode() int {
	if w.status != 0 {
		return w.status
	}
	if w.ResponseWriter.Status() != 0 {
		return w.ResponseWriter.Status()
	}
	return http.StatusOK
}

func (h *OpenAIGatewayHandler) prepareLocalResponseCache(c *gin.Context, apiKey *service.APIKey, endpoint, model string, body []byte) (service.LocalResponseCacheLookup, service.LocalResponseCacheConfig) {
	cfg := h.gatewayService.LocalResponseCacheConfig(c.Request.Context())
	explicitBypass := strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Sub2API-Cache-Control")), "bypass")
	lookup := service.BuildLocalResponseCacheLookupWithOptions(cfg, apiKey.ID, apiKey.GroupID, endpoint, service.PlatformOpenAI, model, body, explicitBypass, service.LocalResponseCacheKeyOptions{
		Headers: service.LocalResponseCacheKeyHeadersFromHTTP(c.Request.Header),
	})
	if lookup.Key == "" {
		c.Header(service.LocalResponseCacheHeader, service.LocalResponseCacheHeaderBypass)
		if cfg.Enabled {
			h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "lookup_bypass:"+lookup.Reason)
		}
	}
	return lookup, cfg
}

func (h *OpenAIGatewayHandler) tryWriteLocalResponseCacheHit(c *gin.Context, lookup service.LocalResponseCacheLookup, reqLog *zap.Logger) bool {
	if lookup.Key == "" {
		return false
	}
	entry, err := h.gatewayService.GetLocalResponseCache(c.Request.Context(), lookup.Key)
	if errors.Is(err, redis.Nil) && strings.TrimSpace(lookup.LegacyKey) != "" {
		entry, err = h.gatewayService.GetLocalResponseCache(c.Request.Context(), lookup.LegacyKey)
	}
	if err != nil {
		if !errors.Is(err, redis.Nil) && reqLog != nil {
			reqLog.Warn("local_response_cache.get_failed", zap.Error(err))
		}
		if errors.Is(err, redis.Nil) {
			h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "lookup_miss")
		} else {
			h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "lookup_error")
		}
		return false
	}
	if entry == nil || len(entry.Body) == 0 {
		h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "lookup_miss")
		return false
	}
	for k, v := range entry.Headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			c.Header(k, v)
		}
	}
	if entry.ContentType != "" {
		c.Header("Content-Type", entry.ContentType)
	}
	c.Header(service.LocalResponseCacheHeader, service.LocalResponseCacheHeaderHit)
	c.Status(entry.StatusCode)
	_, _ = c.Writer.Write(entry.Body)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "lookup_hit")
	return true
}

func (h *OpenAIGatewayHandler) installLocalResponseCacheCapture(c *gin.Context, lookup service.LocalResponseCacheLookup, cfg service.LocalResponseCacheConfig) *localResponseCacheCaptureWriter {
	if lookup.Key == "" {
		return nil
	}
	c.Header(service.LocalResponseCacheHeader, service.LocalResponseCacheHeaderMiss)
	capture := newLocalResponseCacheCaptureWriter(c.Writer, cfg.MaxBodySize)
	c.Writer = capture
	return capture
}

func (h *OpenAIGatewayHandler) persistLocalResponseCache(c *gin.Context, lookup service.LocalResponseCacheLookup, cfg service.LocalResponseCacheConfig, capture *localResponseCacheCaptureWriter, err error, reqLog *zap.Logger) {
	if lookup.Key == "" || capture == nil {
		return
	}
	if err != nil {
		h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_skip:forward_error")
		return
	}
	if capture.overLimit {
		h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_skip:body_too_large")
		return
	}
	if capture.writeErr != nil {
		h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_skip:write_error")
		return
	}
	status := capture.StatusCode()
	if status != http.StatusOK || capture.body.Len() == 0 {
		if status != http.StatusOK {
			h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_skip:status_not_ok")
		} else {
			h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_skip:empty_body")
		}
		return
	}
	contentType := c.Writer.Header().Get("Content-Type")
	if contentType == "" {
		contentType = c.GetHeader("Content-Type")
	}
	if !isLocalResponseCacheableContentType(contentType) {
		h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_skip:content_type")
		return
	}
	if isLocalResponseCacheStreamingContentType(contentType) && !isLocalResponseCacheCompleteSSE(capture.body.Bytes()) {
		h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_skip:stream_incomplete")
		return
	}
	entry := &service.LocalResponseCacheEntry{
		StatusCode:  status,
		ContentType: contentType,
		Body:        append([]byte(nil), capture.body.Bytes()...),
		Headers: map[string]string{
			"Content-Type": contentType,
		},
		CreatedAt: time.Now(),
	}
	if setErr := h.gatewayService.SetLocalResponseCache(c.Request.Context(), lookup.Key, entry, cfg.TTL); setErr != nil {
		if reqLog != nil {
			reqLog.Warn("local_response_cache.set_failed", zap.Error(setErr))
		}
		h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_failed")
		return
	}
	h.gatewayService.RecordLocalResponseCacheStat(c.Request.Context(), "store_success")
}

func isLocalResponseCacheableContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "application/json") || strings.Contains(ct, "text/event-stream")
}

func isLocalResponseCacheStreamingContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

func isLocalResponseCacheCompleteSSE(body []byte) bool {
	text := string(body)
	return strings.Contains(text, "data: [DONE]") ||
		strings.Contains(text, "event: response.completed") ||
		strings.Contains(text, "event: response.done") ||
		strings.Contains(text, "event: message_stop") ||
		strings.Contains(text, `"type":"response.completed"`) ||
		strings.Contains(text, `"type":"response.done"`) ||
		strings.Contains(text, `"type":"message_stop"`)
}

var _ gin.ResponseWriter = (*localResponseCacheCaptureWriter)(nil)
var _ http.Flusher = (*localResponseCacheCaptureWriter)(nil)
