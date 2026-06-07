package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	opsAIAnalysisWorkerName            = "ops_ai_analysis_worker"
	opsAIAnalysisWorkerDefaultInterval = 5 * time.Second
	opsAIAnalysisWorkerDefaultTimeout  = 2 * time.Minute
)

type OpsAIAnalysisTaskExecutor interface {
	ExecuteOpsAIAnalysisTask(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error)
}

type opsAIAnalysisSampleExecutor struct {
	svc *OpsService
}

func (e *opsAIAnalysisSampleExecutor) ExecuteOpsAIAnalysisTask(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error) {
	if task == nil {
		return 0, errors.New("AI analysis task is nil")
	}
	if contextData == nil {
		return 0, errors.New("AI analysis context is nil")
	}
	return len(contextData.Samples), nil
}

func (s *OpsService) StartAIAnalysisWorker() {
	if s == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] not started (ops disabled)", opsAIAnalysisWorkerName)
		return
	}
	if s.opsRepo == nil || s.settingRepo == nil {
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] not started (missing deps)", opsAIAnalysisWorkerName)
		return
	}
	s.aiWorkerStartOnce.Do(func() {
		s.aiWorkerCtx, s.aiWorkerCancel = context.WithCancel(context.Background())
		go s.runAIAnalysisWorkerLoop()
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] started interval=%s timeout=%s", opsAIAnalysisWorkerName, s.aiAnalysisWorkerInterval(), s.aiAnalysisTaskTimeout())
	})
}

func (s *OpsService) StopAIAnalysisWorker() {
	if s == nil {
		return
	}
	s.aiWorkerStopOnce.Do(func() {
		if s.aiWorkerCancel != nil {
			s.aiWorkerCancel()
		}
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] stopped", opsAIAnalysisWorkerName)
	})
}

func (s *OpsService) runAIAnalysisWorkerLoop() {
	interval := s.aiAnalysisWorkerInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.runAIAnalysisWorkerOnce()
	for {
		select {
		case <-s.aiWorkerCtx.Done():
			return
		case <-ticker.C:
			s.runAIAnalysisWorkerOnce()
		}
	}
}

func (s *OpsService) runAIAnalysisWorkerOnce() {
	if s == nil || s.opsRepo == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&s.aiWorkerRunning, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.aiWorkerRunning, 0)

	ctx := s.aiWorkerCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return
	}
	cfg, err := s.loadOpsAIAnalysisConfigForUpdate(ctx)
	if err != nil {
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] load config failed: %v", opsAIAnalysisWorkerName, err)
		return
	}
	normalizeOpsAIAnalysisConfig(cfg)
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" || strings.TrimSpace(cfg.APIKeyEncrypted) == "" {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		task, err := s.opsRepo.ClaimNextAIAnalysisTask(ctx)
		if err != nil {
			logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] claim task failed: %v", opsAIAnalysisWorkerName, err)
			return
		}
		if task == nil {
			return
		}
		s.executeAIAnalysisTask(ctx, task)
	}
}

func (s *OpsService) executeAIAnalysisTask(parent context.Context, task *OpsAIAnalysisTask) {
	if s == nil || s.opsRepo == nil || task == nil {
		return
	}
	timeout := s.aiAnalysisTaskTimeout()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] task started id=%d source=%s trigger=%s model=%s", opsAIAnalysisWorkerName, task.ID, task.SourceType, task.TriggerType, task.Model)
	analysisContext, err := s.BuildAIAnalysisContext(ctx, task)
	if err != nil {
		if errors.Is(parent.Err(), context.Canceled) || errors.Is(err, context.Canceled) && errors.Is(parent.Err(), context.Canceled) {
			logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] task interrupted during sampling id=%d err=%v", opsAIAnalysisWorkerName, task.ID, err)
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.markAIAnalysisTaskFailed(task.ID, errors.New("AI 分析任务执行超时"))
			return
		}
		s.markAIAnalysisTaskFailed(task.ID, err)
		return
	}
	executor := s.getAIAnalysisTaskExecutor()

	sampleCount, err := executor.ExecuteOpsAIAnalysisTask(ctx, task, analysisContext)
	if err != nil {
		if errors.Is(parent.Err(), context.Canceled) || errors.Is(err, context.Canceled) && errors.Is(parent.Err(), context.Canceled) {
			logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] task interrupted id=%d err=%v", opsAIAnalysisWorkerName, task.ID, err)
			return
		}
		msg := truncateOpsAIAnalysisError(err.Error(), 500)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			msg = "AI 分析任务执行超时"
		}
		finishedAt := time.Now()
		_, updateErr := s.opsRepo.UpdateAIAnalysisTask(context.Background(), task.ID, &OpsAIAnalysisTaskUpdate{Status: OpsAIAnalysisStatusFailed, ErrorMessage: &msg, FinishedAt: &finishedAt})
		if updateErr != nil {
			logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] mark failed failed id=%d err=%v", opsAIAnalysisWorkerName, task.ID, updateErr)
			return
		}
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] task failed id=%d err=%s", opsAIAnalysisWorkerName, task.ID, msg)
		return
	}
	if sampleCount < 0 {
		sampleCount = 0
	}
	finishedAt := time.Now()
	_, err = s.opsRepo.UpdateAIAnalysisTask(context.Background(), task.ID, &OpsAIAnalysisTaskUpdate{Status: OpsAIAnalysisStatusCompleted, SampleCount: &sampleCount, FinishedAt: &finishedAt})
	if err != nil {
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] mark completed failed id=%d err=%v", opsAIAnalysisWorkerName, task.ID, err)
		return
	}
	logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] task completed id=%d sample_count=%d", opsAIAnalysisWorkerName, task.ID, sampleCount)
}

func (s *OpsService) markAIAnalysisTaskFailed(taskID int64, err error) {
	if s == nil || s.opsRepo == nil || taskID <= 0 || err == nil {
		return
	}
	msg := truncateOpsAIAnalysisError(err.Error(), 500)
	finishedAt := time.Now()
	_, updateErr := s.opsRepo.UpdateAIAnalysisTask(context.Background(), taskID, &OpsAIAnalysisTaskUpdate{Status: OpsAIAnalysisStatusFailed, ErrorMessage: &msg, FinishedAt: &finishedAt})
	if updateErr != nil {
		logger.LegacyPrintf("service.ops_ai_analysis_worker", "[%s] mark failed failed id=%d err=%v", opsAIAnalysisWorkerName, taskID, updateErr)
	}
}

func (s *OpsService) SetAIAnalysisTaskExecutor(executor OpsAIAnalysisTaskExecutor) {
	if s == nil {
		return
	}
	s.aiExecutorMu.Lock()
	defer s.aiExecutorMu.Unlock()
	s.aiAnalysisTaskExecutor = executor
}

func (s *OpsService) getAIAnalysisTaskExecutor() OpsAIAnalysisTaskExecutor {
	if s == nil {
		return &opsAIAnalysisSampleExecutor{}
	}
	s.aiExecutorMu.Lock()
	executor := s.aiAnalysisTaskExecutor
	s.aiExecutorMu.Unlock()
	if executor == nil {
		executor = &opsAIAnalysisSampleExecutor{svc: s}
	}
	return executor
}

func (s *OpsService) aiAnalysisWorkerInterval() time.Duration {
	return opsAIAnalysisWorkerDefaultInterval
}

func (s *OpsService) aiAnalysisTaskTimeout() time.Duration {
	if s == nil {
		return opsAIAnalysisWorkerDefaultTimeout
	}
	cfg, err := s.loadOpsAIAnalysisConfigForUpdate(context.Background())
	if err != nil {
		return opsAIAnalysisWorkerDefaultTimeout
	}
	normalizeOpsAIAnalysisConfig(cfg)
	if cfg.TimeoutSeconds > 0 {
		return time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return opsAIAnalysisWorkerDefaultTimeout
}

func (s *OpsService) BuildAIAnalysisContext(ctx context.Context, task *OpsAIAnalysisTask) (*OpsAIAnalysisContext, error) {
	if s == nil || task == nil {
		return nil, errors.New("AI analysis task is nil")
	}
	filter, err := unifiedErrorFilterFromAIAnalysisTask(task)
	if err != nil {
		return nil, err
	}
	maxSamples := s.aiAnalysisMaxSamples()
	filter.Page = 1
	filter.PageSize = maxSamples
	filter.SortBy = "occurred_at"
	filter.SortOrder = "desc"
	filter.AIAnalysis = OpsUnifiedAIAnalysisAll

	list, err := s.opsRepo.ListUnifiedErrorsForAIAnalysis(ctx, filter, maxSamples)
	if err != nil {
		return nil, err
	}
	out := &OpsAIAnalysisContext{Task: task, Total: 0}
	if list != nil {
		out.Total = list.Total
		out.Samples = make([]*OpsAIAnalysisSample, 0, len(list.Items))
		for _, item := range list.Items {
			if sample := buildOpsAIAnalysisSample(item); sample != nil {
				out.Samples = append(out.Samples, sample)
			}
		}
	}
	return out, nil
}

func unifiedErrorFilterFromAIAnalysisTask(task *OpsAIAnalysisTask) (*OpsUnifiedErrorListFilter, error) {
	if task == nil {
		return nil, errors.New("AI analysis task is nil")
	}
	filter := &OpsUnifiedErrorListFilter{StartTime: &task.TimeStart, EndTime: &task.TimeEnd}
	var raw map[string]any
	if strings.TrimSpace(task.FiltersJSON) != "" {
		if err := json.Unmarshal([]byte(task.FiltersJSON), &raw); err != nil {
			return nil, err
		}
	}
	if raw == nil {
		raw = map[string]any{}
	}
	converted, _, err := opsUnifiedFilterFromAIAnalysisFilters(raw, task.TimeStart, task.TimeEnd)
	if err != nil {
		return nil, err
	}
	converted.Page = filter.Page
	converted.PageSize = filter.PageSize
	return converted, nil
}

func buildOpsAIAnalysisSample(item *OpsUnifiedErrorItem) *OpsAIAnalysisSample {
	if item == nil {
		return nil
	}
	return &OpsAIAnalysisSample{
		ID:                     item.ID,
		OccurredAt:             item.OccurredAt,
		ErrorCategory:          item.ErrorCategory,
		ErrorSubcategory:       item.ErrorSubcategory,
		ClientErrorSubcategory: item.ClientErrorSubcategory,
		ErrorResult:            item.ErrorResult,
		Severity:               item.Severity,
		StatusCode:             item.StatusCode,
		Platform:               item.Platform,
		Model:                  item.Model,
		GroupID:                entityID(item.Group),
		APIKeyID:               entityID(item.APIKey),
		UserID:                 entityID(item.User),
		UpstreamAccountID:      entityID(item.UpstreamAccount),
		Summary:                redactAIContextText(item.Summary, 500),
		SameKindCount:          item.SameKindCount,
	}
}

func entityID(ref *OpsUnifiedEntityRef) *int64 {
	if ref == nil || ref.ID <= 0 {
		return nil
	}
	id := ref.ID
	return &id
}

func (s *OpsService) aiAnalysisMaxSamples() int {
	cfg, err := s.loadOpsAIAnalysisConfigForUpdate(context.Background())
	if err != nil {
		return 50
	}
	normalizeOpsAIAnalysisConfig(cfg)
	if cfg.MaxSamples > 0 {
		return cfg.MaxSamples
	}
	return 50
}

func redactAIContextText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = redactSensitiveText(value)
	return truncateRunes(value, maxRunes)
}

var (
	opsAIEmailRedactRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	opsAIKeyRedactRe   = regexp.MustCompile(`(?i)\b(?:sk|ak|pk|rk|ya29|ghp|gho|ghu|ghs|github_pat)[-_A-Za-z0-9]{8,}\b`)
)

func redactSensitiveText(value string) string {
	value = opsAIEmailRedactRe.ReplaceAllString(value, "[REDACTED_EMAIL]")
	value = opsAIKeyRedactRe.ReplaceAllString(value, "[REDACTED_TOKEN]")
	patterns := []string{"authorization", "bearer", "api_key", "apikey", "x-api-key", "cookie", "token", "secret", "password", "proxy"}
	lower := strings.ToLower(value)
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return "[REDACTED]"
		}
	}
	return value
}

func truncateOpsAIAnalysisError(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
