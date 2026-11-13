//go:build unit

package antigravity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// usageMetadata 缺 promptTokenCount、只有 cachedContentTokenCount 时，input_tokens 下限为 0，
// 缓存命中仍计入 cache_read_input_tokens。
func TestGeminiUsageMapping_MissingPromptTokenCountKeepsInputNonNegative(t *testing.T) {
	const geminiBody = `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":"STOP"}],` +
		`"usageMetadata":{"cachedContentTokenCount":30}}`

	t.Run("non-stream", func(t *testing.T) {
		_, usage, err := TransformGeminiToClaude([]byte(geminiBody), "gemini-3.1-pro-preview")
		require.NoError(t, err)
		require.NotNil(t, usage)
		require.Zero(t, usage.InputTokens)
		require.Equal(t, 30, usage.CacheReadInputTokens)
	})

	t.Run("stream", func(t *testing.T) {
		p := NewStreamingProcessor("gemini-3.1-pro-preview")
		out := p.ProcessLine(`data: {"response":` + geminiBody + `}`)
		require.True(t, strings.Contains(string(out), `"message_start"`))
		require.NotContains(t, string(out), `"input_tokens":-`, "message_start 的 input_tokens 不应为负")
		_, usage := p.Finish()
		require.NotNil(t, usage)
		require.Zero(t, usage.InputTokens)
		require.Equal(t, 30, usage.CacheReadInputTokens)
	})
}
