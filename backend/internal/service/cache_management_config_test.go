package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	advancedCfg.CompressionThresholdKB = 1
	_, err = svc.UpdateAdvancedCacheConfig(context.Background(), advancedCfg)
	require.NoError(t, err)

	advancedCfg.CompressionThresholdKB = 2
	_, err = svc.UpdateAdvancedCacheConfig(context.Background(), advancedCfg)
	require.Error(t, err)
	require.Equal(t, "ADVANCED_CACHE_CONFIG_INVALID", infraerrors.Reason(err))

	cacheCfg = DefaultCacheManagementConfig()
	cacheCfg.MaxResponseBytes = 512 * 1024
	_, err = svc.UpdateCacheManagementConfig(context.Background(), cacheCfg)
	require.NoError(t, err)

	advancedCfg = DefaultAdvancedCacheConfig()
	advancedCfg.CompressionThresholdKB = 513
	_, err = svc.UpdateAdvancedCacheConfig(context.Background(), advancedCfg)
	require.Error(t, err)
	require.Equal(t, "ADVANCED_CACHE_CONFIG_INVALID", infraerrors.Reason(err))
}

type semanticCacheEncryptorStub struct{}

func (semanticCacheEncryptorStub) Encrypt(plaintext string) (string, error) {
	runes := []rune(plaintext)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return "enc:" + string(runes), nil
}

func (semanticCacheEncryptorStub) Decrypt(ciphertext string) (string, error) {
	plain := []rune(strings.TrimPrefix(ciphertext, "enc:"))
	for i, j := 0, len(plain)-1; i < j; i, j = i+1, j-1 {
		plain[i], plain[j] = plain[j], plain[i]
	}
	return string(plain), nil
}

func TestSettingServiceSemanticCacheConfigDefaults(t *testing.T) {
	svc := NewSettingService(&cacheManagementSettingRepoStub{}, &config.Config{})

	cfg, err := svc.GetSemanticCacheConfig(context.Background())

	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Equal(t, "observe", cfg.Stage)
	require.Empty(t, cfg.Platforms)
	require.Empty(t, cfg.ModelAllowlist)
	require.Empty(t, cfg.SemanticModelBaseURL)
	require.Empty(t, cfg.SemanticAPIKeyMasked)
	require.Empty(t, cfg.SemanticAPIKeyEncrypted)
	require.Empty(t, cfg.SemanticModelName)
	require.Equal(t, "default", cfg.Namespace)
	require.Nil(t, cfg.EmbeddingDimension)
	require.Equal(t, "v1", cfg.RuleVersion)
	require.Equal(t, 0.98, cfg.SimilarityThreshold)
	require.Equal(t, 10, cfg.MaxReuseMinutes)
	require.Equal(t, 20, cfg.MaxCandidates)
	require.Empty(t, cfg.GrayAPIKeyIDs)
	require.True(t, cfg.ReviewMode)
	require.Equal(t, 1.0, cfg.QualityRollbackThresholdPercent)
	require.False(t, cfg.AutoClosed)
}

func TestSettingServiceUpdateSemanticCacheConfigEncryptsMasksNormalizesAndPersists(t *testing.T) {
	repo := &cacheManagementSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSecretEncryptor(semanticCacheEncryptorStub{})
	dimension := 3072

	cfg, err := svc.UpdateSemanticCacheConfig(context.Background(), SemanticCacheConfig{
		Enabled:                         true,
		Stage:                           " gray ",
		Platforms:                       []string{" OpenAI ", "openai", "claude"},
		ModelAllowlist:                  []string{" gpt-5.5 ", "", "GPT-5.5", "claude-3"},
		SemanticModelBaseURL:            " https://semantic.example.com/v1 ",
		SemanticAPIKey:                  "sk-semantic-secret",
		SemanticModelName:               " text-embedding-3-large ",
		Namespace:                       " tenant-a ",
		EmbeddingDimension:              &dimension,
		RuleVersion:                     " v2 ",
		SimilarityThreshold:             0.9755,
		MaxReuseMinutes:                 30,
		MaxCandidates:                   50,
		GrayAPIKeyIDs:                   []int64{9, 3, 9},
		ReviewMode:                      true,
		QualityRollbackThresholdPercent: 2.25,
	})

	require.NoError(t, err)
	require.Equal(t, "gray", cfg.Stage)
	require.Equal(t, []string{"openai", "claude"}, cfg.Platforms)
	require.Equal(t, []string{"gpt-5.5", "claude-3"}, cfg.ModelAllowlist)
	require.Equal(t, "https://semantic.example.com/v1", cfg.SemanticModelBaseURL)
	require.Equal(t, "****cret", cfg.SemanticAPIKeyMasked)
	require.Empty(t, cfg.SemanticAPIKeyEncrypted)
	require.Empty(t, cfg.SemanticAPIKey)
	require.Equal(t, "text-embedding-3-large", cfg.SemanticModelName)
	require.Equal(t, "tenant-a", cfg.Namespace)
	require.Equal(t, []int64{9, 3}, cfg.GrayAPIKeyIDs)
	require.Contains(t, repo.sets, SettingKeySemanticCacheConfig)

	raw := repo.sets[SettingKeySemanticCacheConfig]
	require.NotContains(t, raw, "sk-semantic-secret")
	require.NotContains(t, raw, "semantic_api_key_masked")
	require.Contains(t, raw, "enc:terces-citnames-ks")

	got, err := svc.GetSemanticCacheConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "****cret", got.SemanticAPIKeyMasked)
	require.Empty(t, got.SemanticAPIKeyEncrypted)
	require.Empty(t, got.SemanticAPIKey)
}

func TestSettingServiceUpdateSemanticCacheConfigPreservesAPIKey(t *testing.T) {
	repo := &cacheManagementSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSecretEncryptor(semanticCacheEncryptorStub{})
	seed := validSemanticCacheConfig("sk-old-secret")
	_, err := svc.UpdateSemanticCacheConfig(context.Background(), seed)
	require.NoError(t, err)

	update := validSemanticCacheConfig("")
	update.SemanticModelName = "text-embedding-3-small"
	cfg, err := svc.UpdateSemanticCacheConfig(context.Background(), update)

	require.NoError(t, err)
	require.Equal(t, "****cret", cfg.SemanticAPIKeyMasked)
	require.Contains(t, repo.values[SettingKeySemanticCacheConfig], "enc:terces-dlo-ks")
	require.NotContains(t, repo.values[SettingKeySemanticCacheConfig], "sk-old-secret")
}

func TestSettingServiceUpdateSemanticCacheConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SemanticCacheConfig)
	}{
		{name: "invalid stage", mutate: func(c *SemanticCacheConfig) { c.Stage = "draft" }},
		{name: "invalid platform", mutate: func(c *SemanticCacheConfig) { c.Platforms = []string{"openai", "bedrock"} }},
		{name: "invalid url", mutate: func(c *SemanticCacheConfig) { c.SemanticModelBaseURL = "ftp://semantic.example.com" }},
		{name: "base url too long", mutate: func(c *SemanticCacheConfig) { c.SemanticModelBaseURL = "https://" + strings.Repeat("a", 489) + ".com" }},
		{name: "model too long", mutate: func(c *SemanticCacheConfig) { c.SemanticModelName = strings.Repeat("m", 101) }},
		{name: "namespace too long", mutate: func(c *SemanticCacheConfig) { c.Namespace = strings.Repeat("n", 101) }},
		{name: "dimension invalid", mutate: func(c *SemanticCacheConfig) { d := 0; c.EmbeddingDimension = &d }},
		{name: "similarity too low", mutate: func(c *SemanticCacheConfig) { c.SimilarityThreshold = 0.8999 }},
		{name: "similarity too many decimals", mutate: func(c *SemanticCacheConfig) { c.SimilarityThreshold = 0.98765 }},
		{name: "reuse too high", mutate: func(c *SemanticCacheConfig) { c.MaxReuseMinutes = 1441 }},
		{name: "candidates too low", mutate: func(c *SemanticCacheConfig) { c.MaxCandidates = -1 }},
		{name: "quality rollback invalid", mutate: func(c *SemanticCacheConfig) { c.QualityRollbackThresholdPercent = 100.001 }},
		{name: "negative gray id", mutate: func(c *SemanticCacheConfig) { c.GrayAPIKeyIDs = []int64{1, -1} }},
		{name: "enabled missing url", mutate: func(c *SemanticCacheConfig) { c.SemanticModelBaseURL = "" }},
		{name: "enabled missing model", mutate: func(c *SemanticCacheConfig) { c.SemanticModelName = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cacheManagementSettingRepoStub{}
			svc := NewSettingService(repo, &config.Config{})
			svc.SetSecretEncryptor(semanticCacheEncryptorStub{})
			cfg := validSemanticCacheConfig("sk-semantic-secret")
			tt.mutate(&cfg)

			_, err := svc.UpdateSemanticCacheConfig(context.Background(), cfg)

			require.Error(t, err)
			require.Equal(t, "SEMANTIC_CACHE_CONFIG_INVALID", infraerrors.Reason(err))
		})
	}
}

func TestSettingServiceUpdateSemanticCacheConfigRequiresEncryptorWhenKeyProvided(t *testing.T) {
	svc := NewSettingService(&cacheManagementSettingRepoStub{}, &config.Config{})

	_, err := svc.UpdateSemanticCacheConfig(context.Background(), validSemanticCacheConfig("sk-semantic-secret"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "secret encryptor not initialized")
}

func validSemanticCacheConfig(apiKey string) SemanticCacheConfig {
	return SemanticCacheConfig{
		Enabled:                         true,
		Stage:                           "observe",
		Platforms:                       []string{"openai"},
		ModelAllowlist:                  []string{"gpt-5.5"},
		SemanticModelBaseURL:            "https://semantic.example.com/v1",
		SemanticAPIKey:                  apiKey,
		SemanticModelName:               "text-embedding-3-large",
		Namespace:                       "default",
		RuleVersion:                     "v1",
		SimilarityThreshold:             0.98,
		MaxReuseMinutes:                 10,
		MaxCandidates:                   20,
		GrayAPIKeyIDs:                   []int64{12},
		ReviewMode:                      true,
		QualityRollbackThresholdPercent: 1.0,
	}
}
