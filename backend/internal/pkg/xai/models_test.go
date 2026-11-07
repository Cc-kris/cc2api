package xai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// V02-T22 / V02-AC50：grok-4.7 进模型表和默认映射，bare grok / grok-latest 仍指向 4.6。
func TestDefaultModelsIncludesGrok46And47(t *testing.T) {
	t.Parallel()

	ids := DefaultModelIDs()
	require.Contains(t, ids, "grok-4.7")
	require.Contains(t, ids, "grok-4.6")

	mapping := DefaultModelMapping()
	require.Equal(t, "grok-4.7", mapping["grok-4.7"])
	require.Equal(t, "grok-4.7", mapping["grok-4.7-latest"])
	require.Equal(t, "grok-4.6", mapping["grok-4.6"])
	require.Equal(t, "grok-4.6", mapping["grok-4.6-latest"])
	// 上游 935db6851 没有改默认指向
	require.Equal(t, "grok-4.6", mapping["grok"])
	require.Equal(t, "grok-4.6", mapping["grok-latest"])
}

func TestDefaultModelsAreCopies(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	models[0].ID = "mutated"
	require.NotEqual(t, "mutated", DefaultModels()[0].ID)
}
