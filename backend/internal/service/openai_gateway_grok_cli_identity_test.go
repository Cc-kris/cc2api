package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestApplyGrokCLIHeadersUsesPinnedCLIIdentity(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")
	headers := http.Header{}
	headers.Set("User-Agent", "sub2api-grok/1.0")

	applyGrokCLIHeaders(headers)

	require.Equal(t, "1.0.46", headers.Get("X-Grok-Client-Version"))
	require.Equal(t, xai.CLIUserAgent("1.0.46"), headers.Get("User-Agent"))
	require.Contains(t, headers.Get("User-Agent"), "grok-pager/1.0.46 grok-shell/1.0.46 (")
	require.Equal(t, "grok-pager", headers.Get("X-Grok-Client-Identifier"))
	require.Equal(t, "interactive", headers.Get("X-Grok-Client-Mode"))
	require.Len(t, headers.Values("X-Grok-Client-Version"), 1)
}

// V02-AC27：非法或过低的覆盖值被忽略，回落 1.0.46。
func TestApplyGrokCLIHeadersIgnoresInvalidVersionOverride(t *testing.T) {
	for _, override := range []string{"1.0.5", "abc", "0.2.93", "1.0.12", "1.0.14\r\nX-Injected: true"} {
		t.Run(override, func(t *testing.T) {
			t.Setenv(xai.CLIVersionEnv, override)
			headers := http.Header{}

			applyGrokCLIHeaders(headers)

			require.Equal(t, xai.CLIClientVersion, headers.Get("X-Grok-Client-Version"))
			require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), headers.Get("User-Agent"))
			require.Empty(t, headers.Get("X-Injected"))
		})
	}
}

func TestApplyGrokCLIHeadersAcceptsSupportedVersionOverride(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "1.0.50")
	headers := http.Header{}

	applyGrokCLIHeaders(headers)

	require.Equal(t, "1.0.50", headers.Get("X-Grok-Client-Version"))
	require.Equal(t, xai.CLIUserAgent("1.0.50"), headers.Get("User-Agent"))
}
