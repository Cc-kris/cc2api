package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 只移植 5.5 判定用例；EffortLevelsForModel 用例随 V05。

func TestIsOpus55OpenRouterExactAlias(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"claude-opus-5-5", "anthropic/claude-opus-5.5"} {
		require.True(t, IsOpus55(model), model)
	}
	for _, model := range []string{"claude-opus-5", "anthropic/claude-opus-5.6", "anthropic/claude-opus-5.5-preview"} {
		require.False(t, IsOpus55(model), model)
	}
}

func TestIsSonnet55(t *testing.T) {
	t.Parallel()
	for _, model := range []string{
		"claude-sonnet-5-5",
		"anthropic/claude-sonnet-5.5",
		"anthropic.claude-sonnet-5-5",
		"us.anthropic.claude-sonnet-5-5",
		"us-gov.anthropic.claude-sonnet-5-5",
		"global.anthropic.claude-sonnet-5-5-thinking",
	} {
		require.True(t, IsSonnet55(model), model)
	}
	for _, model := range []string{"claude-sonnet-5", "claude-sonnet-5-5-preview", "claude-opus-5-5"} {
		require.False(t, IsSonnet55(model), model)
	}
}

func TestIsHaiku55(t *testing.T) {
	t.Parallel()
	for _, model := range []string{
		"claude-haiku-5-5",
		"anthropic/claude-haiku-5.5",
		"anthropic.claude-haiku-5-5",
		"us.anthropic.claude-haiku-5-5",
		"global.anthropic.claude-haiku-5-5-thinking",
	} {
		require.True(t, IsHaiku55(model), model)
	}
	for _, model := range []string{"claude-haiku-5", "claude-haiku-4-5", "claude-haiku-4-5-20251001", "claude-sonnet-5-5", "claude-opus-5-5"} {
		require.False(t, IsHaiku55(model), model)
	}
}
