package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/stretchr/testify/require"
)

// TestBuiltinGeminiModelNamesPassUpstreamPathGuard 是 V01-R02 的上线风险检查：
// 代码里内置的模型名、默认模型映射（映射前和映射后）都必须是合规的上游 URL 路径片段，
// 否则收紧校验后这些默认配置会直接 400。生产数据库里管理员自定义的映射不在此覆盖。
func TestBuiltinGeminiModelNamesPassUpstreamPathGuard(t *testing.T) {
	names := map[string]string{}
	add := func(source, name string) {
		names[name] = source
	}

	for _, m := range gemini.DefaultModels() {
		add("gemini.DefaultModels", strings.TrimPrefix(m.Name, "models/"))
	}
	for _, m := range geminicli.DefaultModels {
		add("geminicli.DefaultModels", m.ID)
	}
	add("geminicli.DefaultTestModel", geminicli.DefaultTestModel)
	for _, m := range antigravity.DefaultModels() {
		add("antigravity.DefaultModels", m.ID)
	}
	for _, m := range antigravity.DefaultGeminiModels() {
		add("antigravity.DefaultGeminiModels", strings.TrimPrefix(m.Name, "models/"))
	}
	for from, to := range domain.DefaultAntigravityModelMapping {
		add("domain.DefaultAntigravityModelMapping(key)", from)
		add("domain.DefaultAntigravityModelMapping(value)", to)
	}
	add("monitorPrimaryModelForProvider(gemini)", monitorPrimaryModelForProvider(MonitorProviderGemini))
	add("monitorPrimaryModelForProvider(antigravity)", monitorPrimaryModelForProvider(MonitorProviderAntigravity))
	for _, name := range []string{"gemini-3-flash", "gemini-3.1-pro-high", "gemini-3.1-pro-low"} {
		add("ensureAntigravityDefaultPassthroughs", name)
	}

	require.NotEmpty(t, names)
	for name, source := range names {
		require.True(t, IsSafeGeminiModelPathSegment(name), "builtin model %q from %s must pass the upstream path guard", name, source)
		_, err := buildGeminiAIStudioModelActionURL("https://generativelanguage.googleapis.com", name, "generateContent", false)
		require.NoError(t, err, "builtin model %q from %s", name, source)
	}
	t.Logf("checked %d builtin model names", len(names))
}

// TestChannelMonitorGeminiRejectsNonConformingModel：渠道监控的 Gemini 适配器把模型名
// 拼进 /v1beta/models/{model}:generateContent，不合规的模型名在发请求前就被拒绝。
func TestChannelMonitorGeminiRejectsNonConformingModel(t *testing.T) {
	for _, model := range []string{"../../x", "gemini-2.5-pro?a=b", "gemini-2.5-pro#x", "a/b", "..", "gemini\r\npro"} {
		_, _, status, err := callProvider(t.Context(), MonitorProviderGemini, "https://generativelanguage.googleapis.com", "k", model, "hi", nil)
		require.Error(t, err, "model=%q", model)
		require.Contains(t, err.Error(), "invalid gemini model")
		require.Zero(t, status)
	}
}
