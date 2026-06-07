package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLocalResponseCacheCaptureWriter_CapturesStreamingWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := newLocalResponseCacheCaptureWriter(c.Writer, 1024)
	c.Writer = capture
	c.Header("Content-Type", "text/event-stream")

	c.Status(http.StatusOK)
	_, err := c.Writer.Write([]byte("data: one\n\n"))
	require.NoError(t, err)
	c.Writer.Flush()
	_, err = c.Writer.Write([]byte("data: [DONE]\n\n"))
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, capture.StatusCode())
	require.False(t, capture.overLimit)
	require.Equal(t, "data: one\n\ndata: [DONE]\n\n", capture.body.String())
	require.Equal(t, "data: one\n\ndata: [DONE]\n\n", recorder.Body.String())
}

func TestLocalResponseCacheCaptureWriter_OverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := newLocalResponseCacheCaptureWriter(c.Writer, 4)
	c.Writer = capture

	_, err := c.Writer.Write([]byte("123"))
	require.NoError(t, err)
	_, err = c.Writer.Write([]byte("456"))
	require.NoError(t, err)

	require.True(t, capture.overLimit)
	require.Equal(t, "123", capture.body.String())
	require.Equal(t, "123456", recorder.Body.String())
}

func TestIsLocalResponseCacheableContentType(t *testing.T) {
	require.True(t, isLocalResponseCacheableContentType("application/json; charset=utf-8"))
	require.True(t, isLocalResponseCacheableContentType("text/event-stream"))
	require.False(t, isLocalResponseCacheableContentType("image/png"))
}

type localResponseCacheTestStore struct {
	entries     map[string]*service.LocalResponseCacheEntry
	mu          sync.Mutex
	stats       map[string]int64
	minuteStats []*service.LocalResponseCacheMinuteStatEvent
}

func (s *localResponseCacheTestStore) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, redis.Nil
}

func (s *localResponseCacheTestStore) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (s *localResponseCacheTestStore) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *localResponseCacheTestStore) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

func (s *localResponseCacheTestStore) GetLocalResponse(_ context.Context, key string) (*service.LocalResponseCacheEntry, error) {
	if s == nil || s.entries == nil {
		return nil, redis.Nil
	}
	entry, ok := s.entries[key]
	if !ok {
		return nil, redis.Nil
	}
	return entry, nil
}

func (s *localResponseCacheTestStore) SetLocalResponse(_ context.Context, key string, entry *service.LocalResponseCacheEntry, _ time.Duration) error {
	if s.entries == nil {
		s.entries = map[string]*service.LocalResponseCacheEntry{}
	}
	s.entries[key] = entry
	return nil
}

func (s *localResponseCacheTestStore) IncrLocalResponseCacheStats(_ context.Context, field string, delta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stats == nil {
		s.stats = map[string]int64{}
	}
	s.stats[field] += delta
	return nil
}

func (s *localResponseCacheTestStore) GetLocalResponseCacheStats(context.Context) (*service.LocalResponseCacheStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counters := map[string]int64{}
	for k, v := range s.stats {
		counters[k] = v
	}
	return &service.LocalResponseCacheStats{
		Entries:  int64(len(s.entries)),
		Counters: counters,
	}, nil
}

func (s *localResponseCacheTestStore) RecordLocalResponseCacheMinuteStats(_ context.Context, entries []*service.LocalResponseCacheMinuteStatEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		cp := *entry
		s.minuteStats = append(s.minuteStats, &cp)
	}
	return nil
}

func (s *localResponseCacheTestStore) requireStatEventually(t *testing.T, field string, want int64) {
	t.Helper()
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.stats[field] == want
	}, 2*time.Second, 20*time.Millisecond)
}

func (s *localResponseCacheTestStore) requireMinuteStatEventually(t *testing.T, match func(*service.LocalResponseCacheMinuteStatEvent) bool) *service.LocalResponseCacheMinuteStatEvent {
	t.Helper()
	var got *service.LocalResponseCacheMinuteStatEvent
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, item := range s.minuteStats {
			if match(item) {
				got = item
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond)
	return got
}

func newLocalResponseCacheTestHandler(store *localResponseCacheTestStore) *OpenAIGatewayHandler {
	return &OpenAIGatewayHandler{
		gatewayService: service.NewOpenAIGatewayService(
			nil, nil, nil, nil, nil, nil,
			store,
			nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		),
	}
}

func TestTryWriteLocalResponseCacheHit_ReplaysSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &localResponseCacheTestStore{entries: map[string]*service.LocalResponseCacheEntry{
		"hit-key": {
			StatusCode:  http.StatusOK,
			ContentType: "text/event-stream",
			Body:        []byte("data: hello\n\ndata: [DONE]\n\n"),
			Headers:     map[string]string{"Content-Type": "text/event-stream"},
		},
	}}
	h := newLocalResponseCacheTestHandler(store)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	ok := h.tryWriteLocalResponseCacheHit(c, service.LocalResponseCacheLookup{Key: "hit-key"}, nil)

	require.True(t, ok)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "hit", recorder.Header().Get(service.LocalResponseCacheHeader))
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	require.Equal(t, "data: hello\n\ndata: [DONE]\n\n", recorder.Body.String())
	store.requireStatEventually(t, "lookup_hit", 1)
}

func TestTryWriteLocalResponseCacheHit_FallsBackToLegacyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &localResponseCacheTestStore{entries: map[string]*service.LocalResponseCacheEntry{
		"legacy-key": {
			StatusCode:  http.StatusOK,
			ContentType: "application/json",
			Body:        []byte(`{"id":"legacy"}`),
			Headers:     map[string]string{"Content-Type": "application/json"},
		},
	}}
	h := newLocalResponseCacheTestHandler(store)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	ok := h.tryWriteLocalResponseCacheHit(c, service.LocalResponseCacheLookup{Key: "cache:v2:openai:10:1:/v1/responses:gpt-5.5:missing", LegacyKey: "legacy-key"}, nil)

	require.True(t, ok)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "hit", recorder.Header().Get(service.LocalResponseCacheHeader))
	require.Equal(t, `{"id":"legacy"}`, recorder.Body.String())
	store.requireStatEventually(t, "lookup_hit", 1)
}

func TestTryWriteLocalResponseCacheHit_RecordsMinuteStatsWithTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(3)
	apiKeyID := int64(10)
	store := &localResponseCacheTestStore{entries: map[string]*service.LocalResponseCacheEntry{
		"hit-key": {
			StatusCode:  http.StatusOK,
			ContentType: "application/json",
			Body:        []byte(`{"usage":{"input_tokens":11,"output_tokens":7}}`),
			Headers:     map[string]string{"Content-Type": "application/json"},
		},
	}}
	h := newLocalResponseCacheTestHandler(store)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	ok := h.tryWriteLocalResponseCacheHit(c, service.LocalResponseCacheLookup{Key: "hit-key", Platform: service.PlatformOpenAI, Model: "gpt-5.5", GroupID: &groupID, APIKeyID: &apiKeyID}, nil)

	require.True(t, ok)
	got := store.requireMinuteStatEventually(t, func(item *service.LocalResponseCacheMinuteStatEvent) bool {
		return item.Hit && item.Candidate && item.Model == "gpt-5.5"
	})
	require.Equal(t, service.PlatformOpenAI, got.Platform)
	require.NotNil(t, got.GroupID)
	require.Equal(t, groupID, *got.GroupID)
	require.NotNil(t, got.APIKeyID)
	require.Equal(t, apiKeyID, *got.APIKeyID)
	require.Equal(t, int64(11), got.InputTokens)
	require.Equal(t, int64(7), got.OutputTokens)
	require.Equal(t, int64(18), got.HitTokens)
}

func TestPersistLocalResponseCache_RequiresCompleteSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &localResponseCacheTestStore{}
	h := newLocalResponseCacheTestHandler(store)
	cfg := service.DefaultLocalResponseCacheConfig()
	lookup := service.LocalResponseCacheLookup{Key: "stream-key"}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Header("Content-Type", "text/event-stream")
	capture := newLocalResponseCacheCaptureWriter(c.Writer, cfg.MaxBodySize)
	c.Writer = capture
	c.Status(http.StatusOK)
	_, err := c.Writer.Write([]byte("data: partial\n\n"))
	require.NoError(t, err)

	h.persistLocalResponseCache(c, lookup, cfg, capture, nil, nil)
	require.Empty(t, store.entries)
	store.requireStatEventually(t, "store_skip:stream_incomplete", 1)

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Header("Content-Type", "text/event-stream")
	capture = newLocalResponseCacheCaptureWriter(c.Writer, cfg.MaxBodySize)
	c.Writer = capture
	c.Status(http.StatusOK)
	_, err = c.Writer.Write([]byte("data: complete\n\ndata: [DONE]\n\n"))
	require.NoError(t, err)

	h.persistLocalResponseCache(c, lookup, cfg, capture, nil, nil)
	require.NotNil(t, store.entries["stream-key"])
	require.Equal(t, "data: complete\n\ndata: [DONE]\n\n", string(store.entries["stream-key"].Body))
	store.requireStatEventually(t, "store_success", 1)
}

func TestPersistLocalResponseCache_SkipsWriteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &localResponseCacheTestStore{}
	h := newLocalResponseCacheTestHandler(store)
	cfg := service.DefaultLocalResponseCacheConfig()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Header("Content-Type", "text/event-stream")
	capture := newLocalResponseCacheCaptureWriter(c.Writer, cfg.MaxBodySize)
	capture.writeErr = http.ErrAbortHandler
	c.Writer = capture
	c.Status(http.StatusOK)
	_, err := c.Writer.Write([]byte("data: complete\n\ndata: [DONE]\n\n"))
	require.NoError(t, err)

	h.persistLocalResponseCache(c, service.LocalResponseCacheLookup{Key: "write-error"}, cfg, capture, nil, nil)

	require.Empty(t, store.entries)
	store.requireStatEventually(t, "store_skip:write_error", 1)
}

func TestPersistLocalResponseCache_RecordsStoreSuccessMinuteStats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(3)
	apiKeyID := int64(10)
	store := &localResponseCacheTestStore{}
	h := newLocalResponseCacheTestHandler(store)
	cfg := service.DefaultLocalResponseCacheConfig()
	lookup := service.LocalResponseCacheLookup{Key: "store-key", Platform: service.PlatformOpenAI, Model: "gpt-5.5", GroupID: &groupID, APIKeyID: &apiKeyID}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Header("Content-Type", "application/json")
	capture := newLocalResponseCacheCaptureWriter(c.Writer, cfg.MaxBodySize)
	c.Writer = capture
	c.Status(http.StatusOK)
	_, err := c.Writer.Write([]byte(`{"usage":{"input_tokens":5,"output_tokens":4}}`))
	require.NoError(t, err)

	h.persistLocalResponseCache(c, lookup, cfg, capture, nil, nil)

	got := store.requireMinuteStatEventually(t, func(item *service.LocalResponseCacheMinuteStatEvent) bool {
		return item.StoreSuccess && item.Candidate && item.Model == "gpt-5.5"
	})
	require.Equal(t, int64(5), got.InputTokens)
	require.Equal(t, int64(4), got.OutputTokens)
	require.Equal(t, int64(0), got.HitTokens)
}
