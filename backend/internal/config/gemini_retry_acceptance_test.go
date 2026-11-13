package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiRetryAcceptanceConfigMatrix(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
		valid bool
	}{
		{"11", 0, false},
		{"-1", 0, false},
		{"3", 3, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_MAX_SAME_ACCOUNT_RETRIES_GEMINI", tc.value)
			cfg, err := LoadForBootstrap()
			if !tc.valid {
				require.ErrorContains(t, err, "gateway.max_same_account_retries_gemini must be between 0-10")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Gateway.MaxSameAccountRetriesGemini)
		})
	}
}
