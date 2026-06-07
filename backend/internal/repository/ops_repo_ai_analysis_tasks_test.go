package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateAIAnalysisTaskIfAllowedCreated(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	userID := int64(7)
	input := &service.OpsAIAnalysisTaskCreateInput{
		SourceType:    service.OpsAIAnalysisSourceUnifiedErrors,
		TriggerType:   service.OpsAIAnalysisTriggerManual,
		TriggerUserID: &userID,
		TimeStart:     start,
		TimeEnd:       end,
		FiltersJSON:   `{"error_categories":["upstream"],"platform":"openai"}`,
		Status:        service.OpsAIAnalysisStatusPending,
		Provider:      "responses",
		Model:         "gpt-5.5",
	}
	createdAt := start.Add(time.Minute)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(opsAIAnalysisTaskCreateAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,.*FROM ops_ai_analysis_tasks.*status IN \('pending','running'\).*LIMIT 1`).
		WithArgs(input.SourceType, nil, start, end, input.FiltersJSON).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_ai_analysis_tasks WHERE status IN \('pending','running'\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`(?s)INSERT INTO ops_ai_analysis_tasks .*RETURNING id, source_type`).
		WithArgs(input.SourceType, nil, input.TriggerType, userID, start, end, input.FiltersJSON, input.Status, input.SampleCount, input.Provider, input.Model).
		WillReturnRows(taskRows().AddRow(int64(99), input.SourceType, nil, input.TriggerType, userID, start, end, input.FiltersJSON, input.Status, 0, input.Provider, input.Model, "", nil, nil, createdAt, createdAt))
	mock.ExpectCommit()

	got, result, err := repo.CreateAIAnalysisTaskIfAllowed(context.Background(), input, 3)
	require.NoError(t, err)
	require.Equal(t, service.OpsAIAnalysisTaskCreateResultCreated, result)
	require.NotNil(t, got)
	require.Equal(t, int64(99), got.ID)
	require.Equal(t, input.Provider, got.Provider)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAIAnalysisTaskIfAllowedDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	input := &service.OpsAIAnalysisTaskCreateInput{SourceType: service.OpsAIAnalysisSourceUnifiedErrors, TriggerType: service.OpsAIAnalysisTriggerManual, TimeStart: start, TimeEnd: end, FiltersJSON: `{"platform":"openai"}`, Status: service.OpsAIAnalysisStatusPending, Provider: "responses", Model: "gpt-5.5"}
	createdAt := start.Add(time.Minute)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(opsAIAnalysisTaskCreateAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,.*LIMIT 1`).
		WithArgs(input.SourceType, nil, start, end, input.FiltersJSON).
		WillReturnRows(taskRows().AddRow(int64(11), input.SourceType, nil, input.TriggerType, nil, start, end, input.FiltersJSON, input.Status, 0, input.Provider, input.Model, "", nil, nil, createdAt, createdAt))
	mock.ExpectRollback()

	got, result, err := repo.CreateAIAnalysisTaskIfAllowed(context.Background(), input, 3)
	require.NoError(t, err)
	require.Equal(t, service.OpsAIAnalysisTaskCreateResultDuplicate, result)
	require.NotNil(t, got)
	require.Equal(t, int64(11), got.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAIAnalysisTaskIfAllowedQueueBusy(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	input := &service.OpsAIAnalysisTaskCreateInput{SourceType: service.OpsAIAnalysisSourceUnifiedErrors, TriggerType: service.OpsAIAnalysisTriggerManual, TimeStart: start, TimeEnd: end, FiltersJSON: `{"platform":"openai"}`, Status: service.OpsAIAnalysisStatusPending, Provider: "responses", Model: "gpt-5.5"}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(opsAIAnalysisTaskCreateAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end,.*LIMIT 1`).
		WithArgs(input.SourceType, nil, start, end, input.FiltersJSON).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_ai_analysis_tasks WHERE status IN \('pending','running'\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectRollback()

	got, result, err := repo.CreateAIAnalysisTaskIfAllowed(context.Background(), input, 3)
	require.NoError(t, err)
	require.Nil(t, got)
	require.Equal(t, service.OpsAIAnalysisTaskCreateResultQueueBusy, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func taskRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "source_type", "source_id", "trigger_type", "trigger_user_id", "time_start", "time_end",
		"filters", "status", "sample_count", "provider", "model", "error_message", "started_at", "finished_at", "created_at", "updated_at",
	})
}

func TestClaimNextAIAnalysisTask(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	createdAt := start.Add(time.Minute)
	mock.ExpectQuery(`(?s)WITH picked AS \(.*FOR UPDATE SKIP LOCKED.*UPDATE ops_ai_analysis_tasks t.*status = 'running'.*RETURNING t\.id`).
		WillReturnRows(taskRows().AddRow(int64(77), service.OpsAIAnalysisSourceUnifiedErrors, nil, service.OpsAIAnalysisTriggerManual, int64(7), start, start.Add(30*time.Minute), `{"platform":"openai"}`, service.OpsAIAnalysisStatusRunning, 0, "responses", "gpt-5.5", "", createdAt, nil, createdAt, createdAt))

	got, err := repo.ClaimNextAIAnalysisTask(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(77), got.ID)
	require.Equal(t, service.OpsAIAnalysisStatusRunning, got.Status)
	require.NotNil(t, got.StartedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimNextAIAnalysisTaskNoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	mock.ExpectQuery(`(?s)WITH picked AS \(`).WillReturnError(sql.ErrNoRows)
	got, err := repo.ClaimNextAIAnalysisTask(context.Background())
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAIAnalysisTask(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	finishedAt := start.Add(2 * time.Minute)
	sampleCount := 5
	message := "done"
	mock.ExpectQuery(`(?s)UPDATE ops_ai_analysis_tasks\s+SET status = \$2,.*finished_at = \$6.*WHERE id = \$1.*RETURNING id`).
		WithArgs(int64(77), service.OpsAIAnalysisStatusCompleted, sampleCount, message, nil, finishedAt).
		WillReturnRows(taskRows().AddRow(int64(77), service.OpsAIAnalysisSourceUnifiedErrors, nil, service.OpsAIAnalysisTriggerManual, int64(7), start, start.Add(30*time.Minute), `{"platform":"openai"}`, service.OpsAIAnalysisStatusCompleted, sampleCount, "responses", "gpt-5.5", message, start.Add(time.Minute), finishedAt, start, finishedAt))

	got, err := repo.UpdateAIAnalysisTask(context.Background(), 77, &service.OpsAIAnalysisTaskUpdate{Status: service.OpsAIAnalysisStatusCompleted, SampleCount: &sampleCount, ErrorMessage: &message, FinishedAt: &finishedAt})
	require.NoError(t, err)
	require.Equal(t, service.OpsAIAnalysisStatusCompleted, got.Status)
	require.Equal(t, sampleCount, got.SampleCount)
	require.Equal(t, message, got.ErrorMessage)
	require.NoError(t, mock.ExpectationsWereMet())
}
