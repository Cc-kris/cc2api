package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEstimateGrokCountTokens(t *testing.T) {
	for _, body := range []string{
		`{"model":"grok-4","messages":[{"role":"user","content":"hello world"}]}`,
		`{"model":"grok-4","system":[{"type":"text","text":"You are helpful."}],"messages":[{"role":"user","content":[{"type":"text","text":"look up weather"}]}],"tools":[{"name":"weather","input_schema":{"type":"object"}}],"tool_choice":{"type":"auto"}}`,
		`{"model":"grok-4","messages":[]}`,
	} {
		got, err := EstimateGrokCountTokens([]byte(body))
		require.NoError(t, err)
		require.Positive(t, got)
	}
}

func TestEstimateGrokCountTokensRejectsInvalidRequests(t *testing.T) {
	for _, body := range []string{
		`{`,
		`{"messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"grok-4","messages":[{"role":"user","content":{"unexpected":true}}]}`,
	} {
		_, err := EstimateGrokCountTokens([]byte(body))
		require.Error(t, err, "body=%s", body)
	}
}
