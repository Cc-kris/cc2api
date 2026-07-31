package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceProtocolRepositoryDeleteDraftProtectsPublishedProtocol(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectExec("DELETE FROM upstream_finance_protocols").WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	err = NewUpstreamFinanceProtocolRepository(db).DeleteDraft(context.Background(), 9)
	require.ErrorIs(t, err, service.ErrUpstreamFinanceProtocolInvalidState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamFinanceProtocolRepositoryPublishIsTransactional(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE upstream_finance_protocol_versions").WithArgs(int64(12), int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE upstream_finance_protocols").WithArgs(int64(9), int64(12), nil).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = NewUpstreamFinanceProtocolRepository(db).PublishVersion(context.Background(), 9, 12, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
