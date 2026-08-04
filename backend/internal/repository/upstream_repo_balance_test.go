package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateUpstreamRollsBackWhenFinanceBalanceSnapshotFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &upstreamRepository{db: db}
	currentBalance := 70.0
	input := &service.UpstreamInput{
		BaseURL:             "https://upstream.example",
		Name:                "Upstream",
		RateMultiplier:      1,
		InitialBalance:      82,
		BalanceAlertEnabled: false,
		Notes:               "",
		CurrentBalance:      &currentBalance,
		BalanceDedupeKey:    "request-1",
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE upstreams").
		WithArgs(int64(5), input.BaseURL, service.NormalizeUpstreamBaseURLForRepo(input.BaseURL), input.Name, input.RateMultiplier, input.InitialBalance, input.BalanceAlertEnabled, input.AlertBalance, input.Notes).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM upstream_platform_rates").WithArgs(int64(5)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id,currency,enabled,balance_kind").WithArgs(int64(5), service.FinanceBalanceWalletName).
		WillReturnRows(sqlmock.NewRows([]string{"id", "currency", "enabled", "balance_kind"}).AddRow(int64(8), "USD", true, "wallet_cash"))
	mock.ExpectExec("INSERT INTO upstream_balance_snapshots").
		WithArgs(int64(8), "manual-balance:request-1", currentBalance, "USD").
		WillReturnError(errors.New("snapshot unavailable"))
	mock.ExpectRollback()

	_, err = repo.UpdateUpstream(context.Background(), 5, input)
	require.ErrorContains(t, err, "record finance balance snapshot")
	require.NoError(t, mock.ExpectationsWereMet())
}
