//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testShell = `<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8" /><title>CCAI - AI API Gateway</title>` +
	`<script type="module" src="/assets/index.js"></script></head><body><div id="app"></div></body></html>`

const testSSR = `<!doctype html><html lang="en-US"><head><meta charset="utf-8">` +
	`<meta name="viewport" content="width=device-width,initial-scale=1"><title>CCAI News</title>` +
	`<link rel="canonical" href="https://ccai.xyz/en/news"><link rel="alternate" hreflang="zh" href="https://ccai.xyz/zh/news"></head>` +
	`<body><main><h1>CCAI News</h1><a href="/news/en/x">x $1 ${2}</a></main></body></html>`

func TestMergePublicSSRIntoShell(t *testing.T) {
	out := string(mergePublicSSRIntoShell([]byte(testShell), []byte(testSSR)))

	require.Contains(t, out, `<html lang="en-US">`, "html lang follows the SSR language")
	require.Equal(t, 1, strings.Count(out, "<title>"), "only the SSR title survives")
	require.Contains(t, out, "<title>CCAI News</title>")
	require.Contains(t, out, `rel="canonical" href="https://ccai.xyz/en/news"`)
	require.Contains(t, out, `hreflang="zh"`)
	require.Equal(t, 1, strings.Count(strings.ToLower(out), "charset="), "no duplicate charset meta")
	require.Contains(t, out, `<script type="module" src="/assets/index.js"></script>`, "SPA bundle still loads")
	require.Contains(t, out, `<div id="app"><main><h1>CCAI News</h1><a href="/news/en/x">x $1 ${2}</a></main></div>`,
		"SSR body sits inside the mount point verbatim")
}

func TestMergePublicSSRIntoShellFallsBackOnUnexpectedShell(t *testing.T) {
	out := mergePublicSSRIntoShell([]byte(`<html><body>no mount</body></html>`), []byte(testSSR))
	require.Equal(t, testSSR, string(out))
}

func TestServePublicSSRInShellOnlyWrapsHTML200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shell := func(*gin.Context) []byte { return []byte(testShell) }

	cases := []struct {
		name      string
		handler   gin.HandlerFunc
		wantShell bool
		wantCode  int
	}{
		{"html page", func(c *gin.Context) { c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(testSSR)) }, true, http.StatusOK},
		{"redirect", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/zh/news") }, false, http.StatusMovedPermanently},
		{"not found", func(c *gin.Context) { c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(testSSR)) }, false, http.StatusNotFound},
		{"robots", func(c *gin.Context) { c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte("User-agent: *")) }, false, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) { servePublicSSRInShell(c, shell) })
			r.GET("/page", tc.handler)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/page", nil))
			require.Equal(t, tc.wantCode, w.Code)
			require.Equal(t, tc.wantShell, strings.Contains(w.Body.String(), "/assets/index.js"))
			if tc.wantCode == http.StatusMovedPermanently {
				require.Equal(t, "/zh/news", w.Header().Get("Location"))
			}
		})
	}
}
