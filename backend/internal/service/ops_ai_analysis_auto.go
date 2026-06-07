package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const opsAIAutoMaxActiveTasks = 3

func (s *OpsService) MaybeCreateAutoAIAnalysisTaskForAlert(ctx context.Context, rule *OpsAlertRule, event *OpsAlertEvent) {
	if s == nil || s.opsRepo == nil || rule == nil || event == nil || event.ID <= 0 {
		return
	}
	if !rule.AutoAIAnalysis || !isAutoAIAnalysisTriggerLevel(rule) {
		return
	}
	cfg, err := s.loadOpsAIAnalysisConfigForUpdate(ctx)
	if err != nil {
		logger.LegacyPrintf("service.ops_ai_analysis_auto", "load AI analysis config failed: %v", err)
		return
	}
	normalizeOpsAIAnalysisConfig(cfg)
	if !isAutoAIAnalysisConfigured(cfg) || !autoAIAnalysisLevelAllowed(cfg, rule) {
		return
	}
	if !s.allowAutoAIAnalysisNow(cfg) {
		logger.LegacyPrintf("service.ops_ai_analysis_auto", "auto AI analysis skipped by global rate limit event=%d rule=%d", event.ID, rule.ID)
		return
	}

	input, err := buildAutoAIAnalysisTaskInput(rule, event, cfg)
	if err != nil {
		logger.LegacyPrintf("service.ops_ai_analysis_auto", "build auto AI analysis input failed event=%d rule=%d err=%v", event.ID, rule.ID, err)
		return
	}
	task, result, err := s.opsRepo.CreateAIAnalysisTaskIfAllowed(ctx, input, opsAIAutoMaxActiveTasks)
	if err != nil {
		logger.LegacyPrintf("service.ops_ai_analysis_auto", "create auto AI analysis task failed event=%d rule=%d err=%v", event.ID, rule.ID, err)
		return
	}
	if result != OpsAIAnalysisTaskCreateResultCreated || task == nil || task.ID <= 0 {
		logger.LegacyPrintf("service.ops_ai_analysis_auto", "auto AI analysis skipped event=%d rule=%d result=%s", event.ID, rule.ID, result)
		return
	}
	if err := s.opsRepo.UpdateAlertEventAITaskID(ctx, event.ID, task.ID); err != nil {
		logger.LegacyPrintf("service.ops_ai_analysis_auto", "link auto AI analysis task failed event=%d task=%d err=%v", event.ID, task.ID, err)
		return
	}
	event.AITaskID = &task.ID
}

func isAutoAIAnalysisConfigured(cfg *OpsAIAnalysisConfig) bool {
	return cfg != nil && cfg.Enabled && strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "" && strings.TrimSpace(cfg.APIKeyEncrypted) != ""
}

func isAutoAIAnalysisTriggerLevel(rule *OpsAlertRule) bool {
	level := opsAIAnalysisRuleLevel(rule)
	return level == "P0" || level == "P1"
}

func autoAIAnalysisLevelAllowed(cfg *OpsAIAnalysisConfig, rule *OpsAlertRule) bool {
	level := opsAIAnalysisRuleLevel(rule)
	for _, item := range cfg.AutoLevels {
		if strings.EqualFold(strings.TrimSpace(item), level) {
			return true
		}
	}
	return false
}

func opsAIAnalysisRuleLevel(rule *OpsAlertRule) string {
	if rule == nil {
		return ""
	}
	level := strings.TrimSpace(rule.TriggerLevel)
	if level == "" {
		level = strings.TrimSpace(rule.Severity)
	}
	return strings.ToUpper(level)
}

func (s *OpsService) allowAutoAIAnalysisNow(cfg *OpsAIAnalysisConfig) bool {
	limit := 10
	if cfg != nil && cfg.GlobalRateLimitPerMinute > 0 {
		limit = cfg.GlobalRateLimitPerMinute
	}
	if s.aiAutoAnalysisLimiter == nil {
		s.aiAutoAnalysisLimiter = newSlidingWindowLimiter(limit, time.Minute)
	}
	s.aiAutoAnalysisLimiter.SetLimit(limit)
	return s.aiAutoAnalysisLimiter.Allow(time.Now().UTC())
}

func buildAutoAIAnalysisTaskInput(rule *OpsAlertRule, event *OpsAlertEvent, cfg *OpsAIAnalysisConfig) (*OpsAIAnalysisTaskCreateInput, error) {
	start, end := autoAIAnalysisWindow(rule, event)
	filters := buildAutoAIAnalysisFilters(rule, event)
	if key := strings.TrimSpace(event.EventKey); key != "" {
		filters["alert_event_key"] = key
	}
	dedupSince := start
	if cfg != nil && cfg.AutoDedupMinutes > 0 {
		dedupSince = end.Add(-time.Duration(cfg.AutoDedupMinutes) * time.Minute)
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}
	sourceID := event.ID
	return &OpsAIAnalysisTaskCreateInput{
		SourceType:  OpsAIAnalysisSourceAlertEvent,
		SourceID:    &sourceID,
		TriggerType: OpsAIAnalysisTriggerAuto,
		TimeStart:   start,
		TimeEnd:     end,
		FiltersJSON: string(filtersJSON),
		Status:      OpsAIAnalysisStatusPending,
		Provider:    cfg.InterfaceType,
		Model:       cfg.Model,
		DedupSince:  &dedupSince,
	}, nil
}

func autoAIAnalysisWindow(rule *OpsAlertRule, event *OpsAlertEvent) (time.Time, time.Time) {
	end := event.FiredAt
	if end.IsZero() {
		end = event.CreatedAt
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}
	windowMinutes := 1
	if rule != nil && rule.WindowMinutes > 0 {
		windowMinutes = rule.WindowMinutes
	}
	start := end.Add(-time.Duration(windowMinutes) * time.Minute)
	return start, end
}

func buildAutoAIAnalysisFilters(rule *OpsAlertRule, event *OpsAlertEvent) map[string]any {
	filters := map[string]any{
		"error_results": []string{OpsUnifiedErrorResultFinalFailed},
		"severity":      []string{opsAIAnalysisRuleLevel(rule)},
	}
	if rule != nil && len(rule.ErrorCategories) > 0 {
		categories := make([]string, 0, len(rule.ErrorCategories))
		for _, category := range rule.ErrorCategories {
			if item := strings.TrimSpace(category); item != "" {
				categories = append(categories, item)
			}
		}
		if len(categories) > 0 {
			filters["error_categories"] = categories
		}
	}
	for key, value := range event.Dimensions {
		switch key {
		case "platform", "model", "request_id", "keyword":
			if s := strings.TrimSpace(toOpsAIAnalysisString(value)); s != "" {
				filters[key] = s
			}
		case "group_id", "user_id", "api_key_id", "upstream_account_id":
			if n, ok := toOpsAIAnalysisInt64(value); ok && n > 0 {
				filters[key] = n
			}
		}
	}
	return filters
}

func toOpsAIAnalysisString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func toOpsAIAnalysisInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
	case float32:
		f := float64(v)
		if f == float64(int64(f)) {
			return int64(f), true
		}
	}
	return 0, false
}
