package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type codexModelsHTTPUpstreamStub struct {
	request  *http.Request
	response *http.Response
	err      error
}

func (s *codexModelsHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	return s.response, s.err
}

func (s *codexModelsHTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.request = req
	return s.response, s.err
}

func TestFetchCodexModelsManifestConvertsStandardOpenAIList(t *testing.T) {
	upstream := &codexModelsHTTPUpstreamStub{response: codexModelsTestResponse(http.StatusOK, `{
		"object":"list",
		"data":[{"id":"gpt-5.6"},{"id":"gpt-image-2"}]
	}`)}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 11, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example/v1"},
	}

	manifest, err := service.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")

	require.NoError(t, err)
	require.NotNil(t, manifest)
	require.Equal(t, "gpt-5.6", gjson.GetBytes(manifest.Body, "models.0.slug").String())
	require.Equal(t, "gpt-image-2", gjson.GetBytes(manifest.Body, "models.1.slug").String())
	require.Equal(t, "gpt-5.6", gjson.GetBytes(manifest.Body, "models.0.display_name").String())
	require.Equal(t, "shell_command", gjson.GetBytes(manifest.Body, "models.0.shell_type").String())
	require.Equal(t, "list", gjson.GetBytes(manifest.Body, "models.0.visibility").String())
	require.Equal(t, "bytes", gjson.GetBytes(manifest.Body, "models.0.truncation_policy.mode").String())
	require.Equal(t, int64(10_000), gjson.GetBytes(manifest.Body, "models.0.truncation_policy.limit").Int())
	require.Equal(t, "image", gjson.GetBytes(manifest.Body, "models.0.input_modalities.1").String())
	require.Equal(t, "https://upstream.example/v1/models?client_version=0.137.0", upstream.request.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.request.Header.Get("Authorization"))
}

func TestFetchCodexModelsManifestPreservesNativeManifest(t *testing.T) {
	native := codexModelsNativeTestManifest("gpt-5.6")
	upstream := &codexModelsHTTPUpstreamStub{response: codexModelsTestResponse(http.StatusOK, native)}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example/v1"},
	}

	manifest, err := service.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")

	require.NoError(t, err)
	require.JSONEq(t, native, string(manifest.Body))
}

func TestFetchCodexModelsManifestUsesOfficialOpenAIBaseURLByDefault(t *testing.T) {
	upstream := &codexModelsHTTPUpstreamStub{response: codexModelsTestResponse(http.StatusOK, `{
		"object":"list",
		"data":[{"id":"gpt-5.6"}]
	}`)}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 16, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
	}

	manifest, err := service.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")

	require.NoError(t, err)
	require.Equal(t, "gpt-5.6", gjson.GetBytes(manifest.Body, "models.0.slug").String())
	require.Equal(t, "https://api.openai.com/v1/models?client_version=0.137.0", upstream.request.URL.String())
}

func TestFetchCodexModelsManifestRejectsInvalidEnvelope(t *testing.T) {
	upstream := &codexModelsHTTPUpstreamStub{response: codexModelsTestResponse(http.StatusOK, `{"data":[]}`)}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 13, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example/v1"},
	}

	manifest, err := service.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")

	require.Nil(t, manifest)
	require.ErrorContains(t, err, "missing top-level models array")
	require.True(t, IsRetryableCodexModelsManifestError(err))
}

func TestFetchCodexModelsManifestRejectsIncompleteModelEntry(t *testing.T) {
	upstream := &codexModelsHTTPUpstreamStub{response: codexModelsTestResponse(http.StatusOK, `{"models":[{"slug":"gpt-5.6"}]}`)}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 15, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example/v1"},
	}

	manifest, err := service.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")

	require.Nil(t, manifest)
	require.ErrorContains(t, err, "missing required field")
	require.ErrorContains(t, err, "display_name")
	require.True(t, IsRetryableCodexModelsManifestError(err))
}

func TestFetchCodexModelsManifestOAuthUsesChatGPTEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/codex/models", r.URL.Path)
		require.Equal(t, "0.137.0", r.URL.Query().Get("client_version"))
		require.Equal(t, "Bearer oauth-token", r.Header.Get("Authorization"))
		require.Equal(t, "chatgpt-account", r.Header.Get("ChatGPT-Account-ID"))
		require.Equal(t, "codex_cli_rs", r.Header.Get("Originator"))
		require.Equal(t, "0.137.0", r.Header.Get("Version"))
		w.Header().Set("ETag", `"manifest-v1"`)
		_, _ = io.WriteString(w, codexModelsNativeTestManifest("gpt-5.6"))
	}))
	defer server.Close()

	oldURL := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL + "/backend-api/codex/models"
	t.Cleanup(func() { chatgptCodexModelsURL = oldURL })

	service := &OpenAIGatewayService{}
	account := &Account{
		ID: 14, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
	}

	manifest, err := service.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")

	require.NoError(t, err)
	require.Equal(t, `"manifest-v1"`, manifest.ETag)
	require.Equal(t, "gpt-5.6", gjson.GetBytes(manifest.Body, "models.0.slug").String())
}

func codexModelsNativeTestManifest(slug string) string {
	return `{"models":[{` +
		`"slug":"` + slug + `",` +
		`"display_name":"` + slug + `",` +
		`"description":null,` +
		`"default_reasoning_level":"low",` +
		`"supported_reasoning_levels":[{"effort":"low","description":"Fast"}],` +
		`"shell_type":"shell_command",` +
		`"visibility":"list",` +
		`"supported_in_api":true,` +
		`"priority":1,` +
		`"availability_nux":null,` +
		`"upgrade":null,` +
		`"base_instructions":"You are Codex.",` +
		`"support_verbosity":false,` +
		`"default_verbosity":null,` +
		`"apply_patch_tool_type":null,` +
		`"truncation_policy":{"mode":"bytes","limit":10000},` +
		`"supports_parallel_tool_calls":true,` +
		`"experimental_supported_tools":[],` +
		`"input_modalities":["text","image"]` +
		`}]}`
}

func codexModelsTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
