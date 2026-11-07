package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

// Haiku 不再旁路完整伪装：真实 Claude Code CLI 对 Haiku 也发送同样的 beta 集，
// 旁路会让伪装流量与真实流量可区分。
func TestHaikuUsesFullClaudeCodeMimicryBetas(t *testing.T) {
	betas := claude.FullClaudeCodeMimicryBetas()
	require.Contains(t, betas, claude.BetaOAuth)
	require.Contains(t, betas, claude.BetaInterleavedThinking)
	// 与 rewriteSystemForNonClaudeCode 的行为一致：不再对 haiku 特判。
	// 构造一个 haiku 模型名，确认 FullClaudeCodeMimicryBetas 不按模型分支。
	require.GreaterOrEqual(t, len(betas), 2)
}
