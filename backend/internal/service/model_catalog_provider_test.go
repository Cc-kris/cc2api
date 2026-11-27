package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type failingPricingRemoteClient struct{}

func (failingPricingRemoteClient) FetchPricingJSON(context.Context, string) ([]byte, error) {
	return nil, errors.New("catalog provider unavailable")
}

func (failingPricingRemoteClient) FetchHashText(context.Context, string) (string, error) {
	return "", errors.New("catalog hash unavailable")
}

func TestPricingProviderForPlatform(t *testing.T) {
	// 单 provider 平台保持原样，gemini 必须同时命中价格表里的
	// "gemini" 与 "vertex_ai-*"：只映射成 "google" 会得到空目录。
	tests := map[string][]string{
		"openai":      {"openai"},
		"anthropic":   {"anthropic"},
		"gemini":      {"gemini", "vertex_ai-language-models", "vertex_ai-embedding-models"},
		"antigravity": {"anthropic"},
		"grok":        {"xai"},
	}
	for platform, expected := range tests {
		actual, ok := PricingProvidersForPlatform(platform)
		require.True(t, ok, platform)
		require.Equal(t, expected, actual, platform)
	}
	_, ok := PricingProvidersForPlatform("unknown")
	require.False(t, ok)

	// 兼容入口：多 provider 时以逗号连接。
	merged, ok := PricingProviderForPlatform(" GEMINI ")
	require.True(t, ok)
	require.Equal(t, "gemini,vertex_ai-language-models,vertex_ai-embedding-models", merged)
	single, ok := PricingProviderForPlatform("openai")
	require.True(t, ok)
	require.Equal(t, "openai", single)
}

func TestPricingServiceModelCatalogSnapshotAggregatesMultipleProviders(t *testing.T) {
	updatedAt := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	pricing := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gemini-4-argon":     {LiteLLMProvider: "gemini"},
			"gemini-2.5-flash":   {LiteLLMProvider: "vertex_ai-language-models"},
			"text-embedding-004": {LiteLLMProvider: "vertex_ai-embedding-models"},
			"gpt-5.6":            {LiteLLMProvider: "openai"},
		},
		localHash: "catalog-checksum", lastUpdated: updatedAt,
	}
	catalog := NewPricingServiceModelCatalog(pricing)
	snapshot, err := catalog.SnapshotForPlatform("gemini")
	require.NoError(t, err)
	require.Equal(t, "gemini,vertex_ai-language-models,vertex_ai-embedding-models", snapshot.Provider)
	require.Equal(t, []string{"gemini-2.5-flash", "gemini-4-argon", "text-embedding-004"}, snapshot.Models)
}

func TestNaturalModelLessOrdersNumbersCaseAndUnicodeDeterministically(t *testing.T) {
	models := []string{"模型10", "GPT-2", "gpt-10", "模型2", "gpt-02", "GPT-1"}
	sort.Slice(models, func(i, j int) bool { return naturalModelLess(models[i], models[j]) })
	require.Equal(t, []string{"GPT-1", "GPT-2", "gpt-02", "gpt-10", "模型2", "模型10"}, models)
}

func TestPricingServiceModelCatalogSnapshotIsFilteredSortedAndCopied(t *testing.T) {
	updatedAt := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	pricing := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"GPT-5.6": {LiteLLMProvider: "openai"},
			"gpt-5.5": {LiteLLMProvider: "OpenAI"},
			"claude":  {LiteLLMProvider: "anthropic"},
		},
		localHash: "catalog-checksum", lastUpdated: updatedAt,
	}
	catalog := NewPricingServiceModelCatalog(pricing)
	snapshot, err := catalog.SnapshotForPlatform(" OPENAI ")
	require.NoError(t, err)
	require.Equal(t, "openai", snapshot.Provider)
	require.Equal(t, "catalog-checksum", snapshot.Checksum)
	require.Equal(t, updatedAt, snapshot.UpdatedAt)
	require.Equal(t, []string{"gpt-5.5", "GPT-5.6"}, snapshot.Models)

	snapshot.Models[0] = "mutated"
	require.Contains(t, pricing.pricingData, "gpt-5.5")
}

func TestPricingCatalogFallsBackToBundledFileWhenRemoteProviderFails(t *testing.T) {
	root := t.TempDir()
	fallback := filepath.Join(root, "fallback.json")
	require.NoError(t, os.WriteFile(fallback, []byte(`{
		"gpt-fallback":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000002,"litellm_provider":"openai"},
		"claude-fallback":{"input_cost_per_token":0.000003,"output_cost_per_token":0.000004,"litellm_provider":"anthropic"}
	}`), 0o600))
	cfg := &config.Config{}
	cfg.Pricing.DataDir = filepath.Join(root, "data")
	cfg.Pricing.FallbackFile = fallback
	cfg.Pricing.RemoteURL = "https://pricing.example.test/catalog.json"
	cfg.Pricing.UpdateIntervalHours = 24
	cfg.Pricing.HashCheckIntervalMinutes = 10
	cfg.Security.URLAllowlist.Enabled = false

	pricing := NewPricingService(cfg, failingPricingRemoteClient{})
	require.NoError(t, pricing.Initialize())
	t.Cleanup(pricing.Stop)
	catalog := NewPricingServiceModelCatalog(pricing)

	openAI, err := catalog.SnapshotForPlatform(PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-fallback"}, openAI.Models)
	anthropic, err := catalog.SnapshotForPlatform(PlatformAnthropic)
	require.NoError(t, err)
	require.Equal(t, []string{"claude-fallback"}, anthropic.Models)
	require.NotEmpty(t, openAI.Checksum)
	require.FileExists(t, filepath.Join(root, "data", "model_pricing.json"))
}
