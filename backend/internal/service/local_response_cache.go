package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	localResponseCacheStatsQueueSize     = 4096
	localResponseCacheStatsFlushBatch    = 128
	localResponseCacheStatsFlushInterval = time.Second
	localResponseCacheStatsFlushTimeout  = 500 * time.Millisecond
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
	StatusCode  int               `json:"status_code"`
	ContentType string            `json:"content_type"`
	Body        []byte            `json:"body"`
	Headers     map[string]string `json:"headers,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type LocalResponseCacheStore interface {
	GetLocalResponse(ctx context.Context, key string) (*LocalResponseCacheEntry, error)
	SetLocalResponse(ctx context.Context, key string, entry *LocalResponseCacheEntry, ttl time.Duration) error
}

type LocalResponseCacheStatsStore interface {
	IncrLocalResponseCacheStats(ctx context.Context, field string, delta int64) error
	GetLocalResponseCacheStats(ctx context.Context) (*LocalResponseCacheStats, error)
}

type LocalResponseCacheStats struct {
	Entries  int64            `json:"entries"`
	Bytes    int64            `json:"bytes"`
	Counters map[string]int64 `json:"counters"`
}

type LocalResponseCacheLookup struct {
	Key    string
	Reason string
}

func (s *OpenAIGatewayService) LocalResponseCacheConfig(ctx context.Context) LocalResponseCacheConfig {
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
	return store.GetLocalResponse(ctx, key)
}

func (s *OpenAIGatewayService) SetLocalResponseCache(ctx context.Context, key string, entry *LocalResponseCacheEntry, ttl time.Duration) error {
	if s == nil || s.cache == nil || entry == nil {
		return nil
	}
	store, ok := s.cache.(LocalResponseCacheStore)
	if !ok {
		return nil
	}
	return store.SetLocalResponse(ctx, key, entry, ttl)
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

func (s *OpenAIGatewayService) runLocalResponseCacheStatsWriter(store LocalResponseCacheStatsStore) {
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
		case field := <-s.localResponseCacheStatsQueue:
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
	if cfg.MaxTemperature >= 0 {
		if t := gjson.GetBytes(body, "temperature"); t.Exists() && t.Num > cfg.MaxTemperature {
			return LocalResponseCacheLookup{Reason: "temperature_too_high"}
		}
	}
	canonical, ok := canonicalJSON(body)
	if !ok {
		return LocalResponseCacheLookup{Reason: "invalid_json"}
	}
	seed := strings.Join([]string{
		"v1",
		int64ToString(apiKeyID),
		int64ToString(*groupID),
		strings.TrimSpace(endpoint),
		strings.TrimSpace(platform),
		strings.TrimSpace(model),
		string(canonical),
	}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	return LocalResponseCacheLookup{Key: hex.EncodeToString(sum[:])}
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

func hasLocalResponseCacheUnsafeFields(body []byte) bool {
	for _, path := range []string{"tools", "tool_choice", "functions", "function_call", "parallel_tool_calls"} {
		if gjson.GetBytes(body, path).Exists() {
			return true
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
