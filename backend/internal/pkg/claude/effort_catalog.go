package claude

import (
	"strings"
	"unicode"
)

// 本文件只移植上游 effort_catalog.go 里的 5.5 型号判定（V02-T19）。
// 推理强度目录（effortFamilies、EffortLevelsForModel 等）随 V05 协议兼容一起移植，
// 届时整份替换本文件即可。

// IsOpus55 identifies the fixed Opus 5.5 ID after provider/local suffix normalization.
func IsOpus55(model string) bool {
	return normalizeEffortModelID(model) == "claude-opus-5-5"
}

// IsSonnet55 identifies the fixed Sonnet 5.5 ID after provider/local suffix normalization.
func IsSonnet55(model string) bool {
	return normalizeEffortModelID(model) == "claude-sonnet-5-5"
}

// IsHaiku55 identifies the fixed Haiku 5.5 ID after provider/local suffix normalization.
func IsHaiku55(model string) bool {
	return normalizeEffortModelID(model) == "claude-haiku-5-5"
}

func normalizeEffortModelID(model string) string {
	id := strings.ToLower(strings.TrimSpace(model))
	id = strings.TrimPrefix(id, "models/")
	if slash := strings.IndexByte(id, '/'); slash >= 0 {
		id = strings.TrimPrefix(strings.TrimSpace(id[slash+1:]), "models/")
	}
	for _, prefix := range []string{"us.", "eu.", "apac.", "jp.", "au.", "us-gov.", "global."} {
		id = strings.TrimPrefix(id, prefix)
	}
	id = strings.TrimPrefix(id, "anthropic.")
	id = strings.TrimSuffix(id, "-thinking")
	// OpenRouter uses dotted minor versions for some models. Normalize them
	// before effort, thinking, and billing family lookups.
	if id == "claude-opus-5.5" {
		id = "claude-opus-5-5"
	}
	if id == "claude-sonnet-5.5" {
		id = "claude-sonnet-5-5"
	}
	if id == "claude-haiku-5.5" {
		id = "claude-haiku-5-5"
	}
	if mapped, ok := ModelIDReverseOverrides[id]; ok {
		id = mapped
	}
	if len(id) >= 9 {
		suffix := id[len(id)-9:]
		if suffix[0] == '-' {
			digits := true
			for _, r := range suffix[1:] {
				if !unicode.IsDigit(r) {
					digits = false
					break
				}
			}
			if digits {
				id = id[:len(id)-9]
			}
		}
	}
	return id
}
