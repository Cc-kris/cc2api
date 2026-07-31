package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamWalletRepositorySoftDeleteIsAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &upstreamWalletRepository{db: db}
	deletedAt := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE upstream_wallets").WithArgs(int64(9), deletedAt).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE upstream_wallet_accounts").WithArgs(int64(9), deletedAt).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM upstream_wallet_accounts").WithArgs(int64(9), deletedAt).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.SoftDeleteWallet(context.Background(), 9, deletedAt))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamWalletRepositoryAssignmentRejectsTimeBeforeConfirmedFinance(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &upstreamWalletRepository{db: db}
	effectiveAt := time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC)
	confirmedAt := effectiveAt.Add(time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.normalized_base_url").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"normalized_base_url"}).AddRow("https://upstream.test"))
	mock.ExpectQuery("SELECT id,normalized_account_base_url").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "normalized_account_base_url"}).AddRow(int64(3), "https://upstream.test"))
	mock.ExpectQuery("SELECT MAX\\(ul.created_at\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(confirmedAt))
	mock.ExpectRollback()
	err = repo.AssignWalletAccounts(context.Background(), 9, service.UpstreamWalletAssignmentInput{
		AccountIDs: []int64{3}, EffectiveAt: effectiveAt, Reason: "historical correction",
	})
	require.ErrorIs(t, err, service.ErrUpstreamWalletAssignmentTooEarly)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamWalletRepositoryAssignmentRollsBackOnFutureConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &upstreamWalletRepository{db: db}
	effectiveAt := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.normalized_base_url").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"normalized_base_url"}).AddRow("https://upstream.test"))
	mock.ExpectQuery("SELECT id,normalized_account_base_url").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "normalized_account_base_url"}).AddRow(int64(3), "https://upstream.test"))
	mock.ExpectQuery("SELECT MAX\\(ul.created_at\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)").WithArgs(sqlmock.AnyArg(), effectiveAt).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	err = repo.AssignWalletAccounts(context.Background(), 9, service.UpstreamWalletAssignmentInput{
		AccountIDs: []int64{3}, EffectiveAt: effectiveAt, Reason: "move wallet owner",
	})
	require.True(t, errors.Is(err, service.ErrUpstreamWalletAssignmentConflict))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamWalletRepositoryAssignmentRejectsCrossUpstreamAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &upstreamWalletRepository{db: db}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.normalized_base_url").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"normalized_base_url"}).AddRow("https://wallet-upstream.test"))
	mock.ExpectQuery("SELECT id,normalized_account_base_url").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "normalized_account_base_url"}).AddRow(int64(3), "https://other-upstream.test"))
	mock.ExpectRollback()
	err = repo.AssignWalletAccounts(context.Background(), 9, service.UpstreamWalletAssignmentInput{
		AccountIDs: []int64{3}, EffectiveAt: time.Now().UTC(), Reason: "cross upstream assignment",
	})
	require.ErrorIs(t, err, service.ErrUpstreamWalletAccountMismatch)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamWalletRepositoryListsOnlyActiveEffectiveAccountBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &upstreamWalletRepository{db: db}
	effectiveAt := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT uwa.account_id.*w.deleted_at IS NULL AND w.enabled=TRUE.*a.deleted_at IS NULL AND a.status='active'.*uwa.effective_from <= \$2.*uwa.effective_to IS NULL OR uwa.effective_to > \$2`).
		WithArgs(int64(9), effectiveAt).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(3)).AddRow(int64(7)))

	accountIDs, err := repo.ListActiveWalletAccountIDs(context.Background(), 9, effectiveAt)
	require.NoError(t, err)
	require.Equal(t, []int64{3, 7}, accountIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}
