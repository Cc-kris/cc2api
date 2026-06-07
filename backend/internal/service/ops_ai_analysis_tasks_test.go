package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func seedManualAIConfig(t *testing.T, svc *OpsService) {
	t.Helper()
	svc.SetSecretEncryptor(opsAIConfigEncryptorStub{})
	if _, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), validOpsAIConfigUpdate("sk-task-secret")); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func validManualAITaskRequest(start, end time.Time) *OpsAIAnalysisTaskCreateRequest {
	return &OpsAIAnalysisTaskCreateRequest{
		SourceType: OpsAIAnalysisSourceUnifiedErrors,
		TimeStart:  start.Format(time.RFC3339),
		TimeEnd:    end.Format(time.RFC3339),
		Filters: map[string]any{
			"error_categories": []any{"upstream"},
			"platform":         "openai",
		},
	}
}

func TestCreateManualAIAnalysisTaskSuccess(t *testing.T) {
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	var created *OpsAIAnalysisTaskCreateInput
	repo := &opsRepoMock{
		ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
			if filter.StartTime == nil || !filter.StartTime.Equal(start) || filter.EndTime == nil || !filter.EndTime.Equal(end) {
				t.Fatalf("unexpected time range: %+v", filter)
			}
			if got := strings.Join(filter.ErrorCategories, ","); got != "upstream" {
				t.Fatalf("categories = %q", got)
			}
			if filter.Platform != "openai" {
				t.Fatalf("platform = %q", filter.Platform)
			}
			return &OpsUnifiedErrorList{Total: 2, Items: []*OpsUnifiedErrorItem{{ID: 1}}}, nil
		},
		CreateAIAnalysisTaskIfAllowedFn: func(ctx context.Context, input *OpsAIAnalysisTaskCreateInput, maxActive int) (*OpsAIAnalysisTask, OpsAIAnalysisTaskCreateResult, error) {
			created = input
			if maxActive != opsAIManualMaxActiveTasks {
				t.Fatalf("maxActive = %d", maxActive)
			}
			return &OpsAIAnalysisTask{ID: 99, Status: input.Status, SampleCount: input.SampleCount}, OpsAIAnalysisTaskCreateResultCreated, nil
		},
	}
	svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	seedManualAIConfig(t, svc)

	got, err := svc.CreateManualAIAnalysisTask(context.Background(), validManualAITaskRequest(start, end), 7)

	if err != nil {
		t.Fatalf("CreateManualAIAnalysisTask() error = %v", err)
	}
	if got.TaskID != 99 || got.Status != OpsAIAnalysisStatusPending || got.SampleCount != 0 || got.MatchedErrorCount != 2 {
		t.Fatalf("unexpected response: %+v", got)
	}
	if created == nil || created.TriggerUserID == nil || *created.TriggerUserID != 7 || created.TriggerType != OpsAIAnalysisTriggerManual {
		t.Fatalf("unexpected created input: %+v", created)
	}
	if !strings.Contains(created.FiltersJSON, "error_categories") || strings.Contains(created.FiltersJSON, "sk-task-secret") {
		t.Fatalf("bad filters json: %s", created.FiltersJSON)
	}
}

func TestCreateManualAIAnalysisTaskFailures(t *testing.T) {
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)

	t.Run("not_configured", func(t *testing.T) {
		svc := NewOpsService(&opsRepoMock{}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		_, err := svc.CreateManualAIAnalysisTask(context.Background(), validManualAITaskRequest(start, end), 7)
		if !infraerrors.IsBadRequest(err) || !strings.Contains(err.Error(), "请先配置") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("time_range_too_large", func(t *testing.T) {
		svc := NewOpsService(&opsRepoMock{}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		seedManualAIConfig(t, svc)
		_, err := svc.CreateManualAIAnalysisTask(context.Background(), validManualAITaskRequest(start, start.Add(25*time.Hour)), 7)
		if !infraerrors.IsBadRequest(err) || !strings.Contains(err.Error(), "24 小时") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("queue_busy", func(t *testing.T) {
		repo := &opsRepoMock{
			ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
				return &OpsUnifiedErrorList{Total: 1, Items: []*OpsUnifiedErrorItem{{ID: 1}}}, nil
			},
			CreateAIAnalysisTaskIfAllowedFn: func(ctx context.Context, input *OpsAIAnalysisTaskCreateInput, maxActive int) (*OpsAIAnalysisTask, OpsAIAnalysisTaskCreateResult, error) {
				return nil, OpsAIAnalysisTaskCreateResultQueueBusy, nil
			},
		}
		svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		seedManualAIConfig(t, svc)
		_, err := svc.CreateManualAIAnalysisTask(context.Background(), validManualAITaskRequest(start, end), 7)
		if !infraerrors.IsTooManyRequests(err) || !strings.Contains(err.Error(), "队列繁忙") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		repo := &opsRepoMock{
			ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
				return &OpsUnifiedErrorList{Total: 1, Items: []*OpsUnifiedErrorItem{{ID: 1}}}, nil
			},
			CreateAIAnalysisTaskIfAllowedFn: func(ctx context.Context, input *OpsAIAnalysisTaskCreateInput, maxActive int) (*OpsAIAnalysisTask, OpsAIAnalysisTaskCreateResult, error) {
				return &OpsAIAnalysisTask{ID: 10, Status: OpsAIAnalysisStatusPending}, OpsAIAnalysisTaskCreateResultDuplicate, nil
			},
		}
		svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		seedManualAIConfig(t, svc)
		_, err := svc.CreateManualAIAnalysisTask(context.Background(), validManualAITaskRequest(start, end), 7)
		if !infraerrors.IsConflict(err) || !strings.Contains(err.Error(), "处理中") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("no_errors", func(t *testing.T) {
		repo := &opsRepoMock{ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
			return &OpsUnifiedErrorList{Total: 0}, nil
		}}
		svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
		seedManualAIConfig(t, svc)
		_, err := svc.CreateManualAIAnalysisTask(context.Background(), validManualAITaskRequest(start, end), 7)
		if !infraerrors.IsBadRequest(err) || !strings.Contains(err.Error(), "暂无可分析") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestNormalizeManualAIAnalysisTaskInputCanonicalizesFilters(t *testing.T) {
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	svc := NewOpsService(&opsRepoMock{}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	cfg := &OpsAIAnalysisConfig{InterfaceType: "responses", Model: "gpt-5.5"}

	req1 := &OpsAIAnalysisTaskCreateRequest{
		SourceType: OpsAIAnalysisSourceUnifiedErrors,
		TimeStart:  start.Format(time.RFC3339),
		TimeEnd:    end.Format(time.RFC3339),
		Filters: map[string]any{
			"error_categories": []any{"upstream", "client"},
			"severity":         []any{"P2", "P0"},
			"status_code":      []any{float64(500), float64(429)},
		},
	}
	req2 := &OpsAIAnalysisTaskCreateRequest{
		SourceType: OpsAIAnalysisSourceUnifiedErrors,
		TimeStart:  start.Format(time.RFC3339),
		TimeEnd:    end.Format(time.RFC3339),
		Filters: map[string]any{
			"error_categories": []any{"client", "upstream"},
			"severity":         []any{"P0", "P2"},
			"status_code":      []any{float64(429), float64(500)},
		},
	}

	input1, _, err := svc.normalizeManualAIAnalysisTaskInput(req1, 7, cfg)
	if err != nil {
		t.Fatalf("normalize req1: %v", err)
	}
	input2, _, err := svc.normalizeManualAIAnalysisTaskInput(req2, 7, cfg)
	if err != nil {
		t.Fatalf("normalize req2: %v", err)
	}
	if input1.FiltersJSON != input2.FiltersJSON {
		t.Fatalf("filters json should be canonicalized:\n%s\n%s", input1.FiltersJSON, input2.FiltersJSON)
	}
	if !strings.Contains(input1.FiltersJSON, `"status_code":[429,500]`) {
		t.Fatalf("status codes not sorted: %s", input1.FiltersJSON)
	}
}

func TestNormalizeManualAIAnalysisTaskInputRejectsInvalidSourceID(t *testing.T) {
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	svc := NewOpsService(&opsRepoMock{}, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	cfg := &OpsAIAnalysisConfig{InterfaceType: "responses", Model: "gpt-5.5"}
	badID := int64(-1)
	req := validManualAITaskRequest(start, end)
	req.SourceID = &badID

	_, _, err := svc.normalizeManualAIAnalysisTaskInput(req, 7, cfg)
	if !infraerrors.IsBadRequest(err) || !strings.Contains(err.Error(), "source_id") {
		t.Fatalf("err = %v", err)
	}
}
