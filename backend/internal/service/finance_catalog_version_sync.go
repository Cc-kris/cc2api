package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type FinanceSystemPriceVersion struct {
	CatalogChecksum string
	Provider        string
	ModelName       string
	BillingMode     string
	PriceDetail     map[string]any
	EffectiveFrom   time.Time
}

type FinanceCatalogVersionRepository interface {
	SyncSystemPriceVersions(ctx context.Context, checksum string, effectiveFrom time.Time, versions []FinanceSystemPriceVersion) (bool, error)
}

type FinanceCatalogVersionSync struct {
	pricing    *PricingService
	repository FinanceCatalogVersionRepository
	now        func() time.Time
}

func NewFinanceCatalogVersionSync(pricing *PricingService, repository FinanceCatalogVersionRepository) *FinanceCatalogVersionSync {
	return &FinanceCatalogVersionSync{pricing: pricing, repository: repository, now: time.Now}
}

func (s *FinanceCatalogVersionSync) Sync(ctx context.Context) (bool, error) {
	if s == nil || s.pricing == nil || s.repository == nil {
		return false, fmt.Errorf("finance catalog sync dependencies are unavailable")
	}
	snapshot := s.pricing.Snapshot()
	if strings.TrimSpace(snapshot.Checksum) == "" || len(snapshot.Models) == 0 {
		return false, fmt.Errorf("pricing catalog snapshot is empty")
	}
	effectiveFrom := snapshot.UpdatedAt
	if effectiveFrom.IsZero() {
		effectiveFrom = s.now()
	}
	versions := buildFinanceSystemPriceVersions(snapshot, effectiveFrom)
	if len(versions) == 0 {
		return false, fmt.Errorf("pricing catalog contains no finance-compatible prices")
	}
	return s.repository.SyncSystemPriceVersions(ctx, snapshot.Checksum, effectiveFrom, versions)
}

func buildFinanceSystemPriceVersions(snapshot PricingCatalogSnapshot, effectiveFrom time.Time) []FinanceSystemPriceVersion {
	names := make([]string, 0, len(snapshot.Models))
	for name := range snapshot.Models {
		names = append(names, name)
	}
	sort.Strings(names)
	versions := make([]FinanceSystemPriceVersion, 0, len(names))
	for _, name := range names {
		pricing := snapshot.Models[name]
		detail, billingMode := financeDetailFromLiteLLM(pricing)
		if len(FinancePriceDetailToMap(detail)) == 0 {
			continue
		}
		versions = append(versions, FinanceSystemPriceVersion{
			CatalogChecksum: snapshot.Checksum,
			Provider:        strings.ToLower(strings.TrimSpace(pricing.LiteLLMProvider)),
			ModelName:       name,
			BillingMode:     billingMode,
			PriceDetail:     FinancePriceDetailToMap(detail),
			EffectiveFrom:   effectiveFrom,
		})
	}
	return versions
}

func financeDetailFromLiteLLM(pricing LiteLLMModelPricing) (FinancePriceDetail, string) {
	card := FinanceRateCard{
		Input:        financePerTokenToMillion(pricing.InputCostPerToken),
		Output:       financePerTokenToMillion(pricing.OutputCostPerToken),
		CacheRead:    financePerTokenToMillion(pricing.CacheReadInputTokenCost),
		CacheWrite5m: financePerTokenToMillion(pricing.CacheCreationInputTokenCost),
		CacheWrite1h: financePerTokenToMillion(pricing.CacheCreationInputTokenCostAbove1hr),
		ImageOutput:  financePerTokenToMillion(pricing.OutputCostPerImageToken),
		PerImage:     financePositiveFloat(pricing.OutputCostPerImage),
	}
	fast := FinanceRateCard{
		Input:     financePerTokenToMillion(pricing.InputCostPerTokenPriority),
		Output:    financePerTokenToMillion(pricing.OutputCostPerTokenPriority),
		CacheRead: financePerTokenToMillion(pricing.CacheReadInputTokenCostPriority),
	}
	detail := FinancePriceDetail{Standard: card}
	if financeRateCardHasAnyPrice(fast) {
		detail.Fast = &fast
	}
	billingMode := "token"
	if card.PerImage != nil && card.Input == nil && card.Output == nil && card.ImageOutput == nil {
		billingMode = "image"
	}
	return detail, billingMode
}

func financePerTokenToMillion(value float64) *decimal.Decimal {
	if value <= 0 {
		return nil
	}
	result := decimal.NewFromFloat(value).Mul(financeMillion)
	return &result
}

func financePositiveFloat(value float64) *decimal.Decimal {
	if value <= 0 {
		return nil
	}
	result := decimal.NewFromFloat(value)
	return &result
}

func financeRateCardHasAnyPrice(card FinanceRateCard) bool {
	return card.Input != nil || card.Output != nil || card.CacheRead != nil || card.CacheWrite5m != nil || card.CacheWrite1h != nil || card.ImageOutput != nil || card.PerRequest != nil || card.PerImage != nil || card.PerSecond != nil
}
