package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type cacheManagementSettingRepoStub struct {
	values map[string]string
	sets   map[string]string
}

func (r *cacheManagementSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *cacheManagementSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *cacheManagementSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	if r.sets == nil {
		r.sets = map[string]string{}
	}
	r.values[key] = value
	r.sets[key] = value
	return nil
}

func (r *cacheManagementSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *cacheManagementSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *cacheManagementSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *cacheManagementSettingRepoStub) Delete(context.Context, string) error { return nil }

func withAdvancedCacheMemorySafeLimitProbe(t *testing.T, limit int, err error) {
	t.Helper()
	previous := advancedCacheMemorySafeLimitProbe
	advancedCacheMemorySafeLimitProbe = func(context.Context) (int, error) {
		return limit, err
	}
	t.Cleanup(func() {
		advancedCacheMemorySafeLimitProbe = previous
	})
}

func TestSettingServiceCacheManagementConfigDefaults(t *testing.T) {
	svc := NewSettingService(&cacheManagementSettingRepoStub{}, &config.Config{})

	cfg, err := svc.GetCacheManagementConfig(context.Background())

	require.NoError(t, err)
	require.False(t, cfg.GlobalEnabled)
	require.False(t, cfg.Platforms.OpenAI.Enabled)
	require.False(t, cfg.Platforms.Claude.Enabled)
	require.False(t, cfg.Platforms.Gemini.Enabled)
	require.Equal(t, 600, cfg.TTLSeconds)
	require.Equal(t, 256*1024, cfg.MaxRequestBytes)
	require.Equal(t, 512*1024, cfg.MaxResponseBytes)
	require.Equal(t, 0.3, cfg.MaxTemperature)
	require.Empty(t, cfg.ModelAllowlist)
	require.Empty(t, cfg.ModelBlocklist)
	require.Equal(t, "X-Sub2API-Cache-Control", cfg.BypassHeader.Name)
	require.Equal(t, "bypass", cfg.BypassHeader.Value)
}

func TestSettingServiceUpdateCacheManagementConfigNormalizesAndPersists(t *testing.T) {
	repo := &cacheManagementSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	cfg, err := svc.UpdateCacheManagementConfig(context.Background(), CacheManagementConfig{
		GlobalEnabled: true,
		Platforms: CacheManagementPlatformsConfig{
			OpenAI: CacheManagementPlatformConfig{Enabled: true},
			Claude: CacheManagementPlatformConfig{Enabled: true},
		},
		TTLSeconds:       3600,
		MaxRequestBytes:  1024,
		MaxResponseBytes: 10 * 1024 * 1024,
		MaxTemperature:   1.5,
		ModelAllowlist:   []string{" gpt-4o ", "", "GPT-4o", "claude-3"},
		ModelBlocklist:   []string{" gemini-pro ", "gemini-pro"},
		BypassHeader: CacheManagementBypassHeader{
			Name:  "client-name",
			Value: "client-value",
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"gpt-4o", "claude-3"}, cfg.ModelAllowlist)
	require.Equal(t, []string{"gemini-pro"}, cfg.ModelBlocklist)
	require.Equal(t, "X-Sub2API-Cache-Control", cfg.BypassHeader.Name)
	require.Equal(t, "bypass", cfg.BypassHeader.Value)
	require.Contains(t, repo.sets, SettingKeyCacheManagementConfig)

	var stored CacheManagementConfig
	require.NoError(t, json.Unmarshal([]byte(repo.sets[SettingKeyCacheManagementConfig]), &stored))
	require.Equal(t, cfg, stored)
}

func TestSettingServiceUpdateCacheManagementConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CacheManagementConfig)
	}{
		{name: "ttl too low", mutate: func(c *CacheManagementConfig) { c.TTLSeconds = 59 }},
		{name: "request too low", mutate: func(c *CacheManagementConfig) { c.MaxRequestBytes = 1023 }},
		{name: "request too high", mutate: func(c *CacheManagementConfig) { c.MaxRequestBytes = 5*1024*1024 + 1 }},
		{name: "response too low", mutate: func(c *CacheManagementConfig) { c.MaxResponseBytes = 1023 }},
		{name: "response too high", mutate: func(c *CacheManagementConfig) { c.MaxResponseBytes = 10*1024*1024 + 1 }},
		{name: "temperature too high", mutate: func(c *CacheManagementConfig) { c.MaxTemperature = 2.1 }},
		{name: "allow block conflict", mutate: func(c *CacheManagementConfig) {
			c.ModelAllowlist = []string{"gpt-4o"}
			c.ModelBlocklist = []string{" GPT-4O "}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultCacheManagementConfig()
			tt.mutate(&cfg)
			svc := NewSettingService(&cacheManagementSettingRepoStub{}, &config.Config{})

			_, err := svc.UpdateCacheManagementConfig(context.Background(), cfg)

			require.Error(t, err)
			require.Equal(t, "CACHE_CONFIG_INVALID", infraerrors.Reason(err))
		})
	}
}

func TestSettingServiceAdvancedCacheConfigDefaults(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 0, errors.New("probe unavailable"))
	svc := NewSettingService(&cacheManagementSettingRepoStub{}, &config.Config{})

	cfg, err := svc.GetAdvancedCacheConfig(context.Background())

	require.NoError(t, err)
	require.False(t, cfg.AdvancedCacheEnabled)
	require.Empty(t, cfg.GrayScope.APIKeyIDs)
	require.Empty(t, cfg.GrayScope.GroupIDs)
	require.Empty(t, cfg.GrayScope.Models)
	require.Equal(t, 512, cfg.RedisCapacityMB)
	require.Equal(t, 2048, cfg.MemorySafeLimitMB)
	require.True(t, cfg.CompressionEnabled)
	require.Equal(t, 64, cfg.CompressionThresholdKB)
	require.Equal(t, "LRU", cfg.EvictionPolicy)
	require.Equal(t, "1h", cfg.HotWindow)
	require.Equal(t, 5, cfg.HotThreshold)
	require.True(t, cfg.CostSavingEnabled)
	require.True(t, cfg.UpstreamPromptCacheEnabled)
}

func TestSettingServiceUpdateAdvancedCacheConfigNormalizesDerivesAndPersists(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 4096, nil)
	repo := &cacheManagementSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	cfg, err := svc.UpdateAdvancedCacheConfig(context.Background(), AdvancedCacheConfig{
		AdvancedCacheEnabled:       true,
		GrayScope:                  AdvancedCacheGrayScope{APIKeyIDs: []int64{3, 1, 3}, GroupIDs: []int64{8, 8, 2}, Models: []string{" gpt-4o ", "", "GPT-4o", "claude-3"}},
		RedisCapacityMB:            768,
		MemorySafeLimitMB:          64,
		CompressionEnabled:         true,
		CompressionThresholdKB:     128,
		EvictionPolicy:             " LFU ",
		HotWindow:                  " 6h ",
		HotThreshold:               10,
		CostSavingEnabled:          true,
		UpstreamPromptCacheEnabled: true,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{3, 1}, cfg.GrayScope.APIKeyIDs)
	require.Equal(t, []int64{8, 2}, cfg.GrayScope.GroupIDs)
	require.Equal(t, []string{"gpt-4o", "claude-3"}, cfg.GrayScope.Models)
	require.Equal(t, 4096, cfg.MemorySafeLimitMB)
	require.Equal(t, "LFU", cfg.EvictionPolicy)
	require.Equal(t, "6h", cfg.HotWindow)
	require.Contains(t, repo.sets, SettingKeyAdvancedCacheConfig)

	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(repo.sets[SettingKeyAdvancedCacheConfig]), &stored))
	require.NotContains(t, stored, "memory_safe_limit_mb")

	got, err := svc.GetAdvancedCacheConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, cfg, got)
}

func TestSettingServiceUpdateAdvancedCacheConfigValidation(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	tests := []struct {
		name   string
		mutate func(*AdvancedCacheConfig)
	}{
		{name: "negative api key id", mutate: func(c *AdvancedCacheConfig) { c.GrayScope.APIKeyIDs = []int64{1, -1} }},
		{name: "negative group id", mutate: func(c *AdvancedCacheConfig) { c.GrayScope.GroupIDs = []int64{-2} }},
		{name: "redis capacity too low", mutate: func(c *AdvancedCacheConfig) { c.RedisCapacityMB = 63 }},
		{name: "redis capacity above memory safe limit", mutate: func(c *AdvancedCacheConfig) { c.RedisCapacityMB = 2049 }},
		{name: "compression threshold too low", mutate: func(c *AdvancedCacheConfig) { c.CompressionThresholdKB = 0 }},
		{name: "compression threshold above max response", mutate: func(c *AdvancedCacheConfig) { c.CompressionThresholdKB = 10*1024 + 1 }},
		{name: "invalid eviction policy", mutate: func(c *AdvancedCacheConfig) { c.EvictionPolicy = "FIFO" }},
		{name: "invalid hot window", mutate: func(c *AdvancedCacheConfig) { c.HotWindow = "30m" }},
		{name: "hot threshold too low", mutate: func(c *AdvancedCacheConfig) { c.HotThreshold = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultAdvancedCacheConfig()
			tt.mutate(&cfg)
			svc := NewSettingService(&cacheManagementSettingRepoStub{}, &config.Config{})

			_, err := svc.UpdateAdvancedCacheConfig(context.Background(), cfg)

			require.Error(t, err)
			require.Equal(t, "ADVANCED_CACHE_CONFIG_INVALID", infraerrors.Reason(err))
		})
	}
}

func TestSettingServiceAdvancedCacheCompressionThresholdUsesCacheConfigMaxResponse(t *testing.T) {
	withAdvancedCacheMemorySafeLimitProbe(t, 2048, nil)
	repo := &cacheManagementSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	cacheCfg := DefaultCacheManagementConfig()
	cacheCfg.MaxResponseBytes = 1024
	_, err := svc.UpdateCacheManagementConfig(context.Background(), cacheCfg)
	require.NoError(t, err)

	advancedCfg := DefaultAdvancedCacheConfig()
	advancedCfg.CompressionThresholdKB = 10 * 1024
	_, err = svc.UpdateAdvancedCacheConfig(context.Background(), advancedCfg)
	require.NoError(t, err)

	advancedCfg.CompressionThresholdKB = 10*1024 + 1
	_, err = svc.UpdateAdvancedCacheConfig(context.Background(), advancedCfg)
	require.Error(t, err)
	require.Equal(t, "ADVANCED_CACHE_CONFIG_INVALID", infraerrors.Reason(err))
}
