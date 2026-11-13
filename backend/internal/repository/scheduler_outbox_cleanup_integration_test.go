//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerOutboxCleanupGraceAndExclusiveLeasePostgres(t *testing.T) {
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "TRUNCATE scheduler_outbox RESTART IDENTITY")
	})
	_, err = integrationDB.ExecContext(ctx, `
 INSERT INTO scheduler_outbox (event_type, created_at)
 SELECT 'full_rebuild', CASE WHEN n <= 9900 THEN NOW() - INTERVAL '20 seconds' ELSE NOW() END
 FROM generate_series(1, 10000) n`)
	require.NoError(t, err)
	repo := NewSchedulerOutboxRepository(integrationDB)
	lease, acquired, err := repo.TryAcquireCleanupLock(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, lease)
	defer lease.Release()
	competitor, acquired, err := repo.TryAcquireCleanupLock(ctx)
	require.NoError(t, err)
	require.False(t, acquired, "another connection must not concurrently clean")
	require.Nil(t, competitor)
	deleted, err := repo.DeleteConsumedUpTo(ctx, 10000, 10000)
	require.NoError(t, err)
	require.EqualValues(t, 9900, deleted)
	var remaining int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox").Scan(&remaining))
	require.Equal(t, 100, remaining, "the ten-second commit-order grace retains fresh rows")
	lease.Release()
	next, acquired, err := repo.TryAcquireCleanupLock(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	next.Release()
}

func TestSchedulerOutboxPendingTimestampIgnoresConsumedRowsPostgres(t *testing.T) {
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "TRUNCATE scheduler_outbox RESTART IDENTITY")
	})
	_, err = integrationDB.ExecContext(ctx, `
 INSERT INTO scheduler_outbox (event_type, created_at) VALUES
 ('full_rebuild', NOW() - INTERVAL '1 hour'), ('full_rebuild', NOW())`)
	require.NoError(t, err)
	pendingAt, found, err := NewSchedulerOutboxRepository(integrationDB).FirstCreatedAtAfter(ctx, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.WithinDuration(t, time.Now(), pendingAt, 5*time.Second)
	_, found, err = NewSchedulerOutboxRepository(integrationDB).FirstCreatedAtAfter(ctx, 2)
	require.NoError(t, err)
	require.False(t, found)
}
