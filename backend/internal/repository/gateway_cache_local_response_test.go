package repository

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheLocalResponseSetGet(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &gatewayCache{rdb: rdb}
	ctx := context.Background()

	entry := &service.LocalResponseCacheEntry{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        []byte(`{"id":"resp_1"}`),
		Headers:     map[string]string{"Content-Type": "application/json"},
		CreatedAt:   time.Now().UTC(),
	}

	require.NoError(t, cache.SetLocalResponse(ctx, "hash-a", entry, time.Minute))

	got, err := cache.GetLocalResponse(ctx, "hash-a")
	require.NoError(t, err)
	require.Equal(t, entry.StatusCode, got.StatusCode)
	require.Equal(t, entry.ContentType, got.ContentType)
	require.Equal(t, entry.Body, got.Body)
	require.Equal(t, entry.Headers, got.Headers)
	require.True(t, mr.Exists(buildLocalResponseCacheKey("hash-a")))
}
