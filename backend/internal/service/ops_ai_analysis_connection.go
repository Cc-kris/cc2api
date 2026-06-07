package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	OpsAIAnalysisConnectionStatusSuccess     = "success"
	OpsAIAnalysisConnectionStatusConfigError = "config_error"
	OpsAIAnalysisConnectionStatusAuthFailed  = "auth_failed"
	OpsAIAnalysisConnectionStatusNetworkFail = "network_failed"
	OpsAIAnalysisConnectionStatusTimeout     = "timeout"
	OpsAIAnalysisConnectionStatusFailed      = "failed"
)

type OpsAIAnalysisConnectionTestResult struct {
	Success       bool   `json:"success"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	InterfaceType string `json:"interface_type"`
	BaseURL       string `json:"base_url"`
	Model         string `json:"model"`
	DurationMS    int64  `json:"duration_ms"`
	HTTPStatus    int    `json:"http_status,omitempty"`
}

func (s *OpsService) TestOpsAIAnalysisConnection(ctx context.Context) (*OpsAIAnalysisConnectionTestResult, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := s.loadOpsAIAnalysisConfigForUpdate(ctx)
	if err != nil {
		return nil, err
	}
	normalizeOpsAIAnalysisConfig(cfg)
	result := &OpsAIAnalysisConnectionTestResult{
		Status:        OpsAIAnalysisConnectionStatusConfigError,
		InterfaceType: cfg.InterfaceType,
		BaseURL:       cfg.BaseURL,
		Model:         cfg.Model,
	}
	if cfg.BaseURL == "" || cfg.Model == "" || cfg.APIKeyEncrypted == "" {
		result.Message = "请先配置 AI 分析服务"
		return result, nil
	}
	if s == nil || s.secretEncryptor == nil {
		result.Message = "AI 分析秘钥解密服务不可用"
		return result, nil
	}
	apiKey, err := s.secretEncryptor.Decrypt(cfg.APIKeyEncrypted)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		result.Message = "AI 分析秘钥不可用"
		return result, nil
	}
	apiKey = strings.TrimSpace(apiKey)

	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 60
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout > 30*time.Second {
		// 测试连接不需要等待完整分析超时，避免后台页面长时间挂起。
		timeout = 30 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := buildOpsAIAnalysisProbeRequest(probeCtx, cfg, apiKey)
	if err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if err := validateOpsAIAnalysisOutboundURL(probeCtx, req.URL); err != nil {
		result.Status = OpsAIAnalysisConnectionStatusConfigError
		result.Message = "AI 分析服务地址不允许访问"
		return result, nil
	}

	client := s.aiAnalysisHTTPClient
	if client == nil {
		client = newOpsAIAnalysisHTTPClient(timeout)
	}
	started := time.Now()
	resp, err := client.Do(req)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded) || isNetTimeout(err) {
			result.Status = OpsAIAnalysisConnectionStatusTimeout
			result.Message = "AI 分析服务连接超时"
			return result, nil
		}
		result.Status = OpsAIAnalysisConnectionStatusNetworkFail
		result.Message = "无法连接 AI 分析服务"
		return result, nil
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		result.Status = OpsAIAnalysisConnectionStatusSuccess
		result.Message = "AI 分析服务连接成功"
		return result, nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		result.Status = OpsAIAnalysisConnectionStatusAuthFailed
		result.Message = "AI 分析服务认证失败，请检查 API 秘钥"
		return result, nil
	}
	result.Status = OpsAIAnalysisConnectionStatusFailed
	result.Message = fmt.Sprintf("AI 分析服务返回异常状态码 %d", resp.StatusCode)
	return result, nil
}

func buildOpsAIAnalysisProbeRequest(ctx context.Context, cfg *OpsAIAnalysisConfig, apiKey string) (*http.Request, error) {
	if cfg == nil {
		return nil, errors.New("AI 分析配置无效")
	}
	contentType := "application/json"
	var endpoint string
	var payload map[string]any
	switch cfg.InterfaceType {
	case "responses":
		endpoint = buildOpenAIResponsesURL(cfg.BaseURL)
		payload = map[string]any{"model": cfg.Model, "input": "ping", "max_output_tokens": 16}
	case "openai_compatible":
		endpoint = buildOpenAIChatCompletionsURL(cfg.BaseURL)
		payload = map[string]any{"model": cfg.Model, "messages": []map[string]string{{"role": "user", "content": "ping"}}, "max_tokens": 16}
	case "anthropic_compatible":
		endpoint = buildOpenAIEndpointURL(cfg.BaseURL, "/v1/messages")
		payload = map[string]any{"model": cfg.Model, "max_tokens": 16, "messages": []map[string]string{{"role": "user", "content": "ping"}}}
	case "gemini_compatible":
		endpoint = buildGeminiCompatibleGenerateContentURL(cfg.BaseURL, cfg.Model)
		payload = map[string]any{"contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "ping"}}}}, "generationConfig": map[string]int{"maxOutputTokens": 16}}
	default:
		return nil, errors.New("interface_type must be one of openai_compatible, responses, anthropic_compatible or gemini_compatible")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	switch cfg.InterfaceType {
	case "anthropic_compatible":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "gemini_compatible":
		req.Header.Set("x-goog-api-key", apiKey)
	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return req, nil
}

func buildGeminiCompatibleGenerateContentURL(base string, model string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	model = url.PathEscape(strings.TrimSpace(model))
	if strings.Contains(base, ":generateContent") {
		return base
	}
	if strings.Contains(base, "/models/") {
		return base + ":generateContent"
	}
	if openAIBaseURLHasVersionSuffix(base) || strings.HasSuffix(base, "/v1beta") || strings.HasSuffix(base, "/v1") {
		return base + "/models/" + model + ":generateContent"
	}
	return base + "/v1beta/models/" + model + ":generateContent"
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func validateOpsAIAnalysisOutboundURL(ctx context.Context, u *url.URL) error {
	if u == nil || strings.TrimSpace(u.Hostname()) == "" {
		return errors.New("invalid AI analysis service address")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid AI analysis service address")
	}
	blocked, err := isPrivateOrLoopbackHost(ctx, u.Hostname())
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("blocked AI analysis service address")
	}
	return nil
}

func newOpsAIAnalysisHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = safeDialContext
	return &http.Client{Timeout: timeout, Transport: transport}
}
