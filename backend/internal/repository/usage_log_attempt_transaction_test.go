//go:build unit

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogCreateRollsBackWhenUpstreamAttemptInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)
	createdAt := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	log := &service.UsageLog{
		UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "req-tx", Model: "gpt-5.5",
		InputTokens: 10, CreatedAt: createdAt,
		UpstreamAttempts: []service.UsageUpstreamAttempt{{
			RequestID: "req-tx", AttemptNo: 1, AccountID: 3, UpstreamModel: "gpt-5.5",
			InputTokens: 10, Billable: true, CreatedAt: createdAt,
		}},
	}

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_logs").WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(99), createdAt),
	)
	mock.ExpectExec("INSERT INTO usage_upstream_attempts").WillReturnError(errors.New("attempt insert failed"))
	mock.ExpectRollback()

	inserted, err := repo.Create(t.Context(), log)
	require.ErrorContains(t, err, "attempt insert failed")
	require.False(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}
