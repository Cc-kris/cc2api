package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) ListAlertRules(ctx context.Context) ([]*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return []*OpsAlertRule{}, nil
	}
	return s.opsRepo.ListAlertRules(ctx)
}

func (s *OpsService) CreateAlertRule(ctx context.Context, rule *OpsAlertRule) (*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if rule == nil {
		return nil, infraerrors.BadRequest("INVALID_RULE", "invalid rule")
	}
	if err := s.validateAlertRuleForSave(ctx, rule, 0); err != nil {
		return nil, err
	}

	created, err := s.opsRepo.CreateAlertRule(ctx, rule)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *OpsService) UpdateAlertRule(ctx context.Context, rule *OpsAlertRule) (*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if rule == nil || rule.ID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE", "invalid rule")
	}
	if err := s.ensureAlertRuleEditable(ctx, rule.ID); err != nil {
		return nil, err
	}
	if err := s.validateAlertRuleForSave(ctx, rule, rule.ID); err != nil {
		return nil, err
	}

	updated, err := s.opsRepo.UpdateAlertRule(ctx, rule)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")
		}
		return nil, err
	}
	return updated, nil
}

func (s *OpsService) ensureAlertRuleEditable(ctx context.Context, ruleID int64) error {
	if s == nil || s.opsRepo == nil || ruleID <= 0 {
		return nil
	}
	rules, err := s.opsRepo.ListAlertRules(ctx)
	if err != nil {
		return err
	}
	for _, existing := range rules {
		if existing == nil || existing.ID != ruleID {
			continue
		}
		if isReadOnlyLegacyAlertRule(existing) {
			return infraerrors.BadRequest("OPS_ALERT_RULE_READONLY_LEGACY", "该规则已按新告警模型迁移")
		}
		return nil
	}
	return nil
}

func isReadOnlyLegacyAlertRule(rule *OpsAlertRule) bool {
	if rule == nil {
		return false
	}
	version := strings.ToLower(strings.TrimSpace(rule.RuleVersion))
	state := strings.ToLower(strings.TrimSpace(rule.MigrationState))
	return version == "v1" && (state == "migrated" || state == "readonly_legacy")
}

func (s *OpsService) validateAlertRuleForSave(ctx context.Context, rule *OpsAlertRule, currentID int64) error {
	if rule == nil {
		return infraerrors.BadRequest("INVALID_RULE", "invalid rule")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if nameLen := len([]rune(rule.Name)); nameLen < 2 || nameLen > 50 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_NAME_INVALID", "规则名称需为 2～50 字")
	}

	if s.opsRepo != nil {
		rules, err := s.opsRepo.ListAlertRules(ctx)
		if err == nil {
			for _, existing := range rules {
				if existing == nil || existing.ID == currentID {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(existing.Name), rule.Name) {
					return infraerrors.BadRequest("OPS_ALERT_RULE_NAME_DUPLICATED", "规则名称已存在")
				}
			}
		}
	}

	if strings.TrimSpace(rule.RuleVersion) == "" {
		if len(rule.ErrorCategories) == 0 && strings.TrimSpace(rule.MetricType) != "" && strings.TrimSpace(rule.MetricType) != "compound_rule" {
			rule.RuleVersion = "v1"
		} else {
			rule.RuleVersion = "v2"
		}
	}
	if strings.TrimSpace(rule.MigrationState) == "" {
		rule.MigrationState = "normal"
	}
	if rule.WindowMinutes != 1 && rule.RuleVersion == "v2" {
		return infraerrors.BadRequest("OPS_ALERT_RULE_WINDOW_INVALID", "本版本固定 1 分钟窗口")
	}
	if rule.WindowMinutes <= 0 {
		rule.WindowMinutes = 1
	}
	if rule.SustainedMinutes <= 0 {
		rule.SustainedMinutes = 1
	}
	if rule.SilenceMinutes < 0 || rule.SilenceMinutes > 1440 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_SILENCE_INVALID", "请输入 0～1440 的整数分钟")
	}
	if rule.CooldownMinutes < 0 {
		rule.CooldownMinutes = rule.SilenceMinutes
	}
	if rule.CooldownMinutes == 0 && rule.SilenceMinutes > 0 {
		rule.CooldownMinutes = rule.SilenceMinutes
	}

	if strings.TrimSpace(rule.RuleVersion) != "v2" {
		return validateLegacyAlertRuleForSave(rule)
	}
	return validateCompoundAlertRuleForSave(rule)
}

func validateLegacyAlertRuleForSave(rule *OpsAlertRule) error {
	if strings.TrimSpace(rule.MetricType) == "" || strings.TrimSpace(rule.Operator) == "" {
		return infraerrors.BadRequest("OPS_ALERT_RULE_LEGACY_INVALID", "metric_type/operator are required")
	}
	if math.IsNaN(rule.Threshold) || math.IsInf(rule.Threshold, 0) || rule.Threshold < 0 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_THRESHOLD_INVALID", "threshold must be a non-negative finite number")
	}
	if strings.TrimSpace(rule.TriggerLevel) == "" {
		rule.TriggerLevel = triggerLevelFromAlertSeverity(rule.Severity)
	}
	if rule.MinFinalFailures <= 0 {
		rule.MinFinalFailures = 1
	}
	if rule.MinSampleCount <= 0 {
		rule.MinSampleCount = 1
	}
	return nil
}

func validateCompoundAlertRuleForSave(rule *OpsAlertRule) error {
	if strings.TrimSpace(rule.MetricType) == "" {
		rule.MetricType = "compound_rule"
	}
	if strings.TrimSpace(rule.Operator) == "" {
		rule.Operator = ">="
	}
	if rule.Threshold <= 0 {
		rule.Threshold = float64(rule.MinFinalFailures)
	}
	if len(rule.ErrorCategories) == 0 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_CATEGORIES_REQUIRED", "请选择错误分类")
	}
	if len(rule.ErrorCategories) > 20 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_CATEGORIES_TOO_MANY", "错误分类最多选择 20 项")
	}
	seenCategories := map[string]struct{}{}
	for _, category := range rule.ErrorCategories {
		category = strings.TrimSpace(category)
		if !isValidOpsAlertErrorCategory(category) {
			return infraerrors.BadRequest("OPS_ALERT_RULE_CATEGORY_INVALID", fmt.Sprintf("错误分类不支持：%s", category))
		}
		if _, ok := seenCategories[category]; ok {
			return infraerrors.BadRequest("OPS_ALERT_RULE_CATEGORY_DUPLICATED", "错误分类不能重复")
		}
		seenCategories[category] = struct{}{}
	}

	rule.TriggerLevel = normalizeAlertTriggerLevel(rule.TriggerLevel)
	if !isValidAlertTriggerLevel(rule.TriggerLevel) {
		return infraerrors.BadRequest("OPS_ALERT_RULE_TRIGGER_LEVEL_INVALID", "trigger_level must be one of: P0, P1, P2, observe")
	}
	rule.Severity = alertSeverityFromTriggerLevel(rule.TriggerLevel)
	if rule.MinFinalFailures < 1 || rule.MinFinalFailures > 100000 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_MIN_FINAL_FAILURES_INVALID", "最小最终失败数需为 1～100000 的整数")
	}
	if rule.MinFailureRate < 0 || rule.MinFailureRate > 100 || math.Round(rule.MinFailureRate*100) != rule.MinFailureRate*100 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_MIN_FAILURE_RATE_INVALID", "请输入 0～100 的百分比")
	}
	if rule.MinSampleCount < 1 || rule.MinSampleCount > 1000000 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_MIN_SAMPLE_INVALID", "请输入大于 0 的整数")
	}
	if rule.MinFailureRate > 0 && rule.MinFinalFailures > rule.MinSampleCount {
		return infraerrors.BadRequest("OPS_ALERT_RULE_MIN_FAILURES_GT_SAMPLE", "最小最终失败数不能大于最小样本量")
	}
	for key, value := range rule.ImpactScope {
		if !isValidAlertImpactScopeKey(key) {
			return infraerrors.BadRequest("OPS_ALERT_RULE_IMPACT_SCOPE_INVALID", fmt.Sprintf("impact_scope contains unsupported key: %s", key))
		}
		if value < 1 || value > 100000 {
			return infraerrors.BadRequest("OPS_ALERT_RULE_IMPACT_SCOPE_VALUE_INVALID", "impact_scope values must be between 1 and 100000")
		}
	}
	if strings.TrimSpace(rule.RecoveredFluctuationPolicy) == "" {
		rule.RecoveredFluctuationPolicy = "record_only"
	}
	if !isValidRecoveredFluctuationPolicy(rule.RecoveredFluctuationPolicy) {
		return infraerrors.BadRequest("OPS_ALERT_RULE_RECOVERED_POLICY_INVALID", "recovered_fluctuation_policy must be one of: record_only, observe_only, alert")
	}
	if rule.RecoveredFluctuationPolicy != "record_only" && rule.MinRecoveredFluctuations < 1 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_MIN_RECOVERED_REQUIRED", "已恢复波动参与告警需配置最小波动数")
	}
	if rule.MinRecoveredFluctuations < 0 || rule.MinRecoveredFluctuations > 100000 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_MIN_RECOVERED_INVALID", "min_recovered_fluctuations must be between 0 and 100000")
	}
	if len(rule.NotificationChannels) == 0 {
		rule.NotificationChannels = []string{"in_app"}
	}
	for _, channel := range rule.NotificationChannels {
		if !isValidAlertNotificationChannel(channel) {
			return infraerrors.BadRequest("OPS_ALERT_RULE_NOTIFICATION_INVALID", "notification_channels must be one of: in_app, email, none")
		}
	}
	if stringSliceContains(rule.NotificationChannels, "none") && len(rule.NotificationChannels) > 1 {
		return infraerrors.BadRequest("OPS_ALERT_RULE_NOTIFICATION_NONE_EXCLUSIVE", "notification_channels 选择 none 时不能同时选择其他方式")
	}
	rule.NotifyEmail = stringSliceContains(rule.NotificationChannels, "email")
	return nil
}

func isValidOpsAlertErrorCategory(category string) bool {
	switch category {
	case "client", "platform", "upstream", "account_pool", "rate_limit", "permission", "balance", "config", "slow_request", "unknown":
		return true
	default:
		return false
	}
}

func normalizeAlertTriggerLevel(level string) string {
	level = strings.TrimSpace(level)
	if strings.EqualFold(level, "observe") || level == "观察" || strings.EqualFold(level, "P3") {
		return "observe"
	}
	return strings.ToUpper(level)
}

func isValidAlertTriggerLevel(level string) bool {
	switch level {
	case "P0", "P1", "P2", "observe":
		return true
	default:
		return false
	}
}

func alertSeverityFromTriggerLevel(level string) string {
	if level == "observe" {
		return "P3"
	}
	return level
}

func triggerLevelFromAlertSeverity(severity string) string {
	if strings.EqualFold(severity, "P3") {
		return "observe"
	}
	return strings.ToUpper(strings.TrimSpace(severity))
}

func isValidAlertImpactScopeKey(key string) bool {
	switch key {
	case "affected_users", "affected_api_keys", "affected_groups", "affected_models", "affected_upstream_accounts":
		return true
	default:
		return false
	}
}

func isValidRecoveredFluctuationPolicy(policy string) bool {
	switch policy {
	case "record_only", "observe_only", "alert":
		return true
	default:
		return false
	}
}

func isValidAlertNotificationChannel(channel string) bool {
	switch channel {
	case "in_app", "email", "none":
		return true
	default:
		return false
	}
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func (s *OpsService) DeleteAlertRule(ctx context.Context, id int64) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if err := s.opsRepo.DeleteAlertRule(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")
		}
		return err
	}
	return nil
}

func (s *OpsService) ListAlertEvents(ctx context.Context, filter *OpsAlertEventFilter) ([]*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return []*OpsAlertEvent{}, nil
	}
	return s.opsRepo.ListAlertEvents(ctx, filter)
}

func (s *OpsService) GetAlertEventByID(ctx context.Context, eventID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if eventID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
	}
	ev, err := s.opsRepo.GetAlertEventByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ALERT_EVENT_NOT_FOUND", "alert event not found")
		}
		return nil, err
	}
	if ev == nil {
		return nil, infraerrors.NotFound("OPS_ALERT_EVENT_NOT_FOUND", "alert event not found")
	}
	return ev, nil
}

func (s *OpsService) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	return s.opsRepo.GetActiveAlertEvent(ctx, ruleID)
}

func (s *OpsService) CreateAlertSilence(ctx context.Context, input *OpsAlertSilence) (*OpsAlertSilence, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_SILENCE", "invalid silence")
	}
	if input.RuleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if strings.TrimSpace(input.Platform) == "" {
		return nil, infraerrors.BadRequest("INVALID_PLATFORM", "invalid platform")
	}
	if input.Until.IsZero() {
		return nil, infraerrors.BadRequest("INVALID_UNTIL", "invalid until")
	}

	created, err := s.opsRepo.CreateAlertSilence(ctx, input)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *OpsService) IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return false, err
	}
	if s.opsRepo == nil {
		return false, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if ruleID <= 0 {
		return false, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if strings.TrimSpace(platform) == "" {
		return false, nil
	}
	return s.opsRepo.IsAlertSilenced(ctx, ruleID, platform, groupID, region, now)
}

func (s *OpsService) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	return s.opsRepo.GetLatestAlertEvent(ctx, ruleID)
}

func (s *OpsService) CreateAlertEvent(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if event == nil {
		return nil, infraerrors.BadRequest("INVALID_EVENT", "invalid event")
	}
	event.EventKey = strings.TrimSpace(event.EventKey)
	if event.LifecycleStatus == "" {
		event.LifecycleStatus = OpsAlertStatusFiring
	}
	if event.LastSeenAt.IsZero() {
		if !event.FiredAt.IsZero() {
			event.LastSeenAt = event.FiredAt
		} else {
			event.LastSeenAt = time.Now().UTC()
		}
	}
	if event.EventKey != "" && event.MergeWindowStart != nil && !event.MergeWindowStart.IsZero() {
		existing, err := s.opsRepo.GetMergeableAlertEvent(ctx, event.EventKey, *event.MergeWindowStart)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID > 0 {
			return s.opsRepo.MergeAlertEvent(ctx, existing.ID, event)
		}
	}

	created, err := s.opsRepo.CreateAlertEvent(ctx, event)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *OpsService) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, note string, processingAction string, operatorID *int64, resolvedAt *time.Time) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if eventID <= 0 {
		return infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
	}
	status = normalizeOpsAlertLifecycleStatus(status)
	if status == "" {
		return infraerrors.BadRequest("INVALID_STATUS", "invalid status")
	}
	if !isValidOpsAlertLifecycleStatus(status) {
		return infraerrors.BadRequest("INVALID_STATUS", "invalid status")
	}
	return s.opsRepo.UpdateAlertEventStatus(ctx, eventID, status, strings.TrimSpace(note), strings.TrimSpace(processingAction), operatorID, resolvedAt)
}

func normalizeOpsAlertLifecycleStatus(status string) string {
	switch strings.TrimSpace(status) {
	case OpsAlertStatusResolved:
		return OpsAlertStatusRecovered
	case OpsAlertStatusManualResolved:
		return OpsAlertStatusClosed
	default:
		return strings.TrimSpace(status)
	}
}

func NormalizeOpsAlertLifecycleStatusForAPI(status string) string {
	return normalizeOpsAlertLifecycleStatus(status)
}

func isValidOpsAlertLifecycleStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case OpsAlertStatusFiring,
		OpsAlertStatusAcknowledged,
		OpsAlertStatusProcessing,
		OpsAlertStatusRecovered,
		OpsAlertStatusClosed,
		OpsAlertStatusSilenced:
		return true
	default:
		return false
	}
}

func IsValidOpsAlertLifecycleStatusForAPI(status string) bool {
	return isValidOpsAlertLifecycleStatus(status)
}

func (s *OpsService) UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if eventID <= 0 {
		return infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
	}
	return s.opsRepo.UpdateAlertEventEmailSent(ctx, eventID, emailSent)
}
