package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestGetAIAnalysisTaskDetailWithReport(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	repo := &opsRepoMock{
		GetAIAnalysisTaskFn: func(ctx context.Context, taskID int64) (*OpsAIAnalysisTask, error) {
			if taskID != 7 {
				t.Fatalf("taskID = %d", taskID)
			}
			return &OpsAIAnalysisTask{ID: taskID, Status: OpsAIAnalysisStatusCompleted, FinishedAt: &now}, nil
		},
		GetAIAnalysisReportFn: func(ctx context.Context, taskID int64) (*OpsAIAnalysisReport, error) {
			return &OpsAIAnalysisReport{TaskID: taskID, Summary: "上游错误", Confidence: "high", ImpactScope: map[string]any{"affected_users": float64(2)}, Evidence: []any{"e1"}, SuggestedActions: []any{"a1"}, ErrorBreakdown: map[string]any{"upstream": float64(1)}}, nil
		},
	}
	svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)

	got, err := svc.GetAIAnalysisTaskDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetAIAnalysisTaskDetail() error = %v", err)
	}
	if got.Task == nil || got.Task.ID != 7 || got.Report == nil || got.Report.Summary != "上游错误" {
		t.Fatalf("unexpected detail: %+v", got)
	}
}

func TestGetAIAnalysisTaskDetailStates(t *testing.T) {
	t.Run("pending_without_report", func(t *testing.T) {
		repo := &opsRepoMock{
			GetAIAnalysisTaskFn: func(ctx context.Context, taskID int64) (*OpsAIAnalysisTask, error) {
				return &OpsAIAnalysisTask{ID: taskID, Status: OpsAIAnalysisStatusPending}, nil
			},
			GetAIAnalysisReportFn: func(ctx context.Context, taskID int64) (*OpsAIAnalysisReport, error) {
				return nil, sql.ErrNoRows
			},
		}
		svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		got, err := svc.GetAIAnalysisTaskDetail(context.Background(), 8)
		if err != nil || got.Report != nil || got.Task.Status != OpsAIAnalysisStatusPending {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		repo := &opsRepoMock{GetAIAnalysisTaskFn: func(ctx context.Context, taskID int64) (*OpsAIAnalysisTask, error) { return nil, sql.ErrNoRows }}
		svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		_, err := svc.GetAIAnalysisTaskDetail(context.Background(), 404)
		if !infraerrors.IsNotFound(err) || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		svc := NewOpsService(&opsRepoMock{}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		_, err := svc.GetAIAnalysisTaskDetail(context.Background(), 0)
		if !infraerrors.IsBadRequest(err) {
			t.Fatalf("err = %v", err)
		}
	})
}
