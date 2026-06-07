package service

import (
	"context"
	"database/sql"
	"errors"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
	result := &OpsAIAnalysisTaskDetailResponse{Task: task}
	report, err := s.opsRepo.GetAIAnalysisReport(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, nil
		}
		return nil, err
	}
	result.Report = report
	return result, nil
}
