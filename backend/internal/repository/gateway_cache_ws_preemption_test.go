package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheOpenAIWSSessionWindowAtomicOwner(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()
	old, err := cache.ClaimOpenAIResponsesSessionWindow(ctx, 1, "scope", []byte("owner-a"), time.Minute)
	require.NoError(t, err)
	require.Empty(t, old)
	old, err = cache.ClaimOpenAIResponsesSessionWindow(ctx, 1, "scope", []byte("owner-b"), time.Minute)
	require.NoError(t, err)
	require.Equal(t, "owner-a", string(old))
	ok, err := cache.CompareAndRefreshOpenAIResponsesSessionWindow(ctx, 1, "scope", []byte("owner-a"), time.Hour)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = cache.CompareAndDeleteOpenAIResponsesSessionWindow(ctx, 1, "scope", []byte("owner-a"))
	require.NoError(t, err)
	require.False(t, ok)
	value, err := mr.Get(buildOpenAIResponsesSessionWindowKey(1, "scope"))
	require.NoError(t, err)
	require.Equal(t, "owner-b", value)
	ok, err = cache.CompareAndRefreshOpenAIResponsesSessionWindow(ctx, 1, "scope", []byte("owner-b"), 2*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	mr.FastForward(90 * time.Second)
	require.True(t, mr.Exists(buildOpenAIResponsesSessionWindowKey(1, "scope")))
	// Distinct groups do not interfere with this owner.
	_, err = cache.ClaimOpenAIResponsesSessionWindow(ctx, 2, "scope", []byte("owner-c"), time.Minute)
	require.NoError(t, err)
	ok, err = cache.CompareAndDeleteOpenAIResponsesSessionWindow(ctx, 1, "scope", []byte("owner-b"))
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, mr.Exists(buildOpenAIResponsesSessionWindowKey(2, "scope")))
	require.False(t, mr.Exists(buildOpenAIResponsesSessionWindowKey(1, "scope")))
}
