package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func opsAlertEventRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "rule_id", "severity", "status", "event_key", "lifecycle_status", "merged_count", "last_seen_at",
		"title", "description", "metric_value", "threshold_value", "dimensions", "trigger_snapshot", "score_snapshot",
		"fired_at", "resolved_at", "recovered_at", "acknowledged_at", "acknowledged_by", "acknowledged_note", "processing_at", "processing_by", "processing_note", "processing_action", "closed_at", "closed_by", "closed_reason", "ai_task_id", "email_sent", "created_at",
	})
}

func TestOpsRepositoryCreateAlertEventPersistsDedupFields(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	metricValue := 12.5
	thresholdValue := 10.0
	event := &service.OpsAlertEvent{
		RuleID:          7,
		Severity:        "P1",
		Status:          service.OpsAlertStatusFiring,
		EventKey:        "upstream|error_rate|7|gpt-5.5|42|P1",
		LifecycleStatus: service.OpsAlertStatusFiring,
		MergedCount:     0,
		LastSeenAt:      now,
		Title:           "P1: 上游错误率",
		Description:     "触发上游错误率",
		MetricValue:     &metricValue,
		ThresholdValue:  &thresholdValue,
		Dimensions:      map[string]any{"group_id": 7, "model": "gpt-5.5"},
		TriggerSnapshot: map[string]any{"rule_id": 7, "metric_value": 12.5},
		FiredAt:         now,
	}

	rows := opsAlertEventRows().AddRow(
		int64(31), int64(7), "P1", service.OpsAlertStatusFiring,
		event.EventKey, service.OpsAlertStatusFiring, 0, now,
		event.Title, event.Description, metricValue, thresholdValue,
		[]byte(`{"group_id":7,"model":"gpt-5.5"}`),
		[]byte(`{"rule_id":7,"metric_value":12.5}`),
		[]byte(`null`),
		now, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, false, now,
	)
	mock.ExpectQuery(`(?s)INSERT INTO ops_alert_events .*event_key.*lifecycle_status.*merged_count.*last_seen_at.*trigger_snapshot.*score_snapshot.*RETURNING`).
		WithArgs(
			sqlmock.AnyArg(), "P1", service.OpsAlertStatusFiring,
			event.EventKey, service.OpsAlertStatusFiring, 0, now,
			event.Title, event.Description, sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			now, sqlmock.AnyArg(), false,
		).
		WillReturnRows(rows)

	got, err := repo.CreateAlertEvent(context.Background(), event)

	require.NoError(t, err)
	require.Equal(t, int64(31), got.ID)
	require.Equal(t, event.EventKey, got.EventKey)
	require.Equal(t, service.OpsAlertStatusFiring, got.LifecycleStatus)
	require.Equal(t, 0, got.MergedCount)
	require.True(t, got.LastSeenAt.Equal(now))
	require.Equal(t, "gpt-5.5", got.Dimensions["model"])
	require.Equal(t, float64(12.5), got.TriggerSnapshot["metric_value"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetMergeableAlertEventByKeyAndWindow(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)
	eventKey := "upstream|error_rate|7|gpt-5.5|42|P1"

	rows := opsAlertEventRows().AddRow(
		int64(31), int64(7), "P1", service.OpsAlertStatusFiring,
		eventKey, service.OpsAlertStatusFiring, 2, now,
		"P1: 上游错误率", "触发上游错误率", 12.5, 10.0,
		[]byte(`{"group_id":7}`), []byte(`{"rule_id":7}`), []byte(`null`),
		now, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, now,
	)
	mock.ExpectQuery(`(?s)SELECT.*FROM ops_alert_events.*WHERE event_key = \$1.*last_seen_at.*>= \$2.*LIMIT 1`).
		WithArgs(eventKey, since).
		WillReturnRows(rows)

	got, err := repo.GetMergeableAlertEvent(context.Background(), eventKey, since)

	require.NoError(t, err)
	require.Equal(t, int64(31), got.ID)
	require.Equal(t, 2, got.MergedCount)
	require.Equal(t, eventKey, got.EventKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryMergeAlertEventUpdatesCountAndLastSeen(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	metricValue := 15.0
	thresholdValue := 10.0
	event := &service.OpsAlertEvent{
		LastSeenAt:      now,
		Description:     "再次触发",
		MetricValue:     &metricValue,
		ThresholdValue:  &thresholdValue,
		Dimensions:      map[string]any{"group_id": 7},
		TriggerSnapshot: map[string]any{"metric_value": 15.0},
	}

	rows := opsAlertEventRows().AddRow(
		int64(31), int64(7), "P1", service.OpsAlertStatusFiring,
		"upstream|error_rate|7|gpt-5.5|42|P1", service.OpsAlertStatusFiring, 3, now,
		"P1: 上游错误率", "再次触发", metricValue, thresholdValue,
		[]byte(`{"group_id":7}`), []byte(`{"metric_value":15}`), []byte(`null`),
		now.Add(-time.Minute), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, now.Add(-time.Minute),
	)
	mock.ExpectQuery(`(?s)UPDATE ops_alert_events.*merged_count = COALESCE\(merged_count, 0\) \+ 1.*RETURNING`).
		WithArgs(int64(31), now, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), event.Description).
		WillReturnRows(rows)

	got, err := repo.MergeAlertEvent(context.Background(), 31, event)

	require.NoError(t, err)
	require.Equal(t, int64(31), got.ID)
	require.Equal(t, 3, got.MergedCount)
	require.True(t, got.LastSeenAt.Equal(now))
	require.Equal(t, "再次触发", got.Description)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryUpdateAlertEventStatusWritesLifecycleFields(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	operatorID := int64(6)
	resolvedAt := time.Date(2026, 6, 7, 10, 5, 0, 0, time.UTC)

	tests := []struct {
		name       string
		status     string
		note       string
		resolvedAt *time.Time
	}{
		{name: "acknowledged", status: service.OpsAlertStatusAcknowledged, note: "已确认"},
		{name: "processing", status: service.OpsAlertStatusProcessing, note: "处理中"},
		{name: "recovered", status: service.OpsAlertStatusRecovered, resolvedAt: &resolvedAt},
		{name: "closed", status: service.OpsAlertStatusClosed, note: "人工关闭", resolvedAt: &resolvedAt},
		{name: "silenced", status: service.OpsAlertStatusSilenced, note: "静默"},
	}
	for _, tt := range tests {
		mock.ExpectExec(`(?s)UPDATE ops_alert_events.*lifecycle_status = \$2.*acknowledged_at.*processing_at.*processing_action.*recovered_at.*closed_at.*resolved_at`).
			WithArgs(int64(31), tt.status, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateAlertEventStatus(context.Background(), 31, tt.status, tt.note, "独立处理动作", &operatorID, tt.resolvedAt)
		require.NoError(t, err, tt.name)
	}

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryUpdateAlertEventStatusKeepsProcessingNoteAndActionSeparate(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	operatorID := int64(6)

	mock.ExpectExec(`(?s)UPDATE ops_alert_events.*processing_note = CASE WHEN \$2 = 'processing'.*processing_action = CASE WHEN \$2 = 'processing'`).
		WithArgs(int64(31), service.OpsAlertStatusProcessing, "处理说明", "切换上游账号", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateAlertEventStatus(context.Background(), 31, service.OpsAlertStatusProcessing, "处理说明", "切换上游账号", &operatorID, nil)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
