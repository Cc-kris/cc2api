package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type FinanceWalletAssignment struct {
	WalletID   int64
	UpstreamID int64
	Currency   string
}

type FinancePriceLookupRepository interface {
	FindAccountFinanceProfileByID(ctx context.Context, profileID int64) (*AccountFinanceProfile, error)
	FindWalletByID(ctx context.Context, walletID int64) (*FinanceWalletAssignment, error)
	FindUpstreamPriceAt(ctx context.Context, walletID int64, model, billingMode, serviceTier string, at time.Time) (*FinancePriceQuote, error)
	FindSystemPriceAt(ctx context.Context, model, billingMode string, at time.Time) (*FinancePriceQuote, error)
}

type FinanceFXPriceLookup interface {
	FindFinanceFXRateAt(ctx context.Context, currency string, at time.Time) (*FinanceFXRateVersion, error)
}

type FinancePriceSelector struct {
	repository FinancePriceLookupRepository
}

func NewFinancePriceSelector(repository FinancePriceLookupRepository) *FinancePriceSelector {
	return &FinancePriceSelector{repository: repository}
}

func (s *FinancePriceSelector) ResolveFXRateAt(ctx context.Context, currency string, at time.Time) (*FinanceFXRateVersion, error) {
	lookup, ok := s.repository.(FinanceFXPriceLookup)
	if !ok {
		return nil, nil
	}
	return lookup.FindFinanceFXRateAt(ctx, currency, at)
}

type FinanceSelectedPrice struct {
	Wallet         *FinanceWalletAssignment
	Profile        *AccountFinanceProfile
	Quote          *FinancePriceQuote
	CostMultiplier *decimal.Decimal
	MissingProfile bool
}

func (s *FinancePriceSelector) Select(ctx context.Context, accountID int64, profileID *int64, model, billingMode, serviceTier string, at time.Time) (*FinanceSelectedPrice, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("finance price repository is unavailable")
	}
	if profileID == nil || *profileID <= 0 {
		return &FinanceSelectedPrice{MissingProfile: true}, nil
	}
	profile, err := s.repository.FindAccountFinanceProfileByID(ctx, *profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.AccountID != accountID {
		return &FinanceSelectedPrice{MissingProfile: true}, nil
	}
	selected := &FinanceSelectedPrice{Profile: profile}
	// Fiat request-charge profiles use the immutable upstream charge as the
	// authority. Platform-credit profiles retain the raw credit evidence, but
	// fall back to the frozen account multiplier and system price when no
	// credit-to-USD conversion is available.
	platformCreditFallback := profile.CostMode == FinanceCostModeRequestCharge && profile.BalanceUnitSemantics == FinanceUnitPlatformCredit
	if profile.CostMode == FinanceCostModeRequestCharge && !platformCreditFallback {
		return selected, nil
	}
	if profile.WalletID != nil {
		selected.Wallet, err = s.repository.FindWalletByID(ctx, *profile.WalletID)
		if err != nil {
			return nil, err
		}
	}
	selected.CostMultiplier = cloneDecimal(profile.AccountMultiplierSnapshot)
	if profile.CostMode == FinanceCostModeContractMultiplier && profile.ContractMultiplier != nil {
		selected.CostMultiplier = cloneDecimal(profile.ContractMultiplier)
	}
	if profile.CostMode == FinanceCostModeManual && selected.Wallet != nil {
		selected.Quote, err = s.repository.FindUpstreamPriceAt(ctx, selected.Wallet.WalletID, model, billingMode, serviceTier, at)
		if err != nil {
			return nil, err
		}
		if selected.Quote != nil {
			return selected, nil
		}
	}
	selected.Quote, err = s.repository.FindSystemPriceAt(ctx, model, billingMode, at)
	if err != nil {
		return nil, err
	}
	return selected, nil
}

// SelectLegacy resolves a pre-finance-launch usage record that has no frozen
// account profile. These records can still use the historical system price;
// they must not be treated as current-account configuration.
func (s *FinancePriceSelector) SelectLegacy(ctx context.Context, model, billingMode, serviceTier string, at time.Time) (*FinanceSelectedPrice, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("finance price repository is unavailable")
	}
	quote, err := s.repository.FindSystemPriceAt(ctx, model, billingMode, at)
	if err != nil {
		return nil, err
	}
	return &FinanceSelectedPrice{Quote: quote}, nil
}

func FinancePriceDetailFromMap(raw map[string]any) (FinancePriceDetail, error) {
	if raw == nil {
		return FinancePriceDetail{}, nil
	}
	standardRaw := raw
	if nested, ok := financeMapValue(raw, "standard", "prices"); ok {
		standardRaw = nested
	}
	standard, err := financeRateCardFromMap(standardRaw)
	if err != nil {
		return FinancePriceDetail{}, err
	}
	detail := FinancePriceDetail{Standard: standard}
	if fastRaw, ok := financeMapValue(raw, "fast", "fast_prices", "priority"); ok {
		fast, err := financeRateCardFromMap(fastRaw)
		if err != nil {
			return FinancePriceDetail{}, fmt.Errorf("fast prices: %w", err)
		}
		detail.Fast = &fast
	}
	if tiersRaw, ok := raw["tiers"].([]any); ok {
		detail.Tiers = make([]FinancePriceTier, 0, len(tiersRaw))
		for index, tierRaw := range tiersRaw {
			tierMap, ok := tierRaw.(map[string]any)
			if !ok {
				return FinancePriceDetail{}, fmt.Errorf("tier %d must be an object", index)
			}
			pricesRaw := tierMap
			if nested, ok := financeMapValue(tierMap, "prices"); ok {
				pricesRaw = nested
			}
			prices, err := financeRateCardFromMap(pricesRaw)
			if err != nil {
				return FinancePriceDetail{}, fmt.Errorf("tier %d: %w", index, err)
			}
			tier := FinancePriceTier{Label: financeStringValue(tierMap, "label", "tier_label"), Prices: prices}
			if value, exists := financeInt64Value(tierMap, "min_quantity", "min_tokens", "min_seconds"); exists {
				tier.MinQuantity = value
			}
			if value, exists := financeInt64Value(tierMap, "max_quantity", "max_tokens", "max_seconds"); exists {
				tier.MaxQuantity = &value
			}
			detail.Tiers = append(detail.Tiers, tier)
		}
		sort.SliceStable(detail.Tiers, func(i, j int) bool { return detail.Tiers[i].MinQuantity < detail.Tiers[j].MinQuantity })
	}
	return detail, nil
}

func FinancePriceDetailToMap(detail FinancePriceDetail) map[string]any {
	result := financeRateCardToMap(detail.Standard)
	if detail.Fast != nil {
		result["fast"] = financeRateCardToMap(*detail.Fast)
	}
	if len(detail.Tiers) > 0 {
		tiers := make([]map[string]any, 0, len(detail.Tiers))
		for _, tier := range detail.Tiers {
			item := map[string]any{
				"min_quantity": tier.MinQuantity,
				"prices":       financeRateCardToMap(tier.Prices),
			}
			if tier.MaxQuantity != nil {
				item["max_quantity"] = *tier.MaxQuantity
			}
			if strings.TrimSpace(tier.Label) != "" {
				item["label"] = tier.Label
			}
			tiers = append(tiers, item)
		}
		result["tiers"] = tiers
	}
	return result
}

func financeRateCardFromMap(raw map[string]any) (FinanceRateCard, error) {
	card := FinanceRateCard{}
	fields := []struct {
		target  **decimal.Decimal
		aliases []string
	}{
		{&card.Input, []string{"input", "input_price"}},
		{&card.Output, []string{"output", "output_price"}},
		{&card.CacheRead, []string{"cache_read", "cache_read_price"}},
		{&card.CacheWrite5m, []string{"cache_write_5m", "cache_creation_5m", "cache_write", "cache_write_price"}},
		{&card.CacheWrite1h, []string{"cache_write_1h", "cache_creation_1h"}},
		{&card.ImageOutput, []string{"image_output", "image_output_token", "image_output_price"}},
		{&card.PerRequest, []string{"per_request", "per_request_price"}},
		{&card.PerImage, []string{"per_image", "image", "image_price"}},
		{&card.PerSecond, []string{"per_second", "video_second", "video_price"}},
	}
	for _, field := range fields {
		value, exists := financeAnyValue(raw, field.aliases...)
		if !exists {
			continue
		}
		value = financeUnwrapPriceValue(value)
		parsed, err := ParseFinanceDecimal(value)
		if err != nil {
			return FinanceRateCard{}, fmt.Errorf("%s: %w", field.aliases[0], err)
		}
		*field.target = parsed
	}
	return card, nil
}

func financeRateCardToMap(card FinanceRateCard) map[string]any {
	result := map[string]any{}
	values := []struct {
		name  string
		value *decimal.Decimal
	}{
		{"input", card.Input},
		{"output", card.Output},
		{"cache_read", card.CacheRead},
		{"cache_write_5m", card.CacheWrite5m},
		{"cache_write_1h", card.CacheWrite1h},
		{"image_output", card.ImageOutput},
		{"per_request", card.PerRequest},
		{"per_image", card.PerImage},
		{"per_second", card.PerSecond},
	}
	for _, item := range values {
		if item.value != nil {
			result[item.name] = item.value.String()
		}
	}
	return result
}

func financeUnwrapPriceValue(value any) any {
	priceMap, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if nested, exists := financeAnyValue(priceMap, "original_price", "price", "value", "amount", "multiplier_price"); exists {
		return nested
	}
	return value
}

func financeMapValue(raw map[string]any, keys ...string) (map[string]any, bool) {
	value, ok := financeAnyValue(raw, keys...)
	if !ok {
		return nil, false
	}
	typed, ok := value.(map[string]any)
	return typed, ok
}

func financeAnyValue(raw map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func financeStringValue(raw map[string]any, keys ...string) string {
	value, ok := financeAnyValue(raw, keys...)
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func financeInt64Value(raw map[string]any, keys ...string) (int64, bool) {
	value, ok := financeAnyValue(raw, keys...)
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
