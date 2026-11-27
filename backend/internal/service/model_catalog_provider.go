package service

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// platformPricingProviders maps a channel platform to every LiteLLM provider
// string that can price models of that platform.
//
// Gemini 必须同时命中多个 provider：上游价格表里 Gemini 系模型分散在
// "gemini" 与 "vertex_ai-*" 之下，而历史实现只映射成 "google"——价格表里
// 一条 "google" 都没有，导致 gemini 平台的系统目录恒为空、渠道「同步模型」
// 返回空数组、模型广场只能靠账号 model_mapping 兜底。
var platformPricingProviders = map[string][]string{
	PlatformOpenAI:    {"openai"},
	PlatformAnthropic: {"anthropic"},
	PlatformGemini: {
		"gemini",
		"vertex_ai-language-models",
		"vertex_ai-embedding-models",
	},
	PlatformAntigravity: {"anthropic"},
	"grok":              {"xai"},
}

// PricingProvidersForPlatform 返回该平台在价格目录中需要匹配的全部 provider。
func PricingProvidersForPlatform(platform string) ([]string, bool) {
	providers, ok := platformPricingProviders[strings.ToLower(strings.TrimSpace(platform))]
	return providers, ok
}

// PricingProviderForPlatform 返回该平台在价格目录中的规范 provider 标识
// （多 provider 时以逗号连接），仅用于日志与快照元数据。
func PricingProviderForPlatform(platform string) (string, bool) {
	providers, ok := PricingProvidersForPlatform(platform)
	if !ok {
		return "", false
	}
	return strings.Join(providers, ","), true
}

type ModelCatalogSnapshot struct {
	Provider  string
	Checksum  string
	UpdatedAt time.Time
	Models    []string
}

type ModelCatalogProvider interface {
	SnapshotForPlatform(platform string) (ModelCatalogSnapshot, error)
}

type PricingServiceModelCatalog struct {
	pricing *PricingService
}

func NewPricingServiceModelCatalog(pricing *PricingService) *PricingServiceModelCatalog {
	return &PricingServiceModelCatalog{pricing: pricing}
}

func (c *PricingServiceModelCatalog) SnapshotForPlatform(platform string) (ModelCatalogSnapshot, error) {
	providers, ok := PricingProvidersForPlatform(platform)
	if !ok || len(providers) == 0 {
		return ModelCatalogSnapshot{}, errors.New("unsupported model catalog platform")
	}
	if c == nil || c.pricing == nil {
		return ModelCatalogSnapshot{}, errors.New("pricing service is unavailable")
	}
	// 同一平台可能对应价格表里的多个 provider（如 gemini 分散在
	// "gemini" 与 "vertex_ai-*" 下），这里做集合匹配，避免只取一个
	// provider 时目录被削成空集。
	wanted := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		wanted[strings.ToLower(strings.TrimSpace(provider))] = struct{}{}
	}
	c.pricing.mu.RLock()
	defer c.pricing.mu.RUnlock()
	models := make([]string, 0)
	for name, pricing := range c.pricing.pricingData {
		if pricing == nil {
			continue
		}
		if _, hit := wanted[strings.ToLower(strings.TrimSpace(pricing.LiteLLMProvider))]; hit {
			models = append(models, name)
		}
	}
	provider := strings.Join(providers, ",")
	sort.Slice(models, func(i, j int) bool {
		return naturalModelLess(models[i], models[j])
	})
	return ModelCatalogSnapshot{
		Provider: provider, Checksum: c.pricing.catalogChecksumLocked(),
		UpdatedAt: c.pricing.lastUpdated, Models: models,
	}, nil
}

func naturalModelLess(left, right string) bool {
	lowerLeft, lowerRight := strings.ToLower(left), strings.ToLower(right)
	for len(lowerLeft) > 0 && len(lowerRight) > 0 {
		leftRune, leftSize := utf8.DecodeRuneInString(lowerLeft)
		rightRune, rightSize := utf8.DecodeRuneInString(lowerRight)
		if unicode.IsDigit(leftRune) && unicode.IsDigit(rightRune) {
			leftDigits, leftRest := splitLeadingDigits(lowerLeft)
			rightDigits, rightRest := splitLeadingDigits(lowerRight)
			leftNumber := strings.TrimLeft(leftDigits, "0")
			rightNumber := strings.TrimLeft(rightDigits, "0")
			if leftNumber == "" {
				leftNumber = "0"
			}
			if rightNumber == "" {
				rightNumber = "0"
			}
			if len(leftNumber) != len(rightNumber) {
				return len(leftNumber) < len(rightNumber)
			}
			if leftNumber != rightNumber {
				return leftNumber < rightNumber
			}
			if len(leftDigits) != len(rightDigits) {
				return len(leftDigits) < len(rightDigits)
			}
			lowerLeft, lowerRight = leftRest, rightRest
			continue
		}
		if leftRune != rightRune {
			return leftRune < rightRune
		}
		lowerLeft = lowerLeft[leftSize:]
		lowerRight = lowerRight[rightSize:]
	}
	if len(lowerLeft) != len(lowerRight) {
		return len(lowerLeft) < len(lowerRight)
	}
	if strings.EqualFold(left, right) {
		return left < right
	}
	return strings.ToLower(left) < strings.ToLower(right)
}

func splitLeadingDigits(value string) (digits, rest string) {
	index := 0
	for index < len(value) {
		r, size := utf8.DecodeRuneInString(value[index:])
		if !unicode.IsDigit(r) {
			break
		}
		index += size
	}
	return value[:index], value[index:]
}
