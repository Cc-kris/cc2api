package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) ListAlertRules(ctx context.Context) ([]*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}

	q := `
SELECT
  id,
  name,
  COALESCE(description, ''),
  enabled,
  COALESCE(severity, ''),
  metric_type,
  operator,
  threshold,
  window_minutes,
  sustained_minutes,
  cooldown_minutes,
  COALESCE(notify_email, true),
  filters,
  COALESCE(rule_version, 'v1'),
  COALESCE(error_categories, '[]'::jsonb),
  COALESCE(trigger_level, ''),
  COALESCE(min_final_failures, 1),
  COALESCE(min_failure_rate, 0),
  COALESCE(min_sample_count, 1),
  COALESCE(impact_scope, '{}'::jsonb),
  COALESCE(recovered_fluctuation_policy, 'record_only'),
  COALESCE(min_recovered_fluctuations, 0),
  COALESCE(auto_ai_analysis, false),
  COALESCE(notification_channels, '["in_app"]'::jsonb),
  COALESCE(silence_minutes, 10),
  COALESCE(migration_state, 'normal'),
  last_triggered_at,
  created_at,
  updated_at
FROM ops_alert_rules
ORDER BY id DESC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*service.OpsAlertRule{}
	for rows.Next() {
		var rule service.OpsAlertRule
		var filtersRaw []byte
		var errorCategoriesRaw []byte
		var impactScopeRaw []byte
		var notificationChannelsRaw []byte
		var lastTriggeredAt sql.NullTime
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Description,
			&rule.Enabled,
			&rule.Severity,
			&rule.MetricType,
			&rule.Operator,
			&rule.Threshold,
			&rule.WindowMinutes,
			&rule.SustainedMinutes,
			&rule.CooldownMinutes,
			&rule.NotifyEmail,
			&filtersRaw,
			&rule.RuleVersion,
			&errorCategoriesRaw,
			&rule.TriggerLevel,
			&rule.MinFinalFailures,
			&rule.MinFailureRate,
			&rule.MinSampleCount,
			&impactScopeRaw,
			&rule.RecoveredFluctuationPolicy,
			&rule.MinRecoveredFluctuations,
			&rule.AutoAIAnalysis,
			&notificationChannelsRaw,
			&rule.SilenceMinutes,
			&rule.MigrationState,
			&lastTriggeredAt,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if lastTriggeredAt.Valid {
			v := lastTriggeredAt.Time
			rule.LastTriggeredAt = &v
		}
		decodeOpsAlertRuleJSONFields(&rule, errorCategoriesRaw, impactScopeRaw, notificationChannelsRaw)
		if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
			var decoded map[string]any
			if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
				rule.Filters = decoded
			}
		}
		out = append(out, &rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) CreateAlertRule(ctx context.Context, input *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return nil, fmt.Errorf("nil input")
	}

	filtersArg, err := opsNullJSONMap(input.Filters)
	if err != nil {
		return nil, err
	}
	errorCategoriesArg, err := opsJSONValue(input.ErrorCategories)
	if err != nil {
		return nil, err
	}
	impactScopeArg, err := opsJSONValue(input.ImpactScope)
	if err != nil {
		return nil, err
	}
	notificationChannelsArg, err := opsJSONValue(input.NotificationChannels)
	if err != nil {
		return nil, err
	}

	q := `
INSERT INTO ops_alert_rules (
  name,
  description,
  enabled,
  severity,
  metric_type,
  operator,
  threshold,
  window_minutes,
  sustained_minutes,
  cooldown_minutes,
  notify_email,
  filters,
  rule_version,
  error_categories,
  trigger_level,
  min_final_failures,
  min_failure_rate,
  min_sample_count,
  impact_scope,
  recovered_fluctuation_policy,
  min_recovered_fluctuations,
  auto_ai_analysis,
  notification_channels,
  silence_minutes,
  migration_state,
  created_at,
  updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,NOW(),NOW()
)
RETURNING
  id,
  name,
  COALESCE(description, ''),
  enabled,
  COALESCE(severity, ''),
  metric_type,
  operator,
  threshold,
  window_minutes,
  sustained_minutes,
  cooldown_minutes,
  COALESCE(notify_email, true),
  filters,
  COALESCE(rule_version, 'v1'),
  COALESCE(error_categories, '[]'::jsonb),
  COALESCE(trigger_level, ''),
  COALESCE(min_final_failures, 1),
  COALESCE(min_failure_rate, 0),
  COALESCE(min_sample_count, 1),
  COALESCE(impact_scope, '{}'::jsonb),
  COALESCE(recovered_fluctuation_policy, 'record_only'),
  COALESCE(min_recovered_fluctuations, 0),
  COALESCE(auto_ai_analysis, false),
  COALESCE(notification_channels, '["in_app"]'::jsonb),
  COALESCE(silence_minutes, 10),
  COALESCE(migration_state, 'normal'),
  last_triggered_at,
  created_at,
  updated_at`

	var out service.OpsAlertRule
	var filtersRaw []byte
	var errorCategoriesRaw []byte
	var impactScopeRaw []byte
	var notificationChannelsRaw []byte
	var lastTriggeredAt sql.NullTime

	if err := r.db.QueryRowContext(
		ctx,
		q,
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.Description),
		input.Enabled,
		strings.TrimSpace(input.Severity),
		strings.TrimSpace(input.MetricType),
		strings.TrimSpace(input.Operator),
		input.Threshold,
		input.WindowMinutes,
		input.SustainedMinutes,
		input.CooldownMinutes,
		input.NotifyEmail,
		filtersArg,
		strings.TrimSpace(input.RuleVersion),
		errorCategoriesArg,
		strings.TrimSpace(input.TriggerLevel),
		input.MinFinalFailures,
		input.MinFailureRate,
		input.MinSampleCount,
		impactScopeArg,
		strings.TrimSpace(input.RecoveredFluctuationPolicy),
		input.MinRecoveredFluctuations,
		input.AutoAIAnalysis,
		notificationChannelsArg,
		input.SilenceMinutes,
		strings.TrimSpace(input.MigrationState),
	).Scan(
		&out.ID,
		&out.Name,
		&out.Description,
		&out.Enabled,
		&out.Severity,
		&out.MetricType,
		&out.Operator,
		&out.Threshold,
		&out.WindowMinutes,
		&out.SustainedMinutes,
		&out.CooldownMinutes,
		&out.NotifyEmail,
		&filtersRaw,
		&out.RuleVersion,
		&errorCategoriesRaw,
		&out.TriggerLevel,
		&out.MinFinalFailures,
		&out.MinFailureRate,
		&out.MinSampleCount,
		&impactScopeRaw,
		&out.RecoveredFluctuationPolicy,
		&out.MinRecoveredFluctuations,
		&out.AutoAIAnalysis,
		&notificationChannelsRaw,
		&out.SilenceMinutes,
		&out.MigrationState,
		&lastTriggeredAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if lastTriggeredAt.Valid {
		v := lastTriggeredAt.Time
		out.LastTriggeredAt = &v
	}
	decodeOpsAlertRuleJSONFields(&out, errorCategoriesRaw, impactScopeRaw, notificationChannelsRaw)
	if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
			out.Filters = decoded
		}
	}

	return &out, nil
}

func (r *opsRepository) UpdateAlertRule(ctx context.Context, input *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return nil, fmt.Errorf("nil input")
	}
	if input.ID <= 0 {
		return nil, fmt.Errorf("invalid id")
	}

	filtersArg, err := opsNullJSONMap(input.Filters)
	if err != nil {
		return nil, err
	}
	errorCategoriesArg, err := opsJSONValue(input.ErrorCategories)
	if err != nil {
		return nil, err
	}
	impactScopeArg, err := opsJSONValue(input.ImpactScope)
	if err != nil {
		return nil, err
	}
	notificationChannelsArg, err := opsJSONValue(input.NotificationChannels)
	if err != nil {
		return nil, err
	}

	q := `
UPDATE ops_alert_rules
SET
  name = $2,
  description = $3,
  enabled = $4,
  severity = $5,
  metric_type = $6,
  operator = $7,
  threshold = $8,
  window_minutes = $9,
  sustained_minutes = $10,
  cooldown_minutes = $11,
  notify_email = $12,
  filters = $13,
  rule_version = $14,
  error_categories = $15,
  trigger_level = $16,
  min_final_failures = $17,
  min_failure_rate = $18,
  min_sample_count = $19,
  impact_scope = $20,
  recovered_fluctuation_policy = $21,
  min_recovered_fluctuations = $22,
  auto_ai_analysis = $23,
  notification_channels = $24,
  silence_minutes = $25,
  migration_state = $26,
  updated_at = NOW()
WHERE id = $1
RETURNING
  id,
  name,
  COALESCE(description, ''),
  enabled,
  COALESCE(severity, ''),
  metric_type,
  operator,
  threshold,
  window_minutes,
  sustained_minutes,
  cooldown_minutes,
  COALESCE(notify_email, true),
  filters,
  COALESCE(rule_version, 'v1'),
  COALESCE(error_categories, '[]'::jsonb),
  COALESCE(trigger_level, ''),
  COALESCE(min_final_failures, 1),
  COALESCE(min_failure_rate, 0),
  COALESCE(min_sample_count, 1),
  COALESCE(impact_scope, '{}'::jsonb),
  COALESCE(recovered_fluctuation_policy, 'record_only'),
  COALESCE(min_recovered_fluctuations, 0),
  COALESCE(auto_ai_analysis, false),
  COALESCE(notification_channels, '["in_app"]'::jsonb),
  COALESCE(silence_minutes, 10),
  COALESCE(migration_state, 'normal'),
  last_triggered_at,
  created_at,
  updated_at`

	var out service.OpsAlertRule
	var filtersRaw []byte
	var errorCategoriesRaw []byte
	var impactScopeRaw []byte
	var notificationChannelsRaw []byte
	var lastTriggeredAt sql.NullTime

	if err := r.db.QueryRowContext(
		ctx,
		q,
		input.ID,
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.Description),
		input.Enabled,
		strings.TrimSpace(input.Severity),
		strings.TrimSpace(input.MetricType),
		strings.TrimSpace(input.Operator),
		input.Threshold,
		input.WindowMinutes,
		input.SustainedMinutes,
		input.CooldownMinutes,
		input.NotifyEmail,
		filtersArg,
		strings.TrimSpace(input.RuleVersion),
		errorCategoriesArg,
		strings.TrimSpace(input.TriggerLevel),
		input.MinFinalFailures,
		input.MinFailureRate,
		input.MinSampleCount,
		impactScopeArg,
		strings.TrimSpace(input.RecoveredFluctuationPolicy),
		input.MinRecoveredFluctuations,
		input.AutoAIAnalysis,
		notificationChannelsArg,
		input.SilenceMinutes,
		strings.TrimSpace(input.MigrationState),
	).Scan(
		&out.ID,
		&out.Name,
		&out.Description,
		&out.Enabled,
		&out.Severity,
		&out.MetricType,
		&out.Operator,
		&out.Threshold,
		&out.WindowMinutes,
		&out.SustainedMinutes,
		&out.CooldownMinutes,
		&out.NotifyEmail,
		&filtersRaw,
		&out.RuleVersion,
		&errorCategoriesRaw,
		&out.TriggerLevel,
		&out.MinFinalFailures,
		&out.MinFailureRate,
		&out.MinSampleCount,
		&impactScopeRaw,
		&out.RecoveredFluctuationPolicy,
		&out.MinRecoveredFluctuations,
		&out.AutoAIAnalysis,
		&notificationChannelsRaw,
		&out.SilenceMinutes,
		&out.MigrationState,
		&lastTriggeredAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if lastTriggeredAt.Valid {
		v := lastTriggeredAt.Time
		out.LastTriggeredAt = &v
	}
	decodeOpsAlertRuleJSONFields(&out, errorCategoriesRaw, impactScopeRaw, notificationChannelsRaw)
	if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
			out.Filters = decoded
		}
	}

	return &out, nil
}

func (r *opsRepository) DeleteAlertRule(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if id <= 0 {
		return fmt.Errorf("invalid id")
	}

	res, err := r.db.ExecContext(ctx, "DELETE FROM ops_alert_rules WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *opsRepository) ListAlertEvents(ctx context.Context, filter *service.OpsAlertEventFilter) ([]*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.OpsAlertEventFilter{}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	where, args := buildOpsAlertEventsWhere(filter)
	args = append(args, limit)
	limitArg := "$" + itoa(len(args))

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(event_key, ''),
  COALESCE(lifecycle_status, status, ''),
  COALESCE(merged_count, 0),
  COALESCE(last_seen_at, fired_at),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  recovered_at,
  acknowledged_at,
  acknowledged_by,
  acknowledged_note,
  processing_at,
  processing_by,
  processing_note,
  processing_action,
  closed_at,
  closed_by,
  closed_reason,
  ai_task_id,
  email_sent,
  created_at
FROM ops_alert_events
` + where + `
ORDER BY fired_at DESC, id DESC
LIMIT ` + limitArg

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*service.OpsAlertEvent{}
	for rows.Next() {
		ev, err := scanOpsAlertEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) GetAlertEventByID(ctx context.Context, eventID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return nil, fmt.Errorf("invalid event id")
	}

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(event_key, ''),
  COALESCE(lifecycle_status, status, ''),
  COALESCE(merged_count, 0),
  COALESCE(last_seen_at, fired_at),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  recovered_at,
  acknowledged_at,
  acknowledged_by,
  acknowledged_note,
  processing_at,
  processing_by,
  processing_note,
  processing_action,
  closed_at,
  closed_by,
  closed_reason,
  ai_task_id,
  email_sent,
  created_at
FROM ops_alert_events
WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, eventID)
	ev, err := scanOpsAlertEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ev, nil
}

func (r *opsRepository) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if ruleID <= 0 {
		return nil, fmt.Errorf("invalid rule id")
	}

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(event_key, ''),
  COALESCE(lifecycle_status, status, ''),
  COALESCE(merged_count, 0),
  COALESCE(last_seen_at, fired_at),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  recovered_at,
  acknowledged_at,
  acknowledged_by,
  acknowledged_note,
  processing_at,
  processing_by,
  processing_note,
  processing_action,
  closed_at,
  closed_by,
  closed_reason,
  ai_task_id,
  email_sent,
  created_at
FROM ops_alert_events
WHERE rule_id = $1
  AND COALESCE(lifecycle_status, status) IN ('firing', 'acknowledged', 'processing', 'silenced')
ORDER BY fired_at DESC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, ruleID)
	ev, err := scanOpsAlertEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ev, nil
}

func (r *opsRepository) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if ruleID <= 0 {
		return nil, fmt.Errorf("invalid rule id")
	}

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(event_key, ''),
  COALESCE(lifecycle_status, status, ''),
  COALESCE(merged_count, 0),
  COALESCE(last_seen_at, fired_at),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  recovered_at,
  acknowledged_at,
  acknowledged_by,
  acknowledged_note,
  processing_at,
  processing_by,
  processing_note,
  processing_action,
  closed_at,
  closed_by,
  closed_reason,
  ai_task_id,
  email_sent,
  created_at
FROM ops_alert_events
WHERE rule_id = $1
ORDER BY fired_at DESC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, ruleID)
	ev, err := scanOpsAlertEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ev, nil
}

func (r *opsRepository) GetMergeableAlertEvent(ctx context.Context, eventKey string, since time.Time) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	eventKey = strings.TrimSpace(eventKey)
	if eventKey == "" {
		return nil, fmt.Errorf("invalid event key")
	}
	if since.IsZero() {
		return nil, fmt.Errorf("invalid merge window")
	}

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(event_key, ''),
  COALESCE(lifecycle_status, status, ''),
  COALESCE(merged_count, 0),
  COALESCE(last_seen_at, fired_at),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  recovered_at,
  acknowledged_at,
  acknowledged_by,
  acknowledged_note,
  processing_at,
  processing_by,
  processing_note,
  processing_action,
  closed_at,
  closed_by,
  closed_reason,
  ai_task_id,
  email_sent,
  created_at
FROM ops_alert_events
WHERE event_key = $1
  AND COALESCE(lifecycle_status, status) IN ('firing', 'acknowledged', 'processing', 'silenced')
  AND COALESCE(last_seen_at, fired_at) >= $2
ORDER BY COALESCE(last_seen_at, fired_at) DESC, id DESC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, eventKey, since)
	ev, err := scanOpsAlertEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ev, nil
}

func (r *opsRepository) CreateAlertEvent(ctx context.Context, event *service.OpsAlertEvent) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}
	if strings.TrimSpace(event.LifecycleStatus) == "" {
		event.LifecycleStatus = service.OpsAlertStatusFiring
	}
	if event.LastSeenAt.IsZero() {
		if !event.FiredAt.IsZero() {
			event.LastSeenAt = event.FiredAt
		} else {
			event.LastSeenAt = time.Now().UTC()
		}
	}

	dimensionsArg, err := opsNullJSONMap(event.Dimensions)
	if err != nil {
		return nil, err
	}
	triggerSnapshotArg, err := opsNullJSONMap(event.TriggerSnapshot)
	if err != nil {
		return nil, err
	}
	scoreSnapshotArg, err := opsNullJSONMap(event.ScoreSnapshot)
	if err != nil {
		return nil, err
	}

	q := `
INSERT INTO ops_alert_events (
  rule_id,
  severity,
  status,
  event_key,
  lifecycle_status,
  merged_count,
  last_seen_at,
  title,
  description,
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  email_sent,
  created_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NOW()
)
RETURNING
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(event_key, ''),
  COALESCE(lifecycle_status, status, ''),
  COALESCE(merged_count, 0),
  COALESCE(last_seen_at, fired_at),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  recovered_at,
  acknowledged_at,
  acknowledged_by,
  acknowledged_note,
  processing_at,
  processing_by,
  processing_note,
  processing_action,
  closed_at,
  closed_by,
  closed_reason,
  ai_task_id,
  email_sent,
  created_at`

	row := r.db.QueryRowContext(
		ctx,
		q,
		opsNullInt64(&event.RuleID),
		opsNullString(event.Severity),
		opsNullString(event.Status),
		opsNullString(event.EventKey),
		opsNullString(event.LifecycleStatus),
		event.MergedCount,
		event.LastSeenAt,
		opsNullString(event.Title),
		opsNullString(event.Description),
		opsNullFloat64(event.MetricValue),
		opsNullFloat64(event.ThresholdValue),
		dimensionsArg,
		triggerSnapshotArg,
		scoreSnapshotArg,
		event.FiredAt,
		opsNullTime(event.ResolvedAt),
		event.EmailSent,
	)
	return scanOpsAlertEvent(row)
}

func (r *opsRepository) MergeAlertEvent(ctx context.Context, eventID int64, event *service.OpsAlertEvent) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return nil, fmt.Errorf("invalid event id")
	}
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}
	dimensionsArg, err := opsNullJSONMap(event.Dimensions)
	if err != nil {
		return nil, err
	}
	triggerSnapshotArg, err := opsNullJSONMap(event.TriggerSnapshot)
	if err != nil {
		return nil, err
	}
	scoreSnapshotArg, err := opsNullJSONMap(event.ScoreSnapshot)
	if err != nil {
		return nil, err
	}

	q := `
UPDATE ops_alert_events
SET last_seen_at = GREATEST(COALESCE(last_seen_at, fired_at), $2),
    merged_count = COALESCE(merged_count, 0) + 1,
    metric_value = $3,
    threshold_value = $4,
    dimensions = $5,
    trigger_snapshot = COALESCE($6::jsonb, trigger_snapshot),
    score_snapshot = COALESCE($7::jsonb, score_snapshot),
    description = COALESCE($8, description)
WHERE id = $1
RETURNING
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(event_key, ''),
  COALESCE(lifecycle_status, status, ''),
  COALESCE(merged_count, 0),
  COALESCE(last_seen_at, fired_at),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  trigger_snapshot,
  score_snapshot,
  fired_at,
  resolved_at,
  recovered_at,
  acknowledged_at,
  acknowledged_by,
  acknowledged_note,
  processing_at,
  processing_by,
  processing_note,
  processing_action,
  closed_at,
  closed_by,
  closed_reason,
  ai_task_id,
  email_sent,
  created_at`

	lastSeenAt := event.LastSeenAt
	if lastSeenAt.IsZero() {
		lastSeenAt = time.Now().UTC()
	}
	row := r.db.QueryRowContext(
		ctx,
		q,
		eventID,
		lastSeenAt,
		opsNullFloat64(event.MetricValue),
		opsNullFloat64(event.ThresholdValue),
		dimensionsArg,
		triggerSnapshotArg,
		scoreSnapshotArg,
		opsNullString(event.Description),
	)
	return scanOpsAlertEvent(row)
}

func (r *opsRepository) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, note string, processingAction string, operatorID *int64, resolvedAt *time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
	}
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("invalid status")
	}

	q := `
UPDATE ops_alert_events
SET status = $2,
    lifecycle_status = $2,
    acknowledged_at = CASE WHEN $2 = 'acknowledged' THEN COALESCE(acknowledged_at, NOW()) ELSE acknowledged_at END,
    acknowledged_by = CASE WHEN $2 = 'acknowledged' THEN COALESCE($5, acknowledged_by) ELSE acknowledged_by END,
    acknowledged_note = CASE WHEN $2 = 'acknowledged' THEN COALESCE($3, acknowledged_note) ELSE acknowledged_note END,
    processing_at = CASE WHEN $2 = 'processing' THEN COALESCE(processing_at, NOW()) ELSE processing_at END,
    processing_by = CASE WHEN $2 = 'processing' THEN COALESCE($5, processing_by) ELSE processing_by END,
    processing_note = CASE WHEN $2 = 'processing' THEN COALESCE($3, processing_note) ELSE processing_note END,
    processing_action = CASE WHEN $2 = 'processing' THEN COALESCE($4, processing_action) ELSE processing_action END,
    recovered_at = CASE WHEN $2 = 'recovered' THEN COALESCE($6, NOW()) ELSE recovered_at END,
    closed_at = CASE WHEN $2 = 'closed' THEN COALESCE($6, NOW()) ELSE closed_at END,
    closed_by = CASE WHEN $2 = 'closed' THEN COALESCE($5, closed_by) ELSE closed_by END,
    closed_reason = CASE WHEN $2 = 'closed' THEN COALESCE($3, closed_reason) ELSE closed_reason END,
    resolved_at = CASE WHEN $2 IN ('recovered', 'closed') THEN COALESCE($6, NOW()) ELSE resolved_at END
WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, eventID, strings.TrimSpace(status), opsNullString(note), opsNullString(processingAction), opsNullInt64(operatorID), opsNullTime(resolvedAt))
	return err
}

func (r *opsRepository) UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
	}

	_, err := r.db.ExecContext(ctx, "UPDATE ops_alert_events SET email_sent = $2 WHERE id = $1", eventID, emailSent)
	return err
}

type opsAlertEventRow interface {
	Scan(dest ...any) error
}

func (r *opsRepository) CreateAlertSilence(ctx context.Context, input *service.OpsAlertSilence) (*service.OpsAlertSilence, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return nil, fmt.Errorf("nil input")
	}
	if input.RuleID <= 0 {
		return nil, fmt.Errorf("invalid rule_id")
	}
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		return nil, fmt.Errorf("invalid platform")
	}
	if input.Until.IsZero() {
		return nil, fmt.Errorf("invalid until")
	}

	q := `
INSERT INTO ops_alert_silences (
  rule_id,
  platform,
  group_id,
  region,
  until,
  reason,
  created_by,
  created_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,NOW()
)
RETURNING id, rule_id, platform, group_id, region, until, COALESCE(reason,''), created_by, created_at`

	row := r.db.QueryRowContext(
		ctx,
		q,
		input.RuleID,
		platform,
		opsNullInt64(input.GroupID),
		opsNullString(input.Region),
		input.Until,
		opsNullString(input.Reason),
		opsNullInt64(input.CreatedBy),
	)

	var out service.OpsAlertSilence
	var groupID sql.NullInt64
	var region sql.NullString
	var createdBy sql.NullInt64
	if err := row.Scan(
		&out.ID,
		&out.RuleID,
		&out.Platform,
		&groupID,
		&region,
		&out.Until,
		&out.Reason,
		&createdBy,
		&out.CreatedAt,
	); err != nil {
		return nil, err
	}
	if groupID.Valid {
		v := groupID.Int64
		out.GroupID = &v
	}
	if region.Valid {
		v := strings.TrimSpace(region.String)
		if v != "" {
			out.Region = &v
		}
	}
	if createdBy.Valid {
		v := createdBy.Int64
		out.CreatedBy = &v
	}
	return &out, nil
}

func (r *opsRepository) IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("nil ops repository")
	}
	if ruleID <= 0 {
		return false, fmt.Errorf("invalid rule id")
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return false, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	q := `
SELECT 1
FROM ops_alert_silences
WHERE rule_id = $1
  AND platform = $2
  AND (group_id IS NOT DISTINCT FROM $3)
  AND (region IS NOT DISTINCT FROM $4)
  AND until > $5
LIMIT 1`

	var dummy int
	err := r.db.QueryRowContext(ctx, q, ruleID, platform, opsNullInt64(groupID), opsNullString(region), now).Scan(&dummy)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func decodeOpsAlertRuleJSONFields(rule *service.OpsAlertRule, errorCategoriesRaw, impactScopeRaw, notificationChannelsRaw []byte) {
	if rule == nil {
		return
	}
	if len(errorCategoriesRaw) > 0 && string(errorCategoriesRaw) != "null" {
		var decoded []string
		if err := json.Unmarshal(errorCategoriesRaw, &decoded); err == nil {
			rule.ErrorCategories = decoded
		}
	}
	if rule.ErrorCategories == nil {
		rule.ErrorCategories = []string{}
	}
	if len(impactScopeRaw) > 0 && string(impactScopeRaw) != "null" {
		var decoded map[string]int
		if err := json.Unmarshal(impactScopeRaw, &decoded); err == nil {
			rule.ImpactScope = decoded
		}
	}
	if rule.ImpactScope == nil {
		rule.ImpactScope = map[string]int{}
	}
	if len(notificationChannelsRaw) > 0 && string(notificationChannelsRaw) != "null" {
		var decoded []string
		if err := json.Unmarshal(notificationChannelsRaw, &decoded); err == nil {
			rule.NotificationChannels = decoded
		}
	}
	if rule.NotificationChannels == nil {
		rule.NotificationChannels = []string{}
	}
}

func opsJSONValue(v any) (any, error) {
	if v == nil {
		return "null", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func scanOpsAlertEvent(row opsAlertEventRow) (*service.OpsAlertEvent, error) {
	var ev service.OpsAlertEvent
	var metricValue sql.NullFloat64
	var thresholdValue sql.NullFloat64
	var dimensionsRaw []byte
	var triggerSnapshotRaw []byte
	var scoreSnapshotRaw []byte
	var resolvedAt sql.NullTime
	var recoveredAt sql.NullTime
	var acknowledgedAt sql.NullTime
	var acknowledgedBy sql.NullInt64
	var acknowledgedNote sql.NullString
	var processingAt sql.NullTime
	var processingBy sql.NullInt64
	var processingNote sql.NullString
	var processingAction sql.NullString
	var closedAt sql.NullTime
	var closedBy sql.NullInt64
	var closedReason sql.NullString
	var aiTaskID sql.NullInt64

	if err := row.Scan(
		&ev.ID,
		&ev.RuleID,
		&ev.Severity,
		&ev.Status,
		&ev.EventKey,
		&ev.LifecycleStatus,
		&ev.MergedCount,
		&ev.LastSeenAt,
		&ev.Title,
		&ev.Description,
		&metricValue,
		&thresholdValue,
		&dimensionsRaw,
		&triggerSnapshotRaw,
		&scoreSnapshotRaw,
		&ev.FiredAt,
		&resolvedAt,
		&recoveredAt,
		&acknowledgedAt,
		&acknowledgedBy,
		&acknowledgedNote,
		&processingAt,
		&processingBy,
		&processingNote,
		&processingAction,
		&closedAt,
		&closedBy,
		&closedReason,
		&aiTaskID,
		&ev.EmailSent,
		&ev.CreatedAt,
	); err != nil {
		return nil, err
	}
	if metricValue.Valid {
		v := metricValue.Float64
		ev.MetricValue = &v
	}
	if thresholdValue.Valid {
		v := thresholdValue.Float64
		ev.ThresholdValue = &v
	}
	if resolvedAt.Valid {
		v := resolvedAt.Time
		ev.ResolvedAt = &v
	}
	if recoveredAt.Valid {
		v := recoveredAt.Time
		ev.RecoveredAt = &v
	}
	if acknowledgedAt.Valid {
		v := acknowledgedAt.Time
		ev.AcknowledgedAt = &v
	}
	if acknowledgedBy.Valid {
		v := acknowledgedBy.Int64
		ev.AcknowledgedBy = &v
	}
	if acknowledgedNote.Valid {
		ev.AcknowledgedNote = acknowledgedNote.String
	}
	if processingAt.Valid {
		v := processingAt.Time
		ev.ProcessingAt = &v
	}
	if processingBy.Valid {
		v := processingBy.Int64
		ev.ProcessingBy = &v
	}
	if processingNote.Valid {
		ev.ProcessingNote = processingNote.String
	}
	if processingAction.Valid {
		ev.ProcessingAction = processingAction.String
	}
	if closedAt.Valid {
		v := closedAt.Time
		ev.ClosedAt = &v
	}
	if closedBy.Valid {
		v := closedBy.Int64
		ev.ClosedBy = &v
	}
	if closedReason.Valid {
		ev.ClosedReason = closedReason.String
	}
	if aiTaskID.Valid {
		v := aiTaskID.Int64
		ev.AITaskID = &v
	}
	if len(dimensionsRaw) > 0 && string(dimensionsRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(dimensionsRaw, &decoded); err == nil {
			ev.Dimensions = decoded
		}
	}
	if len(triggerSnapshotRaw) > 0 && string(triggerSnapshotRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(triggerSnapshotRaw, &decoded); err == nil {
			ev.TriggerSnapshot = decoded
		}
	}
	if len(scoreSnapshotRaw) > 0 && string(scoreSnapshotRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(scoreSnapshotRaw, &decoded); err == nil {
			ev.ScoreSnapshot = decoded
		}
	}
	return &ev, nil
}

func buildOpsAlertEventsWhere(filter *service.OpsAlertEventFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}

	if filter == nil {
		return "WHERE " + strings.Join(clauses, " AND "), args
	}

	if status := strings.TrimSpace(filter.Status); status != "" {
		args = append(args, status)
		clauses = append(clauses, "COALESCE(lifecycle_status, status) = $"+itoa(len(args)))
	}
	if severity := strings.TrimSpace(filter.Severity); severity != "" {
		args = append(args, severity)
		clauses = append(clauses, "severity = $"+itoa(len(args)))
	}
	if filter.EmailSent != nil {
		args = append(args, *filter.EmailSent)
		clauses = append(clauses, "email_sent = $"+itoa(len(args)))
	}
	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, *filter.StartTime)
		clauses = append(clauses, "fired_at >= $"+itoa(len(args)))
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, *filter.EndTime)
		clauses = append(clauses, "fired_at < $"+itoa(len(args)))
	}

	// Cursor pagination (descending by fired_at, then id)
	if filter.BeforeFiredAt != nil && !filter.BeforeFiredAt.IsZero() && filter.BeforeID != nil && *filter.BeforeID > 0 {
		args = append(args, *filter.BeforeFiredAt)
		tsArg := "$" + itoa(len(args))
		args = append(args, *filter.BeforeID)
		idArg := "$" + itoa(len(args))
		clauses = append(clauses, fmt.Sprintf("(fired_at < %s OR (fired_at = %s AND id < %s))", tsArg, tsArg, idArg))
	}
	// Dimensions are stored in JSONB. We filter best-effort without requiring GIN indexes.
	if platform := strings.TrimSpace(filter.Platform); platform != "" {
		args = append(args, platform)
		clauses = append(clauses, "(dimensions->>'platform') = $"+itoa(len(args)))
	}
	if model := strings.TrimSpace(filter.Model); model != "" {
		args = append(args, model)
		clauses = append(clauses, "(dimensions->>'model') = $"+itoa(len(args)))
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, fmt.Sprintf("%d", *filter.GroupID))
		clauses = append(clauses, "(dimensions->>'group_id') = $"+itoa(len(args)))
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

func opsNullJSONMap(v map[string]any) (any, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return sql.NullString{}, nil
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}

func (r *opsRepository) UpdateAlertEventAITaskID(ctx context.Context, eventID int64, taskID int64) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
	}
	if taskID <= 0 {
		return fmt.Errorf("invalid AI analysis task id")
	}
	_, err := r.db.ExecContext(ctx, "UPDATE ops_alert_events SET ai_task_id = $2 WHERE id = $1", eventID, taskID)
	return err
}
