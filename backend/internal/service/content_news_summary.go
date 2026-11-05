package service

import (
	"regexp"
	"strings"
)

var contentNewsSummaryMetadata = regexp.MustCompile(`(?i)(?:Article|Comments|Discussion|Original)\s+URL\s*:|Points\s*:|#\s*Comments\s*:|Submitted\s+by\s*:`)

// CleanContentNewsSummary removes feed metadata that is not part of the
// editorial summary. It is applied at ingestion and public/admin read
// boundaries so existing rows are cleaned without a destructive backfill.
func CleanContentNewsSummary(raw string) string {
	text := strings.Join(strings.Fields(raw), " ")
	if text == "" {
		return ""
	}
	if cut := contentNewsSummaryMetadata.FindStringIndex(text); cut != nil {
		text = strings.TrimRight(text[:cut[0]], " \t·-–—:;,、，。")
	}
	value := []rune(strings.TrimSpace(text))
	if len(value) > 220 {
		return strings.TrimSpace(string(value[:220]))
	}
	return string(value)
}

func CleanContentNewsSummaryWithFallback(rawSummary, bodyText string) string {
	summary := CleanContentNewsSummary(rawSummary)
	if summary == "" {
		return CleanContentNewsSummary(bodyText)
	}
	return summary
}

func cleanContentNewsItemSummary(item *ContentNewsItem) {
	if item == nil || item.Summary == nil {
		return
	}
	bodyText := ""
	if item.BodyText != nil {
		bodyText = *item.BodyText
	}
	clean := CleanContentNewsSummaryWithFallback(*item.Summary, bodyText)
	item.Summary = &clean
}
