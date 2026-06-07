package service

import (
	"context"
	"errors"
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
	ExecuteOpsAIAnalysisTask(ctx context.Context, task *OpsAIAnalysisTask) (int, error)
}

type noopOpsAIAnalysisTaskExecutor struct{}

func (noopOpsAIAnalysisTaskExecutor) ExecuteOpsAIAnalysisTask(ctx context.Context, task *OpsAIAnalysisTask) (int, error) {
	if task == nil {
		return 0, errors.New("AI analysis task is nil")
	}
	return task.SampleCount, nil
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
	executor := s.aiAnalysisTaskExecutor
	if executor == nil {
		executor = noopOpsAIAnalysisTaskExecutor{}
	}

	sampleCount, err := executor.ExecuteOpsAIAnalysisTask(ctx, task)
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

func (s *OpsService) SetAIAnalysisTaskExecutor(executor OpsAIAnalysisTaskExecutor) {
	if s == nil {
		return
	}
	s.aiExecutorMu.Lock()
	defer s.aiExecutorMu.Unlock()
	s.aiAnalysisTaskExecutor = executor
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
