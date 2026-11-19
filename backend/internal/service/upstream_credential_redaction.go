package service

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

const maxGeminiCredentialJSONDepth = 6

var geminiCredentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AQ\.Ab[0-9A-Za-z_-]{40,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
}

// redactGeminiCredentialText redacts only Gemini credential-shaped strings while
// preserving surrounding provider diagnostics. The shared logredact text pass is
// broader (emails, generic tokens) and would alter unrelated upstream messages,
// so it is intentionally not applied to structured Ops/Gemini data here.
func redactGeminiCredentialText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	value, ok := decodeGeminiCredentialJSON(trimmed)
	if !ok {
		return redactGeminiCredentialStrings(raw)
	}
	redacted := redactGeminiCredentialJSON(value, 0)
	if scalar, ok := redacted.(string); ok {
		return scalar
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return redactGeminiCredentialStrings(raw)
	}
	return string(encoded)
}

// Legacy upstream_errors stores detail as a JSON string nested inside another
// JSON array. Recurse into object/array JSON strings so those old records are
// also safe to read, without changing their representation or numeric codes.
func redactGeminiCredentialJSON(value any, depth int) any {
	switch typed := value.(type) {
	case string:
		if depth < maxGeminiCredentialJSONDepth {
			if nested, ok := decodeGeminiCredentialJSON(strings.TrimSpace(typed)); ok {
				switch nested.(type) {
				case map[string]any, []any:
					if encoded, err := json.Marshal(redactGeminiCredentialJSON(nested, depth+1)); err == nil {
						typed = string(encoded)
					}
				}
			}
		}
		return redactGeminiCredentialStrings(typed)
	case []any:
		if depth >= maxGeminiCredentialJSONDepth {
			return typed
		}
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactGeminiCredentialJSON(item, depth+1)
		}
		return out
	case map[string]any:
		if depth >= maxGeminiCredentialJSONDepth {
			return typed
		}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactGeminiCredentialJSON(item, depth+1)
		}
		return out
	default:
		return value
	}
}

func redactGeminiCredentialStrings(value string) string {
	value = geminiCredentialPatterns[0].ReplaceAllString(value, "AQ.***")
	return geminiCredentialPatterns[1].ReplaceAllString(value, "AIza***")
}

func decodeGeminiCredentialJSON(raw string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return value, true
}
