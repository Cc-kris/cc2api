// Package contentmd provides the conservative markdown-to-HTML fallback used by
// the content pipeline. It intentionally supports only a tiny subset of
// markdown (ATX headings, fenced code blocks, unordered list items and
// paragraphs) and escapes every piece of inline text, so the produced HTML is
// always safe to store and to feed into the public renderer allowlist.
package contentmd

import (
	"html"
	"strconv"
	"strings"
)

// Options controls the fallback rendering.
type Options struct {
	// MinHeadingLevel is the smallest heading level that may be emitted.
	// Values <= 0 default to 1. When the public renderer drops <h1> (because the
	// page already owns the document heading), callers pass 2 so a markdown
	// "# Title" still renders as a visible heading instead of plain text.
	MinHeadingLevel int
}

// Render converts a markdown document into escaped, allowlist-friendly HTML.
func Render(markdown string, opts Options) string {
	minLevel := opts.MinHeadingLevel
	if minLevel <= 0 || minLevel > 6 {
		minLevel = 1
	}

	var out strings.Builder
	var list strings.Builder
	inCode := false

	flushList := func() {
		if list.Len() == 0 {
			return
		}
		out.WriteString("<ul>")
		out.WriteString(list.String())
		out.WriteString("</ul>")
		list.Reset()
	}

	for _, raw := range strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			flushList()
			if inCode {
				out.WriteString("</code></pre>")
			} else {
				out.WriteString("<pre><code>")
			}
			inCode = !inCode
			continue
		}
		if inCode {
			out.WriteString(html.EscapeString(raw))
			out.WriteByte('\n')
			continue
		}
		if line == "" {
			flushList()
			continue
		}
		if level := headingLevel(line); level > 0 {
			flushList()
			if level < minLevel {
				level = minLevel
			}
			tag := "h" + strconv.Itoa(level)
			text := strings.TrimSpace(line[level+1:])
			out.WriteString("<" + tag + ">")
			out.WriteString(html.EscapeString(text))
			out.WriteString("</" + tag + ">")
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			list.WriteString("<li>")
			list.WriteString(html.EscapeString(strings.TrimSpace(line[2:])))
			list.WriteString("</li>")
			continue
		}
		flushList()
		out.WriteString("<p>")
		out.WriteString(html.EscapeString(line))
		out.WriteString("</p>")
	}
	flushList()
	if inCode {
		out.WriteString("</code></pre>")
	}
	return out.String()
}

func headingLevel(line string) int {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0
	}
	if strings.TrimSpace(line[level+1:]) == "" {
		return 0
	}
	return level
}

// IsBlank reports whether the rendered output is effectively empty.
func IsBlank(rendered string) bool {
	return strings.TrimSpace(rendered) == ""
}
