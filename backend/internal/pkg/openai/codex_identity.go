package openai

import (
	"strings"

	"golang.org/x/net/http/httpguts"
)

// 本文件移植自上游 8e92c892b `pkg/openai/request.go:202-372` 的 Codex 出站身份函数（V02-T15）。
//
// 只用于出站身份的配对、归一和版本改写。CCAI 的入站准入判定（IsCodexOfficialClient*，
// 前缀匹配）保持原样不动；上游把它们改成了精确集合，会影响 codex_cli_only，本版不取。
// 因此这里用独立的精确集合 codexIdentityOriginators 做配对判定。

// CodexCLIOriginator 是 codex-rs 客户端的默认 originator。
const CodexCLIOriginator = "codex_cli_rs"

// CodexTUIOriginator 是交互式 Codex TUI 使用的 originator。
const CodexTUIOriginator = "codex-tui"

// CodexDefaultOriginator 是网关默认发送的 Codex originator。
// 历史 TUI 身份曾触发上游的身份风控分层，因此默认使用 CLI 身份；TUI 仍保留在官方身份集合中，
// 供兼容归一化。
const CodexDefaultOriginator = CodexCLIOriginator

// codexIdentityFamilyPrefix 覆盖 `Codex ` 前缀家族（Codex Desktop 等）。保留尾随空格，
// 以 HasPrefix 直接比对已归一化（小写 + 去首尾空格）的值。
const codexIdentityFamilyPrefix = "codex "

// codexIdentityOriginators 是 Codex 官方客户端 originator 精确集合（上游
// codexOfficialClientOriginators）。用精确匹配避免 evil-codex_ 之类伪造值被配成官方身份。
var codexIdentityOriginators = map[string]bool{
	"codex_cli_rs":          true,
	CodexTUIOriginator:      true,
	"codex_vscode":          true,
	"codex_vscode_copilot":  true,
	"codex_app":             true,
	"codex_chatgpt_desktop": true,
	"codex_atlas":           true,
	"codex_exec":            true,
	"codex_sdk_ts":          true,
}

// isCodexIdentityOriginator 精确集合 + `Codex ` 家族前缀判定（上游 IsCodexOfficialClientOriginator 最终版）。
func isCodexIdentityOriginator(originator string) bool {
	v := normalizeCodexClientHeader(originator)
	if v == "" {
		return false
	}
	if codexIdentityOriginators[v] {
		return true
	}
	return strings.HasPrefix(v, codexIdentityFamilyPrefix)
}

// codexUATrailerName 取 codex-rs UA 最后一个括号组 `(name; version)` 里的 name。
// CODEX_INTERNAL_ORIGINATOR_OVERRIDE 只改 UA 首段，不改尾部，可借此恢复真实客户端名。
func codexUATrailerName(ua string) string {
	last := strings.LastIndex(ua, "(")
	if last < 0 {
		return ""
	}
	rest := ua[last+1:]
	closeIdx := strings.Index(rest, ")")
	if closeIdx < 0 {
		return ""
	}
	inner := strings.TrimSpace(rest[:closeIdx])
	if semi := strings.Index(inner, ";"); semi >= 0 {
		inner = strings.TrimSpace(inner[:semi])
	}
	return inner
}

// PairCodexClientIdentity 由最终出站 User-Agent 推导与其配套的 originator，必要时归一化
// UA 首段，保证两者一致。上游 /backend-api/codex 会校验 originator 与 UA 首段是否配套，
// 错配一律 404（issue #3901）。
//
// 推导优先级：
//  1. UA 首段是官方 originator（精确集合或 `Codex ` 家族前缀）→ 直接配对，UA 原样保留；
//  2. UA 尾部括号组 `(name; version)` 的 name 是官方 originator → 用尾部 name 重写 UA 首段后配对；
//  3. 均不命中 → ok=false，调用方应整体回退为默认官方身份。
func PairCodexClientIdentity(userAgent string) (originator string, pairedUA string, ok bool) {
	// Validate before trimming so control bytes cannot become a valid identity.
	if !validCodexUserAgentValue(userAgent) {
		return "", "", false
	}
	ua := strings.TrimSpace(userAgent)
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return "", "", false
	}
	if leading := strings.TrimSpace(ua[:slash]); isSaneCodexOriginator(leading) && isCodexIdentityOriginator(leading) {
		leading = canonicalizeCodexOriginator(leading)
		return leading, leading + ua[slash:], true
	}
	if trailer := codexUATrailerName(ua); trailer != "" && !strings.ContainsRune(trailer, '/') &&
		isSaneCodexOriginator(trailer) && isCodexIdentityOriginator(trailer) {
		trailer = canonicalizeCodexOriginator(trailer)
		return trailer, trailer + ua[slash:], true
	}
	return "", "", false
}

func validCodexUserAgentValue(value string) bool {
	if !httpguts.ValidHeaderFieldValue(value) {
		return false
	}
	// httpguts follows the legacy field-value grammar and permits obs-fold.
	// User-Agent is not an obs-folded header; reject CR/LF before forwarding.
	return !strings.ContainsAny(value, "\r\n")
}

// codexOriginatorMaxLen 官方 clientInfo.name 均为短 ASCII 标识，远低于此上限。
const codexOriginatorMaxLen = 64

// isSaneCodexOriginator 拒绝超长或含不可打印/非 ASCII 字节的候选 originator。
func isSaneCodexOriginator(name string) bool {
	if name == "" || len(name) > codexOriginatorMaxLen {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// canonicalizeCodexOriginator 把精确集合的官方 originator 大小写变体归一为规范小写形态；
// `Codex ` 家族保留原大小写。
func canonicalizeCodexOriginator(name string) string {
	if lower := normalizeCodexClientHeader(name); codexIdentityOriginators[lower] {
		return lower
	}
	return name
}

// NormalizeCodexClientIdentityToCLI 将已配对的 TUI 身份归一为 CLI 身份（上游 c591412c4）。
// 同时改写 originator 和 User-Agent 首段，避免触发 #3901 的身份配对校验。
// 版本、操作系统、架构和终端指纹保持不变；尾部 `(name; version)` 组属于 TUI 身份时移除。
func NormalizeCodexClientIdentityToCLI(originator, userAgent string) (string, string, bool) {
	if !strings.EqualFold(strings.TrimSpace(originator), CodexTUIOriginator) {
		return originator, userAgent, false
	}

	ua := strings.TrimSpace(userAgent)
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return CodexCLIOriginator, ua, true
	}

	rest := ua[slash:]
	if trailer := codexUATrailerName(ua); trailer != "" && isCodexIdentityOriginator(trailer) {
		if open := strings.LastIndex(rest, "("); open > 0 {
			rest = strings.TrimRight(rest[:open], " ")
		}
	}
	return CodexCLIOriginator, CodexCLIOriginator + rest, true
}

// CodexUserAgentVersion 提取 Codex UA 的完整版本段，即 `{client}/{version} (...` 中的 version。
// 预发布后缀原样保留，因为出站 version 头必须与 UA 版本段逐字一致。取不到时返回空串。
func CodexUserAgentVersion(userAgent string) string {
	ua := strings.TrimSpace(userAgent)
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return ""
	}
	rest := ua[slash+1:]
	if space := strings.IndexByte(rest, ' '); space >= 0 {
		rest = rest[:space]
	}
	return strings.TrimSpace(rest)
}

// SetCodexUserAgentVersion 用 version 重建 Codex 形态 UA 中的版本声明，其余部分
// （客户端名、OS / 架构 / 终端指纹）原样保留；UA 不是 `{client}/{version}` 形态时返回空串。
// 尾部官方客户端标识组 `(name; version)` 一并更新；OS 组不动。
func SetCodexUserAgentVersion(userAgent, version string) string {
	ua := strings.TrimSpace(userAgent)
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return ""
	}
	client := strings.TrimSpace(ua[:slash])
	if client == "" {
		return ""
	}
	rest := ua[slash+1:]
	tail := ""
	if space := strings.IndexByte(rest, ' '); space >= 0 {
		tail = rest[space:]
	} else if strings.TrimSpace(rest) == "" {
		return ""
	}
	return rewriteCodexUATrailerVersion(client+"/"+version+tail, version)
}

// rewriteCodexUATrailerVersion 把尾部官方客户端标识组 `(name; version)` 的版本改成 version。
func rewriteCodexUATrailerVersion(ua, version string) string {
	open := strings.LastIndex(ua, "(")
	if open < 0 {
		return ua
	}
	closeIdx := strings.Index(ua[open+1:], ")")
	if closeIdx < 0 {
		return ua
	}
	inner := ua[open+1 : open+1+closeIdx]
	semi := strings.Index(inner, ";")
	if semi < 0 {
		return ua
	}
	name := strings.TrimSpace(inner[:semi])
	if name == "" || !isCodexIdentityOriginator(name) {
		return ua
	}
	return ua[:open+1] + name + "; " + version + ua[open+1+closeIdx:]
}
