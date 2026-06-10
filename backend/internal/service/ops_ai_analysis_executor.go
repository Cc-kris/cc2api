package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const opsAIAnalysisExecutorName = "ops_ai_analysis_executor"

// opsAIAnalysisLLMExecutor calls a configured OpenAI-compatible LLM to generate an
// analysis report from sampled error data.
type opsAIAnalysisLLMExecutor struct {
	svc        *OpsService
	httpClient *http.Client
}

// NewOpsAIAnalysisLLMExecutor creates a real executor that calls the configured AI endpoint.
func NewOpsAIAnalysisLLMExecutor(svc *OpsService) OpsAIAnalysisTaskExecutor {
	return &opsAIAnalysisLLMExecutor{
		svc: svc,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (e *opsAIAnalysisLLMExecutor) ExecuteOpsAIAnalysisTask(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error) {
	if e.svc == nil || task == nil || contextData == nil {
		return 0, errors.New("invalid executor state")
	}

	cfg, err := e.svc.loadOpsAIAnalysisConfigForUpdate(ctx)
	if err != nil {
		return 0, fmt.Errorf("load AI config: %w", err)
	}
	if !cfg.Enabled {
		return 0, errors.New("AI analysis is not enabled")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return 0, errors.New("AI analysis base URL not configured")
	}
	if cfg.APIKeyEncrypted == "" {
		return 0, errors.New("AI analysis API key not configured")
	}
	if e.svc.secretEncryptor == nil {
		return 0, errors.New("secret encryptor not initialized")
	}
	plainKey, decErr := e.svc.secretEncryptor.Decrypt(cfg.APIKeyEncrypted)
	if decErr != nil {
		return 0, fmt.Errorf("decrypt AI API key: %w", decErr)
	}
	plainKey = strings.TrimSpace(plainKey)
	if plainKey == "" {
		return 0, errors.New("AI analysis API key is empty after decryption")
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}

	sampleCount := len(contextData.Samples)
	prompt := buildOpsAIAnalysisPrompt(task, contextData)

	report, err := e.callLLM(ctx, baseURL, plainKey, model, prompt)
	if err != nil {
		return sampleCount, fmt.Errorf("call LLM: %w", err)
	}
	report.TaskID = task.ID

	if insertErr := e.svc.opsRepo.InsertAIAnalysisReport(ctx, report); insertErr != nil {
		logger.LegacyPrintf(opsAIAnalysisExecutorName, "[%s] insert report failed task_id=%d err=%v", opsAIAnalysisExecutorName, task.ID, insertErr)
		return sampleCount, fmt.Errorf("insert report: %w", insertErr)
	}

	logger.LegacyPrintf(opsAIAnalysisExecutorName, "[%s] report saved task_id=%d samples=%d confidence=%s", opsAIAnalysisExecutorName, task.ID, sampleCount, report.Confidence)
	return sampleCount, nil
}

// callLLM sends a chat completion request and parses the JSON response.
func (e *opsAIAnalysisLLMExecutor) callLLM(ctx context.Context, baseURL, apiKey, model, prompt string) (*OpsAIAnalysisReport, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type requestBody struct {
		Model       string    `json:"model"`
		Messages    []message `json:"messages"`
		Temperature float64   `json:"temperature"`
	}

	body, _ := json.Marshal(requestBody{
		Model: model,
		Messages: []message{
			{Role: "system", Content: opsAIAnalysisSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
	})

	url := baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := string(raw)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return nil, fmt.Errorf("upstream returned HTTP %d: %s", resp.StatusCode, snippet)
	}

	// Parse OpenAI-style response
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, errors.New("no choices in API response")
	}

	content := strings.TrimSpace(apiResp.Choices[0].Message.Content)
	return parseOpsAIAnalysisReportFromLLM(content)
}

// parseOpsAIAnalysisReportFromLLM extracts the JSON block from the LLM response.
func parseOpsAIAnalysisReportFromLLM(content string) (*OpsAIAnalysisReport, error) {
	// Strip markdown code fences if present
	if idx := strings.Index(content, "```json"); idx >= 0 {
		content = content[idx+7:]
		if end := strings.Index(content, "```"); end >= 0 {
			content = content[:end]
		}
	} else if idx := strings.Index(content, "```"); idx >= 0 {
		content = content[idx+3:]
		if end := strings.Index(content, "```"); end >= 0 {
			content = content[:end]
		}
	}
	content = strings.TrimSpace(content)

	var parsed struct {
		Summary          string          `json:"summary"`
		RootCause        string          `json:"root_cause"`
		Confidence       string          `json:"confidence"`
		ImpactScope      json.RawMessage `json:"impact_scope"`
		Evidence         json.RawMessage `json:"evidence"`
		SuggestedActions json.RawMessage `json:"suggested_actions"`
		ErrorBreakdown   json.RawMessage `json:"error_breakdown"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		// Fallback: treat entire content as summary
		return &OpsAIAnalysisReport{
			Summary:         truncateOpsAIString(content, 2000),
			Confidence:      "low",
			ImpactScopeJSON: "{}",
			EvidenceJSON:    "[]",
			ActionsJSON:     "[]",
			BreakdownJSON:   "{}",
		}, nil
	}

	toJSON := func(raw json.RawMessage, fallback string) string {
		if len(raw) > 0 {
			return string(raw)
		}
		return fallback
	}

	confidence := strings.ToLower(strings.TrimSpace(parsed.Confidence))
	switch confidence {
	case "high", "medium", "low":
	default:
		confidence = "medium"
	}

	return &OpsAIAnalysisReport{
		Summary:         truncateOpsAIString(parsed.Summary, 2000),
		RootCause:       truncateOpsAIString(parsed.RootCause, 2000),
		Confidence:      confidence,
		ImpactScopeJSON: toJSON(parsed.ImpactScope, "{}"),
		EvidenceJSON:    toJSON(parsed.Evidence, "[]"),
		ActionsJSON:     toJSON(parsed.SuggestedActions, "[]"),
		BreakdownJSON:   toJSON(parsed.ErrorBreakdown, "{}"),
	}, nil
}

func truncateOpsAIString(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

// buildOpsAIAnalysisPrompt assembles the user prompt from task context and error samples.
func buildOpsAIAnalysisPrompt(task *OpsAIAnalysisTask, ctx *OpsAIAnalysisContext) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "分析时间窗口：%s ~ %s\n", task.TimeStart.Format("2006-01-02 15:04:05"), task.TimeEnd.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "样本总数：%d（下方展示最多 100 条）\n\n", ctx.Total)

	sb.WriteString("错误样本列表（每条包含：时间、分类、状态码、平台、模型、错误摘要、同类计数）：\n")
	for i, s := range ctx.Samples {
		if i >= 100 {
			break
		}
		category := s.ErrorCategory
		if s.ErrorSubcategory != "" {
			category += "/" + s.ErrorSubcategory
		}
		fmt.Fprintf(&sb, "%d. [%s] %s | HTTP %d | %s | %s | 摘要: %s | 同类: %d\n",
			i+1,
			s.OccurredAt.Format("15:04:05"),
			category,
			s.StatusCode,
			s.Platform,
			s.Model,
			truncateOpsAIString(s.Summary, 200),
			s.SameKindCount,
		)
	}

	sb.WriteString("\n请用中文严格按 JSON 格式返回分析报告，不要输出其他内容。")
	return sb.String()
}

const opsAIAnalysisSystemPrompt = `你是一名 AI API 网关运维专家，负责分析错误日志并生成结构化运维报告。

请根据用户提供的错误样本，严格以 JSON 格式输出以下结构，不要有任何额外文字：

{
  "summary": "一段话概括当前错误情况、主要影响和严重程度（100字以内）",
  "root_cause": "推断的根本原因，例如上游服务故障、配额耗尽、配置错误等（200字以内）",
  "confidence": "high / medium / low（对根因判断的置信度）",
  "impact_scope": {
    "affected_users": 受影响用户数（整数，若无法判断填0）,
    "affected_api_keys": 受影响API Key数（整数，若无法判断填0）,
    "affected_models": ["模型名列表"],
    "affected_upstream_accounts": 受影响上游账号数（整数，若无法判断填0）
  },
  "evidence": [
    "支持根因判断的关键证据，每条一句话",
    "证据2"
  ],
  "suggested_actions": [
    "建议采取的操作，按优先级排列",
    "操作2"
  ],
  "error_breakdown": {
    "错误类型名": 数量
  }
}`
