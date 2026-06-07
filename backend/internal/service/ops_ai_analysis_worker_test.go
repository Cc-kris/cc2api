package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type opsAIWorkerExecutorFunc func(ctx context.Context, task *OpsAIAnalysisTask) (int, error)

func (f opsAIWorkerExecutorFunc) ExecuteOpsAIAnalysisTask(ctx context.Context, task *OpsAIAnalysisTask) (int, error) {
	return f(ctx, task)
}

func newOpsAIWorkerService(t *testing.T, repo *opsRepoMock) *OpsService {
	t.Helper()
	svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	seedManualAIConfig(t, svc)
	return svc
}

func TestOpsAIAnalysisWorkerRunOnceCompletesTask(t *testing.T) {
	claimCount := 0
	updates := make([]*OpsAIAnalysisTaskUpdate, 0, 1)
	repo := &opsRepoMock{
		ClaimNextAIAnalysisTaskFn: func(ctx context.Context) (*OpsAIAnalysisTask, error) {
			claimCount++
			if claimCount == 1 {
				return &OpsAIAnalysisTask{ID: 42, Status: OpsAIAnalysisStatusRunning, SourceType: OpsAIAnalysisSourceUnifiedErrors, TriggerType: OpsAIAnalysisTriggerManual, Model: "gpt-5.5"}, nil
			}
			return nil, nil
		},
		UpdateAIAnalysisTaskFn: func(ctx context.Context, taskID int64, update *OpsAIAnalysisTaskUpdate) (*OpsAIAnalysisTask, error) {
			if taskID != 42 {
				t.Fatalf("taskID = %d", taskID)
			}
			updates = append(updates, update)
			return &OpsAIAnalysisTask{ID: taskID, Status: update.Status, SampleCount: *update.SampleCount}, nil
		},
	}
	svc := newOpsAIWorkerService(t, repo)
	svc.aiWorkerCtx = context.Background()
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask) (int, error) {
		if task.ID != 42 {
			t.Fatalf("task.ID = %d", task.ID)
		}
		return 3, nil
	}))

	svc.runAIAnalysisWorkerOnce()

	if claimCount != 2 {
		t.Fatalf("claimCount = %d", claimCount)
	}
	if len(updates) != 1 || updates[0].Status != OpsAIAnalysisStatusCompleted || updates[0].SampleCount == nil || *updates[0].SampleCount != 3 || updates[0].FinishedAt == nil {
		t.Fatalf("unexpected updates: %+v", updates)
	}
}

func TestOpsAIAnalysisWorkerRunOnceMarksFailed(t *testing.T) {
	boom := errors.New("upstream analysis failed")
	var failed *OpsAIAnalysisTaskUpdate
	claimed := false
	repo := &opsRepoMock{
		ClaimNextAIAnalysisTaskFn: func(ctx context.Context) (*OpsAIAnalysisTask, error) {
			if claimed {
				return nil, nil
			}
			claimed = true
			return &OpsAIAnalysisTask{ID: 43, Status: OpsAIAnalysisStatusRunning, SourceType: OpsAIAnalysisSourceUnifiedErrors, TriggerType: OpsAIAnalysisTriggerManual, Model: "gpt-5.5"}, nil
		},
		UpdateAIAnalysisTaskFn: func(ctx context.Context, taskID int64, update *OpsAIAnalysisTaskUpdate) (*OpsAIAnalysisTask, error) {
			failed = update
			return &OpsAIAnalysisTask{ID: taskID, Status: update.Status}, nil
		},
	}
	svc := newOpsAIWorkerService(t, repo)
	svc.aiWorkerCtx = context.Background()
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask) (int, error) {
		return 0, boom
	}))

	svc.runAIAnalysisWorkerOnce()

	if failed == nil || failed.Status != OpsAIAnalysisStatusFailed || failed.ErrorMessage == nil || !strings.Contains(*failed.ErrorMessage, "upstream analysis failed") || failed.FinishedAt == nil {
		t.Fatalf("unexpected failed update: %+v", failed)
	}
}

func TestOpsAIAnalysisWorkerMarksDeadlineExceededFailed(t *testing.T) {
	var failed *OpsAIAnalysisTaskUpdate
	repo := &opsRepoMock{
		UpdateAIAnalysisTaskFn: func(ctx context.Context, taskID int64, update *OpsAIAnalysisTaskUpdate) (*OpsAIAnalysisTask, error) {
			failed = update
			return &OpsAIAnalysisTask{ID: taskID, Status: update.Status}, nil
		},
	}
	svc := newOpsAIWorkerService(t, repo)
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask) (int, error) {
		return 0, context.DeadlineExceeded
	}))

	svc.executeAIAnalysisTask(context.Background(), &OpsAIAnalysisTask{ID: 44})

	if failed == nil || failed.Status != OpsAIAnalysisStatusFailed || failed.ErrorMessage == nil || !strings.Contains(*failed.ErrorMessage, "超时") || failed.FinishedAt == nil {
		t.Fatalf("deadline exceeded should mark failed: %+v", failed)
	}
}

func TestOpsAIAnalysisWorkerParentCancelKeepsRunning(t *testing.T) {
	updated := false
	repo := &opsRepoMock{
		UpdateAIAnalysisTaskFn: func(ctx context.Context, taskID int64, update *OpsAIAnalysisTaskUpdate) (*OpsAIAnalysisTask, error) {
			updated = true
			return &OpsAIAnalysisTask{ID: taskID, Status: update.Status}, nil
		},
	}
	svc := newOpsAIWorkerService(t, repo)
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask) (int, error) {
		return 0, context.Canceled
	}))
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	svc.executeAIAnalysisTask(parent, &OpsAIAnalysisTask{ID: 45})

	if updated {
		t.Fatalf("parent cancellation should keep task running for later reclaim")
	}
}

func TestOpsAIAnalysisWorkerSkipsWhenNotConfigured(t *testing.T) {
	claimed := false
	repo := &opsRepoMock{ClaimNextAIAnalysisTaskFn: func(ctx context.Context) (*OpsAIAnalysisTask, error) {
		claimed = true
		return nil, nil
	}}
	svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.aiWorkerCtx = context.Background()

	svc.runAIAnalysisWorkerOnce()

	if claimed {
		t.Fatalf("worker should not claim tasks when AI analysis is not configured")
	}
}

func TestTruncateOpsAIAnalysisError(t *testing.T) {
	if got := truncateOpsAIAnalysisError("  abc  ", 10); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := truncateOpsAIAnalysisError("abcdef", 3); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := truncateOpsAIAnalysisError("abcdef", 0); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestOpsAIAnalysisTaskTimeoutUsesConfig(t *testing.T) {
	svc := newOpsAIWorkerService(t, &opsRepoMock{})
	if got := svc.aiAnalysisTaskTimeout(); got != 60*time.Second {
		t.Fatalf("timeout = %s", got)
	}
}
