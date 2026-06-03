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
