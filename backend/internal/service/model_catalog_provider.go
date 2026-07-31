package service

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var platformPricingProvider = map[string]string{
	PlatformOpenAI:      "openai",
	PlatformAnthropic:   "anthropic",
	PlatformGemini:      "google",
	PlatformAntigravity: "anthropic",
	"grok":              "xai",
}

func PricingProviderForPlatform(platform string) (string, bool) {
	provider, ok := platformPricingProvider[strings.ToLower(strings.TrimSpace(platform))]
	return provider, ok
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
	provider, ok := PricingProviderForPlatform(platform)
	if !ok {
		return ModelCatalogSnapshot{}, errors.New("unsupported model catalog platform")
	}
	if c == nil || c.pricing == nil {
		return ModelCatalogSnapshot{}, errors.New("pricing service is unavailable")
	}
	c.pricing.mu.RLock()
	defer c.pricing.mu.RUnlock()
	models := make([]string, 0)
	for name, pricing := range c.pricing.pricingData {
		if pricing != nil && strings.EqualFold(strings.TrimSpace(pricing.LiteLLMProvider), provider) {
			models = append(models, name)
		}
	}
	sort.Slice(models, func(i, j int) bool {
		return naturalModelLess(models[i], models[j])
	})
	return ModelCatalogSnapshot{
		Provider: provider, Checksum: c.pricing.localHash,
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
