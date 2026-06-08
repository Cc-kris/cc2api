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
)

const (
	semanticEmbeddingDefaultTimeout = 3 * time.Second

	SemanticCacheConnectionStatusSuccess     = "success"
	SemanticCacheConnectionStatusConfigError = "config_error"
	SemanticCacheConnectionStatusAuthFailed  = "auth_failed"
	SemanticCacheConnectionStatusNetworkFail = "network_failed"
	SemanticCacheConnectionStatusTimeout     = "timeout"
	SemanticCacheConnectionStatusFailed      = "failed"

	SemanticEmbeddingSkipDisabled          = "disabled"
	SemanticEmbeddingSkipConfigIncomplete  = "config_incomplete"
	SemanticEmbeddingSkipDecryptFailed     = "decrypt_failed"
	SemanticEmbeddingSkipInvalidInput      = "invalid_input"
	SemanticEmbeddingSkipInvalidEndpoint   = "invalid_endpoint"
	SemanticEmbeddingSkipRequestFailed     = "request_failed"
	SemanticEmbeddingSkipTimeout           = "timeout"
	SemanticEmbeddingSkipHTTPStatus        = "http_status"
	SemanticEmbeddingSkipInvalidResponse   = "invalid_response"
	SemanticEmbeddingSkipEmptyVector       = "empty_vector"
	SemanticEmbeddingSkipDimensionMismatch = "dimension_mismatch"
)

type SemanticCacheConnectionTestResult struct {
	Success            bool   `json:"success"`
	Status             string `json:"status"`
	Message            string `json:"message"`
	SemanticModelBaseURL string `json:"semantic_model_base_url"`
	Model              string `json:"model"`
	EmbeddingDimension *int   `json:"embedding_dimension,omitempty"`
	DurationMS         int64  `json:"duration_ms"`
	HTTPStatus         int    `json:"http_status,omitempty"`
}

// SemanticEmbeddingResult describes a semantic embedding attempt.
// Skipped results are the intended degradation path: callers should treat them
// as semantic-cache misses and continue the exact-cache/main-request flow.
type SemanticEmbeddingResult struct {
	Vector     []float64 `json:"vector,omitempty"`
	Dimension  int       `json:"dimension"`
	Model      string    `json:"model"`
	Skipped    bool      `json:"skipped"`
	SkipReason string    `json:"skip_reason,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	HTTPStatus int       `json:"http_status,omitempty"`
}

type semanticEmbeddingHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type semanticEmbeddingClient struct {
	settingService *SettingService
	httpClient     semanticEmbeddingHTTPClient
	timeout        time.Duration
}

func NewSemanticEmbeddingClient(settingService *SettingService) *semanticEmbeddingClient {
	return &semanticEmbeddingClient{
		settingService: settingService,
		timeout:        semanticEmbeddingDefaultTimeout,
	}
}

func (c *semanticEmbeddingClient) SetHTTPClient(client semanticEmbeddingHTTPClient) {
	if c == nil {
		return
	}
	c.httpClient = client
}

func (c *semanticEmbeddingClient) SetTimeout(timeout time.Duration) {
	if c == nil || timeout <= 0 {
		return
	}
	c.timeout = timeout
}

func (c *semanticEmbeddingClient) GenerateEmbedding(ctx context.Context, input string) (*SemanticEmbeddingResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return semanticEmbeddingSkipped(SemanticEmbeddingSkipInvalidInput, ""), nil
	}
	if c == nil || c.settingService == nil {
		return semanticEmbeddingSkipped(SemanticEmbeddingSkipConfigIncomplete, ""), nil
	}

	cfg, err := c.settingService.loadSemanticCacheConfigForUpdate(ctx)
	if err != nil {
		return nil, err
	}
	cfg = normalizeSemanticCacheConfig(cfg)
	result := &SemanticEmbeddingResult{Model: cfg.SemanticModelName}
	if !cfg.Enabled || cfg.AutoClosed || cfg.Stage == "rollback" {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipDisabled
		return result, nil
	}
	if cfg.SemanticModelBaseURL == "" || cfg.SemanticAPIKeyEncrypted == "" || cfg.SemanticModelName == "" {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipConfigIncomplete
		return result, nil
	}
	if c.settingService.secretEncryptor == nil {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipDecryptFailed
		return result, nil
	}
	apiKey, err := c.settingService.secretEncryptor.Decrypt(cfg.SemanticAPIKeyEncrypted)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipDecryptFailed
		return result, nil
	}

	timeout := c.timeout
	if timeout <= 0 {
		timeout = semanticEmbeddingDefaultTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := buildSemanticEmbeddingRequest(reqCtx, cfg, input, strings.TrimSpace(apiKey))
	if err != nil {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipInvalidEndpoint
		return result, nil
	}
	if err := validateSemanticEmbeddingOutboundURL(reqCtx, req.URL); err != nil {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipInvalidEndpoint
		return result, nil
	}

	client := c.httpClient
	if client == nil {
		client = newSemanticEmbeddingHTTPClient(timeout)
	}
	started := time.Now()
	resp, err := client.Do(req)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Skipped = true
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) || isNetTimeout(err) {
			result.SkipReason = SemanticEmbeddingSkipTimeout
		} else {
			result.SkipReason = SemanticEmbeddingSkipRequestFailed
		}
		return result, nil
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipHTTPStatus
		return result, nil
	}

	embedding, model, err := decodeSemanticEmbeddingResponse(resp.Body)
	if err != nil {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipInvalidResponse
		return result, nil
	}
	if len(embedding) == 0 {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipEmptyVector
		return result, nil
	}
	if cfg.EmbeddingDimension != nil && *cfg.EmbeddingDimension != len(embedding) {
		result.Skipped = true
		result.SkipReason = SemanticEmbeddingSkipDimensionMismatch
		result.Dimension = len(embedding)
		return result, nil
	}

	result.Vector = embedding
	result.Dimension = len(embedding)
	if model != "" {
		result.Model = model
	}
	return result, nil
}

func (s *SettingService) TestSemanticCacheConnection(ctx context.Context, req SemanticCacheConfig) (*SemanticCacheConnectionTestResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := &SemanticCacheConnectionTestResult{
		Status:               SemanticCacheConnectionStatusConfigError,
		SemanticModelBaseURL: strings.TrimSpace(req.SemanticModelBaseURL),
		Model:                strings.TrimSpace(req.SemanticModelName),
	}

	existing, err := s.loadSemanticCacheConfigForUpdate(ctx)
	if err != nil {
		return nil, err
	}
	req.SemanticAPIKeyEncrypted = existing.SemanticAPIKeyEncrypted
	if plainKey := strings.TrimSpace(req.SemanticAPIKey); plainKey != "" {
		if s == nil || s.secretEncryptor == nil {
			result.Message = "语义模型秘钥加密服务不可用"
			return result, nil
		}
		encrypted, err := s.secretEncryptor.Encrypt(plainKey)
		if err != nil {
			return nil, err
		}
		req.SemanticAPIKeyEncrypted = encrypted
	}
	req.Enabled = true
	req = normalizeSemanticCacheConfig(req)
	result.SemanticModelBaseURL = req.SemanticModelBaseURL
	result.Model = req.SemanticModelName
	if err := validateSemanticCacheConfig(req); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if s == nil || s.secretEncryptor == nil {
		result.Message = "语义模型秘钥解密服务不可用"
		return result, nil
	}
	apiKey, err := s.secretEncryptor.Decrypt(req.SemanticAPIKeyEncrypted)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		result.Message = "语义模型秘钥不可用"
		return result, nil
	}

	timeout := semanticEmbeddingDefaultTimeout
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	probeReq, err := buildSemanticEmbeddingRequest(probeCtx, req, "semantic cache connection probe", strings.TrimSpace(apiKey))
	if err != nil {
		result.Message = "语义模型服务地址不可用"
		return result, nil
	}
	if err := validateSemanticEmbeddingOutboundURL(probeCtx, probeReq.URL); err != nil {
		result.Message = "语义模型服务地址不允许访问"
		return result, nil
	}

	client := newSemanticEmbeddingHTTPClient(timeout)
	started := time.Now()
	resp, err := client.Do(probeReq)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded) || isNetTimeout(err) {
			result.Status = SemanticCacheConnectionStatusTimeout
			result.Message = "语义模型连接超时"
			return result, nil
		}
		result.Status = SemanticCacheConnectionStatusNetworkFail
		result.Message = "无法连接语义模型服务"
		return result, nil
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			result.Status = SemanticCacheConnectionStatusAuthFailed
			result.Message = "语义模型认证失败，请检查 API Key"
			return result, nil
		}
		result.Status = SemanticCacheConnectionStatusFailed
		result.Message = fmt.Sprintf("语义模型返回异常状态：%d", resp.StatusCode)
		return result, nil
	}

	embedding, model, err := decodeSemanticEmbeddingResponse(resp.Body)
	if err != nil {
		result.Status = SemanticCacheConnectionStatusFailed
		result.Message = "语义模型返回数据不可解析"
		return result, nil
	}
	dimension := len(embedding)
	result.EmbeddingDimension = &dimension
	if model != "" {
		result.Model = model
	}
	result.Success = true
	result.Status = SemanticCacheConnectionStatusSuccess
	result.Message = "语义模型连接成功"
	return result, nil
}

func semanticEmbeddingSkipped(reason string, model string) *SemanticEmbeddingResult {
	return &SemanticEmbeddingResult{
		Model:      model,
		Skipped:    true,
		SkipReason: reason,
	}
}

func buildSemanticEmbeddingRequest(ctx context.Context, cfg SemanticCacheConfig, input string, apiKey string) (*http.Request, error) {
	endpoint := buildOpenAIEndpointURL(cfg.SemanticModelBaseURL, "/v1/embeddings")
	payload := map[string]any{
		"model": cfg.SemanticModelName,
		"input": input,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return req, nil
}

func validateSemanticEmbeddingOutboundURL(ctx context.Context, u *url.URL) error {
	if u == nil || strings.TrimSpace(u.Hostname()) == "" {
		return errors.New("invalid semantic embedding service address")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid semantic embedding service address")
	}
	blocked, err := isPrivateOrLoopbackHost(ctx, u.Hostname())
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("blocked semantic embedding service address")
	}
	return nil
}

func newSemanticEmbeddingHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = safeDialContext
	return &http.Client{Timeout: timeout, Transport: transport}
}

func decodeSemanticEmbeddingResponse(body io.Reader) ([]float64, string, error) {
	var payload struct {
		Model string `json:"model"`
		Data  []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(body, 4<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("parse semantic embedding response: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, payload.Model, nil
	}
	return payload.Data[0].Embedding, strings.TrimSpace(payload.Model), nil
}
