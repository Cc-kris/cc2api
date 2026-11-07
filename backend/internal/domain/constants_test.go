package domain

import "testing"

func TestDefaultAntigravityModelMapping_ImageCompatibilityAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"gemini-2.5-flash-image":         "gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview": "gemini-2.5-flash-image",
		"gemini-3.1-flash-image":         "gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview": "gemini-3.1-flash-image",
		"gemini-3-pro-image":             "gemini-3.1-flash-image",
		"gemini-3-pro-image-preview":     "gemini-3.1-flash-image",
	}

	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}

func TestDefaultAntigravityModelMapping_ContainsNewClaudeModels(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"claude-fable-5-1":         "claude-fable-5-1",
		"claude-fable-5":           "claude-fable-5",
		"claude-opus-4-8":          "claude-opus-4-8",
		"claude-opus-5-5":          "claude-opus-5-5-high",
		"claude-opus-5-5-high":     "claude-opus-5-5-high",
		"claude-opus-5-5-low":      "claude-opus-5-5-low",
		"claude-opus-5-5-medium":   "claude-opus-5-5-medium",
		"claude-sonnet-5-5":        "claude-sonnet-5-5-high",
		"claude-sonnet-5-5-high":   "claude-sonnet-5-5-high",
		"claude-sonnet-5-5-low":    "claude-sonnet-5-5-low",
		"claude-sonnet-5-5-medium": "claude-sonnet-5-5-medium",
	}
	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
	// 上游 Antigravity 没有 opus-5、sonnet-5、haiku-5-5。
	for _, model := range []string{"claude-opus-5", "claude-sonnet-5", "claude-haiku-5-5"} {
		if got, ok := DefaultAntigravityModelMapping[model]; ok {
			t.Fatalf("expected no Antigravity mapping for %q, got %q", model, got)
		}
	}
}

func TestDefaultBedrockModelMapping_ContainsNewClaudeModels(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"claude-fable-5-1":  "anthropic.claude-fable-5-1",
		"claude-fable-5":    "anthropic.claude-fable-5",
		"claude-opus-5":     "us.anthropic.claude-opus-5-v1",
		"claude-opus-4-8":   "us.anthropic.claude-opus-4-8-v1",
		"claude-sonnet-5":   "us.anthropic.claude-sonnet-5-v1",
		"claude-sonnet-5-5": "global.anthropic.claude-sonnet-5-5",
	}
	for from, want := range cases {
		got, ok := DefaultBedrockModelMapping[from]
		if !ok {
			t.Fatalf("expected Bedrock mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected Bedrock mapping for %q: got %q want %q", from, got, want)
		}
	}
	// 上游没有 opus-5-5、haiku-5-5 的 Bedrock ID（V02-T19：不加）。
	for _, model := range []string{"claude-opus-5-5", "claude-haiku-5-5"} {
		if got, ok := DefaultBedrockModelMapping[model]; ok {
			t.Fatalf("expected no Bedrock mapping for %q, got %q", model, got)
		}
	}
}

func TestDefaultAntigravityModelMapping_NewGeminiModels(t *testing.T) {
	t.Parallel()

	for _, base := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
		for _, tier := range []string{"", "-high", "-low", "-medium", "-tiered"} {
			model := base + tier
			if got := DefaultAntigravityModelMapping[model]; got != model {
				t.Fatalf("expected %s to map to itself, got %q", model, got)
			}
		}
	}
	cases := map[string]string{
		"gemini-4-argon":        "gemini-4-argon-high",
		"gemini-4-argon-high":   "gemini-4-argon-high",
		"gemini-4-argon-low":    "gemini-4-argon-low",
		"gemini-4-argon-medium": "gemini-4-argon-medium",
	}
	for from, want := range cases {
		if got := DefaultAntigravityModelMapping[from]; got != want {
			t.Fatalf("expected %s to map to %q, got %q", from, want, got)
		}
	}
}
