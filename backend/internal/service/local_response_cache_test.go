package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildLocalResponseCacheLookup_GroupIsolation(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupA := int64(1)
	groupB := int64(2)
	body := []byte(`{"model":"gpt-5.5","input":"hello","temperature":0.1}`)

	a := BuildLocalResponseCacheLookup(cfg, 10, &groupA, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)
	b := BuildLocalResponseCacheLookup(cfg, 10, &groupB, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)

	require.NotEmpty(t, a.Key)
	require.NotEmpty(t, b.Key)
	require.NotEqual(t, a.Key, b.Key)
	require.True(t, strings.HasPrefix(a.Key, "cache:v2:openai:10:1:/v1/responses:gpt-5.5:"))
	require.NotEmpty(t, a.LegacyKey)
}

func TestBuildLocalResponseCacheLookup_V2Isolation(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)
	body := []byte(`{"model":"gpt-5.5","input":"hello","temperature":0.1}`)

	base := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)
	otherAPIKey := BuildLocalResponseCacheLookup(cfg, 11, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)
	otherModel := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.6", body, false)
	otherPlatform := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformAnthropic, "gpt-5.5", body, false)

	require.NotEmpty(t, base.Key)
	require.NotEqual(t, base.Key, otherAPIKey.Key)
	require.NotEqual(t, base.Key, otherModel.Key)
	require.NotEqual(t, base.Key, otherPlatform.Key)
	require.Contains(t, base.Key, ":10:1:/v1/responses:gpt-5.5:")
}

func TestBuildLocalResponseCacheLookup_HeaderAffectsV2RequestHash(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)
	body := []byte(`{"model":"gpt-5.5","input":"hello"}`)

	jsonLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"content-type": "application/json"},
	})
	sseLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"content-type": "text/event-stream"},
	})

	require.NotEmpty(t, jsonLookup.Key)
	require.NotEmpty(t, sseLookup.Key)
	require.NotEqual(t, jsonLookup.Key, sseLookup.Key)
	require.Equal(t, jsonLookup.LegacyKey, sseLookup.LegacyKey)

	claudeBetaLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/messages", PlatformAnthropic, "claude-sonnet-4.6", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"anthropic-beta": "interleaved-thinking-2025-05-14"},
	})
	claudeNoBetaLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/messages", PlatformAnthropic, "claude-sonnet-4.6", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"anthropic-version": "2023-06-01"},
	})
	require.NotEmpty(t, claudeBetaLookup.Key)
	require.NotEmpty(t, claudeNoBetaLookup.Key)
	require.NotEqual(t, claudeBetaLookup.Key, claudeNoBetaLookup.Key)
}

func TestBuildLocalResponseCacheLookup_CanonicalJSON(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)

	a := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(`{"input":"hello","model":"gpt-5.5"}`), false)
	b := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(`{"model":"gpt-5.5","input":"hello"}`), false)

	require.NotEmpty(t, a.Key)
	require.Equal(t, a.Key, b.Key)
}

func TestBuildLocalResponseCacheLookup_AcceptanceThreePlatformsExactCacheStable(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)
	cases := []struct {
		name     string
		platform string
		endpoint string
		model    string
		body     []byte
	}{
		{name: "openai", platform: PlatformOpenAI, endpoint: "/v1/responses", model: "gpt-5.5", body: []byte(`{"model":"gpt-5.5","input":"hello","temperature":0.1}`)},
		{name: "claude", platform: PlatformAnthropic, endpoint: "/v1/messages", model: "claude-sonnet-4-5", body: []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"temperature":0.1}`)},
		{name: "gemini", platform: PlatformGemini, endpoint: "/v1beta/models/gemini-2.5-pro:generateContent", model: "gemini-2.5-pro", body: []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"temperature":0.1}}`)},
	}

	seen := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := BuildLocalResponseCacheLookup(cfg, 10, &groupID, tc.endpoint, tc.platform, tc.model, tc.body, false)
			reordered := BuildLocalResponseCacheLookup(cfg, 10, &groupID, tc.endpoint, tc.platform, tc.model, tc.body, false)
			require.NotEmpty(t, first.Key)
			require.Equal(t, first.Key, reordered.Key)
			require.Contains(t, first.Key, tc.platform)
			for otherName, otherKey := range seen {
				require.NotEqual(t, otherKey, first.Key, "cache key must not cross platform: %s vs %s", otherName, tc.name)
			}
			seen[tc.name] = first.Key
		})
	}
}

func TestLocalResponseCacheAcceptanceThreePlatformsWarmupThenNineHits(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)
	apiKeyID := int64(10)
	type platformCase struct {
		name     string
		platform string
		endpoint string
		model    string
		body     []byte
		input    int64
		output   int64
	}
	cases := []platformCase{
		{name: "openai", platform: PlatformOpenAI, endpoint: "/v1/responses", model: "gpt-5.5", body: []byte(`{"model":"gpt-5.5","input":"hello","temperature":0.1}`), input: 100, output: 50},
		{name: "claude", platform: PlatformAnthropic, endpoint: "/v1/messages", model: "claude-sonnet-4-5", body: []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"temperature":0.1}`), input: 120, output: 30},
		{name: "gemini", platform: PlatformGemini, endpoint: "/v1beta/models/gemini-2.5-pro:generateContent", model: "gemini-2.5-pro", body: []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"temperature":0.1}}`), input: 90, output: 60},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newLocalResponseCacheMemoryStore()
			svc := &OpenAIGatewayService{cache: store}
			lookup := BuildLocalResponseCacheLookup(cfg, apiKeyID, &groupID, tc.endpoint, tc.platform, tc.model, tc.body, false)
			require.NotEmpty(t, lookup.Key)

			var candidateRequests int64
			var hitRequests int64
			var candidateTokens int64
			var hitTokens int64

			for i := 0; i < 10; i++ {
				candidateRequests++
				candidateTokens += tc.input + tc.output
				entry, err := svc.GetLocalResponseCache(ctx, lookup.Key)
				require.NoError(t, err)
				if i == 0 {
					require.Nil(t, entry, "first request should warm cache and miss")
					require.NoError(t, svc.SetLocalResponseCache(ctx, lookup.Key, &LocalResponseCacheEntry{
						StatusCode:  http.StatusOK,
						ContentType: "application/json",
						Body:        []byte(`{"id":"cached","usage":{"input_tokens":1,"output_tokens":1}}`),
						Platform:    tc.platform,
						Model:       tc.model,
						GroupID:     &groupID,
						APIKeyID:    &apiKeyID,
					}, time.Minute))
					continue
				}
				require.NotNil(t, entry, "repeat request %d should hit cache", i+1)
				require.Equal(t, http.StatusOK, entry.StatusCode)
				hitRequests++
				hitTokens += tc.input + tc.output
			}

			require.Equal(t, 90.0, percent(hitRequests, candidateRequests))
			require.Equal(t, 90.0, percent(hitTokens, candidateTokens))
		})
	}
}

func TestBuildLocalResponseCacheLookup_BypassRules(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "tool", body: `{"model":"gpt-5.5","tools":[]}`, want: "tools_or_functions"},
		{name: "claude tool_use", body: `{"model":"claude-sonnet-4.6","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read","input":{}}]}]}`, want: "tools_or_functions"},
		{name: "claude thinking", body: `{"model":"claude-sonnet-4.6","thinking":{"type":"enabled"},"messages":[{"role":"user","content":"hi"}]}`, want: "tools_or_functions"},
		{name: "claude image", body: `{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"}}]}]}`, want: "tools_or_functions"},
		{name: "claude document", body: `{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"abc"}}]}]}`, want: "tools_or_functions"},
		{name: "temperature", body: `{"model":"gpt-5.5","temperature":0.9}`, want: "temperature_too_high"},
		{name: "sensitive", body: `{"model":"gpt-5.5","input":"password=abc"}`, want: "sensitive_content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(tc.body), false)
			require.Empty(t, lookup.Key)
			require.Equal(t, tc.want, lookup.Reason)
		})
	}
}

func TestBuildLocalResponseCacheLookup_Disabled(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	groupID := int64(1)
	lookup := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(`{"model":"gpt-5.5"}`), false)
	require.Empty(t, lookup.Key)
	require.Equal(t, "disabled", lookup.Reason)
}

func TestLocalResponseCacheAdvancedCompressionRoundTrip(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	ctx := context.Background()
	settings := NewSettingService(&cacheManagementSettingRepoStub{}, nil)
	cfg := DefaultAdvancedCacheConfig()
	cfg.AdvancedCacheEnabled = true
	cfg.GrayScope.Models = []string{"gpt-5.5"}
	cfg.CompressionEnabled = true
	cfg.CompressionThresholdKB = 1
	_, err := settings.UpdateAdvancedCacheConfig(ctx, cfg)
	require.NoError(t, err)

	store := newLocalResponseCacheMemoryStore()
	svc := &OpenAIGatewayService{cache: store, settingService: settings}
	body := []byte(strings.Repeat("advanced-cache-compression", 120))
	groupID := int64(10)
	apiKeyID := int64(20)
	entry := &LocalResponseCacheEntry{
		StatusCode:  200,
		ContentType: "application/json",
		Body:        body,
		Headers:     map[string]string{"Content-Type": "application/json"},
		CreatedAt:   time.Now(),
		Platform:    PlatformOpenAI,
		Model:       "gpt-5.5",
		GroupID:     &groupID,
		APIKeyID:    &apiKeyID,
	}

	require.NoError(t, svc.SetLocalResponseCache(ctx, "key", entry, time.Minute))
	stored := store.entries["key"]
	require.NotNil(t, stored)
	require.Equal(t, localResponseCacheEncodingGzip, stored.Encoding)
	require.Equal(t, int64(len(body)), stored.RawBodyBytes)
	require.Equal(t, int64(len(stored.Body)), stored.StoredBodyBytes)
	require.Less(t, len(stored.Body), len(body))
	require.Equal(t, body, entry.Body, "caller entry must not be mutated")

	got, err := svc.GetLocalResponseCache(ctx, "key")
	require.NoError(t, err)
	require.Equal(t, body, got.Body)
	require.Empty(t, got.Encoding)
	require.Equal(t, int64(len(body)), got.RawBodyBytes)
	require.Equal(t, int64(len(stored.Body)), got.StoredBodyBytes)
}

func TestLocalResponseCacheAdvancedCompressionRequiresGrayScopeMatch(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	ctx := context.Background()
	settings := NewSettingService(&cacheManagementSettingRepoStub{}, nil)
	cfg := DefaultAdvancedCacheConfig()
	cfg.AdvancedCacheEnabled = true
	cfg.GrayScope.Models = []string{"gpt-5.5"}
	cfg.CompressionEnabled = true
	cfg.CompressionThresholdKB = 1
	_, err := settings.UpdateAdvancedCacheConfig(ctx, cfg)
	require.NoError(t, err)

	store := newLocalResponseCacheMemoryStore()
	svc := &OpenAIGatewayService{cache: store, settingService: settings}
	body := []byte(strings.Repeat("advanced-cache-compression", 120))
	entry := &LocalResponseCacheEntry{StatusCode: 200, ContentType: "application/json", Body: body, Model: "gpt-4o"}

	require.NoError(t, svc.SetLocalResponseCache(ctx, "key", entry, time.Minute))
	stored := store.entries["key"]
	require.NotNil(t, stored)
	require.Empty(t, stored.Encoding)
	require.Equal(t, body, stored.Body)
}

func TestLocalResponseCacheAdvancedCompressionEmptyGrayScopeDoesNotAffectTraffic(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	ctx := context.Background()
	settings := NewSettingService(&cacheManagementSettingRepoStub{}, nil)
	cfg := DefaultAdvancedCacheConfig()
	cfg.AdvancedCacheEnabled = true
	cfg.CompressionEnabled = true
	cfg.CompressionThresholdKB = 1
	_, err := settings.UpdateAdvancedCacheConfig(ctx, cfg)
	require.NoError(t, err)

	store := newLocalResponseCacheMemoryStore()
	svc := &OpenAIGatewayService{cache: store, settingService: settings}
	body := []byte(strings.Repeat("advanced-cache-compression", 120))
	entry := &LocalResponseCacheEntry{StatusCode: 200, ContentType: "application/json", Body: body, Model: "gpt-5.5"}

	require.NoError(t, svc.SetLocalResponseCache(ctx, "key", entry, time.Minute))
	stored := store.entries["key"]
	require.NotNil(t, stored)
	require.Empty(t, stored.Encoding)
	require.Equal(t, body, stored.Body)
}

func TestLocalResponseCacheAdvancedCompressionReadFailureReturnsError(t *testing.T) {
	_, err := restoreLocalResponseCacheEntryForRead(&LocalResponseCacheEntry{
		StatusCode:  200,
		ContentType: "application/json",
		Body:        []byte("not-gzip"),
		Encoding:    localResponseCacheEncodingGzip,
	})
	require.Error(t, err)
}

type localResponseCacheMemoryStore struct {
	entries       map[string]*LocalResponseCacheEntry
	evictionCalls chan LocalResponseCacheEvictionRequest
	hotspotCalls  chan LocalResponseCacheHotspotEvent
}

func newLocalResponseCacheMemoryStore() *localResponseCacheMemoryStore {
	return &localResponseCacheMemoryStore{entries: map[string]*LocalResponseCacheEntry{}}
}

func (s *localResponseCacheMemoryStore) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, nil
}

func (s *localResponseCacheMemoryStore) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (s *localResponseCacheMemoryStore) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *localResponseCacheMemoryStore) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

func (s *localResponseCacheMemoryStore) GetLocalResponse(ctx context.Context, key string) (*LocalResponseCacheEntry, error) {
	return s.entries[key], nil
}

func (s *localResponseCacheMemoryStore) SetLocalResponse(ctx context.Context, key string, entry *LocalResponseCacheEntry, ttl time.Duration) error {
	s.entries[key] = cloneLocalResponseCacheEntry(entry)
	return nil
}

func (s *localResponseCacheMemoryStore) IncrLocalResponseCacheStats(ctx context.Context, field string, delta int64) error {
	return nil
}

func (s *localResponseCacheMemoryStore) GetLocalResponseCacheStats(context.Context) (*LocalResponseCacheStats, error) {
	return &LocalResponseCacheStats{}, nil
}

func TestLocalResponseCacheEvictionSchedulesOnlyForAdvancedGrayScopeMatch(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	ctx := context.Background()
	settings := NewSettingService(&cacheManagementSettingRepoStub{}, nil)
	cfg := DefaultAdvancedCacheConfig()
	cfg.AdvancedCacheEnabled = true
	cfg.GrayScope.Models = []string{"gpt-5.5"}
	cfg.RedisCapacityMB = 128
	_, err := settings.UpdateAdvancedCacheConfig(ctx, cfg)
	require.NoError(t, err)

	store := newLocalResponseCacheMemoryStore()
	store.evictionCalls = make(chan LocalResponseCacheEvictionRequest, 1)
	svc := &OpenAIGatewayService{cache: store, settingService: settings}
	entry := &LocalResponseCacheEntry{StatusCode: 200, ContentType: "application/json", Body: []byte(`{"ok":true}`), Model: "gpt-5.5"}

	require.NoError(t, svc.SetLocalResponseCache(ctx, "key", entry, time.Minute))

	select {
	case req := <-store.evictionCalls:
		require.Equal(t, int64(128*1024*1024), req.CapacityBytes)
		require.Equal(t, "LRU", req.Policy)
	case <-time.After(time.Second):
		t.Fatal("expected eviction to be scheduled for gray-scope matched advanced cache entry")
	}
}

func TestLocalResponseCacheEvictionNotScheduledWhenGrayScopeEmpty(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	ctx := context.Background()
	settings := NewSettingService(&cacheManagementSettingRepoStub{}, nil)
	cfg := DefaultAdvancedCacheConfig()
	cfg.AdvancedCacheEnabled = true
	cfg.RedisCapacityMB = 128
	_, err := settings.UpdateAdvancedCacheConfig(ctx, cfg)
	require.NoError(t, err)

	store := newLocalResponseCacheMemoryStore()
	store.evictionCalls = make(chan LocalResponseCacheEvictionRequest, 1)
	svc := &OpenAIGatewayService{cache: store, settingService: settings}
	entry := &LocalResponseCacheEntry{StatusCode: 200, ContentType: "application/json", Body: []byte(`{"ok":true}`), Model: "gpt-5.5"}

	require.NoError(t, svc.SetLocalResponseCache(ctx, "key", entry, time.Minute))

	select {
	case <-store.evictionCalls:
		t.Fatal("eviction must not be scheduled when gray scope is empty")
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *localResponseCacheMemoryStore) EvictLocalResponseCache(ctx context.Context, req LocalResponseCacheEvictionRequest) (*LocalResponseCacheEvictionResult, error) {
	if s.evictionCalls != nil {
		select {
		case s.evictionCalls <- req:
		default:
		}
	}
	return &LocalResponseCacheEvictionResult{}, nil
}

func TestLocalResponseCacheHotspotRecordsOnlyForAdvancedGrayScopeMatch(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	ctx := context.Background()
	settings := NewSettingService(&cacheManagementSettingRepoStub{}, nil)
	cfg := DefaultAdvancedCacheConfig()
	cfg.AdvancedCacheEnabled = true
	cfg.GrayScope.Models = []string{"gpt-5.5"}
	cfg.HotWindow = "6h"
	_, err := settings.UpdateAdvancedCacheConfig(ctx, cfg)
	require.NoError(t, err)

	store := newLocalResponseCacheMemoryStore()
	store.hotspotCalls = make(chan LocalResponseCacheHotspotEvent, 1)
	svc := &OpenAIGatewayService{cache: store, settingService: settings}
	lookup := LocalResponseCacheLookup{Key: "cache-key", Platform: PlatformOpenAI, Model: "gpt-5.5"}

	svc.RecordLocalResponseCacheHotspot(ctx, lookup, 88)

	select {
	case event := <-store.hotspotCalls:
		require.Equal(t, "cache-key", event.CacheKey)
		require.Equal(t, PlatformOpenAI, event.Platform)
		require.Equal(t, "gpt-5.5", event.Model)
		require.Equal(t, int64(88), event.HitTokens)
		require.Equal(t, 6*time.Hour, event.Window)
	case <-time.After(time.Second):
		t.Fatal("expected hotspot event for gray-scope matched advanced cache hit")
	}
}

func TestLocalResponseCacheHotspotNotRecordedWhenGrayScopeEmpty(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	ctx := context.Background()
	settings := NewSettingService(&cacheManagementSettingRepoStub{}, nil)
	cfg := DefaultAdvancedCacheConfig()
	cfg.AdvancedCacheEnabled = true
	_, err := settings.UpdateAdvancedCacheConfig(ctx, cfg)
	require.NoError(t, err)

	store := newLocalResponseCacheMemoryStore()
	store.hotspotCalls = make(chan LocalResponseCacheHotspotEvent, 1)
	svc := &OpenAIGatewayService{cache: store, settingService: settings}
	lookup := LocalResponseCacheLookup{Key: "cache-key", Platform: PlatformOpenAI, Model: "gpt-5.5"}

	svc.RecordLocalResponseCacheHotspot(ctx, lookup, 88)

	select {
	case <-store.hotspotCalls:
		t.Fatal("hotspot must not be recorded when gray scope is empty")
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *localResponseCacheMemoryStore) RecordLocalResponseCacheHotspot(ctx context.Context, event LocalResponseCacheHotspotEvent) error {
	if s.hotspotCalls != nil {
		select {
		case s.hotspotCalls <- event:
		default:
		}
	}
	return nil
}

func (s *localResponseCacheMemoryStore) ListLocalResponseCacheHotspots(ctx context.Context, filter LocalResponseCacheHotspotFilter) ([]LocalResponseCacheHotspot, error) {
	return []LocalResponseCacheHotspot{}, nil
}
