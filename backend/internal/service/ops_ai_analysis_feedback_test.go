package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestUpdateAIAnalysisReportFeedback(t *testing.T) {
	feedbackAt := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	var gotInput *OpsAIAnalysisFeedbackInput
	svc := NewOpsService(&opsRepoMock{
		UpdateAIAnalysisReportFeedbackFn: func(ctx context.Context, input *OpsAIAnalysisFeedbackInput) (*OpsAIAnalysisReport, error) {
			gotInput = input
			return &OpsAIAnalysisReport{
				TaskID:         input.TaskID,
				FeedbackStatus: input.FeedbackStatus,
				FeedbackNote:   input.FeedbackNote,
				FeedbackUserID: &input.FeedbackUserID,
				FeedbackAt:     &feedbackAt,
				CreatedAt:      feedbackAt.Add(-time.Hour),
				UpdatedAt:      feedbackAt,
			}, nil
		},
	}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.UpdateAIAnalysisReportFeedback(context.Background(), &OpsAIAnalysisFeedbackRequest{
		FeedbackStatus: " useful ",
		FeedbackNote:   " 判断准确 ",
	}, 77, 9)
	requireNoError(t, err)
	if resp.TaskID != 77 || resp.FeedbackStatus != OpsAIAnalysisFeedbackUseful || resp.FeedbackNote != "判断准确" || resp.FeedbackUserID != 9 || !resp.FeedbackAt.Equal(feedbackAt) {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if gotInput == nil || gotInput.TaskID != 77 || gotInput.FeedbackStatus != OpsAIAnalysisFeedbackUseful || gotInput.FeedbackNote != "判断准确" || gotInput.FeedbackUserID != 9 {
		t.Fatalf("unexpected repo input: %+v", gotInput)
	}
}

func TestUpdateAIAnalysisReportFeedbackValidation(t *testing.T) {
	svc := NewOpsService(&opsRepoMock{}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	cases := []struct {
		name   string
		req    *OpsAIAnalysisFeedbackRequest
		taskID int64
		userID int64
		reason string
	}{
		{name: "invalid task", req: &OpsAIAnalysisFeedbackRequest{FeedbackStatus: OpsAIAnalysisFeedbackUseful}, taskID: 0, userID: 1, reason: "OPS_AI_ANALYSIS_INVALID_TASK_ID"},
		{name: "invalid user", req: &OpsAIAnalysisFeedbackRequest{FeedbackStatus: OpsAIAnalysisFeedbackUseful}, taskID: 1, userID: 0, reason: "OPS_AI_ANALYSIS_INVALID_FEEDBACK_USER"},
		{name: "nil request", req: nil, taskID: 1, userID: 1, reason: "OPS_AI_ANALYSIS_INVALID_FEEDBACK"},
		{name: "bad status", req: &OpsAIAnalysisFeedbackRequest{FeedbackStatus: "bad"}, taskID: 1, userID: 1, reason: "OPS_AI_ANALYSIS_INVALID_FEEDBACK_STATUS"},
		{name: "long note", req: &OpsAIAnalysisFeedbackRequest{FeedbackStatus: OpsAIAnalysisFeedbackUseful, FeedbackNote: strings.Repeat("你", 501)}, taskID: 1, userID: 1, reason: "OPS_AI_ANALYSIS_FEEDBACK_NOTE_TOO_LONG"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.UpdateAIAnalysisReportFeedback(context.Background(), tc.req, tc.taskID, tc.userID)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := infraerrors.Reason(err); got != tc.reason {
				t.Fatalf("reason = %s, want %s; err=%v", got, tc.reason, err)
			}
		})
	}
}

func TestUpdateAIAnalysisReportFeedbackReportNotFound(t *testing.T) {
	svc := NewOpsService(&opsRepoMock{
		UpdateAIAnalysisReportFeedbackFn: func(ctx context.Context, input *OpsAIAnalysisFeedbackInput) (*OpsAIAnalysisReport, error) {
			return nil, sql.ErrNoRows
		},
	}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := svc.UpdateAIAnalysisReportFeedback(context.Background(), &OpsAIAnalysisFeedbackRequest{FeedbackStatus: OpsAIAnalysisFeedbackUseful}, 77, 9)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, infraerrors.NotFound("OPS_AI_ANALYSIS_REPORT_NOT_FOUND", "AI analysis report not found")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
