package service

import (
	"context"
	"strings"
	"testing"
)

func TestCleanContentNewsSummary(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "HN metadata only",
			raw:  "Article URL: https://example.com/story Comments URL: https://news.ycombinator.com/item?id=1 Points: 18 # Comments: 4",
			want: "",
		},
		{
			name: "editorial text before metadata",
			raw:  "A concise editorial summary. Article URL: https://example.com/story Comments URL: https://example.com/comments",
			want: "A concise editorial summary.",
		},
		{
			name: "ordinary article URL mention remains",
			raw:  "The Article URL format changed in this release.",
			want: "The Article URL format changed in this release.",
		},
		{
			name: "whitespace and length normalized",
			raw:  "  A   summary\nwith spacing  ",
			want: "A summary with spacing",
		},
		{
			name: "limit after cleaning",
			raw:  strings.Repeat("摘", 230) + " Article URL: https://example.com",
			want: strings.Repeat("摘", 220),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanContentNewsSummary(tt.raw); got != tt.want {
				t.Fatalf("CleanContentNewsSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContentNewsAdminReadsCleanStoredSummaryWithoutBackfill(t *testing.T) {
	raw := "Article URL: https://example.com Comments URL: https://example.com/comments"
	repo := &contentNewsAdminRepoStub{item: &ContentNewsItem{ID: 9, Summary: &raw}}
	svc := NewContentNewsAdminService(repo)
	item, err := svc.GetNewsItem(context.Background(), 9, false)
	if err != nil {
		t.Fatal(err)
	}
	if item.Summary == nil || *item.Summary != "" {
		t.Fatalf("response summary = %v, want empty", item.Summary)
	}
	body := "Full article text used when feed metadata has no editorial summary."
	repo.item.BodyText = &body
	item, err = svc.GetNewsItem(context.Background(), 9, false)
	if err != nil {
		t.Fatal(err)
	}
	if item.Summary == nil || *item.Summary != body {
		t.Fatalf("fallback summary = %v, want body excerpt", item.Summary)
	}
	if raw != "Article URL: https://example.com Comments URL: https://example.com/comments" {
		t.Fatal("read cleaning must not mutate stored source data")
	}
}

func TestContentNewsWorkerSummaryUsesSharedCleaning(t *testing.T) {
	raw := "Editorial summary Article URL: https://example.com"
	if got := contentNewsSummary(raw); got != "Editorial summary" {
		t.Fatalf("contentNewsSummary() = %q", got)
	}
}
