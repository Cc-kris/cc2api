package logredact

import (
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// maxRedactDepth 限制递归深度以防止栈溢出
const maxRedactDepth = 32

var defaultSensitiveKeys = map[string]struct{}{
	"authorization_code": {},
	"code":               {},
	"code_verifier":      {},
	"access_token":       {},
	"refresh_token":      {},
	"id_token":           {},
	"client_secret":      {},
	"password":           {},
}

var defaultSensitiveKeyList = []string{
	"authorization_code",
	"code",
	"code_verifier",
	"access_token",
	"refresh_token",
	"id_token",
	"client_secret",
	"password",
}

type textRedactPatterns struct {
	reJSONLike  *regexp.Regexp
	reQueryLike *regexp.Regexp
	rePlain     *regexp.Regexp
}

var (
	reGOCSPX                       = regexp.MustCompile(`GOCSPX-[0-9A-Za-z_-]{24,}`)
	reAIza                         = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)
	reEmail                        = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reToken                        = regexp.MustCompile(`(?i)\b(?:sk|ak|pk|rk|ya29|ghp|gho|ghu|ghs|github_pat)[-_A-Za-z0-9]{8,}\b`)
	reAuthorizationBearer          = regexp.MustCompile("(?i)\\bauthorization\\s*[:=]?\\s*bearer\\s+[^\\s,\"'`]+")
	reBearerToken                  = regexp.MustCompile("(?i)\\bbearer\\s+[^\\s,\"'`]+")
	reAuthorizationRedactedResidue = regexp.MustCompile("(?i)\\bauthorization\\s*[:=]\\s*(?:bearer\\s*)?\\*{3,}(?:\\s+\\*{3,})?")

	defaultTextRedactPatterns = compileTextRedactPatterns(nil)
	extraTextPatternCache     sync.Map // map[string]*textRedactPatterns
)

func RedactMap(input map[string]any, extraKeys ...string) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	keys := buildKeySet(extraKeys)
	redacted, ok := redactValueWithDepth(input, keys, 0).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return redacted
}

func RedactJSON(raw []byte, extraKeys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "<non-json payload redacted>"
	}
	keys := buildKeySet(extraKeys)
	redacted := redactValueWithDepth(value, keys, 0)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "<redacted>"
	}
	return string(encoded)
}

// RedactText 对非结构化文本做轻量脱敏。
//
// 规则：
// - 如果文本本身是 JSON，则按 RedactJSON 处理。
// - 否则尝试对常见 key=value / key:"value" 片段做脱敏。
//
// 注意：该函数用于日志/错误信息兜底，不保证覆盖所有格式。
func RedactText(input string, extraKeys ...string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	raw := []byte(input)
	if json.Valid(raw) {
		return RedactJSON(raw, extraKeys...)
	}

	patterns := getTextRedactPatterns(extraKeys)

	out := input
	out = reGOCSPX.ReplaceAllString(out, "GOCSPX-***")
	out = reAIza.ReplaceAllString(out, "AIza***")
	out = reEmail.ReplaceAllStringFunc(out, RedactEmail)
	out = reToken.ReplaceAllString(out, "******")
	out = reAuthorizationBearer.ReplaceAllString(out, "Authorization: Bearer ***")
	out = reBearerToken.ReplaceAllString(out, "Bearer ***")
	out = patterns.reJSONLike.ReplaceAllString(out, `$1***$3`)
	out = patterns.reQueryLike.ReplaceAllString(out, `$1=***`)
	out = patterns.rePlain.ReplaceAllString(out, `$1$2***`)
	return out
}

func RedactEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	local := []rune(parts[0])
	return string(local[0]) + "***@" + parts[1]
}

func RedactAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return "****"
	}
	return "****" + string(runes[len(runes)-4:])
}

func RedactToken(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "******"
}

func RedactProxyURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return RedactText(value)
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if host == "" {
		return RedactText(value)
	}
	parsed.User = nil
	parsed.Host = maskedHost(host, port)
	return parsed.String()
}

func RedactRequestBody(value string, maxRunes int) string {
	return redactBody(value, maxRunes)
}

func RedactResponseBody(value string, maxRunes int) string {
	return redactBody(value, maxRunes)
}

func RedactAIContext(value string, maxRunes int) string {
	return redactBody(value, maxRunes)
}

func RedactUpstreamAccountName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	runes := []rune(name)
	if len(runes) <= 4 {
		return string(runes[:1]) + "***"
	}
	return string(runes[:2]) + "***" + string(runes[len(runes)-2:])
}

func compileTextRedactPatterns(extraKeys []string) *textRedactPatterns {
	keyAlt := buildKeyAlternation(extraKeys)
	return &textRedactPatterns{
		// JSON-like: "access_token":"..."
		reJSONLike: regexp.MustCompile(`(?i)("(?:` + keyAlt + `)"\s*:\s*")([^"]*)(")`),
		// Query-like: access_token=...
		reQueryLike: regexp.MustCompile(`(?i)\b((?:` + keyAlt + `))=([^&\s]+)`),
		// Plain: access_token: ... / access_token = ...
		rePlain: regexp.MustCompile(`(?i)\b((?:` + keyAlt + `))\b(\s*[:=]\s*)([^,\s]+)`),
	}
}

func getTextRedactPatterns(extraKeys []string) *textRedactPatterns {
	normalizedExtraKeys := normalizeAndSortExtraKeys(extraKeys)
	if len(normalizedExtraKeys) == 0 {
		return defaultTextRedactPatterns
	}

	cacheKey := strings.Join(normalizedExtraKeys, ",")
	if cached, ok := extraTextPatternCache.Load(cacheKey); ok {
		if patterns, ok := cached.(*textRedactPatterns); ok {
			return patterns
		}
	}

	compiled := compileTextRedactPatterns(normalizedExtraKeys)
	actual, _ := extraTextPatternCache.LoadOrStore(cacheKey, compiled)
	if patterns, ok := actual.(*textRedactPatterns); ok {
		return patterns
	}
	return compiled
}

func normalizeAndSortExtraKeys(extraKeys []string) []string {
	if len(extraKeys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(extraKeys))
	keys := make([]string, 0, len(extraKeys))
	for _, key := range extraKeys {
		normalized := normalizeKey(key)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		keys = append(keys, normalized)
	}
	sort.Strings(keys)
	return keys
}

func buildKeyAlternation(extraKeys []string) string {
	seen := make(map[string]struct{}, len(defaultSensitiveKeyList)+len(extraKeys))
	keys := make([]string, 0, len(defaultSensitiveKeyList)+len(extraKeys))
	for _, k := range defaultSensitiveKeyList {
		seen[k] = struct{}{}
		keys = append(keys, regexp.QuoteMeta(k))
	}
	for _, k := range extraKeys {
		n := normalizeKey(k)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		keys = append(keys, regexp.QuoteMeta(n))
	}
	return strings.Join(keys, "|")
}

func buildKeySet(extraKeys []string) map[string]struct{} {
	keys := make(map[string]struct{}, len(defaultSensitiveKeys)+len(extraKeys))
	for k := range defaultSensitiveKeys {
		keys[k] = struct{}{}
	}
	for _, key := range extraKeys {
		normalized := normalizeKey(key)
		if normalized == "" {
			continue
		}
		keys[normalized] = struct{}{}
	}
	return keys
}

func redactValueWithDepth(value any, keys map[string]struct{}, depth int) any {
	if depth > maxRedactDepth {
		return "<depth limit exceeded>"
	}

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			if isSensitiveKey(k, keys) {
				out[k] = "***"
				continue
			}
			out[k] = redactValueWithDepth(val, keys, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactValueWithDepth(item, keys, depth+1)
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string, keys map[string]struct{}) bool {
	_, ok := keys[normalizeKey(key)]
	return ok
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func redactBody(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = RedactText(value,
		"authorization",
		"x-api-key",
		"api_key",
		"apikey",
		"token",
		"secret",
		"cookie",
		"proxy",
		"proxy_url",
		"proxyurl",
	)
	value = reAuthorizationRedactedResidue.ReplaceAllString(value, "Authorization ******")
	value = reAuthorizationBearer.ReplaceAllString(value, "Authorization ******")
	value = reBearerToken.ReplaceAllString(value, "credential ******")
	value = redactProxyURLCandidates(value)
	if maxRunes > 0 {
		value = truncateRunes(value, maxRunes)
	}
	return value
}

func redactProxyURLCandidates(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return value
	}
	replaced := false
	for i, field := range fields {
		trimmed := strings.Trim(field, `"'(),[]{}<>`)
		if trimmed == "" || !strings.Contains(trimmed, "://") {
			continue
		}
		masked := RedactProxyURL(trimmed)
		if masked == trimmed {
			continue
		}
		fields[i] = strings.Replace(field, trimmed, masked, 1)
		replaced = true
	}
	if !replaced {
		return value
	}
	return strings.Join(fields, " ")
}

func maskedHost(host, port string) string {
	if ip := net.ParseIP(host); ip != nil {
		masked := maskIPAddress(host)
		if port != "" {
			return net.JoinHostPort(masked, port)
		}
		return masked
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		for i := 0; i < len(parts)-2; i++ {
			if parts[i] != "" {
				parts[i] = "*"
			}
		}
		if len(parts)-2 >= 0 && parts[len(parts)-2] != "" {
			runes := []rune(parts[len(parts)-2])
			parts[len(parts)-2] = string(runes[:1]) + "***"
		}
		masked := strings.Join(parts, ".")
		if port != "" {
			return net.JoinHostPort(masked, port)
		}
		return masked
	}
	runes := []rune(host)
	if len(runes) > 1 {
		host = string(runes[:1]) + "***"
	} else {
		host = "***"
	}
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	return host
}

func maskIPAddress(host string) string {
	if strings.Contains(host, ":") {
		return "***"
	}
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return "***"
	}
	return parts[0] + "." + parts[1] + ".*.*"
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}
