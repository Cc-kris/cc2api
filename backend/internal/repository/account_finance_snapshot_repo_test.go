package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

func TestAccountFinanceSnapshotRepository_WithAccountSyncLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := NewAccountFinanceSnapshotRepository(db)
	lockKey := "account_finance_multiplier:7:account:7"
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_lock(hashtextextended($1, 0))`)).WithArgs(lockKey).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock(hashtextextended($1, 0))`)).WithArgs(lockKey).WillReturnResult(sqlmock.NewResult(0, 1))
	called := false
	if err := repo.WithAccountSyncLock(context.Background(), 7, "account:7", func(context.Context) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("with lock: %v", err)
	}
	if !called {
		t.Fatal("callback not called")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestAccountFinanceSnapshotRepository_CreateIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := NewAccountFinanceSnapshotRepository(db)
	collectedAt := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	listCost := decimal.RequireFromString("20")
	actualCost := decimal.RequireFromString("5")
	currency := "USD"
	snapshot := &service.AccountFinanceCounterSnapshot{
		AccountID: 7, ScopeKey: "account:7", IdempotencyKey: "sync-2",
		ListCostTotal: &listCost, ActualCostTotal: &actualCost,
		UnitCode: "USD", UnitSemantics: service.AccountFinanceUnitFiatCurrency, Currency: &currency,
		CollectedAt: collectedAt, SafeSnapshot: map[string]any{"usage": "ok"}, Checksum: "checksum",
		DerivationStatus: service.AccountFinanceDerivationBaseline,
	}
	mock.ExpectQuery("INSERT INTO account_finance_counter_snapshots").WillReturnError(sqlmock.ErrCancelled)
	if _, _, err = repo.CreateCounterSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("expected insert error")
	}

	// A uniqueness retry returns the already persisted row instead of creating
	// another observation or another multiplier version.
	mock.ExpectQuery("INSERT INTO account_finance_counter_snapshots").WillReturnRows(sqlmock.NewRows(accountFinanceSnapshotTestColumns()))
	mock.ExpectQuery("FROM account_finance_counter_snapshots").WithArgs(int64(7), "account:7", "sync-2").WillReturnRows(accountFinanceSnapshotTestRow(collectedAt))
	stored, created, err := repo.CreateCounterSnapshot(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if created || stored.ID != 11 || stored.IdempotencyKey != "sync-2" {
		t.Fatalf("unexpected retry result: created=%v snapshot=%#v", created, stored)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestAccountFinanceSnapshotRepository_ResolveVersionUsesEffectiveTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := NewAccountFinanceSnapshotRepository(db)
	effectiveAt := time.Date(2026, 7, 29, 12, 30, 0, 0, time.UTC)
	mock.ExpectQuery("FROM account_upstream_multiplier_changes").WithArgs(int64(7), effectiveAt).WillReturnRows(
		sqlmock.NewRows([]string{"id", "account_id", "old_multiplier", "new_multiplier", "effective_at", "reason"}).
			AddRow(int64(9), int64(7), "0.2200", "0.2500", effectiveAt, "automatic cumulative fiat observation: snapshot_id=11"),
	)
	version, err := repo.ResolveEffectiveMultiplierVersion(context.Background(), 7, effectiveAt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if version == nil || version.ID != 9 || !version.NewMultiplier.Equal(decimal.RequireFromString("0.25")) {
		t.Fatalf("version = %#v", version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func accountFinanceSnapshotTestColumns() []string {
	return []string{
		"id", "account_id", "account_finance_profile_id", "scope_key", "idempotency_key", "upstream_counter_id", "counter_period",
		"list_cost_total", "actual_cost_total", "unit_code", "unit_semantics", "currency",
		"upstream_observed_at", "collected_at", "safe_snapshot", "checksum", "previous_snapshot_id",
		"list_cost_delta", "actual_cost_delta", "observed_multiplier", "derivation_status", "anomaly_code",
		"multiplier_change_id", "multiplier_effective_at", "created_at",
	}
}

func accountFinanceSnapshotTestRow(collectedAt time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(accountFinanceSnapshotTestColumns()).AddRow(
		int64(11), int64(7), int64(31), "account:7", "sync-2", "usage-total", "cycle-a",
		"20.0000000000", "5.0000000000", "USD", service.AccountFinanceUnitFiatCurrency, "USD",
		nil, collectedAt, []byte(`{"usage":"ok"}`), "checksum", int64(10),
		"10.0000000000", "2.5000000000", "0.2500000000", service.AccountFinanceDerivationApplied, nil,
		int64(9), collectedAt.Add(time.Minute), collectedAt,
	)
}
