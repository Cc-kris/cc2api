//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentWritingTaskRepositoryClaimAndVersionCAS(t *testing.T) {
	ctx := context.Background()
	store := NewContentWritingTaskRepository(integrationEntClient)
	id := fmt.Sprintf("integration-writing-%d", time.Now().UnixNano())
	created, err := store.Create(ctx, &service.ContentWritingTask{
		ID: id, Kind: "writing", Topic: "integration", Model: "ccai-test", Status: service.ContentWritingQueued,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), created.Version)

	queue, ok := store.(service.ContentWritingTaskQueue)
	require.True(t, ok)
	claimed, err := queue.ClaimNext(ctx)
	require.NoError(t, err)
	require.Equal(t, id, claimed.ID)
	require.Equal(t, service.ContentWritingRunning, claimed.Status)
	require.Equal(t, int64(2), claimed.Version)

	stale, err := store.Get(ctx, id)
	require.NoError(t, err)
	stale.Status = service.ContentWritingSucceeded
	result := "draft"
	stale.Result = &result
	require.NoError(t, store.Update(ctx, stale))
	require.Equal(t, int64(3), stale.Version)

	stale.Status = service.ContentWritingFailed
	stale.Version = 2
	require.ErrorIs(t, store.Update(ctx, stale), service.ErrContentArticleConflict)
}
