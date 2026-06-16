package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const opsAIAnalysisTaskCreateAdvisoryLockID int64 = 260608026

func (r *opsRepository) CreateAIAnalysisTaskIfAllowed(ctx context.Context, input *service.OpsAIAnalysisTaskCreateInput, maxActive int) (*service.OpsAIAnalysisTask, service.OpsAIAnalysisTaskCreateResult, error) {
	if input == nil {
		return nil, "", errors.New("invalid AI analysis task")
	}
	if maxActive <= 0 {
		return nil, "", errors.New("invalid max active AI analysis tasks")
	}
	if r == nil || r.db == nil {
		return nil, "", fmt.Errorf("nil ops repository")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, opsAIAnalysisTaskCreateAdvisoryLockID); err != nil {
		return nil, "", err
	}

	existing, err := findRecentAutoAIAnalysisTaskByAlertKeyTx(ctx, tx, input)
	if err != nil {
		return nil, "", err
	}
	if existing != nil {
		return existing, service.OpsAIAnalysisTaskCreateResultDuplicate, nil
	}

	existing, err = findActiveAIAnalysisTaskTx(ctx, tx, input)
	if err != nil {
		return nil, "", err
	}
	if existing != nil {
		return existing, service.OpsAIAnalysisTaskCreateResultDuplicate, nil
	}

	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ops_ai_analysis_tasks WHERE status IN ('pending','running')`).Scan(&active); err != nil {
		return nil, "", err
	}
	if active >= maxActive {
		return nil, service.OpsAIAnalysisTaskCreateResultQueueBusy, nil
	}

	task, err := createAIAnalysisTaskTx(ctx, tx, input)
	if err != nil {
		return nil, "", err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	return task, service.OpsAIAnalysisTaskCreateResultCreated, nil
}

func createAIAnalysisTaskTx(ctx context.Context, tx *sql.Tx, input *service.OpsAIAnalysisTaskCreateInput) (*service.OpsAIAnalysisTask, error) {
	row := tx.QueryRowContext(ctx, `
INSERT INTO ops_ai_analysis_tasks (
  source_type, source_id, trigger_type, trigger_user_id, time_start, time_end, filters,
  status, sample_count, provider, model, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,NOW(),NOW())
RETURNING id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,
          filters::text, status, sample_count, provider, model, COALESCE(error_message,''),
          started_at, finished_at, created_at, updated_at`,
		input.SourceType,
		opsNullInt64(input.SourceID),
		input.TriggerType,
		opsNullInt64(input.TriggerUserID),
		input.TimeStart,
		input.TimeEnd,
		input.FiltersJSON,
		input.Status,
		input.SampleCount,
		input.Provider,
		input.Model,
	)
	return scanOpsAIAnalysisTask(row)
}

func findRecentAutoAIAnalysisTaskByAlertKeyTx(ctx context.Context, tx *sql.Tx, input *service.OpsAIAnalysisTaskCreateInput) (*service.OpsAIAnalysisTask, error) {
	if input == nil || input.SourceType != service.OpsAIAnalysisSourceAlertEvent || input.TriggerType != service.OpsAIAnalysisTriggerAuto {
		return nil, nil
	}
	alertKey := alertEventKeyFromAIAnalysisFilters(input.FiltersJSON)
	if alertKey == "" {
		return nil, nil
	}
	dedupSince := input.TimeStart
	if input.DedupSince != nil && !input.DedupSince.IsZero() {
		dedupSince = *input.DedupSince
	}
	row := tx.QueryRowContext(ctx, `
SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,
       filters::text, status, sample_count, provider, model, COALESCE(error_message,''),
       started_at, finished_at, created_at, updated_at
FROM ops_ai_analysis_tasks
WHERE source_type = $1
  AND trigger_type = $2
  AND filters->>'alert_event_key' = $3
  AND created_at >= $4
  AND status IN ('pending','running','completed')
ORDER BY created_at DESC
LIMIT 1`, input.SourceType, input.TriggerType, alertKey, dedupSince)
	task, err := scanOpsAIAnalysisTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return task, nil
}

func alertEventKeyFromAIAnalysisFilters(filtersJSON string) string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(filtersJSON), &raw); err != nil {
		return ""
	}
	value, ok := raw["alert_event_key"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func findActiveAIAnalysisTaskTx(ctx context.Context, tx *sql.Tx, input *service.OpsAIAnalysisTaskCreateInput) (*service.OpsAIAnalysisTask, error) {
	row := tx.QueryRowContext(ctx, `
SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,
       filters::text, status, sample_count, provider, model, COALESCE(error_message,''),
       started_at, finished_at, created_at, updated_at
FROM ops_ai_analysis_tasks
WHERE status IN ('pending','running')
  AND source_type = $1
  AND (($2::bigint IS NULL AND source_id IS NULL) OR source_id = $2::bigint)
  AND time_start = $3
  AND time_end = $4
  AND filters = $5::jsonb
ORDER BY created_at DESC
LIMIT 1`, input.SourceType, opsNullInt64(input.SourceID), input.TimeStart, input.TimeEnd, input.FiltersJSON)
	task, err := scanOpsAIAnalysisTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return task, nil
}

type opsAIAnalysisTaskScanner interface {
	Scan(dest ...any) error
}

func scanOpsAIAnalysisTask(row opsAIAnalysisTaskScanner) (*service.OpsAIAnalysisTask, error) {
	var task service.OpsAIAnalysisTask
	var sourceID sql.NullInt64
	var triggerUserID sql.NullInt64
	var provider sql.NullString
	var model sql.NullString
	var errorMessage sql.NullString
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	if err := row.Scan(
		&task.ID,
		&task.SourceType,
		&sourceID,
		&task.TriggerType,
		&triggerUserID,
		&task.TimeStart,
		&task.TimeEnd,
		&task.FiltersJSON,
		&task.Status,
		&task.SampleCount,
		&provider,
		&model,
		&errorMessage,
		&startedAt,
		&finishedAt,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return nil, err
	}
	task.SourceID = sqlNullInt64Ptr(sourceID)
	task.TriggerUserID = sqlNullInt64Ptr(triggerUserID)
	if provider.Valid {
		task.Provider = provider.String
	}
	if model.Valid {
		task.Model = model.String
	}
	if errorMessage.Valid {
		task.ErrorMessage = errorMessage.String
	}
	if startedAt.Valid {
		t := startedAt.Time
		task.StartedAt = &t
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		task.FinishedAt = &t
	}
	return &task, nil
}

func (r *opsRepository) ClaimNextAIAnalysisTask(ctx context.Context) (*service.OpsAIAnalysisTask, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	row := r.db.QueryRowContext(ctx, `
WITH picked AS (
  SELECT id
  FROM ops_ai_analysis_tasks
  WHERE status = 'pending'
  ORDER BY created_at ASC, id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE ops_ai_analysis_tasks t
SET status = 'running', started_at = COALESCE(t.started_at, NOW()), updated_at = NOW(), error_message = NULL
FROM picked
WHERE t.id = picked.id
RETURNING t.id, t.source_type, t.source_id, t.trigger_type, t.trigger_user_id, t.time_start, t.time_end,
          t.filters::text, t.status, t.sample_count, t.provider, t.model, COALESCE(t.error_message,''),
          t.started_at, t.finished_at, t.created_at, t.updated_at`)
	task, err := scanOpsAIAnalysisTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return task, nil
}

func (r *opsRepository) UpdateAIAnalysisTask(ctx context.Context, taskID int64, update *service.OpsAIAnalysisTaskUpdate) (*service.OpsAIAnalysisTask, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if taskID <= 0 || update == nil {
		return nil, errors.New("invalid AI analysis task update")
	}
	row := r.db.QueryRowContext(ctx, `
UPDATE ops_ai_analysis_tasks
SET status = $2,
    sample_count = COALESCE($3, sample_count),
    error_message = $4,
    started_at = COALESCE($5, started_at),
    finished_at = $6,
    updated_at = NOW()
WHERE id = $1
RETURNING id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,
          filters::text, status, sample_count, provider, model, COALESCE(error_message,''),
          started_at, finished_at, created_at, updated_at`,
		taskID,
		update.Status,
		opsAINullInt(update.SampleCount),
		opsNullString(update.ErrorMessage),
		opsNullTime(update.StartedAt),
		opsNullTime(update.FinishedAt),
	)
	return scanOpsAIAnalysisTask(row)
}

func opsAINullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func (r *opsRepository) GetAIAnalysisTask(ctx context.Context, taskID int64) (*service.OpsAIAnalysisTask, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if taskID <= 0 {
		return nil, errors.New("invalid AI analysis task id")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,
       filters::text, status, sample_count, provider, model, COALESCE(error_message,''),
       started_at, finished_at, created_at, updated_at
FROM ops_ai_analysis_tasks
WHERE id = $1`, taskID)
	return scanOpsAIAnalysisTask(row)
}

func (r *opsRepository) InsertAIAnalysisReport(ctx context.Context, report *service.OpsAIAnalysisReport) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if report == nil || report.TaskID <= 0 {
		return errors.New("invalid AI analysis report")
	}
	impactScope := report.ImpactScopeJSON
	if impactScope == "" {
		impactScope = "{}"
	}
	evidence := report.EvidenceJSON
	if evidence == "" {
		evidence = "[]"
	}
	actions := report.ActionsJSON
	if actions == "" {
		actions = "[]"
	}
	breakdown := report.BreakdownJSON
	if breakdown == "" {
		breakdown = "{}"
	}
	confidence := report.Confidence
	if confidence == "" {
		confidence = "medium"
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO ops_ai_analysis_reports
  (task_id, summary, root_cause, impact_scope, evidence, suggested_actions, error_breakdown, confidence)
VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, $8)
ON CONFLICT (task_id) DO UPDATE SET
  summary = EXCLUDED.summary,
  root_cause = EXCLUDED.root_cause,
  impact_scope = EXCLUDED.impact_scope,
  evidence = EXCLUDED.evidence,
  suggested_actions = EXCLUDED.suggested_actions,
  error_breakdown = EXCLUDED.error_breakdown,
  confidence = EXCLUDED.confidence,
  updated_at = now()`,
		report.TaskID,
		report.Summary,
		report.RootCause,
		impactScope,
		evidence,
		actions,
		breakdown,
		confidence,
	)
	return err
}

func (r *opsRepository) GetAIAnalysisReport(ctx context.Context, taskID int64) (*service.OpsAIAnalysisReport, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if taskID <= 0 {
		return nil, errors.New("invalid AI analysis task id")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT task_id, summary, COALESCE(root_cause,''), impact_scope::text, evidence::text,
       suggested_actions::text, error_breakdown::text, confidence, feedback_status,
       COALESCE(feedback_note,''), feedback_user_id, feedback_at, created_at, updated_at
FROM ops_ai_analysis_reports
WHERE task_id = $1`, taskID)
	return scanOpsAIAnalysisReport(row)
}

func (r *opsRepository) GetLatestAutoAIAnalysisTask(ctx context.Context) (*service.OpsAIAnalysisTask, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,
       filters::text, status, sample_count, provider, model, COALESCE(error_message,''),
       started_at, finished_at, created_at, updated_at
FROM ops_ai_analysis_tasks
WHERE trigger_type = 'auto'
ORDER BY created_at DESC
LIMIT 1`)
	task, err := scanOpsAIAnalysisTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return task, nil
}

func (r *opsRepository) ListAIAnalysisTasks(ctx context.Context, limit int) ([]*service.OpsAIAnalysisTask, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,
       filters::text, status, sample_count, provider, model, COALESCE(error_message,''),
       started_at, finished_at, created_at, updated_at
FROM ops_ai_analysis_tasks
ORDER BY created_at DESC, id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tasks := make([]*service.OpsAIAnalysisTask, 0)
	for rows.Next() {
		task, err := scanOpsAIAnalysisTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *opsRepository) ListAIAnalysisReportHistory(ctx context.Context, limit int) ([]*service.OpsAIAnalysisReportHistoryItem, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT
  t.id, t.source_type, t.source_id, t.trigger_type, t.trigger_user_id, t.time_start, t.time_end,
  t.filters::text, t.status, t.sample_count, t.provider, t.model, COALESCE(t.error_message,''),
  t.started_at, t.finished_at, t.created_at, t.updated_at,
  r.task_id, r.summary, COALESCE(r.root_cause,''), r.impact_scope::text, r.evidence::text,
  r.suggested_actions::text, r.error_breakdown::text, r.confidence, r.feedback_status,
  COALESCE(r.feedback_note,''), r.feedback_user_id, r.feedback_at, r.created_at, r.updated_at
FROM ops_ai_analysis_reports r
JOIN ops_ai_analysis_tasks t ON t.id = r.task_id
ORDER BY r.created_at DESC, r.task_id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsAIAnalysisReportHistoryItem, 0)
	for rows.Next() {
		task, report, err := scanOpsAIAnalysisTaskAndReport(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, &service.OpsAIAnalysisReportHistoryItem{Task: task, Report: report})
	}
	return items, rows.Err()
}

func (r *opsRepository) UpdateAIAnalysisReportFeedback(ctx context.Context, input *service.OpsAIAnalysisFeedbackInput) (*service.OpsAIAnalysisReport, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if input == nil || input.TaskID <= 0 {
		return nil, errors.New("invalid AI analysis task id")
	}
	row := r.db.QueryRowContext(ctx, `
UPDATE ops_ai_analysis_reports
SET feedback_status = $2,
    feedback_note = NULLIF($3, ''),
    feedback_user_id = $4,
    feedback_at = NOW(),
    updated_at = NOW()
WHERE task_id = $1
RETURNING task_id, summary, COALESCE(root_cause,''), impact_scope::text, evidence::text,
          suggested_actions::text, error_breakdown::text, confidence, feedback_status,
          COALESCE(feedback_note,''), feedback_user_id, feedback_at, created_at, updated_at`,
		input.TaskID,
		input.FeedbackStatus,
		input.FeedbackNote,
		input.FeedbackUserID,
	)
	return scanOpsAIAnalysisReport(row)
}

type opsAIAnalysisReportScanner interface {
	Scan(dest ...any) error
}

func scanOpsAIAnalysisTaskAndReport(row opsAIAnalysisReportScanner) (*service.OpsAIAnalysisTask, *service.OpsAIAnalysisReport, error) {
	var task service.OpsAIAnalysisTask
	var report service.OpsAIAnalysisReport
	if err := row.Scan(
		&task.ID,
		&task.SourceType,
		&task.SourceID,
		&task.TriggerType,
		&task.TriggerUserID,
		&task.TimeStart,
		&task.TimeEnd,
		&task.FiltersJSON,
		&task.Status,
		&task.SampleCount,
		&task.Provider,
		&task.Model,
		&task.ErrorMessage,
		&task.StartedAt,
		&task.FinishedAt,
		&task.CreatedAt,
		&task.UpdatedAt,
		&report.TaskID,
		&report.Summary,
		&report.RootCause,
		&report.ImpactScopeJSON,
		&report.EvidenceJSON,
		&report.ActionsJSON,
		&report.BreakdownJSON,
		&report.Confidence,
		&report.FeedbackStatus,
		&report.FeedbackNote,
		&report.FeedbackUserID,
		&report.FeedbackAt,
		&report.CreatedAt,
		&report.UpdatedAt,
	); err != nil {
		return nil, nil, err
	}
	report.ImpactScope = decodeJSONValue(report.ImpactScopeJSON, map[string]any{})
	report.Evidence = decodeJSONValue(report.EvidenceJSON, []any{})
	report.SuggestedActions = decodeJSONValue(report.ActionsJSON, []any{})
	report.ErrorBreakdown = decodeJSONValue(report.BreakdownJSON, map[string]any{})
	return &task, &report, nil
}

func scanOpsAIAnalysisReport(row opsAIAnalysisReportScanner) (*service.OpsAIAnalysisReport, error) {
	var report service.OpsAIAnalysisReport
	if err := row.Scan(
		&report.TaskID,
		&report.Summary,
		&report.RootCause,
		&report.ImpactScopeJSON,
		&report.EvidenceJSON,
		&report.ActionsJSON,
		&report.BreakdownJSON,
		&report.Confidence,
		&report.FeedbackStatus,
		&report.FeedbackNote,
		&report.FeedbackUserID,
		&report.FeedbackAt,
		&report.CreatedAt,
		&report.UpdatedAt,
	); err != nil {
		return nil, err
	}
	report.ImpactScope = decodeJSONValue(report.ImpactScopeJSON, map[string]any{})
	report.Evidence = decodeJSONValue(report.EvidenceJSON, []any{})
	report.SuggestedActions = decodeJSONValue(report.ActionsJSON, []any{})
	report.ErrorBreakdown = decodeJSONValue(report.BreakdownJSON, map[string]any{})
	return &report, nil
}

func decodeJSONValue(raw string, fallback any) any {
	if raw == "" {
		return fallback
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return fallback
	}
	return out
}
