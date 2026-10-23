package repository

import (
	"strings"
	"testing"
)

func TestPublicBodyHTMLMarkdownFallback(t *testing.T) {
	got := publicBodyHTML("", "# CCAI\n\n正文 **安全**\n\n```go\nfmt.Println(1)\n```")
	if !strings.Contains(got, "<h1>CCAI</h1>") || !strings.Contains(got, "<p>正文 **安全**</p>") || !strings.Contains(got, "<pre><code>") {
		t.Fatalf("unexpected markdown fallback: %s", got)
	}
	if got := publicBodyHTML("<p>html</p>", "# markdown"); got != "<p>html</p>" {
		t.Fatalf("html body should win, got %q", got)
	}
}
