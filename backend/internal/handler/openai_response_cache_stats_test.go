package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitLocalResponseCacheReasonCounters(t *testing.T) {
	reasons, total := splitLocalResponseCacheReasonCounters(map[string]int64{
		"lookup_hit":                         3,
		"lookup_bypass:tools_or_functions":   7,
		"lookup_bypass:temperature_too_high": 2,
		"store_skip:stream_incomplete":       4,
	}, "lookup_bypass:")

	require.Equal(t, int64(9), total)
	require.Equal(t, map[string]int64{
		"tools_or_functions":   7,
		"temperature_too_high": 2,
	}, reasons)
}
