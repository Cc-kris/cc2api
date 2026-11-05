package repository

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// installGrokIdentityTestClient 把一个固定 RoundTripper 塞进 Do 会命中的客户端缓存位，
// 让测试能看到传输层最终发出的请求头。
func installGrokIdentityTestClient(t *testing.T, accountID int64, rt roundTripFunc) *httpUpstreamService {
	t.Helper()
	upstream := NewHTTPUpstream(nil)
	svc, ok := upstream.(*httpUpstreamService)
	require.True(t, ok)

	isolation := svc.getIsolationMode()
	profile := service.HTTPUpstreamProfileDefault
	proxyKey := directProxyKey
	protocolMode := svc.resolveProtocolMode(profile, proxyKey, nil)
	settings := svc.resolvePoolSettings(isolation, 1)
	settings = svc.applyProfilePoolSettings(settings, profile)
	cacheKey := buildCacheKey(isolation, proxyKey, accountID, protocolMode)
	svc.clients[cacheKey] = &upstreamClientEntry{
		client:       &http.Client{Transport: rt},
		proxyKey:     proxyKey,
		poolKey:      buildPoolKey(settings, protocolMode),
		protocolMode: protocolMode,
	}
	return svc
}

func TestHTTPUpstreamDoAppliesGrokCLIIdentityBeforeOAuthRoundTrip(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")

	for _, endpoint := range []string{"responses", "chat/completions", "images/generations", "videos/request-1"} {
		t.Run(endpoint, func(t *testing.T) {
			var capturedHeaders http.Header
			svc := installGrokIdentityTestClient(t, 4084, func(req *http.Request) (*http.Response, error) {
				capturedHeaders = req.Header.Clone()
				statusCode := http.StatusOK
				if req.Header.Get("X-XAI-Token-Auth") != "xai-grok-cli" {
					statusCode = http.StatusForbidden
				}
				return &http.Response{StatusCode: statusCode, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
			})

			req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/"+endpoint, nil)
			require.NoError(t, err)
			req.Header.Set("User-Agent", "legacy-client/1.0")

			resp, err := svc.Do(req, "", 4084, 1)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NoError(t, resp.Body.Close())

			require.Equal(t, xai.CLIClientVersion, capturedHeaders.Get("x-grok-client-version"))
			require.Equal(t, "1.0.46", capturedHeaders.Get("x-grok-client-version"))
			require.Equal(t, "grok-pager", capturedHeaders.Get("x-grok-client-identifier"))
			require.Equal(t, "interactive", capturedHeaders.Get("x-grok-client-mode"))
			require.Equal(t, "authenticate-response", capturedHeaders.Get("x-authenticateresponse"))
			require.Equal(t, "xai-grok-cli", capturedHeaders.Get("X-XAI-Token-Auth"))
			require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), capturedHeaders.Get("User-Agent"))
		})
	}
}

// 账号级 header 覆盖在服务层最后执行，传输层必须再把身份头盖回去（V02-AC26）。
func TestHTTPUpstreamDoGrokCLIIdentityOverridesAccountHeaderOverrides(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")

	var capturedHeaders http.Header
	svc := installGrokIdentityTestClient(t, 4085, func(req *http.Request) (*http.Response, error) {
		capturedHeaders = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
	})

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "custom-override/9.9")
	req.Header.Set("X-Grok-Client-Version", "0.2.93")
	req.Header.Set("X-Grok-Client-Identifier", "custom")
	req.Header.Set("X-Grok-Client-Mode", "headless")
	req.Header.Set("X-Authenticateresponse", "no")
	req.Header.Set("X-XAI-Token-Auth", "other")

	resp, err := svc.Do(req, "", 4085, 1)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), capturedHeaders.Get("User-Agent"))
	require.Equal(t, []string{xai.CLIClientVersion}, capturedHeaders.Values("X-Grok-Client-Version"))
	require.Equal(t, []string{xai.CLIClientIdentifier}, capturedHeaders.Values("X-Grok-Client-Identifier"))
	require.Equal(t, []string{xai.CLIClientMode}, capturedHeaders.Values("X-Grok-Client-Mode"))
	require.Equal(t, []string{"authenticate-response"}, capturedHeaders.Values("X-Authenticateresponse"))
	require.Equal(t, []string{"xai-grok-cli"}, capturedHeaders.Values("X-XAI-Token-Auth"))
}

func TestHTTPUpstreamDoFallsBackToOfficialGrokAPIOnCLIAccessDenied(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")

	payload := []byte(`{"model":"grok-4.5","input":"hello"}`)
	var calls int
	var fallbackBody []byte
	var fallbackHeaders http.Header
	svc := installGrokIdentityTestClient(t, 4421, func(req *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		if calls == 1 {
			require.Equal(t, grokCLIProxyHost, req.URL.Hostname())
			require.Equal(t, "xai-grok-cli", req.Header.Get("X-XAI-Token-Auth"))
			require.Equal(t, xai.CLIClientIdentifier, req.Header.Get("x-grok-client-identifier"))
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"Access denied"}`)),
				Request:    req,
			}, nil
		}
		fallbackBody = body
		fallbackHeaders = req.Header.Clone()
		require.Equal(t, grokOfficialAPIHost, req.URL.Hostname())
		require.Equal(t, "/v1/responses", req.URL.Path)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":"response-ok"}`)),
			Request:    req,
		}, nil
	})

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer oauth-token")

	resp, err := svc.Do(req, "", 4421, 1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.JSONEq(t, `{"id":"response-ok"}`, string(responseBody))
	require.Equal(t, 2, calls)
	require.Equal(t, payload, fallbackBody)
	require.Equal(t, "Bearer oauth-token", fallbackHeaders.Get("Authorization"))
	// 回落到 api.x.ai 时 CLI 身份头全部删掉
	require.Empty(t, fallbackHeaders.Get("X-XAI-Token-Auth"))
	require.Empty(t, fallbackHeaders.Get("x-grok-client-version"))
	require.Empty(t, fallbackHeaders.Get("x-grok-client-identifier"))
	require.Empty(t, fallbackHeaders.Get("x-grok-client-mode"))
	require.Empty(t, fallbackHeaders.Get("x-authenticateresponse"))
	require.Empty(t, fallbackHeaders.Get("User-Agent"))
}

func TestApplyGrokCLIProxyTransportHeaders(t *testing.T) {
	t.Run("uses pinned stable version for the CLI proxy", func(t *testing.T) {
		t.Setenv(xai.CLIVersionEnv, "")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "legacy-client/1.0")

		applyGrokCLIProxyTransportHeaders(req)

		require.Equal(t, xai.CLIClientVersion, req.Header.Get("x-grok-client-version"))
		require.Equal(t, "xai-grok-cli", req.Header.Get("X-XAI-Token-Auth"))
		require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), req.Header.Get("User-Agent"))
	})

	t.Run("accepts a valid operator override", func(t *testing.T) {
		t.Setenv(xai.CLIVersionEnv, "1.0.47-alpha.1")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/chat/completions", nil)
		require.NoError(t, err)

		applyGrokCLIProxyTransportHeaders(req)

		require.Equal(t, "1.0.47-alpha.1", req.Header.Get("x-grok-client-version"))
		require.Equal(t, xai.CLIUserAgent("1.0.47-alpha.1"), req.Header.Get("User-Agent"))
	})

	t.Run("rejects an unsafe override", func(t *testing.T) {
		t.Setenv(xai.CLIVersionEnv, "1.0.47\r\nX-Injected: true")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
		require.NoError(t, err)

		applyGrokCLIProxyTransportHeaders(req)

		require.Equal(t, xai.CLIClientVersion, req.Header.Get("x-grok-client-version"))
		require.Empty(t, req.Header.Get("X-Injected"))
	})

	// 服务层下限 1.0.13，传输层下限 1.0.46：1.0.20 在服务层可用，到传输层回落 1.0.46。
	for _, version := range []string{"1.0.5", "1.0.20", "1.0.45", "1.0.46-beta.1", "abc"} {
		t.Run("rejects an override below the transport minimum "+version, func(t *testing.T) {
			t.Setenv(xai.CLIVersionEnv, version)
			req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
			require.NoError(t, err)

			applyGrokCLIProxyTransportHeaders(req)

			require.Equal(t, xai.CLIClientVersion, req.Header.Get("x-grok-client-version"))
			require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), req.Header.Get("User-Agent"))
		})
	}

	// Every entry sits above the pinned minimum, so a rejection here can only be
	// caused by the malformed semver and never by the version being too old.
	for _, version := range []string{
		"1.0.047",
		"1.0.47-alpha..1",
		"1.1",
		"2",
		"1.0.47+build.1",
	} {
		t.Run("rejects invalid semver "+version, func(t *testing.T) {
			t.Setenv(xai.CLIVersionEnv, version)
			req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
			require.NoError(t, err)

			applyGrokCLIProxyTransportHeaders(req)

			require.Equal(t, xai.CLIClientVersion, req.Header.Get("x-grok-client-version"))
			require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), req.Header.Get("User-Agent"))
		})
	}

	t.Run("leaves direct xAI API requests unchanged", func(t *testing.T) {
		t.Setenv(xai.CLIVersionEnv, "1.0.47")
		req, err := http.NewRequest(http.MethodPost, "https://api.x.ai/v1/responses", nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "direct-api-client/1.0")

		applyGrokCLIProxyTransportHeaders(req)

		require.Empty(t, req.Header.Get("x-grok-client-version"))
		require.Empty(t, req.Header.Get("x-grok-client-identifier"))
		require.Empty(t, req.Header.Get("X-XAI-Token-Auth"))
		require.Equal(t, "direct-api-client/1.0", req.Header.Get("User-Agent"))
	})
}
