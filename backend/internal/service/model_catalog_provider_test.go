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
	tests := map[string]string{
		"openai": "openai", "anthropic": "anthropic", "gemini": "google",
		"antigravity": "anthropic", "grok": "xai",
	}
	for platform, expected := range tests {
		actual, ok := PricingProviderForPlatform(platform)
		require.True(t, ok)
		require.Equal(t, expected, actual)
	}
	_, ok := PricingProviderForPlatform("unknown")
	require.False(t, ok)
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
