package richtext

import (
	"strings"
	"testing"
)

func TestSanitizeKeepsWhitelistedMarkup(t *testing.T) {
	in := `<h2>标题</h2><p>文本<strong>粗</strong><em>斜</em><u>下</u><s>删</s><br><code>c</code></p>` +
		`<ul><li>a</li></ul><ol><li>b</li></ol><blockquote>q</blockquote><pre>p</pre>`
	out := Sanitize(in)
	if out != in {
		t.Fatalf("whitelisted markup changed:\nwant %s\n got %s", in, out)
	}
}

func TestSanitizeDropsScriptAndStyleWithContent(t *testing.T) {
	cases := []string{
		`<script>alert(1)</script><p>ok</p>`,
		`<style>p{color:red}</style><p>ok</p>`,
		`<iframe src="https://evil.example"></iframe><p>ok</p>`,
		`<svg onload="alert(1)"></svg><p>ok</p>`,
	}
	for _, in := range cases {
		out := Sanitize(in)
		if out != `<p>ok</p>` {
			t.Fatalf("dangerous node leaked for %q: %s", in, out)
		}
	}
}

func TestSanitizeDropsEventHandlersAndStyleAttrs(t *testing.T) {
	out := Sanitize(`<p onclick="alert(1)" style="color:red" class="x">y</p>`)
	if out != `<p>y</p>` {
		t.Fatalf("unexpected attributes kept: %s", out)
	}
}

func TestSanitizeRejectsUnsafeURLs(t *testing.T) {
	bad := []string{
		`javascript:alert(1)`,
		`//evil.example/x`,
		`data:text/html;base64,PHNjcmlwdD4=`,
		`vbscript:msgbox(1)`,
	}
	for _, href := range bad {
		out := Sanitize(`<a href="` + href + `">x</a>`)
		if strings.Contains(out, "href") {
			t.Fatalf("unsafe href kept (%s): %s", href, out)
		}
		if strings.Contains(out, "<a") {
			t.Fatalf("anchor should be unwrapped when href is unsafe (%s): %s", href, out)
		}
	}
}

func TestSanitizeKeepsSafeURLsAndTargetRel(t *testing.T) {
	in := `<a href="https://example.com/a" target="_blank" rel="noopener noreferrer">ok</a>`
	if out := Sanitize(in); out != in {
		t.Fatalf("safe link changed: %s", out)
	}
	in = `<a href="/news/zh-CN/a">ok</a>`
	if out := Sanitize(in); out != in {
		t.Fatalf("relative link changed: %s", out)
	}
	if out := Sanitize(`<a href="/x" target="_top">ok</a>`); strings.Contains(out, "target") {
		t.Fatalf("unexpected target kept: %s", out)
	}
}

func TestSanitizeImages(t *testing.T) {
	if out := Sanitize(`<img src="/media/content/1.png" alt="x">`); out != `<img src="/media/content/1.png" alt="x">` {
		t.Fatalf("safe image changed: %s", out)
	}
	if out := Sanitize(`<img src="//evil.example/a.png" alt="x">`); strings.Contains(out, "src") {
		t.Fatalf("protocol-relative image src kept: %s", out)
	}
}

func TestSanitizeEmptyAndPlainText(t *testing.T) {
	if out := Sanitize(""); out != "" {
		t.Fatalf("empty body should stay empty, got %q", out)
	}
	if out := Sanitize("纯文本"); out != "纯文本" {
		t.Fatalf("plain text should pass through, got %q", out)
	}
}
