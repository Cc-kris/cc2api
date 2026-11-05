//go:build embed

package web

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// Public content pages are rendered twice: Go emits crawlable HTML (title,
// canonical, hreflang, JSON-LD, full body) and the SPA renders the styled,
// interactive page with the language switcher. Serving only the Go HTML left
// browsers on an unstyled page with no switcher; serving only the SPA would
// hide the content from crawlers. So the SSR document is merged into the SPA
// shell: SSR <head> tags replace the shell <title>, the SSR <body> goes inside
// <div id="app"> (Vue replaces it on mount), and <html lang> follows the SSR.

var (
	ssrHTMLLangPattern   = regexp.MustCompile(`(?is)<html[^>]*\blang="([^"]*)"`)
	ssrHeadPattern       = regexp.MustCompile(`(?is)<head[^>]*>(.*?)</head>`)
	ssrBodyPattern       = regexp.MustCompile(`(?is)<body[^>]*>(.*)</body>`)
	ssrDuplicateMeta     = regexp.MustCompile(`(?is)<meta\s+(charset=[^>]*|name="viewport"[^>]*)>\s*`)
	shellTitlePattern    = regexp.MustCompile(`(?is)<title>.*?</title>`)
	shellHTMLLangPattern = regexp.MustCompile(`(?is)(<html[^>]*\blang=")[^"]*(")`)
	shellAppDivPattern   = regexp.MustCompile(`(?is)<div id="app">\s*</div>`)
)

// mergePublicSSRIntoShell returns shell with the SSR head/body spliced in, or
// the SSR document unchanged when either side lacks the expected structure.
func mergePublicSSRIntoShell(shell, ssr []byte) []byte {
	head := ssrHeadPattern.FindSubmatch(ssr)
	body := ssrBodyPattern.FindSubmatch(ssr)
	if head == nil || body == nil || !shellAppDivPattern.Match(shell) || !bytes.Contains(shell, []byte("</head>")) {
		return ssr
	}
	headTags := ssrDuplicateMeta.ReplaceAll(head[1], nil)

	out := shellTitlePattern.ReplaceAll(shell, nil)
	out = bytes.Replace(out, []byte("</head>"), append(append([]byte{}, headTags...), []byte("</head>")...), 1)
	if lang := ssrHTMLLangPattern.FindSubmatch(ssr); lang != nil {
		out = shellHTMLLangPattern.ReplaceAllFunc(out, func(m []byte) []byte {
			sub := shellHTMLLangPattern.FindSubmatch(m)
			return append(append(append([]byte{}, sub[1]...), lang[1]...), sub[2]...)
		})
	}
	appDiv := append(append([]byte(`<div id="app">`), body[1]...), []byte(`</div>`)...)
	loc := shellAppDivPattern.FindIndex(out)
	merged := make([]byte, 0, len(out)+len(appDiv))
	merged = append(merged, out[:loc[0]]...)
	merged = append(merged, appDiv...)
	merged = append(merged, out[loc[1]:]...)
	return merged
}

// servePublicSSRInShell runs the Gin SSR handler into a buffer and, for a 200
// HTML response, writes the shell-merged document instead. Redirects, 404s,
// robots.txt and sitemap.xml pass through byte for byte.
func servePublicSSRInShell(c *gin.Context, shell func(c *gin.Context) []byte) {
	original := c.Writer
	buffered := &bufferedResponseWriter{ResponseWriter: original, status: http.StatusOK}
	c.Writer = buffered
	c.Next()
	c.Writer = original

	body := buffered.body.Bytes()
	contentType := original.Header().Get("Content-Type")
	if buffered.status == http.StatusOK && strings.HasPrefix(contentType, "text/html") {
		if shellHTML := shell(c); len(shellHTML) > 0 {
			body = mergePublicSSRIntoShell(shellHTML, body)
			original.Header().Del("Content-Length")
			original.Header().Del("ETag")
		}
	}
	original.WriteHeader(buffered.status)
	_, _ = original.Write(body)
}

// bufferedResponseWriter holds the status and body until the handler returns.
type bufferedResponseWriter struct {
	gin.ResponseWriter
	body    bytes.Buffer
	status  int
	written bool
}

func (w *bufferedResponseWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}

func (w *bufferedResponseWriter) WriteHeaderNow() { w.written = true }

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	w.written = true
	return w.body.Write(data)
}

func (w *bufferedResponseWriter) WriteString(s string) (int, error) {
	w.written = true
	return w.body.WriteString(s)
}

func (w *bufferedResponseWriter) Status() int   { return w.status }
func (w *bufferedResponseWriter) Size() int     { return w.body.Len() }
func (w *bufferedResponseWriter) Written() bool { return w.written }
func (w *bufferedResponseWriter) Flush()        {}

func (w *bufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}
