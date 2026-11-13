//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMigrationsSchema is the V04 AC01 real-PostgreSQL schema gate. The
// integration harness applies the embedded migrations to a fresh database.
func TestMigrationsSchema(t *testing.T) {
	ctx := context.Background()
	var dataType, nullable string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
 SELECT data_type, is_nullable FROM information_schema.columns
 WHERE table_schema = 'public' AND table_name = 'scheduler_outbox' AND column_name = 'dedup_key'
 `).Scan(&dataType, &nullable))
	require.Equal(t, "text", dataType)
	require.Equal(t, "YES", nullable)

	var valid, unique bool
	var predicate, definition string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
 SELECT i.indisvalid, i.indisunique, pg_get_expr(i.indpred, i.indrelid), pg_get_indexdef(i.indexrelid)
 FROM pg_index i
 JOIN pg_class idx ON idx.oid = i.indexrelid
 JOIN pg_namespace ns ON ns.oid = idx.relnamespace
 WHERE ns.nspname = 'public' AND idx.relname = $1
 `, schedulerOutboxPendingDedupKeyIndex).Scan(&valid, &unique, &predicate, &definition))
	require.True(t, valid, "246 must not leave an INVALID index")
	require.True(t, unique)
	require.Equal(t, "(dedup_key IS NOT NULL)", predicate)
	require.Contains(t, definition, "ON public.scheduler_outbox USING btree (dedup_key)")

	var recorded int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
 SELECT COUNT(*) FROM schema_migrations WHERE filename IN ($1, $2)
 `, "245_scheduler_outbox_dedup_key.sql", schedulerOutboxPendingDedupKeyMigration).Scan(&recorded))
	require.Equal(t, 2, recorded)
}

// A failed concurrent UNIQUE build leaves a real INVALID index. Exercise the
// shared recovery helper without changing the live scheduler table or ledger;
// the filename dispatch and 246 ledger retry are covered by the sqlmock test.
func TestMigrationsSchedulerOutboxInvalidIndexRecoveryPostgres(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	table := fmt.Sprintf("scheduler_invalid_probe_%d", suffix)
	index := fmt.Sprintf("idx_scheduler_invalid_probe_%d", suffix)
	_, err := integrationDB.ExecContext(ctx, "CREATE TABLE "+table+" (dedup_key text)")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table+" CASCADE")
	})
	_, err = integrationDB.ExecContext(ctx, "INSERT INTO "+table+" (dedup_key) VALUES ('duplicate'), ('duplicate'), (NULL), (NULL)")
	require.NoError(t, err)
	build := "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS " + index + " ON " + table + " (dedup_key) WHERE dedup_key IS NOT NULL"
	_, err = integrationDB.ExecContext(ctx, build)
	require.Error(t, err, "duplicate keys must interrupt the first concurrent build")
	invalid, err := indexIsInvalid(ctx, integrationDB, index)
	require.NoError(t, err)
	require.True(t, invalid)

	require.NoError(t, dropInvalidIndexIfPresent(ctx, integrationDB, index))
	var missing bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT to_regclass($1) IS NULL", index).Scan(&missing))
	require.True(t, missing, "recovery must drop, not silently retain, the INVALID index")
	_, err = integrationDB.ExecContext(ctx, "DELETE FROM "+table+" WHERE ctid IN (SELECT ctid FROM "+table+" WHERE dedup_key='duplicate' LIMIT 1)")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, build)
	require.NoError(t, err)
	var valid bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT indisvalid FROM pg_index WHERE indexrelid=to_regclass($1)", index).Scan(&valid))
	require.True(t, valid)
	// A retry of recovery against a valid index must preserve it.
	require.NoError(t, dropInvalidIndexIfPresent(ctx, integrationDB, index))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT indisvalid FROM pg_index WHERE indexrelid=to_regclass($1)", index).Scan(&valid))
	require.True(t, valid)
}
