// Package richtext 提供内容中心富文本正文的白名单清洗。
//
// 清洗规则与 docs/v0.4 约定一致：只保留少量排版标签，丢弃脚本/样式等
// 危险节点（连同内容），URL 属性只允许 http/https 与站内相对地址，
// protocol-relative 的 //host 形式一律拒绝。
package richtext

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

var (
	// linkValue 只接受带协议的完整地址、单斜杠相对路径、锚点、查询串
	// 以及不含斜杠的裸相对路径；"//evil.com" 被显式排除。
	linkValue = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9+.\-]*:|/(?:[^/]|$)|#|\?|[^/#?])`)
	targetRe  = regexp.MustCompile(`^_(?:blank|self)$`)
	relRe     = regexp.MustCompile(`(?i)^(?:noopener|noreferrer|nofollow|ugc|external)(?:\s+(?:noopener|noreferrer|nofollow|ugc|external))*$`)
)

var policy = newPolicy()

func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	p.AllowElements(
		// h1 必须保留：markdown 兜底渲染（contentmd MinHeadingLevel=1）会把
		// 首个 "# " 标题输出成 <h1>，剥掉会丢标题语义，详见
		// repository.TestPublicBodyHTMLMarkdownFallback。
		"p", "h1", "h2", "h3",
		"strong", "em", "u", "s",
		"ul", "ol", "li",
		"blockquote", "br", "code", "pre",
	)

	p.AllowAttrs("href").Matching(linkValue).OnElements("a")
	p.AllowAttrs("target").Matching(targetRe).OnElements("a")
	p.AllowAttrs("rel").Matching(relRe).OnElements("a")

	p.AllowAttrs("src").Matching(linkValue).OnElements("img")
	p.AllowAttrs("alt").OnElements("img")

	p.RequireParseableURLs(true)
	p.AllowRelativeURLs(true)
	p.AllowURLSchemes("http", "https")

	// 这些节点的内容没有编辑价值，保留文本反而会泄漏脚本/样式源码。
	p.SkipElementsContent("script", "style", "iframe", "object", "embed", "noscript", "template", "svg")

	return p
}

// Sanitize 清洗富文本 HTML。空输入返回空串；非白名单标签被去掉，
// 其中纯文本保留（危险节点除外）。
func Sanitize(body string) string {
	if body == "" {
		return ""
	}
	return policy.Sanitize(body)
}
