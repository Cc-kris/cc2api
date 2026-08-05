package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestValidFinanceAlertTransition(t *testing.T) {
	require.True(t, validFinanceAlertTransition("open", "acknowledged"))
	require.True(t, validFinanceAlertTransition("acknowledged", "open"))
	require.True(t, validFinanceAlertTransition("resolved", "open"))
	require.False(t, validFinanceAlertTransition("resolved", "ignored"))
	require.False(t, validFinanceAlertTransition("open", "open"))
}

func TestFinanceAlertRepositoryUpsertAggregatesOpenAlert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &financeAlertRepository{db: db}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	impact := decimal.RequireFromString("12.5")
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("negative_profit:global").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("negative_profit:global").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("INSERT INTO finance_alerts").
		WithArgs("negative_profit", "critical", "negative_profit:global", "loss", "cost exceeds revenue", "global", nil, "12.5", int64(2), now).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	err = repo.UpsertFinanceAlertSignals(context.Background(), []service.FinanceAlertSignal{{
		AlertType: "negative_profit", Severity: "critical", AggregationKey: "negative_profit:global",
		Title: "loss", Description: "cost exceeds revenue", DimensionType: "global", ImpactAmount: &impact, RequestCount: 2, OccurredAt: now,
	}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFinanceAlertRepositoryLocksSignalsInStableAggregationOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &financeAlertRepository{db: db}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	for _, key := range []string{"negative_profit:a", "negative_profit:z"} {
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(key).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(key).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	}
	mock.ExpectCommit()
	err = repo.UpsertFinanceAlertSignals(context.Background(), []service.FinanceAlertSignal{
		{AlertType: "negative_profit", AggregationKey: "negative_profit:z", OccurredAt: now},
		{AlertType: "negative_profit", AggregationKey: "negative_profit:a", OccurredAt: now},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFinanceAlertRepositoryListExcludesHistoricalAdminDerivedAlerts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &financeAlertRepository{db: db}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery(`(?s)missing_scope\.impact_amount.*FROM finance_alerts alert.*ufr\.cost_status=alert\.alert_type.*alert\.aggregation_key='payment_fee_uncollected:'\|\|COALESCE.*provider_key.*WHERE.*ufr\.usage_log_id=alert\.dimension_id.*LIMIT \$4 OFFSET \$5`).
		WithArgs(start, end, service.RoleAdmin, 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "alert_type", "severity", "title", "description", "dimension_type", "dimension_id", "impact_amount",
			"request_count", "occurrence_count", "status", "first_occurred_at", "last_occurred_at", "assignee_id", "handled_by", "handled_note", "handled_at", "total",
		}))

	items, total, err := repo.ListFinanceAlerts(t.Context(), service.FinanceReportFilter{StartAt: start, EndBefore: end}, service.FinanceAlertListRequest{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFinanceAlertRepositoryStatusUpdateWritesAuditAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &financeAlertRepository{db: db}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT aggregation_key FROM finance_alerts").WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"aggregation_key"}).AddRow("negative_profit:global"))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("negative_profit:global").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT status FROM finance_alerts").WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("open"))
	mock.ExpectExec("UPDATE finance_alerts").WithArgs(int64(9), "acknowledged", int64(7), "checking", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO finance_alert_status_audits").WithArgs(int64(9), "open", "acknowledged", "checking", int64(7), now).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT id,alert_type").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "alert_type", "severity", "title", "description", "dimension_type", "dimension_id", "impact_amount",
		"request_count", "occurrence_count", "status", "first_occurred_at", "last_occurred_at", "assignee_id", "handled_by", "handled_note", "handled_at", "total",
	}).AddRow(9, "negative_profit", "critical", "loss", "desc", "global", nil, "2.0", 1, 1, "acknowledged", now, now, nil, 7, "checking", now, 1))
	mock.ExpectCommit()
	item, err := repo.UpdateFinanceAlertStatus(context.Background(), 9, "acknowledged", "checking", 7, now)
	require.NoError(t, err)
	require.Equal(t, "acknowledged", item.Status)
	require.Equal(t, int64(7), *item.HandledBy)
	require.NoError(t, mock.ExpectationsWereMet())
}
