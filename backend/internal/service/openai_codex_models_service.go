package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

// chatgptCodexModelsURL is a variable so tests can replace the upstream with a
// local server without changing production configuration.
var chatgptCodexModelsURL = "https://chatgpt.com/backend-api/codex/models"

const (
	codexModelsManifestBodyLimit    int64 = 8 << 20
	codexModelsManifestErrorLimit   int64 = 2 << 10
	codexModelsManifestRequestLimit       = 15 * time.Second
)

// CodexModelsManifest carries the upstream payload and cache validators needed
// by Codex Desktop's model manager.
type CodexModelsManifest struct {
	Body        []byte
	ETag        string
	NotModified bool
}

type codexModelsManifestUpstreamError struct {
	err       error
	retryable bool
}

func (e *codexModelsManifestUpstreamError) Error() string { return e.err.Error() }
func (e *codexModelsManifestUpstreamError) Unwrap() error { return e.err }

// IsRetryableCodexModelsManifestError reports whether selecting another
// account can recover the same manifest request.
func IsRetryableCodexModelsManifestError(err error) bool {
	var upstreamErr *codexModelsManifestUpstreamError
	return errors.As(err, &upstreamErr) && upstreamErr.retryable
}

// FetchCodexModelsManifest fetches Codex's capability manifest. OAuth accounts
// use the ChatGPT Codex backend; custom API-key accounts use their /v1/models
// endpoint and standard OpenAI lists are converted to Codex's envelope.
func (s *OpenAIGatewayService) FetchCodexModelsManifest(ctx context.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	if account == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_ACCOUNT_REQUIRED", "account is required")
	}

	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		clientVersion = openAICodexProbeVersion
	}

	requestEndpoint := chatgptCodexModelsURL
	authToken := ""
	useAPIKeyUpstream := false
	appendModelsPath := false
	switch {
	case account.IsOpenAIOAuth():
		var tokenErr error
		authToken, _, tokenErr = s.GetAccessToken(ctx, account)
		if tokenErr != nil || strings.TrimSpace(authToken) == "" {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_TOKEN_MISSING", "account has no Codex backend access token")
		}
		authToken = strings.TrimSpace(authToken)
	case account.IsOpenAIApiKey():
		baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
		authToken = strings.TrimSpace(account.GetOpenAIApiKey())
		if authToken == "" {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_API_KEY_MISSING", "account has no API key for the Codex models upstream")
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_API_KEY_UPSTREAM_INVALID", "invalid Codex models upstream base URL: %v", err)
		}
		requestEndpoint = normalizedBaseURL
		useAPIKeyUpstream = true
		appendModelsPath = true
	default:
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_ACCOUNT_TYPE_UNSUPPORTED", "account type %q cannot fetch the Codex models manifest", account.Type)
	}

	requestURL, err := buildCodexModelsManifestURL(requestEndpoint, appendModelsPath, clientVersion)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "build Codex models request URL: %v", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, codexModelsManifestRequestLimit)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "create Codex models request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("Version", clientVersion)
	req.Header.Set("User-Agent", codexCLIUserAgent)
	if etag := strings.TrimSpace(ifNoneMatch); etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if !useAPIKeyUpstream {
		req.Host = "chatgpt.com"
		if accountID := strings.TrimSpace(account.GetChatGPTAccountID()); accountID != "" {
			req.Header.Set("ChatGPT-Account-ID", accountID)
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	var resp *http.Response
	if useAPIKeyUpstream {
		if s == nil || s.httpUpstream == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_UPSTREAM_NOT_CONFIGURED", "Codex models upstream HTTP client is not configured")
		}
		req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
		resp, err = s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	} else {
		client, clientErr := httpclient.GetClient(httpclient.Options{
			ProxyURL:              proxyURL,
			Timeout:               codexModelsManifestRequestLimit,
			ResponseHeaderTimeout: 10 * time.Second,
		})
		if clientErr != nil {
			return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_PROXY_INVALID", "invalid proxy configuration: %v", clientErr)
		}
		resp, err = client.Do(req)
	}
	if err != nil {
		return nil, &codexModelsManifestUpstreamError{
			err:       infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "Codex models manifest request failed: %v", err),
			retryable: !errors.Is(err, context.Canceled),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return &CodexModelsManifest{ETag: resp.Header.Get("ETag"), NotModified: true}, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, codexModelsManifestErrorLimit))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, &codexModelsManifestUpstreamError{
			err: infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "Codex models manifest upstream error %d: %s", resp.StatusCode, message),
			retryable: resp.StatusCode == http.StatusTooManyRequests ||
				(resp.StatusCode >= http.StatusInternalServerError && resp.StatusCode < 600),
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, codexModelsManifestBodyLimit))
	if err != nil {
		return nil, &codexModelsManifestUpstreamError{
			err:       infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "read Codex models manifest response: %v", err),
			retryable: true,
		}
	}
	if useAPIKeyUpstream {
		body = convertOpenAIModelListToCodexManifest(body)
	}
	if err := validateCodexModelsManifestEnvelope(body); err != nil {
		return nil, &codexModelsManifestUpstreamError{
			err:       infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_INVALID_MANIFEST", "Codex models manifest upstream returned an invalid envelope: %v", err),
			retryable: true,
		}
	}
	return &CodexModelsManifest{Body: body, ETag: resp.Header.Get("ETag")}, nil
}

func convertOpenAIModelListToCodexManifest(body []byte) []byte {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil || envelope == nil {
		return body
	}
	if _, ok := envelope["models"]; ok {
		return body
	}
	data, ok := envelope["data"]
	if !ok {
		return body
	}
	var entries []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return body
	}
	models := make([]codexModelManifestEntry, 0, len(entries))
	for _, entry := range entries {
		if id := strings.TrimSpace(entry.ID); id != "" {
			models = append(models, newCodexModelManifestEntry(id, len(models)+1))
		}
	}
	if len(models) == 0 {
		return body
	}
	converted, err := json.Marshal(map[string][]codexModelManifestEntry{"models": models})
	if err != nil {
		return body
	}
	return converted
}

// codexModelManifestEntry is the smallest complete ModelInfo contract accepted
// by current Codex clients. A standard OpenAI /v1/models item only has an id,
// so the remaining values are deliberately conservative defaults rather than
// claims about upstream-specific capabilities.
type codexModelManifestEntry struct {
	Slug                              string             `json:"slug"`
	DisplayName                       string             `json:"display_name"`
	Description                       *string            `json:"description"`
	DefaultReasoningLevel             string             `json:"default_reasoning_level"`
	SupportedReasoningLevels          []codexReasoning   `json:"supported_reasoning_levels"`
	ShellType                         string             `json:"shell_type"`
	Visibility                        string             `json:"visibility"`
	SupportedInAPI                    bool               `json:"supported_in_api"`
	Priority                          int                `json:"priority"`
	AdditionalSpeedTiers              []string           `json:"additional_speed_tiers"`
	ServiceTiers                      []any              `json:"service_tiers"`
	DefaultServiceTier                *string            `json:"default_service_tier"`
	AvailabilityNUX                   *codexAvailability `json:"availability_nux"`
	Upgrade                           *any               `json:"upgrade"`
	BaseInstructions                  string             `json:"base_instructions"`
	IncludeSkillsUsageInstructions    bool               `json:"include_skills_usage_instructions"`
	SupportsReasoningSummaryParameter bool               `json:"supports_reasoning_summary_parameter"`
	DefaultReasoningSummary           string             `json:"default_reasoning_summary"`
	SupportVerbosity                  bool               `json:"support_verbosity"`
	DefaultVerbosity                  *string            `json:"default_verbosity"`
	ApplyPatchToolType                *string            `json:"apply_patch_tool_type"`
	WebSearchToolType                 string             `json:"web_search_tool_type"`
	TruncationPolicy                  codexTruncation    `json:"truncation_policy"`
	SupportsParallelToolCalls         bool               `json:"supports_parallel_tool_calls"`
	SupportsImageDetailOriginal       bool               `json:"supports_image_detail_original"`
	ExperimentalSupportedTools        []string           `json:"experimental_supported_tools"`
	InputModalities                   []string           `json:"input_modalities"`
	SupportsSearchTool                bool               `json:"supports_search_tool"`
	UseResponsesLite                  bool               `json:"use_responses_lite"`
}

type codexReasoning struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type codexAvailability struct {
	Message string `json:"message"`
}

type codexTruncation struct {
	Mode  string `json:"mode"`
	Limit int    `json:"limit"`
}

func newCodexModelManifestEntry(id string, priority int) codexModelManifestEntry {
	return codexModelManifestEntry{
		Slug:                  id,
		DisplayName:           id,
		Description:           nil,
		DefaultReasoningLevel: "low",
		SupportedReasoningLevels: []codexReasoning{{
			Effort:      "low",
			Description: "Fast responses with lighter reasoning.",
		}},
		ShellType:                         "shell_command",
		Visibility:                        "list",
		SupportedInAPI:                    true,
		Priority:                          priority,
		AdditionalSpeedTiers:              []string{},
		ServiceTiers:                      []any{},
		DefaultServiceTier:                nil,
		AvailabilityNUX:                   nil,
		Upgrade:                           nil,
		BaseInstructions:                  "You are Codex, a coding agent.",
		IncludeSkillsUsageInstructions:    false,
		SupportsReasoningSummaryParameter: false,
		DefaultReasoningSummary:           "auto",
		SupportVerbosity:                  false,
		DefaultVerbosity:                  nil,
		ApplyPatchToolType:                nil,
		WebSearchToolType:                 "text",
		TruncationPolicy:                  codexTruncation{Mode: "bytes", Limit: 10_000},
		SupportsParallelToolCalls:         true,
		SupportsImageDetailOriginal:       false,
		ExperimentalSupportedTools:        []string{},
		InputModalities:                   []string{"text", "image"},
		SupportsSearchTool:                false,
		UseResponsesLite:                  false,
	}
}

func validateCodexModelsManifestEnvelope(body []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode JSON object: %w", err)
	}
	models, ok := envelope["models"]
	if !ok {
		return errors.New("missing top-level models array")
	}
	models = bytes.TrimSpace(models)
	if len(models) == 0 || models[0] != '[' {
		return errors.New("top-level models field is not an array")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(models, &entries); err != nil {
		return fmt.Errorf("decode top-level models array: %w", err)
	}
	for index, entry := range entries {
		if err := validateCodexModelsManifestEntry(entry); err != nil {
			return fmt.Errorf("invalid models[%d]: %w", index, err)
		}
	}
	return nil
}

func validateCodexModelsManifestEntry(entry json.RawMessage) error {
	var model map[string]json.RawMessage
	if err := json.Unmarshal(entry, &model); err != nil || model == nil {
		return errors.New("model is not a JSON object")
	}
	for _, field := range []string{
		"slug", "display_name", "supported_reasoning_levels", "shell_type", "visibility",
		"supported_in_api", "priority", "base_instructions", "support_verbosity", "truncation_policy",
		"supports_parallel_tool_calls", "experimental_supported_tools",
	} {
		if _, ok := model[field]; !ok {
			return fmt.Errorf("missing required field %q", field)
		}
	}
	return nil
}

func buildCodexModelsManifestURL(endpoint string, appendModelsPath bool, clientVersion string) (*url.URL, error) {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if requestURL.Fragment != "" {
		return nil, errors.New("URL fragments are not supported")
	}
	query := requestURL.Query()
	requestURL.RawQuery = ""
	requestURL.ForceQuery = false
	if appendModelsPath {
		requestURL, err = url.Parse(buildOpenAIModelsURL(requestURL.String()))
		if err != nil {
			return nil, err
		}
	}
	query.Set("client_version", clientVersion)
	requestURL.RawQuery = query.Encode()
	return requestURL, nil
}
