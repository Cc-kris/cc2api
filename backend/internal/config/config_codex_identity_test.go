package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// V02-T15：gateway.disable_codex_identity_enforcement 默认 false（强制统一开启），可由环境变量关闭。
func TestLoadDefaultCodexIdentityEnforcementEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.Gateway.DisableCodexIdentityEnforcement)
}

func TestLoadCodexIdentityEnforcementDisabledFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_DISABLE_CODEX_IDENTITY_ENFORCEMENT", "true")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.DisableCodexIdentityEnforcement)
}
