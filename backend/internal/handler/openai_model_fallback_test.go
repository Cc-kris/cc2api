package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyOpenAIClientDefaultModelFallback(t *testing.T) {
	t.Run("missing model gets fallback", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
		updated, model, applied, err := applyOpenAIClientDefaultModelFallback(body, "", "gpt-4o")
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, "gpt-4o", model)
		require.Equal(t, "gpt-4o", gjson.GetBytes(updated, "model").String())
	})

	t.Run("codex-current gets fallback", func(t *testing.T) {
		body := []byte(`{"model":"codex-current","messages":[{"role":"user","content":"hello"}]}`)
		updated, model, applied, err := applyOpenAIClientDefaultModelFallback(body, "codex-current", "gpt-4o")
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, "gpt-4o", model)
		require.Equal(t, "gpt-4o", gjson.GetBytes(updated, "model").String())
	})

	t.Run("explicit model is preserved", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4","messages":[]}`)
		updated, model, applied, err := applyOpenAIClientDefaultModelFallback(body, "gpt-5.4", "gpt-4o")
		require.NoError(t, err)
		require.False(t, applied)
		require.Equal(t, "gpt-5.4", model)
		require.JSONEq(t, string(body), string(updated))
	})
}
