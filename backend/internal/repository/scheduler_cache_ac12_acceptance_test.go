//go:build unit

package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// AC12 explicitly requires 100 stable reads, including independent top-level
// objects. This is a command-instrumented cache contract test, not HTTP E2E.
func TestSchedulerCacheAC12HundredStableReads(t *testing.T) {
	cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(12100)
	memoPublish(t, cache, bucket, memoTestAccount(1, "a"), memoTestAccount(2, "b"))
	first := memoRead(t, cache, bucket)
	previous := first
	for i := 1; i < 100; i++ {
		recorder.reset()
		current := memoRead(t, cache, bucket)
		require.Equal(t, first, current)
		require.Zero(t, recorder.countPrefix("zrange "))
		require.Zero(t, recorder.countPrefix("mget "+schedulerAccountMetaPrefix))
		require.Len(t, recorder.recorded(), 1, "freshness still checked on every warm read")
		for j := range current {
			require.NotSame(t, first[j], current[j])
			require.NotSame(t, previous[j], current[j])
		}
		previous = current
	}
}
