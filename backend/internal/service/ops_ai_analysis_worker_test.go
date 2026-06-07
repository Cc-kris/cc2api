package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type opsAIWorkerExecutorFunc func(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error)

func (f opsAIWorkerExecutorFunc) ExecuteOpsAIAnalysisTask(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error) {
	return f(ctx, task, contextData)
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
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error) {
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
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error) {
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
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error) {
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
	svc.SetAIAnalysisTaskExecutor(opsAIWorkerExecutorFunc(func(ctx context.Context, task *OpsAIAnalysisTask, contextData *OpsAIAnalysisContext) (int, error) {
		return 0, context.Canceled
	}))
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	svc.executeAIAnalysisTask(parent, &OpsAIAnalysisTask{ID: 45})

	if updated {
		t.Fatalf("parent cancellation should keep task running for later reclaim")
	}
}

func TestOpsAIAnalysisWorkerSamplingParentCancelKeepsRunning(t *testing.T) {
	updated := false
	repo := &opsRepoMock{
		ListUnifiedErrorsForAIAnalysisFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter, maxSamples int) (*OpsUnifiedErrorList, error) {
			return nil, context.Canceled
		},
		UpdateAIAnalysisTaskFn: func(ctx context.Context, taskID int64, update *OpsAIAnalysisTaskUpdate) (*OpsAIAnalysisTask, error) {
			updated = true
			return &OpsAIAnalysisTask{ID: taskID, Status: update.Status}, nil
		},
	}
	svc := newOpsAIWorkerService(t, repo)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	svc.executeAIAnalysisTask(parent, &OpsAIAnalysisTask{ID: 46, TimeStart: time.Now(), TimeEnd: time.Now().Add(time.Minute)})
	if updated {
		t.Fatalf("sampling parent cancellation should keep task running")
	}
}

func TestRedactAIContextTextRedactsEmailAndToken(t *testing.T) {
	got := redactAIContextText("contact real-user@example.com with sk-1234567890abcdef", 500)
	if strings.Contains(got, "real-user@example.com") || strings.Contains(got, "sk-1234567890abcdef") {
		t.Fatalf("sensitive text leaked: %s", got)
	}
	if !strings.Contains(got, "[REDACTED") {
		t.Fatalf("missing redaction marker: %s", got)
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

func TestBuildAIAnalysisContextSamplesAreRedacted(t *testing.T) {
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	apiKeyID := int64(22)
	userID := int64(11)
	groupID := int64(33)
	accountID := int64(44)
	capturedFilter := (*OpsUnifiedErrorListFilter)(nil)
	repo := &opsRepoMock{
		ListUnifiedErrorsForAIAnalysisFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter, maxSamples int) (*OpsUnifiedErrorList, error) {
			capturedFilter = filter
			if maxSamples != 50 || filter.PageSize != 50 {
				t.Fatalf("max_samples/page_size = %d/%d", maxSamples, filter.PageSize)
			}
			if filter.StartTime == nil || !filter.StartTime.Equal(start) || filter.EndTime == nil || !filter.EndTime.Equal(end) {
				t.Fatalf("unexpected time range: %+v", filter)
			}
			return &OpsUnifiedErrorList{Total: 2, Items: []*OpsUnifiedErrorItem{
				{
					ID:               1,
					OccurredAt:       start.Add(time.Minute),
					ErrorCategory:    "upstream",
					ErrorSubcategory: "upstream_http_error",
					ErrorResult:      OpsUnifiedErrorResultFinalFailed,
					Severity:         "P1",
					StatusCode:       500,
					User:             &OpsUnifiedEntityRef{ID: userID, Email: "real-user@example.com", Display: "real-user@example.com"},
					APIKey:           &OpsUnifiedEntityRef{ID: apiKeyID, Display: "API Key #22"},
					Group:            &OpsUnifiedEntityRef{ID: groupID, Name: "VIP group"},
					UpstreamAccount:  &OpsUnifiedEntityRef{ID: accountID, Name: "secret account"},
					Platform:         "openai",
					Model:            "gpt-5.5",
					Summary:          "Authorization Bearer sk-real-token should not pass",
					SameKindCount:    7,
				},
			}}, nil
		},
	}
	svc := newOpsAIWorkerService(t, repo)
	task := &OpsAIAnalysisTask{ID: 9, TimeStart: start, TimeEnd: end, FiltersJSON: `{"error_categories":["upstream"],"platform":"openai"}`}

	got, err := svc.BuildAIAnalysisContext(context.Background(), task)
	if err != nil {
		t.Fatalf("BuildAIAnalysisContext() error = %v", err)
	}
	if capturedFilter == nil || len(capturedFilter.ErrorCategories) != 1 || capturedFilter.ErrorCategories[0] != "upstream" || capturedFilter.Platform != "openai" {
		t.Fatalf("unexpected filter: %+v", capturedFilter)
	}
	if got.Total != 2 || len(got.Samples) != 1 {
		t.Fatalf("unexpected context: %+v", got)
	}
	sample := got.Samples[0]
	if sample.UserID == nil || *sample.UserID != userID || sample.APIKeyID == nil || *sample.APIKeyID != apiKeyID || sample.GroupID == nil || *sample.GroupID != groupID || sample.UpstreamAccountID == nil || *sample.UpstreamAccountID != accountID {
		t.Fatalf("ids not preserved: %+v", sample)
	}
	serialized := mustJSON(t, got)
	for _, forbidden := range []string{"real-user@example.com", "sk-real-token", "Bearer", "secret account", "VIP group"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("AI context leaked %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "[REDACTED]") {
		t.Fatalf("expected redacted marker: %s", serialized)
	}
}

func TestBuildAIAnalysisContextRejectsInvalidTaskFilters(t *testing.T) {
	svc := newOpsAIWorkerService(t, &opsRepoMock{})
	_, err := svc.BuildAIAnalysisContext(context.Background(), &OpsAIAnalysisTask{ID: 9, TimeStart: time.Now(), TimeEnd: time.Now().Add(time.Minute), FiltersJSON: `{bad`})
	if err == nil {
		t.Fatalf("expected invalid filters error")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
