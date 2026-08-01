package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

type PricingItem string

const (
	PricingItemInput        PricingItem = "input"
	PricingItemOutput       PricingItem = "output"
	PricingItemCacheRead    PricingItem = "cache_read"
	PricingItemCacheWrite5m PricingItem = "cache_write_5m"
	PricingItemCacheWrite1h PricingItem = "cache_write_1h"
	PricingItemImageOutput  PricingItem = "image_output"
	PricingItemPerRequest   PricingItem = "per_request"
	PricingItemPerSecond    PricingItem = "per_second"
)

type PriceUnit string

const (
	PriceUnitPerMillionTokens      PriceUnit = "per_1m_tokens"
	PriceUnitPerMillionCacheTokens PriceUnit = "per_1m_cache_tokens"
	PriceUnitPerRequest            PriceUnit = "per_request"
	PriceUnitPerImage              PriceUnit = "per_image"
	PriceUnitPerSecond             PriceUnit = "per_second"
)

type PricingServiceTier string

const (
	PricingServiceTierStandard PricingServiceTier = "standard"
	PricingServiceTierFast     PricingServiceTier = "fast"
)

const unitPriceScale int32 = 8

var millionTokens = decimal.NewFromInt(1_000_000)

// ModelPricingPresence distinguishes an absent price from an explicit free
// price. It is intentionally separate from the price values so legacy pricing
// records can continue to infer presence from non-zero fields.
type ModelPricingPresence struct {
	Input         bool
	Output        bool
	CacheRead     bool
	CacheWrite    bool
	CacheWrite5m  bool
	CacheWrite1h  bool
	ImageOutput   bool
	FastInput     bool
	FastOutput    bool
	FastCacheRead bool
}

func (p *ModelPricing) ensurePresence() {
	if p == nil || p.Presence != nil {
		return
	}
	p.Presence = &ModelPricingPresence{
		Input:         p.InputPricePerToken != 0,
		Output:        p.OutputPricePerToken != 0,
		CacheRead:     p.CacheReadPricePerToken != 0,
		CacheWrite:    p.CacheCreationPricePerToken != 0,
		CacheWrite5m:  p.CacheCreation5mPrice != 0,
		CacheWrite1h:  p.CacheCreation1hPrice != 0,
		ImageOutput:   p.ImageOutputPricePerToken != 0,
		FastInput:     p.InputPricePerTokenPriority != 0,
		FastOutput:    p.OutputPricePerTokenPriority != 0,
		FastCacheRead: p.CacheReadPricePerTokenPriority != 0,
	}
}

func (p *ModelPricing) hasPrice(field PricingItem, fast bool, value float64) bool {
	if p == nil || p.Presence == nil {
		return value != 0
	}
	switch field {
	case PricingItemInput:
		if fast {
			return p.Presence.FastInput
		}
		return p.Presence.Input
	case PricingItemOutput:
		if fast {
			return p.Presence.FastOutput
		}
		return p.Presence.Output
	case PricingItemCacheRead:
		if fast {
			return p.Presence.FastCacheRead
		}
		return p.Presence.CacheRead
	case PricingItemCacheWrite5m:
		return p.Presence.CacheWrite5m || (!p.SupportsCacheBreakdown && p.Presence.CacheWrite)
	case PricingItemCacheWrite1h:
		return p.Presence.CacheWrite1h
	case PricingItemImageOutput:
		return p.Presence.ImageOutput
	default:
		return value != 0
	}
}

// DecimalUnitPrice is the canonical customer-facing unit price. JSON values
// are emitted as fixed-scale strings to avoid JavaScript floating-point loss.
type DecimalUnitPrice struct {
	Original        decimal.Decimal `json:"-"`
	MultiplierPrice decimal.Decimal `json:"-"`
	Unit            PriceUnit       `json:"unit"`
}

func (p DecimalUnitPrice) MarshalJSON() ([]byte, error) {
	type wire struct {
		Original        string    `json:"original"`
		MultiplierPrice string    `json:"multiplier_price"`
		Unit            PriceUnit `json:"unit"`
	}
	return json.Marshal(wire{
		Original:        p.Original.StringFixed(unitPriceScale),
		MultiplierPrice: p.MultiplierPrice.StringFixed(unitPriceScale),
		Unit:            p.Unit,
	})
}

type FastPriceView struct {
	Input        *DecimalUnitPrice `json:"input"`
	Output       *DecimalUnitPrice `json:"output"`
	CacheRead    *DecimalUnitPrice `json:"cache_read"`
	CacheWrite5m *DecimalUnitPrice `json:"cache_write_5m"`
	CacheWrite1h *DecimalUnitPrice `json:"cache_write_1h"`
}

type TierPriceView struct {
	MinTokens  int               `json:"min_tokens"`
	MaxTokens  *int              `json:"max_tokens"`
	TierLabel  string            `json:"tier_label,omitempty"`
	SortOrder  int               `json:"sort_order"`
	Input      *DecimalUnitPrice `json:"input"`
	Output     *DecimalUnitPrice `json:"output"`
	CacheRead  *DecimalUnitPrice `json:"cache_read"`
	CacheWrite *DecimalUnitPrice `json:"cache_write"`
	PerRequest *DecimalUnitPrice `json:"per_request"`
}

type ModelPriceView struct {
	BillingMode  BillingMode       `json:"billing_mode"`
	Input        *DecimalUnitPrice `json:"input"`
	Output       *DecimalUnitPrice `json:"output"`
	CacheRead    *DecimalUnitPrice `json:"cache_read"`
	CacheWrite5m *DecimalUnitPrice `json:"cache_write_5m"`
	CacheWrite1h *DecimalUnitPrice `json:"cache_write_1h"`
	ImageOutput  *DecimalUnitPrice `json:"image_output"`
	PerRequest   *DecimalUnitPrice `json:"per_request"`
	PerSecond    *DecimalUnitPrice `json:"per_second"`
	Fast         *FastPriceView    `json:"fast"`
	Tiers        []TierPriceView   `json:"tiers"`
}

func NewDecimalUnitPrice(original, multiplier decimal.Decimal, unit PriceUnit) (*DecimalUnitPrice, error) {
	if original.IsNegative() {
		return nil, errors.New("unit price must not be negative")
	}
	if multiplier.IsNegative() {
		return nil, errors.New("price multiplier must not be negative")
	}
	if unit == "" {
		return nil, errors.New("price unit is required")
	}
	return &DecimalUnitPrice{
		Original:        original,
		MultiplierPrice: original.Mul(multiplier),
		Unit:            unit,
	}, nil
}

func tokenUnitPrice(value float64, multiplier decimal.Decimal, unit PriceUnit) (*DecimalUnitPrice, error) {
	return NewDecimalUnitPrice(decimal.NewFromFloat(value).Mul(millionTokens), multiplier, unit)
}

func directUnitPrice(value float64, multiplier decimal.Decimal, unit PriceUnit) (*DecimalUnitPrice, error) {
	return NewDecimalUnitPrice(decimal.NewFromFloat(value), multiplier, unit)
}

// BuildModelPriceView converts a resolved price into the same Decimal unit-price
// representation used by the model square and sales-pricing snapshot.
func BuildModelPriceView(resolved *ResolvedPricing, multiplier decimal.Decimal) (*ModelPriceView, error) {
	if resolved == nil {
		return nil, errors.New("resolved pricing is required")
	}
	if multiplier.IsNegative() {
		return nil, errors.New("price multiplier must not be negative")
	}
	mode := resolved.Mode
	if mode == "" {
		mode = BillingModeToken
	}
	if !mode.IsValid() {
		return nil, fmt.Errorf("invalid billing mode: %s", mode)
	}

	view := &ModelPriceView{BillingMode: mode, Tiers: []TierPriceView{}}
	var err error
	switch mode {
	case BillingModeToken:
		if err = populateTokenPriceView(view, resolved.BasePricing, multiplier); err != nil {
			return nil, err
		}
	case BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond:
		if resolved.DefaultPerRequestPricePresent || resolved.DefaultPerRequestPrice != 0 {
			unit := PriceUnitPerRequest
			switch mode {
			case BillingModeImage:
				unit = PriceUnitPerImage
			case BillingModeVideo, BillingModePerSecond:
				unit = PriceUnitPerSecond
			}
			price, priceErr := directUnitPrice(resolved.DefaultPerRequestPrice, multiplier, unit)
			if priceErr != nil {
				return nil, priceErr
			}
			if unit == PriceUnitPerSecond {
				view.PerSecond = price
			} else {
				view.PerRequest = price
			}
		}
	}

	view.Tiers, err = normalizeTierPrices(resolved, multiplier)
	if err != nil {
		return nil, err
	}
	return view, nil
}

func populateTokenPriceView(view *ModelPriceView, pricing *ModelPricing, multiplier decimal.Decimal) error {
	if pricing == nil {
		return nil
	}
	var err error
	if pricing.hasPrice(PricingItemInput, false, pricing.InputPricePerToken) {
		view.Input, err = tokenUnitPrice(pricing.InputPricePerToken, multiplier, PriceUnitPerMillionTokens)
		if err != nil {
			return err
		}
	}
	if pricing.hasPrice(PricingItemOutput, false, pricing.OutputPricePerToken) {
		view.Output, err = tokenUnitPrice(pricing.OutputPricePerToken, multiplier, PriceUnitPerMillionTokens)
		if err != nil {
			return err
		}
	}
	if pricing.hasPrice(PricingItemCacheRead, false, pricing.CacheReadPricePerToken) {
		view.CacheRead, err = tokenUnitPrice(pricing.CacheReadPricePerToken, multiplier, PriceUnitPerMillionCacheTokens)
		if err != nil {
			return err
		}
	}
	if pricing.hasPrice(PricingItemCacheWrite5m, false, pricing.CacheCreation5mPrice) {
		value := pricing.CacheCreation5mPrice
		if value == 0 && !pricing.SupportsCacheBreakdown {
			value = pricing.CacheCreationPricePerToken
		}
		view.CacheWrite5m, err = tokenUnitPrice(value, multiplier, PriceUnitPerMillionCacheTokens)
		if err != nil {
			return err
		}
	}
	if pricing.hasPrice(PricingItemCacheWrite1h, false, pricing.CacheCreation1hPrice) {
		view.CacheWrite1h, err = tokenUnitPrice(pricing.CacheCreation1hPrice, multiplier, PriceUnitPerMillionCacheTokens)
		if err != nil {
			return err
		}
	}
	if pricing.hasPrice(PricingItemImageOutput, false, pricing.ImageOutputPricePerToken) {
		view.ImageOutput, err = tokenUnitPrice(pricing.ImageOutputPricePerToken, multiplier, PriceUnitPerMillionTokens)
		if err != nil {
			return err
		}
	}

	fast := &FastPriceView{}
	if pricing.hasPrice(PricingItemInput, true, pricing.InputPricePerTokenPriority) {
		fast.Input, err = tokenUnitPrice(pricing.InputPricePerTokenPriority, multiplier, PriceUnitPerMillionTokens)
		if err != nil {
			return err
		}
	}
	if pricing.hasPrice(PricingItemOutput, true, pricing.OutputPricePerTokenPriority) {
		fast.Output, err = tokenUnitPrice(pricing.OutputPricePerTokenPriority, multiplier, PriceUnitPerMillionTokens)
		if err != nil {
			return err
		}
	}
	if pricing.hasPrice(PricingItemCacheRead, true, pricing.CacheReadPricePerTokenPriority) {
		fast.CacheRead, err = tokenUnitPrice(pricing.CacheReadPricePerTokenPriority, multiplier, PriceUnitPerMillionCacheTokens)
		if err != nil {
			return err
		}
	}
	// Priority billing changes input, output and cache-read prices when the
	// upstream provides dedicated values. Cache creation keeps the regular price
	// in the billing engine, so expose the same immutable unit prices in the Fast
	// section instead of silently omitting billable cache writes.
	fastAvailable := fast.Input != nil || fast.Output != nil || fast.CacheRead != nil
	if fastAvailable && pricing.hasPrice(PricingItemCacheWrite5m, false, pricing.CacheCreation5mPrice) {
		value := pricing.CacheCreation5mPrice
		if value == 0 && !pricing.SupportsCacheBreakdown {
			value = pricing.CacheCreationPricePerToken
		}
		fast.CacheWrite5m, err = tokenUnitPrice(value, multiplier, PriceUnitPerMillionCacheTokens)
		if err != nil {
			return err
		}
	}
	if fastAvailable && pricing.hasPrice(PricingItemCacheWrite1h, false, pricing.CacheCreation1hPrice) {
		fast.CacheWrite1h, err = tokenUnitPrice(pricing.CacheCreation1hPrice, multiplier, PriceUnitPerMillionCacheTokens)
		if err != nil {
			return err
		}
	}
	if fast.Input != nil || fast.Output != nil || fast.CacheRead != nil || fast.CacheWrite5m != nil || fast.CacheWrite1h != nil {
		view.Fast = fast
	}
	return nil
}

func normalizeTierPrices(resolved *ResolvedPricing, multiplier decimal.Decimal) ([]TierPriceView, error) {
	intervals := resolved.Intervals
	if resolved.Mode != BillingModeToken {
		intervals = resolved.RequestTiers
	}
	if len(intervals) == 0 {
		return []TierPriceView{}, nil
	}

	sorted := append([]PricingInterval(nil), intervals...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].MinTokens != sorted[j].MinTokens {
			return sorted[i].MinTokens < sorted[j].MinTokens
		}
		if sorted[i].SortOrder != sorted[j].SortOrder {
			return sorted[i].SortOrder < sorted[j].SortOrder
		}
		return sorted[i].TierLabel < sorted[j].TierLabel
	})

	views := make([]TierPriceView, 0, len(sorted))
	for i := range sorted {
		iv := sorted[i]
		if iv.MinTokens < 0 || (iv.MaxTokens != nil && *iv.MaxTokens <= iv.MinTokens) {
			return nil, fmt.Errorf("invalid pricing tier bounds at index %d", i)
		}
		if resolved.Mode == BillingModeToken && i > 0 {
			prev := sorted[i-1]
			if prev.MaxTokens == nil || iv.MinTokens < *prev.MaxTokens {
				return nil, fmt.Errorf("overlapping pricing tiers at index %d", i)
			}
		}
		view := TierPriceView{
			MinTokens: iv.MinTokens,
			MaxTokens: iv.MaxTokens,
			TierLabel: iv.TierLabel,
			SortOrder: iv.SortOrder,
		}
		var err error
		if iv.InputPrice != nil {
			view.Input, err = tokenUnitPrice(*iv.InputPrice, multiplier, PriceUnitPerMillionTokens)
		}
		if err == nil && iv.OutputPrice != nil {
			view.Output, err = tokenUnitPrice(*iv.OutputPrice, multiplier, PriceUnitPerMillionTokens)
		}
		if err == nil && iv.CacheReadPrice != nil {
			view.CacheRead, err = tokenUnitPrice(*iv.CacheReadPrice, multiplier, PriceUnitPerMillionCacheTokens)
		}
		if err == nil && iv.CacheWritePrice != nil {
			view.CacheWrite, err = tokenUnitPrice(*iv.CacheWritePrice, multiplier, PriceUnitPerMillionCacheTokens)
		}
		if err == nil && iv.PerRequestPrice != nil {
			unit := PriceUnitPerRequest
			switch resolved.Mode {
			case BillingModeImage:
				unit = PriceUnitPerImage
			case BillingModeVideo, BillingModePerSecond:
				unit = PriceUnitPerSecond
			}
			view.PerRequest, err = directUnitPrice(*iv.PerRequestPrice, multiplier, unit)
		}
		if err != nil {
			return nil, fmt.Errorf("pricing tier %d: %w", i, err)
		}
		views = append(views, view)
	}
	return views, nil
}
