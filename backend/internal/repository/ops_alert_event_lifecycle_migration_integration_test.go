//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration144BackfillsLegacyOpsAlertEventsAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx, `
CREATE TEMP TABLE ops_alert_events (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT,
    severity VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'firing',
    title VARCHAR(200),
    description TEXT,
    metric_value DOUBLE PRECISION,
    threshold_value DOUBLE PRECISION,
    dimensions JSONB,
    fired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    email_sent BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`)
	require.NoError(t, err)

	firedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	resolvedAt := time.Date(2026, 6, 7, 10, 5, 0, 0, time.UTC)

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_alert_events (id, rule_id, severity, status, title, fired_at, resolved_at)
VALUES
    (1, 11, 'P1', 'firing', 'still firing', $1, NULL),
    (2, 12, 'P1', 'resolved', 'auto recovered', $1, $2),
    (3, 13, 'P0', 'manual_resolved', 'manually closed', $1, $2)
`, firedAt, resolvedAt)
	require.NoError(t, err)

	content, err := migrations.FS.ReadFile("144_ops_alert_event_lifecycle_fields.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "first migration execution should succeed")
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "second migration execution should be idempotent")

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_alert_events").Scan(&rowCount))
	require.Equal(t, 3, rowCount, "legacy rows must be preserved")

	rows, err := tx.QueryContext(ctx, `
SELECT id, lifecycle_status, merged_count, last_seen_at, recovered_at, closed_at
FROM ops_alert_events
ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()

	type migratedRow struct {
		id              int64
		lifecycleStatus string
		mergedCount     int
		lastSeenAt      time.Time
		recoveredAt     *time.Time
		closedAt        *time.Time
	}
	var got []migratedRow
	for rows.Next() {
		var row migratedRow
		require.NoError(t, rows.Scan(&row.id, &row.lifecycleStatus, &row.mergedCount, &row.lastSeenAt, &row.recoveredAt, &row.closedAt))
		got = append(got, row)
	}
	require.NoError(t, rows.Err())
	require.Len(t, got, 3)

	require.Equal(t, "firing", got[0].lifecycleStatus)
	require.Equal(t, 0, got[0].mergedCount)
	require.True(t, got[0].lastSeenAt.Equal(firedAt))
	require.Nil(t, got[0].recoveredAt)
	require.Nil(t, got[0].closedAt)

	require.Equal(t, "recovered", got[1].lifecycleStatus)
	require.Equal(t, 0, got[1].mergedCount)
	require.True(t, got[1].lastSeenAt.Equal(firedAt))
	require.NotNil(t, got[1].recoveredAt)
	require.True(t, got[1].recoveredAt.Equal(resolvedAt))
	require.Nil(t, got[1].closedAt)

	require.Equal(t, "closed", got[2].lifecycleStatus)
	require.Equal(t, 0, got[2].mergedCount)
	require.True(t, got[2].lastSeenAt.Equal(firedAt))
	require.NotNil(t, got[2].recoveredAt)
	require.True(t, got[2].recoveredAt.Equal(resolvedAt))
	require.NotNil(t, got[2].closedAt)
	require.True(t, got[2].closedAt.Equal(resolvedAt))

	_, err = tx.ExecContext(ctx, "INSERT INTO ops_alert_events (id, rule_id, severity, status, title) VALUES (4, 14, 'P2', 'firing', 'new default row')")
	require.NoError(t, err)

	var lifecycleStatus string
	var mergedCount int
	var lastSeenAt time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT lifecycle_status, merged_count, last_seen_at
FROM ops_alert_events
WHERE id = 4`).Scan(&lifecycleStatus, &mergedCount, &lastSeenAt))
	require.Equal(t, "firing", lifecycleStatus)
	require.Equal(t, 0, mergedCount)
	require.False(t, lastSeenAt.IsZero())
}
