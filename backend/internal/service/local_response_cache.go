package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	LocalResponseCacheHeader       = "X-Sub2API-Cache"
	LocalResponseCacheHeaderHit    = "hit"
	LocalResponseCacheHeaderMiss   = "miss"
	LocalResponseCacheHeaderBypass = "bypass"

	DefaultLocalResponseCacheTTL             = 10 * time.Minute
	DefaultLocalResponseCacheMaxRequestBytes = 256 * 1024
	DefaultLocalResponseCacheMaxBodyBytes    = 512 * 1024
	DefaultLocalResponseCacheMaxTemperature  = 0.3
	LocalResponseCacheRuleVersion            = "v2"
	LocalResponseCacheLegacyRuleVersion      = "v1"

	localResponseCacheStatsQueueSize     = 4096
	localResponseCacheStatsFlushBatch    = 128
	localResponseCacheStatsFlushInterval = time.Second
	localResponseCacheStatsFlushTimeout  = 500 * time.Millisecond

	localResponseCacheMinuteStatsQueueSize    = 4096
	localResponseCacheMinuteStatsFlushBatch   = 128
	localResponseCacheMinuteStatsFlushTimeout = 500 * time.Millisecond
)

type LocalResponseCacheConfig struct {
	Enabled        bool
	TTL            time.Duration
	MaxRequestSize int
	MaxBodySize    int
	MaxTemperature float64
}

func DefaultLocalResponseCacheConfig() LocalResponseCacheConfig {
	return LocalResponseCacheConfig{
		TTL:            DefaultLocalResponseCacheTTL,
		MaxRequestSize: DefaultLocalResponseCacheMaxRequestBytes,
		MaxBodySize:    DefaultLocalResponseCacheMaxBodyBytes,
		MaxTemperature: DefaultLocalResponseCacheMaxTemperature,
	}
}

type LocalResponseCacheEntry struct {
	StatusCode      int               `json:"status_code"`
	ContentType     string            `json:"content_type"`
	Body            []byte            `json:"body"`
	Headers         map[string]string `json:"headers,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	Encoding        string            `json:"encoding,omitempty"`
	RawBodyBytes    int64             `json:"raw_body_bytes,omitempty"`
	StoredBodyBytes int64             `json:"stored_body_bytes,omitempty"`
	LastAccessedAt  time.Time         `json:"last_accessed_at,omitempty"`
	HitCount        int64             `json:"hit_count,omitempty"`
	Platform        string            `json:"platform,omitempty"`
	Model           string            `json:"model,omitempty"`
	GroupID         *int64            `json:"group_id,omitempty"`
	APIKeyID        *int64            `json:"api_key_id,omitempty"`
}

type LocalResponseCacheStore interface {
	GetLocalResponse(ctx context.Context, key string) (*LocalResponseCacheEntry, error)
	SetLocalResponse(ctx context.Context, key string, entry *LocalResponseCacheEntry, ttl time.Duration) error
}

type LocalResponseCacheStatsStore interface {
	IncrLocalResponseCacheStats(ctx context.Context, field string, delta int64) error
	GetLocalResponseCacheStats(ctx context.Context) (*LocalResponseCacheStats, error)
}

type LocalResponseCacheEvictionStore interface {
	EvictLocalResponseCache(ctx context.Context, req LocalResponseCacheEvictionRequest) (*LocalResponseCacheEvictionResult, error)
}

type LocalResponseCacheHotspotStore interface {
	RecordLocalResponseCacheHotspot(ctx context.Context, event LocalResponseCacheHotspotEvent) error
	ListLocalResponseCacheHotspots(ctx context.Context, filter LocalResponseCacheHotspotFilter) ([]LocalResponseCacheHotspot, error)
}

type LocalResponseCacheEvictionRequest struct {
	CapacityBytes int64
	Policy        string
}

type LocalResponseCacheEvictionResult struct {
	BytesBefore  int64
	BytesAfter   int64
	ScannedKeys  int64
	DeletedKeys  int64
	DeletedBytes int64
}

type LocalResponseCacheHotspotEvent struct {
	CacheKey  string
	Platform  string
	Model     string
	GroupID   *int64
	APIKeyID  *int64
	HitTokens int64
	HitAt     time.Time
	Window    time.Duration
}

type LocalResponseCacheHotspotFilter struct {
	Window   time.Duration
	Limit    int
	Platform string
	Model    string
	GroupID  *int64
	APIKeyID *int64
}

type LocalResponseCacheHotspot struct {
	Rank      int       `json:"rank"`
	CacheKey  string    `json:"cache_key"`
	Platform  string    `json:"platform"`
	Model     string    `json:"model"`
	GroupID   *int64    `json:"group_id,omitempty"`
	APIKeyID  *int64    `json:"api_key_id,omitempty"`
	HitCount  int64     `json:"hit_count"`
	HitTokens int64     `json:"hit_tokens"`
	LastHitAt time.Time `json:"last_hit_at"`
}

type LocalResponseCacheMinuteStatsStore interface {
	RecordLocalResponseCacheMinuteStats(ctx context.Context, entries []*LocalResponseCacheMinuteStatEvent) error
}

type LocalResponseCacheStats struct {
	Entries  int64            `json:"entries"`
	Bytes    int64            `json:"bytes"`
	Counters map[string]int64 `json:"counters"`
}

type LocalResponseCacheLookup struct {
	Key       string
	LegacyKey string
	Reason    string
	Platform  string
	Model     string
	GroupID   *int64
	APIKeyID  *int64
}

type LocalResponseCacheMinuteStatEvent struct {
	At              time.Time
	Platform        string
	Model           string
	GroupID         *int64
	APIKeyID        *int64
	CacheType       string
	TotalRequests   int64
	Candidate       bool
	Hit             bool
	BypassReason    string
	StoreSuccess    bool
	StoreSkipReason string
	InputTokens     int64
	OutputTokens    int64
	HitTokens       int64
}

type LocalResponseCacheKeyOptions struct {
	Headers map[string]string
}

func (s *OpenAIGatewayService) LocalResponseCacheConfig(ctx context.Context) LocalResponseCacheConfig {
	cfg := DefaultLocalResponseCacheConfig()
	if s == nil || s.settingService == nil {
		return cfg
	}
	cfg.Enabled = s.settingService.IsLocalResponseCacheEnabled(ctx)
	return cfg
}

func (s *GatewayService) LocalResponseCacheConfig(ctx context.Context) LocalResponseCacheConfig {
	cfg := DefaultLocalResponseCacheConfig()
	if s == nil || s.settingService == nil {
		return cfg
	}
	cfg.Enabled = s.settingService.IsLocalResponseCacheEnabled(ctx)
	return cfg
}

func (s *OpenAIGatewayService) GetLocalResponseCache(ctx context.Context, key string) (*LocalResponseCacheEntry, error) {
	if s == nil || s.cache == nil {
		return nil, nil
	}
	store, ok := s.cache.(LocalResponseCacheStore)
	if !ok {
		return nil, nil
	}
	entry, err := store.GetLocalResponse(ctx, key)
	if err != nil || entry == nil {
		return entry, err
	}
	restored, err := restoreLocalResponseCacheEntryForRead(entry)
	if err != nil {
		s.RecordLocalResponseCacheStat(ctx, "decompression_failed")
		return nil, err
	}
	return restored, nil
}

func (s *GatewayService) GetLocalResponseCache(ctx context.Context, key string) (*LocalResponseCacheEntry, error) {
	if s == nil || s.cache == nil {
		return nil, nil
	}
	store, ok := s.cache.(LocalResponseCacheStore)
	if !ok {
		return nil, nil
	}
	entry, err := store.GetLocalResponse(ctx, key)
	if err != nil || entry == nil {
		return entry, err
	}
	restored, err := restoreLocalResponseCacheEntryForRead(entry)
	if err != nil {
		s.RecordLocalResponseCacheStat(ctx, "decompression_failed")
		return nil, err
	}
	return restored, nil
}

func (s *OpenAIGatewayService) SetLocalResponseCache(ctx context.Context, key string, entry *LocalResponseCacheEntry, ttl time.Duration) error {
	if s == nil || s.cache == nil || entry == nil {
		return nil
	}
	store, ok := s.cache.(LocalResponseCacheStore)
	if !ok {
		return nil
	}
	prepared, compressed, err := prepareLocalResponseCacheEntryForStore(ctx, s.settingService, entry)
	if err != nil {
		s.RecordLocalResponseCacheStat(ctx, "compression_failed")
		return err
	}
	if compressed {
		s.RecordLocalResponseCacheStat(ctx, "compression_success")
	}
	if err := store.SetLocalResponse(ctx, key, prepared, ttl); err != nil {
		return err
	}
	s.scheduleLocalResponseCacheEviction(ctx, prepared)
	return nil
}

func (s *GatewayService) SetLocalResponseCache(ctx context.Context, key string, entry *LocalResponseCacheEntry, ttl time.Duration) error {
	if s == nil || s.cache == nil || entry == nil {
		return nil
	}
	store, ok := s.cache.(LocalResponseCacheStore)
	if !ok {
		return nil
	}
	prepared, compressed, err := prepareLocalResponseCacheEntryForStore(ctx, s.settingService, entry)
	if err != nil {
		s.RecordLocalResponseCacheStat(ctx, "compression_failed")
		return err
	}
	if compressed {
		s.RecordLocalResponseCacheStat(ctx, "compression_success")
	}
	if err := store.SetLocalResponse(ctx, key, prepared, ttl); err != nil {
		return err
	}
	s.scheduleLocalResponseCacheEviction(ctx, prepared)
	return nil
}

func (s *OpenAIGatewayService) scheduleLocalResponseCacheEviction(ctx context.Context, entry *LocalResponseCacheEntry) {
	if s == nil {
		return
	}
	scheduleLocalResponseCacheEviction(ctx, s.settingService, entry, s.cache, s.RecordLocalResponseCacheStat)
}

func (s *GatewayService) scheduleLocalResponseCacheEviction(ctx context.Context, entry *LocalResponseCacheEntry) {
	if s == nil {
		return
	}
	scheduleLocalResponseCacheEviction(ctx, s.settingService, entry, s.cache, s.RecordLocalResponseCacheStat)
}

func (s *OpenAIGatewayService) RecordLocalResponseCacheHotspot(ctx context.Context, lookup LocalResponseCacheLookup, hitTokens int64) {
	if s == nil {
		return
	}
	recordLocalResponseCacheHotspot(ctx, s.settingService, s.cache, lookup, hitTokens)
}

func (s *GatewayService) RecordLocalResponseCacheHotspot(ctx context.Context, lookup LocalResponseCacheLookup, hitTokens int64) {
	if s == nil {
		return
	}
	recordLocalResponseCacheHotspot(ctx, s.settingService, s.cache, lookup, hitTokens)
}

func (s *GeminiMessagesCompatService) RecordLocalResponseCacheHotspot(ctx context.Context, lookup LocalResponseCacheLookup, hitTokens int64) {
	if s == nil {
		return
	}
	recordLocalResponseCacheHotspot(ctx, s.settingService, s.cache, lookup, hitTokens)
}

func recordLocalResponseCacheHotspot(ctx context.Context, settingService *SettingService, cache GatewayCache, lookup LocalResponseCacheLookup, hitTokens int64) {
	if cache == nil || strings.TrimSpace(lookup.Key) == "" {
		return
	}
	store, ok := cache.(LocalResponseCacheHotspotStore)
	if !ok {
		return
	}
	entry := &LocalResponseCacheEntry{Platform: lookup.Platform, Model: lookup.Model, GroupID: lookup.GroupID, APIKeyID: lookup.APIKeyID}
	cfg, ok := advancedLocalResponseCacheConfigForEntry(ctx, settingService, entry)
	if !ok {
		return
	}
	event := LocalResponseCacheHotspotEvent{
		CacheKey:  lookup.Key,
		Platform:  lookup.Platform,
		Model:     lookup.Model,
		GroupID:   lookup.GroupID,
		APIKeyID:  lookup.APIKeyID,
		HitTokens: hitTokens,
		HitAt:     time.Now(),
		Window:    advancedCacheHotWindowDuration(cfg.HotWindow),
	}
	go func() {
		hotspotCtx, cancel := context.WithTimeout(context.Background(), localResponseCacheHotspotTimeout)
		defer cancel()
		_ = store.RecordLocalResponseCacheHotspot(hotspotCtx, event)
	}()
}

func advancedCacheHotWindowDuration(raw string) time.Duration {
	switch strings.TrimSpace(raw) {
	case "15m":
		return 15 * time.Minute
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

func scheduleLocalResponseCacheEviction(ctx context.Context, settingService *SettingService, entry *LocalResponseCacheEntry, cache GatewayCache, recordStat func(context.Context, string)) {
	if cache == nil || entry == nil {
		return
	}
	store, ok := cache.(LocalResponseCacheEvictionStore)
	if !ok {
		return
	}
	cfg, ok := advancedLocalResponseCacheConfigForEntry(ctx, settingService, entry)
	if !ok || cfg.RedisCapacityMB <= 0 {
		return
	}
	req := LocalResponseCacheEvictionRequest{
		CapacityBytes: int64(cfg.RedisCapacityMB) * 1024 * 1024,
		Policy:        cfg.EvictionPolicy,
	}
	go func() {
		evictCtx, cancel := context.WithTimeout(context.Background(), localResponseCacheEvictionTimeout)
		defer cancel()
		result, err := store.EvictLocalResponseCache(evictCtx, req)
		if err != nil {
			if recordStat != nil {
				recordStat(context.Background(), "eviction_failed")
			}
			return
		}
		if result != nil && result.DeletedKeys > 0 && recordStat != nil {
			recordStat(context.Background(), "eviction_success")
		}
	}()
}

const localResponseCacheEncodingGzip = "gzip"

const localResponseCacheEvictionTimeout = 2 * time.Second
const localResponseCacheHotspotTimeout = 500 * time.Millisecond

func prepareLocalResponseCacheEntryForStore(ctx context.Context, settingService *SettingService, entry *LocalResponseCacheEntry) (*LocalResponseCacheEntry, bool, error) {
	prepared := cloneLocalResponseCacheEntry(entry)
	if prepared == nil || len(prepared.Body) == 0 || strings.TrimSpace(prepared.Encoding) != "" {
		return prepared, false, nil
	}
	cfg, ok := advancedLocalResponseCacheConfigForEntry(ctx, settingService, prepared)
	if !ok || !cfg.CompressionEnabled {
		return prepared, false, nil
	}
	thresholdBytes := cfg.CompressionThresholdKB * 1024
	if thresholdBytes <= 0 || len(prepared.Body) <= thresholdBytes {
		return prepared, false, nil
	}
	compressedBody, err := compressLocalResponseCacheBody(prepared.Body)
	if err != nil {
		return nil, false, err
	}
	prepared.RawBodyBytes = int64(len(prepared.Body))
	prepared.StoredBodyBytes = int64(len(compressedBody))
	prepared.Encoding = localResponseCacheEncodingGzip
	prepared.Body = compressedBody
	return prepared, true, nil
}

func restoreLocalResponseCacheEntryForRead(entry *LocalResponseCacheEntry) (*LocalResponseCacheEntry, error) {
	if entry == nil {
		return nil, nil
	}
	encoding := strings.TrimSpace(strings.ToLower(entry.Encoding))
	if encoding == "" {
		return entry, nil
	}
	if encoding != localResponseCacheEncodingGzip {
		return nil, fmt.Errorf("unsupported local response cache encoding: %s", entry.Encoding)
	}
	restored := cloneLocalResponseCacheEntry(entry)
	body, err := decompressLocalResponseCacheBody(restored.Body)
	if err != nil {
		return nil, err
	}
	restored.Body = body
	restored.Encoding = ""
	if restored.RawBodyBytes == 0 {
		restored.RawBodyBytes = int64(len(body))
	}
	if restored.StoredBodyBytes == 0 {
		restored.StoredBodyBytes = int64(len(entry.Body))
	}
	return restored, nil
}

func advancedLocalResponseCacheConfigForEntry(ctx context.Context, settingService *SettingService, entry *LocalResponseCacheEntry) (AdvancedCacheConfig, bool) {
	if settingService == nil || entry == nil {
		return AdvancedCacheConfig{}, false
	}
	cfg, err := settingService.GetAdvancedCacheConfig(ctx)
	if err != nil || !cfg.AdvancedCacheEnabled {
		return cfg, false
	}
	if !localResponseCacheGrayScopeMatchesEntry(cfg.GrayScope, entry) {
		return cfg, false
	}
	return cfg, true
}

func localResponseCacheGrayScopeMatchesEntry(scope AdvancedCacheGrayScope, entry *LocalResponseCacheEntry) bool {
	if entry == nil {
		return false
	}
	if len(scope.APIKeyIDs) > 0 {
		if entry.APIKeyID != nil && int64InServiceList(*entry.APIKeyID, scope.APIKeyIDs) {
			return true
		}
	}
	if len(scope.GroupIDs) > 0 {
		if entry.GroupID != nil && int64InServiceList(*entry.GroupID, scope.GroupIDs) {
			return true
		}
	}
	if len(scope.Models) > 0 {
		if stringInServiceList(entry.Model, scope.Models) {
			return true
		}
	}
	return false
}

func compressLocalResponseCacheBody(body []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressLocalResponseCacheBody(body []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func cloneLocalResponseCacheEntry(entry *LocalResponseCacheEntry) *LocalResponseCacheEntry {
	if entry == nil {
		return nil
	}
	cloned := *entry
	if entry.Body != nil {
		cloned.Body = append([]byte(nil), entry.Body...)
	}
	if entry.Headers != nil {
		cloned.Headers = make(map[string]string, len(entry.Headers))
		for k, v := range entry.Headers {
			cloned.Headers[k] = v
		}
	}
	cloned.GroupID = cloneLocalResponseCacheInt64Ptr(entry.GroupID)
	cloned.APIKeyID = cloneLocalResponseCacheInt64Ptr(entry.APIKeyID)
	return &cloned
}

func cloneLocalResponseCacheInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringInServiceList(value string, values []string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	for _, item := range values {
		if strings.TrimSpace(strings.ToLower(item)) == value {
			return true
		}
	}
	return false
}

func int64InServiceList(value int64, values []int64) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) RecordLocalResponseCacheStat(ctx context.Context, field string) {
	field = strings.TrimSpace(field)
	if s == nil || s.cache == nil || field == "" {
		return
	}
	store, ok := s.cache.(LocalResponseCacheStatsStore)
	if !ok {
		return
	}
	s.localResponseCacheStatsOnce.Do(func() {
		s.localResponseCacheStatsQueue = make(chan string, localResponseCacheStatsQueueSize)
		go s.runLocalResponseCacheStatsWriter(store)
	})
	select {
	case s.localResponseCacheStatsQueue <- field:
	default:
	}
}

func (s *GatewayService) RecordLocalResponseCacheStat(ctx context.Context, field string) {
	field = strings.TrimSpace(field)
	if s == nil || s.cache == nil || field == "" {
		return
	}
	store, ok := s.cache.(LocalResponseCacheStatsStore)
	if !ok {
		return
	}
	s.localResponseCacheStatsOnce.Do(func() {
		s.localResponseCacheStatsQueue = make(chan string, localResponseCacheStatsQueueSize)
		go runLocalResponseCacheStatsWriter(store, s.localResponseCacheStatsQueue)
	})
	select {
	case s.localResponseCacheStatsQueue <- field:
	default:
	}
}

func (s *OpenAIGatewayService) RecordLocalResponseCacheMinuteStat(ctx context.Context, event LocalResponseCacheMinuteStatEvent) {
	if s == nil || s.cache == nil {
		return
	}
	store, ok := s.cache.(LocalResponseCacheMinuteStatsStore)
	if !ok {
		return
	}
	normalizeLocalResponseCacheMinuteStatEvent(&event)
	if event.Platform == "" || event.Model == "" || event.CacheType == "" {
		return
	}
	s.localResponseCacheMinuteStatsOnce.Do(func() {
		s.localResponseCacheMinuteStatsQueue = make(chan LocalResponseCacheMinuteStatEvent, localResponseCacheMinuteStatsQueueSize)
		go s.runLocalResponseCacheMinuteStatsWriter(store)
	})
	select {
	case s.localResponseCacheMinuteStatsQueue <- event:
	default:
	}
}

func (s *OpenAIGatewayService) runLocalResponseCacheMinuteStatsWriter(store LocalResponseCacheMinuteStatsStore) {
	ticker := time.NewTicker(localResponseCacheStatsFlushInterval)
	defer ticker.Stop()
	pending := make([]*LocalResponseCacheMinuteStatEvent, 0, localResponseCacheMinuteStatsFlushBatch)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = make([]*LocalResponseCacheMinuteStatEvent, 0, localResponseCacheMinuteStatsFlushBatch)
		ctx, cancel := context.WithTimeout(context.Background(), localResponseCacheMinuteStatsFlushTimeout)
		defer cancel()
		_ = store.RecordLocalResponseCacheMinuteStats(ctx, batch)
	}
	for {
		select {
		case event := <-s.localResponseCacheMinuteStatsQueue:
			normalizeLocalResponseCacheMinuteStatEvent(&event)
			pending = append(pending, &event)
			if len(pending) >= localResponseCacheMinuteStatsFlushBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func normalizeLocalResponseCacheMinuteStatEvent(event *LocalResponseCacheMinuteStatEvent) {
	if event == nil {
		return
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.At = event.At.UTC().Truncate(time.Minute)
	event.Platform = strings.TrimSpace(strings.ToLower(event.Platform))
	event.Model = strings.TrimSpace(event.Model)
	if event.CacheType == "" {
		event.CacheType = "exact"
	}
	event.CacheType = strings.TrimSpace(strings.ToLower(event.CacheType))
	event.BypassReason = strings.TrimSpace(event.BypassReason)
	event.StoreSkipReason = strings.TrimSpace(event.StoreSkipReason)
	if event.TotalRequests <= 0 {
		event.TotalRequests = 1
	}
	if event.InputTokens < 0 {
		event.InputTokens = 0
	}
	if event.OutputTokens < 0 {
		event.OutputTokens = 0
	}
	if event.HitTokens < 0 {
		event.HitTokens = 0
	}
}

func (s *OpenAIGatewayService) runLocalResponseCacheStatsWriter(store LocalResponseCacheStatsStore) {
	runLocalResponseCacheStatsWriter(store, s.localResponseCacheStatsQueue)
}

func runLocalResponseCacheStatsWriter(store LocalResponseCacheStatsStore, queue <-chan string) {
	ticker := time.NewTicker(localResponseCacheStatsFlushInterval)
	defer ticker.Stop()
	pending := map[string]int64{}
	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = map[string]int64{}
		ctx, cancel := context.WithTimeout(context.Background(), localResponseCacheStatsFlushTimeout)
		defer cancel()
		for field, delta := range batch {
			_ = store.IncrLocalResponseCacheStats(ctx, field, delta)
			if ctx.Err() != nil {
				return
			}
		}
	}
	for {
		select {
		case field := <-queue:
			if strings.TrimSpace(field) == "" {
				continue
			}
			pending[field]++
			if len(pending) >= localResponseCacheStatsFlushBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (s *OpenAIGatewayService) GetLocalResponseCacheStats(ctx context.Context) (*LocalResponseCacheStats, error) {
	if s == nil || s.cache == nil {
		return &LocalResponseCacheStats{Counters: map[string]int64{}}, nil
	}
	store, ok := s.cache.(LocalResponseCacheStatsStore)
	if !ok {
		return &LocalResponseCacheStats{Counters: map[string]int64{}}, nil
	}
	stats, err := store.GetLocalResponseCacheStats(ctx)
	if stats == nil {
		stats = &LocalResponseCacheStats{Counters: map[string]int64{}}
	}
	if stats.Counters == nil {
		stats.Counters = map[string]int64{}
	}
	return stats, err
}

func BuildLocalResponseCacheLookup(cfg LocalResponseCacheConfig, apiKeyID int64, groupID *int64, endpoint, platform, model string, body []byte, explicitBypass bool) LocalResponseCacheLookup {
	return BuildLocalResponseCacheLookupWithOptions(cfg, apiKeyID, groupID, endpoint, platform, model, body, explicitBypass, LocalResponseCacheKeyOptions{})
}

func BuildLocalResponseCacheLookupWithOptions(cfg LocalResponseCacheConfig, apiKeyID int64, groupID *int64, endpoint, platform, model string, body []byte, explicitBypass bool, opts LocalResponseCacheKeyOptions) LocalResponseCacheLookup {
	if !cfg.Enabled {
		return LocalResponseCacheLookup{Reason: "disabled"}
	}
	if explicitBypass {
		return LocalResponseCacheLookup{Reason: "explicit_bypass"}
	}
	if apiKeyID <= 0 {
		return LocalResponseCacheLookup{Reason: "no_api_key"}
	}
	if groupID == nil || *groupID <= 0 {
		return LocalResponseCacheLookup{Reason: "no_group"}
	}
	if cfg.MaxRequestSize > 0 && len(body) > cfg.MaxRequestSize {
		return LocalResponseCacheLookup{Reason: "request_too_large"}
	}
	if !gjson.ValidBytes(body) {
		return LocalResponseCacheLookup{Reason: "invalid_json"}
	}
	if hasLocalResponseCacheUnsafeFields(body) {
		return LocalResponseCacheLookup{Reason: "tools_or_functions"}
	}
	if hasLocalResponseCacheSensitiveContent(body) {
		return LocalResponseCacheLookup{Reason: "sensitive_content"}
	}
	if cfg.MaxTemperature >= 0 && localResponseCacheTemperatureTooHigh(body, cfg.MaxTemperature) {
		return LocalResponseCacheLookup{Reason: "temperature_too_high"}
	}
	canonical, ok := canonicalJSON(body)
	if !ok {
		return LocalResponseCacheLookup{Reason: "invalid_json"}
	}
	requestHash := buildLocalResponseCacheRequestHash(canonical, endpoint, platform, model, opts.Headers)
	key := strings.Join([]string{
		"cache",
		LocalResponseCacheRuleVersion,
		strings.TrimSpace(platform),
		int64ToString(apiKeyID),
		int64ToString(*groupID),
		strings.TrimSpace(endpoint),
		strings.TrimSpace(model),
		requestHash,
	}, ":")
	return LocalResponseCacheLookup{
		Key:       key,
		LegacyKey: buildLegacyLocalResponseCacheKey(apiKeyID, *groupID, endpoint, platform, model, canonical),
		Platform:  strings.TrimSpace(platform),
		Model:     strings.TrimSpace(model),
		GroupID:   groupID,
		APIKeyID:  &apiKeyID,
	}
}

func LocalResponseCacheKeyHeadersFromHTTP(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, name := range []string{"Content-Type", "Accept", "OpenAI-Beta", "Anthropic-Version", "Anthropic-Beta", "X-Goog-Api-Version"} {
		value := strings.TrimSpace(header.Get(name))
		if value != "" {
			out[strings.ToLower(name)] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildLocalResponseCacheRequestHash(canonical []byte, endpoint, platform, model string, headers map[string]string) string {
	normalized := map[string]any{
		"endpoint": strings.TrimSpace(endpoint),
		"platform": strings.TrimSpace(platform),
		"model":    strings.TrimSpace(model),
		"body":     json.RawMessage(canonical),
	}
	if len(headers) > 0 {
		clean := map[string]string{}
		for k, v := range headers {
			key := strings.ToLower(strings.TrimSpace(k))
			value := strings.TrimSpace(v)
			if key != "" && value != "" {
				clean[key] = value
			}
		}
		if len(clean) > 0 {
			normalized["headers"] = clean
		}
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		payload = canonical
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func buildLegacyLocalResponseCacheKey(apiKeyID, groupID int64, endpoint, platform, model string, canonical []byte) string {
	seed := strings.Join([]string{
		LocalResponseCacheLegacyRuleVersion,
		int64ToString(apiKeyID),
		int64ToString(groupID),
		strings.TrimSpace(endpoint),
		strings.TrimSpace(platform),
		strings.TrimSpace(model),
		string(canonical),
	}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func canonicalJSON(body []byte) ([]byte, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return out, true
}

func localResponseCacheTemperatureTooHigh(body []byte, maxTemperature float64) bool {
	for _, path := range []string{"temperature", "generationConfig.temperature", "generation_config.temperature"} {
		if t := gjson.GetBytes(body, path); t.Exists() && t.Num > maxTemperature {
			return true
		}
	}
	return false
}

func hasLocalResponseCacheUnsafeFields(body []byte) bool {
	for _, path := range []string{"tools", "tool_choice", "functions", "function_call", "functionCall", "function_response", "functionResponse", "parallel_tool_calls", "thinking"} {
		if gjson.GetBytes(body, path).Exists() {
			return true
		}
	}
	if containsLocalResponseCacheToolUse(body) {
		return true
	}
	if containsLocalResponseCacheMultimodalContent(body) {
		return true
	}
	return false
}

func containsLocalResponseCacheToolUse(body []byte) bool {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return false
	}
	return containsLocalResponseCacheToolUseValue(value)
}

func containsLocalResponseCacheToolUseValue(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if _, ok := v["functionCall"]; ok {
			return true
		}
		if _, ok := v["functionResponse"]; ok {
			return true
		}
		if itemType, ok := v["type"].(string); ok && strings.EqualFold(strings.TrimSpace(itemType), "tool_use") {
			return true
		}
		for _, child := range v {
			if containsLocalResponseCacheToolUseValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsLocalResponseCacheToolUseValue(child) {
				return true
			}
		}
	}
	return false
}

func containsLocalResponseCacheMultimodalContent(body []byte) bool {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return false
	}
	return containsLocalResponseCacheMultimodalValue(value)
}

func containsLocalResponseCacheMultimodalValue(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if _, ok := v["inlineData"]; ok {
			return true
		}
		if _, ok := v["fileData"]; ok {
			return true
		}
		if _, ok := v["inline_data"]; ok {
			return true
		}
		if _, ok := v["file_data"]; ok {
			return true
		}
		if itemType, ok := v["type"].(string); ok {
			switch strings.ToLower(strings.TrimSpace(itemType)) {
			case "image", "document", "file", "audio", "video", "input_image", "input_file":
				return true
			}
		}
		if _, ok := v["source"]; ok {
			if itemType, _ := v["type"].(string); itemType != "" && !strings.EqualFold(strings.TrimSpace(itemType), "text") {
				return true
			}
		}
		for _, child := range v {
			if containsLocalResponseCacheMultimodalValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsLocalResponseCacheMultimodalValue(child) {
				return true
			}
		}
	}
	return false
}

func hasLocalResponseCacheSensitiveContent(body []byte) bool {
	lower := strings.ToLower(string(body))
	for _, token := range []string{"authorization", "api_key", "apikey", "password", "private key", "secret_access_key", "access_token", "refresh_token", "cookie"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
