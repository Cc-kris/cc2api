package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const opsAIAnalysisFeedbackNoteMaxLength = 500

func (s *OpsService) UpdateAIAnalysisReportFeedback(ctx context.Context, req *OpsAIAnalysisFeedbackRequest, taskID int64, feedbackUserID int64) (*OpsAIAnalysisFeedbackResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if taskID <= 0 {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_TASK_ID", "invalid task id")
	}
	if feedbackUserID <= 0 {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FEEDBACK_USER", "invalid feedback user")
	}
	if req == nil {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FEEDBACK", "invalid feedback request")
	}
	status := strings.TrimSpace(req.FeedbackStatus)
	if !isValidOpsAIAnalysisFeedbackStatus(status) {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FEEDBACK_STATUS", "invalid feedback status")
	}
	note := strings.TrimSpace(req.FeedbackNote)
	if len([]rune(note)) > opsAIAnalysisFeedbackNoteMaxLength {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_FEEDBACK_NOTE_TOO_LONG", "feedback note is too long")
	}
	report, err := s.opsRepo.UpdateAIAnalysisReportFeedback(ctx, &OpsAIAnalysisFeedbackInput{
		TaskID:         taskID,
		FeedbackStatus: status,
		FeedbackNote:   note,
		FeedbackUserID: feedbackUserID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_AI_ANALYSIS_REPORT_NOT_FOUND", "AI analysis report not found")
		}
		return nil, err
	}
	if report.FeedbackAt == nil || report.FeedbackUserID == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_AI_ANALYSIS_FEEDBACK_NOT_SAVED", "AI analysis feedback was not saved")
	}
	return &OpsAIAnalysisFeedbackResponse{
		TaskID:         report.TaskID,
		FeedbackStatus: report.FeedbackStatus,
		FeedbackNote:   report.FeedbackNote,
		FeedbackUserID: *report.FeedbackUserID,
		FeedbackAt:     *report.FeedbackAt,
	}, nil
}

func isValidOpsAIAnalysisFeedbackStatus(status string) bool {
	switch status {
	case OpsAIAnalysisFeedbackNone,
		OpsAIAnalysisFeedbackUseful,
		OpsAIAnalysisFeedbackNotUseful,
		OpsAIAnalysisFeedbackWrongCategory:
		return true
	default:
		return false
	}
}
