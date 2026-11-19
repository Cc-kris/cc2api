package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripOpenAIResponsesInputNamespaces_NoDirectNamespaceReturnsOriginalSlice(t *testing.T) {
	body := []byte(`{"tools":[{"type":"namespace","name":"tools","tools":[]}],"input":[{"type":"message","content":"` + strings.Repeat("x", 1<<20) + `"}]}`)
	got, err := stripOpenAIResponsesInputNamespaces(body, false)
	require.NoError(t, err)
	require.Equal(t, body, got)
	require.Same(t, &body[0], &got[0], "no-op must return the original request bytes")
	// Allocation counts are measured by BenchmarkStripOpenAIResponsesInputNamespaces.
	// AllocsPerRun samples process-wide allocations and is unreliable alongside
	// this package's background workers; keep this unit assertion deterministic.
}

func TestStripOpenAIResponsesInputNamespaces_IgnoresNestedAndStringNamespace(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"tools":[{"type":"namespace","name":"tools"}],"input":[{"type":"message","metadata":{"namespace":"nested"},"content":"literal \\\"namespace\\\" , ] } [ {"}]}`),
		[]byte(`{"tools":[{"type":"namespace","name":"tools"}],"input":[{"type":"message","content":[{"type":"text","text":"namespace ] } , { ["}]}]}`),
	} {
		got, err := stripOpenAIResponsesInputNamespaces(body, false)
		require.NoError(t, err)
		require.Equal(t, body, got)
		require.Same(t, &body[0], &got[0])
	}
}
