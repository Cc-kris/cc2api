package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateGrokVideoTaskBindingPersistsOwner(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	groupID := int64(7)
	mock.ExpectExec("INSERT INTO grok_video_task_bindings").
		WithArgs(int64(51), int64(41), &groupID, "video-request-123", int64(63)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.CreateGrokVideoTaskBinding(context.Background(), 51, 41, &groupID, "video-request-123", 63))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateGrokVideoTaskBindingRejectsOwnershipConflict(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	groupID := int64(7)
	mock.ExpectExec("INSERT INTO grok_video_task_bindings").
		WithArgs(int64(51), int64(41), &groupID, "video-request-123", int64(63)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT account_id, group_id").
		WithArgs(int64(51), int64(41), "video-request-123").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id"}).AddRow(int64(99), groupID))

	err := repo.CreateGrokVideoTaskBinding(context.Background(), 51, 41, &groupID, "video-request-123", 63)
	require.ErrorIs(t, err, service.ErrGrokVideoTaskBindingConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGrokVideoTaskAccountIDScopesNilGroup(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	mock.ExpectQuery("SELECT account_id[[:space:]]+FROM grok_video_task_bindings").
		WithArgs(int64(51), int64(41), "video-request-123").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))

	accountID, err := repo.GetGrokVideoTaskAccountID(context.Background(), 51, 41, nil, "video-request-123")
	require.NoError(t, err)
	require.Equal(t, int64(63), accountID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGrokVideoTaskAccountIDHidesOtherOwner(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	groupID := int64(7)
	mock.ExpectQuery("SELECT account_id[[:space:]]+FROM grok_video_task_bindings").
		WithArgs(int64(51), int64(42), "video-request-123", groupID).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}))

	_, err := repo.GetGrokVideoTaskAccountID(context.Background(), 51, 42, &groupID, "video-request-123")
	require.ErrorIs(t, err, service.ErrUsageLogNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
