package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// W4 AI-05 回归：编辑配置时前端不回传明文 Key（apiKey 为空串），保存必须保留原密钥，
// 否则一次「改个名字」就会把已存的密钥清空。
func TestAIConfigSaveKeepsStoredKeyWhenAPIKeyOmitted(t *testing.T) {
	oldPath := contentAIConfigPath
	contentAIConfigPath = filepath.Join(t.TempDir(), "ai.json")
	t.Cleanup(func() { contentAIConfigPath = oldPath })
	t.Setenv("TOTP_ENCRYPTION_KEY", strings.Repeat("ab", 32))

	require.NoError(t, saveContentAIProviderConfig(ContentAIProviderConfig{
		ID: "cfg-keep-key", Label: "Keep", Provider: "openai", IsDefault: true,
		BaseURL: "https://example.test/v1", APIKey: "sk-keep-key-0001",
		Models: []string{"model-a"}, DefaultModel: "model-a",
	}))

	before, _, revision, err := ListAIProviderConfigs()
	require.NoError(t, err)
	seeded, ok := findAIConfig(before, "cfg-keep-key")
	require.True(t, ok, "seeded config must exist")
	require.True(t, seeded.HasAPIKey, "seed config must carry a key")

	err = (&ContentArticleAdminService{}).MutateAIProviderConfig(
		context.Background(), "save", "cfg-keep-key", revision,
		ContentAIProviderConfig{
			ID: "cfg-keep-key", Label: "Keep renamed", Provider: "openai",
			BaseURL: "https://example.test/v1", Models: []string{"model-a"}, DefaultModel: "model-a",
		},
	)
	require.NoError(t, err)

	after, _, _, err := ListAIProviderConfigs()
	require.NoError(t, err)
	saved, ok := findAIConfig(after, "cfg-keep-key")
	require.True(t, ok, "config must survive the rename save")
	require.Equal(t, "Keep renamed", saved.Label)
	require.True(t, saved.HasAPIKey, "stored API key was dropped by an empty-key save")
	require.NotEmpty(t, saved.APIKeyMasked)
}

func findAIConfig(list []ContentAIProviderConfig, id string) (ContentAIProviderConfig, bool) {
	for _, c := range list {
		if c.ID == id {
			return c, true
		}
	}
	return ContentAIProviderConfig{}, false
}
