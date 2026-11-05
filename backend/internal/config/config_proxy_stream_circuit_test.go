package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// V02-T06：gateway.openai_proxy_stream_circuit.* 默认值、环境变量覆盖和非负校验。
func TestLoadDefaultOpenAIProxyStreamCircuitConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.Gateway.OpenAIProxyStreamCircuit.Disabled)
	require.Equal(t, 2, cfg.Gateway.OpenAIProxyStreamCircuit.FailureThreshold)
	require.Equal(t, 60, cfg.Gateway.OpenAIProxyStreamCircuit.WindowSeconds)
	require.Equal(t, 600, cfg.Gateway.OpenAIProxyStreamCircuit.TTLSeconds)
}

func TestLoadOpenAIProxyStreamCircuitFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_DISABLED", "true")
	t.Setenv("GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_FAILURE_THRESHOLD", "5")
	t.Setenv("GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_WINDOW_SECONDS", "30")
	t.Setenv("GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_TTL_SECONDS", "120")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.OpenAIProxyStreamCircuit.Disabled)
	require.Equal(t, 5, cfg.Gateway.OpenAIProxyStreamCircuit.FailureThreshold)
	require.Equal(t, 30, cfg.Gateway.OpenAIProxyStreamCircuit.WindowSeconds)
	require.Equal(t, 120, cfg.Gateway.OpenAIProxyStreamCircuit.TTLSeconds)
}

func TestValidateOpenAIProxyStreamCircuitNonNegative(t *testing.T) {
	cases := map[string]struct {
		mutate  func(*Config)
		wantErr string
	}{
		"failure_threshold": {func(c *Config) { c.Gateway.OpenAIProxyStreamCircuit.FailureThreshold = -1 }, "gateway.openai_proxy_stream_circuit.failure_threshold must be non-negative"},
		"window_seconds":    {func(c *Config) { c.Gateway.OpenAIProxyStreamCircuit.WindowSeconds = -1 }, "gateway.openai_proxy_stream_circuit.window_seconds must be non-negative"},
		"ttl_seconds":       {func(c *Config) { c.Gateway.OpenAIProxyStreamCircuit.TTLSeconds = -1 }, "gateway.openai_proxy_stream_circuit.ttl_seconds must be non-negative"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			cfg, err := Load()
			require.NoError(t, err)
			tc.mutate(cfg)
			require.ErrorContains(t, cfg.Validate(), tc.wantErr)
		})
	}
}
