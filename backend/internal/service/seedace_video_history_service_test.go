package service

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func newSeedaceVideoHistoryTestService(t *testing.T) (*SeedaceVideoHistoryService, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))
	cleanup := func() { _ = client.Close(); _ = db.Close() }
	return NewSeedaceVideoHistoryService(client), mock, cleanup
}

func TestSeedaceVideoHistoryServiceUpsertStoresMessages(t *testing.T) {
	svc, mock, cleanup := newSeedaceVideoHistoryTestService(t)
	defer cleanup()
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	messages := []SeedaceVideoHistoryMessage{{ID: "m1", Role: "user", Content: "生成视频"}, {ID: "m2", Role: "assistant", Content: "已生成视频", Status: "completed", TaskID: "task-1"}}
	rawMessages, err := json.Marshal(messages)
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO seedace_video_session_histories (user_id, session_id, summary, generation_count, messages, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, NOW())
		ON CONFLICT (user_id, session_id)
		DO UPDATE SET
			summary = EXCLUDED.summary,
			generation_count = EXCLUDED.generation_count,
			messages = EXCLUDED.messages,
			updated_at = NOW()
		RETURNING session_id, summary, generation_count, updated_at, messages
	`)).WithArgs(int64(7), "session-1", "生成视频摘要", 1, string(rawMessages)).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "summary", "generation_count", "updated_at", "messages"}).AddRow("session-1", "生成视频摘要", 1, now, rawMessages))

	record, err := svc.Upsert(context.Background(), UpsertSeedaceVideoHistoryInput{UserID: 7, SessionID: "session-1", Summary: "生成视频摘要", GenerationCount: 1, Messages: messages})

	require.NoError(t, err)
	require.Equal(t, "session-1", record.SessionID)
	require.Equal(t, "task-1", record.Messages[1].TaskID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSeedaceVideoHistoryServiceListReturnsLatestUserRecords(t *testing.T) {
	svc, mock, cleanup := newSeedaceVideoHistoryTestService(t)
	defer cleanup()
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	rawMessages := []byte(`[{"id":"m1","role":"user","content":"生成视频"}]`)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT session_id, summary, generation_count, updated_at, messages
		FROM seedace_video_session_histories
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2
	`)).WithArgs(int64(7), 20).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "summary", "generation_count", "updated_at", "messages"}).AddRow("session-1", "生成视频摘要", 1, now, rawMessages))

	records, err := svc.List(context.Background(), 7, 0)

	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "session-1", records[0].SessionID)
	require.Equal(t, "生成视频", records[0].Messages[0].Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSeedaceVideoHistoryServiceRejectsEmptyMessages(t *testing.T) {
	svc, _, cleanup := newSeedaceVideoHistoryTestService(t)
	defer cleanup()

	_, err := svc.Upsert(context.Background(), UpsertSeedaceVideoHistoryInput{UserID: 7, SessionID: "session-1", Summary: "摘要", GenerationCount: 1})

	require.Error(t, err)
	require.Contains(t, err.Error(), "messages cannot be empty")
}
