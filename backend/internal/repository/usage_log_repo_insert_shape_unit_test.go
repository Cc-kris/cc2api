package repository

// 移植自上游 de27905e8 usage_log_repo_insert_shape_unit_test.go，按 CCAI 的单文件 usage_log_repo.go 适配；
// 另加 V03-R11 五个审计字段的参数位置断言。

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var (
	usageLogStaticInsertShapeRe = regexp.MustCompile(`(?s)INSERT INTO usage_logs \((.*?)\) VALUES \((.*?)\)`)
	usageLogPlaceholderRe       = regexp.MustCompile(`\$(\d+)`)
)

func newUsageLogSQLCapturingMock(t *testing.T, captured *[]string) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	matcher := sqlmock.QueryMatcherFunc(func(_, actualSQL string) error {
		*captured = append(*captured, actualSQL)
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

// requireStaticInsertMatchesArgTypes 断言手写 INSERT 的列数与占位符数都等于 usageLogInsertArgTypes，
// 且占位符恰为 $1..$N 各出现一次。合并其他分支加列时漏补占位符会在这里失败。
func requireStaticInsertMatchesArgTypes(t *testing.T, query string) {
	t.Helper()
	m := usageLogStaticInsertShapeRe.FindStringSubmatch(query)
	require.Len(t, m, 3, "unrecognised INSERT shape:\n%s", query)

	want := len(usageLogInsertArgTypes)
	columns := 0
	for _, col := range strings.Split(m[1], ",") {
		if strings.TrimSpace(col) != "" {
			columns++
		}
	}
	require.Equal(t, want, columns, "INSERT column list must match usageLogInsertArgTypes")

	seen := make(map[int]struct{}, want)
	for _, ph := range usageLogPlaceholderRe.FindAllStringSubmatch(m[2], -1) {
		n, err := strconv.Atoi(ph[1])
		require.NoError(t, err)
		_, dup := seen[n]
		require.False(t, dup, "duplicate placeholder $%d", n)
		seen[n] = struct{}{}
	}
	require.Len(t, seen, want, "VALUES placeholder count must match usageLogInsertArgTypes")
	for i := 1; i <= want; i++ {
		_, ok := seen[i]
		require.True(t, ok, "missing placeholder $%d", i)
	}
}

func auditTestUsageLog() *service.UsageLog {
	responseModel := "claude-sonnet-4-20250514"
	mismatch := true
	sessionID := "sess-abc"
	effort := "max"
	upstreamRequestID := "req_abc"
	return &service.UsageLog{
		UserID:    1,
		APIKeyID:  2,
		AccountID: 3,
		RequestID: "client:insert-shape",
		Model:     "claude-3",
		UsageAuditFields: service.UsageAuditFields{
			UpstreamResponseModel:    &responseModel,
			UpstreamModelMismatch:    &mismatch,
			SessionID:                &sessionID,
			RequestedReasoningEffort: &effort,
			UpstreamRequestID:        &upstreamRequestID,
		},
		InputTokens: 10,
		CreatedAt:   time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
	}
}

func TestUsageLogStaticInsertShape_PlaceholdersMatchArgTypes(t *testing.T) {
	log := auditTestUsageLog()
	prepared := prepareUsageLogInsert(log)
	args := anySliceToDriverValues(prepared.args)

	t.Run("createSingle", func(t *testing.T) {
		var captured []string
		db, mock := newUsageLogSQLCapturingMock(t, &captured)
		repo := &usageLogRepository{sql: db}

		mock.ExpectQuery("INSERT INTO usage_logs").
			WithArgs(args...).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), log.CreatedAt))

		inserted, err := repo.createSingle(context.Background(), db, log)
		require.NoError(t, err)
		require.True(t, inserted)
		require.NoError(t, mock.ExpectationsWereMet())
		require.Len(t, captured, 1)
		requireStaticInsertMatchesArgTypes(t, captured[0])
	})

	t.Run("execUsageLogInsertNoResult", func(t *testing.T) {
		var captured []string
		db, mock := newUsageLogSQLCapturingMock(t, &captured)

		mock.ExpectExec("INSERT INTO usage_logs").
			WithArgs(args...).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, execUsageLogInsertNoResult(context.Background(), db, prepared))
		require.NoError(t, mock.ExpectationsWereMet())
		require.Len(t, captured, 1)
		requireStaticInsertMatchesArgTypes(t, captured[0])
	})
}

// TestUsageLogInsertQueries_ListAuditColumns 断言所有 INSERT/CTE 列清单（单条、批量、best-effort）都带上 5 个审计列。
func TestUsageLogInsertQueries_ListAuditColumns(t *testing.T) {
	prepared := prepareUsageLogInsert(auditTestUsageLog())
	batchQuery, batchArgs := buildUsageLogBatchInsertQuery([]string{"k"}, map[string]usageLogInsertPrepared{"k": prepared})
	bestEffortQuery, bestEffortArgs := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
	require.Len(t, batchArgs, len(usageLogInsertArgTypes)+1)
	require.Len(t, bestEffortArgs, len(usageLogInsertArgTypes))

	for _, col := range []string{"upstream_response_model", "upstream_model_mismatch", "session_id", "requested_reasoning_effort", "upstream_request_id"} {
		// 批量：input CTE 列、INSERT 列、SELECT 列各一次；best-effort：input CTE 列、INSERT 列、SELECT 列各一次。
		require.Equal(t, 3, strings.Count(batchQuery, col+"\n")+strings.Count(batchQuery, col+","), "batch query column %s", col)
		require.Equal(t, 3, strings.Count(bestEffortQuery, col+"\n")+strings.Count(bestEffortQuery, col+","), "best-effort query column %s", col)
		require.Contains(t, usageLogSelectColumns, col)
	}
}

// TestPrepareUsageLogInsert_AuditArgWiring 钉住 5 个审计参数在末尾的位置和类型；缺失时落 NULL 而不是空串或 false。
func TestPrepareUsageLogInsert_AuditArgWiring(t *testing.T) {
	prepared := prepareUsageLogInsert(auditTestUsageLog())
	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	base := len(prepared.args) - 5

	wantText := map[int]string{0: "claude-sonnet-4-20250514", 2: "sess-abc", 3: "max", 4: "req_abc"}
	for offset, want := range wantText {
		arg, ok := prepared.args[base+offset].(sql.NullString)
		require.True(t, ok, "offset %d should be sql.NullString, got %T", offset, prepared.args[base+offset])
		require.True(t, arg.Valid)
		require.Equal(t, want, arg.String)
		require.Equal(t, "text", usageLogInsertArgTypes[base+offset])
	}
	mismatch, ok := prepared.args[base+1].(sql.NullBool)
	require.True(t, ok, "upstream_model_mismatch should be sql.NullBool, got %T", prepared.args[base+1])
	require.True(t, mismatch.Valid)
	require.True(t, mismatch.Bool)
	require.Equal(t, "boolean", usageLogInsertArgTypes[base+1])

	absent := prepareUsageLogInsert(&service.UsageLog{UserID: 1, APIKeyID: 2, RequestID: "client:absent", Model: "gpt-5", CreatedAt: time.Now().UTC()})
	for offset := 0; offset < 5; offset++ {
		switch v := absent.args[base+offset].(type) {
		case sql.NullString:
			require.False(t, v.Valid, "offset %d must be NULL", offset)
		case sql.NullBool:
			require.False(t, v.Valid, "offset %d must be NULL (three-state mismatch)", offset)
		default:
			t.Fatalf("unexpected arg type %T at offset %d", v, offset)
		}
	}
}
