package service

import (
	"context"
	"database/sql"
	"errors"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

func (s *OpsService) GetAIAnalysisTaskDetail(ctx context.Context, taskID int64) (*OpsAIAnalysisTaskDetailResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if taskID <= 0 {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_TASK_ID", "invalid task id")
	}
	task, err := s.opsRepo.GetAIAnalysisTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_AI_ANALYSIS_TASK_NOT_FOUND", "AI analysis task not found")
		}
		return nil, err
	}
	sanitizedTask := *task
	sanitizedTask.ErrorMessage = logredact.RedactAIContext(task.ErrorMessage, 500)
	result := &OpsAIAnalysisTaskDetailResponse{Task: &sanitizedTask}
	report, err := s.opsRepo.GetAIAnalysisReport(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, nil
		}
		return nil, err
	}
	result.Report = sanitizeOpsAIAnalysisReport(report)
	return result, nil
}

func (s *OpsService) GetLatestAutoAIAnalysisTask(ctx context.Context) (*OpsAIAnalysisTaskDetailResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	task, err := s.opsRepo.GetLatestAutoAIAnalysisTask(ctx)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, nil
	}
	sanitizedTask := *task
	sanitizedTask.ErrorMessage = logredact.RedactAIContext(task.ErrorMessage, 500)
	result := &OpsAIAnalysisTaskDetailResponse{Task: &sanitizedTask}
	if task.Status == OpsAIAnalysisStatusCompleted {
		report, err := s.opsRepo.GetAIAnalysisReport(ctx, task.ID)
		if err == nil {
			result.Report = sanitizeOpsAIAnalysisReport(report)
		}
	}
	return result, nil
}

func sanitizeOpsAIAnalysisReport(report *OpsAIAnalysisReport) *OpsAIAnalysisReport {
	if report == nil {
		return nil
	}
	out := *report
	out.Summary = logredact.RedactAIContext(report.Summary, 500)
	out.RootCause = logredact.RedactAIContext(report.RootCause, 500)
	out.FeedbackNote = logredact.RedactAIContext(report.FeedbackNote, 500)
	out.ImpactScope = sanitizeOpsAIAnalysisValue(report.ImpactScope)
	out.Evidence = sanitizeOpsAIAnalysisValue(report.Evidence)
	out.SuggestedActions = sanitizeOpsAIAnalysisValue(report.SuggestedActions)
	out.ErrorBreakdown = sanitizeOpsAIAnalysisValue(report.ErrorBreakdown)
	return &out
}

func sanitizeOpsAIAnalysisValue(value any) any {
	switch v := value.(type) {
	case string:
		return logredact.RedactAIContext(v, 500)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeOpsAIAnalysisValue(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = sanitizeOpsAIAnalysisValue(item)
		}
		return out
	default:
		return value
	}
}
