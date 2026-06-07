package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestMaybeCreateAutoAIAnalysisTaskForAlertCreatesAndLinksTask(t *testing.T) {
	now := time.Date(2026, 6, 8, 13, 0, 0, 0, time.UTC)
	var created *OpsAIAnalysisTaskCreateInput
	var linkedEventID int64
	var linkedTaskID int64
	repo := &opsRepoMock{
		CreateAIAnalysisTaskIfAllowedFn: func(ctx context.Context, input *OpsAIAnalysisTaskCreateInput, maxActive int) (*OpsAIAnalysisTask, OpsAIAnalysisTaskCreateResult, error) {
			created = input
			if maxActive != opsAIAutoMaxActiveTasks {
				t.Fatalf("maxActive = %d", maxActive)
			}
			return &OpsAIAnalysisTask{ID: 501, Status: OpsAIAnalysisStatusPending}, OpsAIAnalysisTaskCreateResultCreated, nil
		},
		UpdateAlertEventAITaskIDFn: func(ctx context.Context, eventID int64, taskID int64) error {
			linkedEventID = eventID
			linkedTaskID = taskID
			return nil
		},
	}
	svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	seedManualAIConfig(t, svc)
	rule := &OpsAlertRule{ID: 9, Severity: "P1", TriggerLevel: "P1", WindowMinutes: 1, AutoAIAnalysis: true, ErrorCategories: []string{"upstream"}}
	event := &OpsAlertEvent{ID: 77, RuleID: 9, Severity: "P1", EventKey: "upstream|final_failed|group-3|P1", FiredAt: now, Dimensions: map[string]any{"platform": "openai", "group_id": int64(3)}}

	svc.MaybeCreateAutoAIAnalysisTaskForAlert(context.Background(), rule, event)

	if created == nil {
		t.Fatal("expected auto task")
	}
	if created.SourceType != OpsAIAnalysisSourceAlertEvent || created.SourceID == nil || *created.SourceID != 77 || created.TriggerType != OpsAIAnalysisTriggerAuto || created.TriggerUserID != nil {
		t.Fatalf("unexpected input identity: %+v", created)
	}
	if !created.TimeStart.Equal(now.Add(-time.Minute)) || !created.TimeEnd.Equal(now) {
		t.Fatalf("unexpected window: %s - %s", created.TimeStart, created.TimeEnd)
	}
	if created.DedupSince == nil || !created.DedupSince.Equal(now.Add(-10*time.Minute)) {
		t.Fatalf("unexpected dedup since: %v", created.DedupSince)
	}
	if !strings.Contains(created.FiltersJSON, `"error_categories":["upstream"]`) || !strings.Contains(created.FiltersJSON, `"platform":"openai"`) || !strings.Contains(created.FiltersJSON, `"group_id":3`) || !strings.Contains(created.FiltersJSON, `"alert_event_key":"upstream|final_failed|group-3|P1"`) {
		t.Fatalf("unexpected filters: %s", created.FiltersJSON)
	}
	if linkedEventID != 77 || linkedTaskID != 501 || event.AITaskID == nil || *event.AITaskID != 501 {
		t.Fatalf("link failed event=%d task=%d eventTask=%v", linkedEventID, linkedTaskID, event.AITaskID)
	}
}

func TestMaybeCreateAutoAIAnalysisTaskForAlertSkipsWhenNotEligible(t *testing.T) {
	start := time.Date(2026, 6, 8, 13, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		seedConfig bool
		rule       *OpsAlertRule
	}{
		{name: "config missing", seedConfig: false, rule: &OpsAlertRule{ID: 1, Severity: "P1", TriggerLevel: "P1", AutoAIAnalysis: true}},
		{name: "rule disabled", seedConfig: true, rule: &OpsAlertRule{ID: 1, Severity: "P1", TriggerLevel: "P1", AutoAIAnalysis: false}},
		{name: "P2 skipped", seedConfig: true, rule: &OpsAlertRule{ID: 1, Severity: "P2", TriggerLevel: "P2", AutoAIAnalysis: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			repo := &opsRepoMock{CreateAIAnalysisTaskIfAllowedFn: func(ctx context.Context, input *OpsAIAnalysisTaskCreateInput, maxActive int) (*OpsAIAnalysisTask, OpsAIAnalysisTaskCreateResult, error) {
				called = true
				return &OpsAIAnalysisTask{ID: 1}, OpsAIAnalysisTaskCreateResultCreated, nil
			}}
			svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
			if tc.seedConfig {
				seedManualAIConfig(t, svc)
			}
			svc.MaybeCreateAutoAIAnalysisTaskForAlert(context.Background(), tc.rule, &OpsAlertEvent{ID: 2, FiredAt: start})
			if called {
				t.Fatal("auto task should not be created")
			}
		})
	}
}

func TestMaybeCreateAutoAIAnalysisTaskForAlertAppliesGlobalRateLimit(t *testing.T) {
	created := 0
	repo := &opsRepoMock{CreateAIAnalysisTaskIfAllowedFn: func(ctx context.Context, input *OpsAIAnalysisTaskCreateInput, maxActive int) (*OpsAIAnalysisTask, OpsAIAnalysisTaskCreateResult, error) {
		created++
		return &OpsAIAnalysisTask{ID: int64(500 + created)}, OpsAIAnalysisTaskCreateResultCreated, nil
	}}
	svc := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.SetSecretEncryptor(opsAIConfigEncryptorStub{})
	_, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), &OpsAIAnalysisConfigUpdateRequest{Enabled: true, BaseURL: "https://ai.example.com/v1", APIKey: "sk-test", Model: "gpt-5.5", InterfaceType: "responses", TimeoutSeconds: 60, MaxSamples: 50, AutoDedupMinutes: 10, GlobalRateLimitPerMinute: 1, AutoLevels: []string{"P1"}, ManualEnabled: true})
	if err != nil {
		t.Fatalf("seed config: %v", err)
	}
	rule := &OpsAlertRule{ID: 1, Severity: "P1", TriggerLevel: "P1", AutoAIAnalysis: true}
	svc.MaybeCreateAutoAIAnalysisTaskForAlert(context.Background(), rule, &OpsAlertEvent{ID: 10, FiredAt: time.Now().UTC()})
	svc.MaybeCreateAutoAIAnalysisTaskForAlert(context.Background(), rule, &OpsAlertEvent{ID: 11, FiredAt: time.Now().UTC()})
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
}
