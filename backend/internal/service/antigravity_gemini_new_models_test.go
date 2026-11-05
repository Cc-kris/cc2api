package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// V02-T21 / V02-AC48：Gemini 3.6/3.7/3.8 Flash（含档位）在 Antigravity 账号上可用。
// 默认映射账号直接命中 DefaultAntigravityModelMapping；显式映射账号靠运行时透传；
// gemini-4-argon 简称映射到 -high，不走透传（由迁移 221 给显式映射账号补）。

var gemini3xFlashTierModels = []string{
	"gemini-3.6-flash", "gemini-3.6-flash-high", "gemini-3.6-flash-low", "gemini-3.6-flash-medium", "gemini-3.6-flash-tiered",
	"gemini-3.7-flash", "gemini-3.7-flash-high", "gemini-3.7-flash-low", "gemini-3.7-flash-medium", "gemini-3.7-flash-tiered",
	"gemini-3.8-flash", "gemini-3.8-flash-high", "gemini-3.8-flash-low", "gemini-3.8-flash-medium", "gemini-3.8-flash-tiered",
}

func TestAntigravityNewGeminiModels_DefaultMappingAccount(t *testing.T) {
	for _, account := range []*Account{
		{Platform: domain.PlatformAntigravity},
		{Platform: domain.PlatformAntigravity, Credentials: map[string]any{}},
		{Platform: domain.PlatformAntigravity, Credentials: map[string]any{"model_mapping": map[string]any{}}},
	} {
		for _, model := range gemini3xFlashTierModels {
			require.Equal(t, model, mapAntigravityModel(account, model), model)
			require.True(t, account.IsModelSupported(model), model)
		}
		require.Equal(t, "gemini-4-argon-high", mapAntigravityModel(account, "gemini-4-argon"))
		for _, tier := range []string{"high", "low", "medium"} {
			model := "gemini-4-argon-" + tier
			require.Equal(t, model, mapAntigravityModel(account, model))
		}
	}
}

func TestAntigravityNewGeminiModels_ExplicitMappingAccountUsesRuntimePassthrough(t *testing.T) {
	account := &Account{
		Platform: domain.PlatformAntigravity,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"claude-sonnet-4-6": "claude-sonnet-4-6",
		}},
	}
	for _, model := range gemini3xFlashTierModels {
		require.Equal(t, model, mapAntigravityModel(account, model), model)
		require.True(t, account.IsModelSupported(model), model)
		require.True(t, account.isAntigravityInjectedDefaultPassthrough(model, model), model)
	}
	// argon 不在运行时透传里，显式映射账号要靠迁移 221 才有
	require.False(t, account.IsModelSupported("gemini-4-argon"))
	require.Equal(t, "", mapAntigravityModel(account, "gemini-4-argon"))
}

func TestAntigravityNewGeminiModels_PassthroughDoesNotOverrideAdminConfig(t *testing.T) {
	account := &Account{
		Platform: domain.PlatformAntigravity,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"gemini-3.8-flash": "gemini-3-flash",
			"gemini-3.7-*":     "gemini-3-flash",
		}},
	}
	// 管理员写的精确键优先
	require.Equal(t, "gemini-3-flash", mapAntigravityModel(account, "gemini-3.8-flash"))
	require.False(t, account.isAntigravityInjectedDefaultPassthrough("gemini-3.8-flash", "gemini-3-flash"))
	// 管理员的通配符已覆盖时不再注入自映射
	require.Equal(t, "gemini-3-flash", mapAntigravityModel(account, "gemini-3.7-flash-high"))
	_, injected := account.GetModelMapping()["gemini-3.7-flash-high"]
	require.False(t, injected)
	// 其他没配的照常透传
	require.Equal(t, "gemini-3.6-flash-low", mapAntigravityModel(account, "gemini-3.6-flash-low"))
	require.True(t, account.isAntigravityInjectedDefaultPassthrough("gemini-3.6-flash-low", "gemini-3.6-flash-low"))
}

func TestIsAntigravityInjectedDefaultPassthrough(t *testing.T) {
	explicit := &Account{Platform: domain.PlatformAntigravity, Credentials: map[string]any{"model_mapping": map[string]any{
		"gemini-3.6-flash": "gemini-3.6-flash",
		"claude-opus-4-7":  "claude-opus-4-7",
	}}}
	require.False(t, explicit.isAntigravityInjectedDefaultPassthrough("gemini-3.6-flash", "gemini-3.6-flash"), "admin-written entry")
	require.True(t, explicit.isAntigravityInjectedDefaultPassthrough("gemini-3.7-flash", "gemini-3.7-flash"), "runtime-injected entry")
	require.False(t, explicit.isAntigravityInjectedDefaultPassthrough("gemini-3.7-flash", "gemini-3-flash"), "key != target")
	require.False(t, explicit.isAntigravityInjectedDefaultPassthrough("claude-opus-4-7", "claude-opus-4-7"), "not a passthrough model")

	emptyMapping := &Account{Platform: domain.PlatformAntigravity, Credentials: map[string]any{"model_mapping": map[string]any{}}}
	require.False(t, emptyMapping.isAntigravityInjectedDefaultPassthrough("gemini-3.7-flash", "gemini-3.7-flash"), "empty mapping uses default catalog")

	other := &Account{Platform: domain.PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{"x": "x"}}}
	require.False(t, other.isAntigravityInjectedDefaultPassthrough("gemini-3.7-flash", "gemini-3.7-flash"))

	var nilAccount *Account
	require.False(t, nilAccount.isAntigravityInjectedDefaultPassthrough("gemini-3.7-flash", "gemini-3.7-flash"))
}

func TestAntigravityDefaultPassthroughModels_MatchesUpstreamList(t *testing.T) {
	require.Len(t, antigravityDefaultPassthroughModels, 18)
	for _, model := range append([]string{"gemini-3-flash", "gemini-3.1-pro-high", "gemini-3.1-pro-low"}, gemini3xFlashTierModels...) {
		require.Contains(t, antigravityDefaultPassthroughModels, model)
		require.Equal(t, model, domain.DefaultAntigravityModelMapping[model], "passthrough model must self-map in default mapping")
	}
}
