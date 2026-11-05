package antigravity

import "testing"

func TestDefaultModels_ContainsNewAndLegacyImageModels(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	byID := make(map[string]ClaudeModel, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	requiredIDs := []string{
		"claude-fable-5-1",
		"claude-fable-5",
		"claude-opus-4-8",
		"claude-opus-4-6-thinking",
		"claude-opus-5-5-high",
		"claude-opus-5-5-low",
		"claude-opus-5-5-medium",
		"claude-sonnet-5-5-high",
		"claude-sonnet-5-5-low",
		"claude-sonnet-5-5-medium",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3-pro-image", // legacy compatibility
	}

	for _, id := range requiredIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected model %q to be exposed in DefaultModels", id)
		}
	}
}
