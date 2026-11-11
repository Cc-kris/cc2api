//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// mustAs asserts that v has dynamic type T and returns it, failing the test otherwise.
func mustAs[T any](t testing.TB, v any) T {
	t.Helper()
	got, ok := v.(T)
	require.Truef(t, ok, "unexpected type %T, want %T", v, got)
	return got
}

// testPtrFloat64 returns a pointer to the given float64 value.
func testPtrFloat64(v float64) *float64 { return &v }

// testPtrInt returns a pointer to the given int value.
func testPtrInt(v int) *int { return &v }

// testPtrString returns a pointer to the given string value.
func testPtrString(v string) *string { return &v }

// testPtrBool returns a pointer to the given bool value.
func testPtrBool(v bool) *bool { return &v } //nolint:unused // test helper kept for the unit-tagged suite; not referenced by the current tests
