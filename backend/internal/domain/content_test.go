package domain

import "testing"

func TestContentEnumValidators(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid func(string) bool
	}{
		{"content status", ContentStatusPublished, IsContentStatus},
		{"source type", ContentNewsSourceRSS, IsContentNewsSourceType},
		{"news publish mode", ContentNewsPublishAuto, IsContentNewsPublishMode},
		{"automation mode", ContentAutomationAutoPublish, IsContentAutomationPublishMode},
		{"fetch status", ContentFetchSucceeded, IsContentFetchStatus},
		{"fetch trigger", ContentFetchTriggerCron, IsContentFetchTrigger},
		{"translation status", ContentTranslationReview, IsContentTranslationStatus},
		{"home mode", HomeModeStructured, IsHomeMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.valid(tt.value) {
				t.Fatalf("expected %q to be valid", tt.value)
			}
			if tt.valid("invalid") {
				t.Fatal("expected invalid value to be rejected")
			}
		})
	}
}
