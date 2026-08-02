package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

type modelSquareGroupRepoStub struct {
	GroupRepository
	groups []Group
}

func (s *modelSquareGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	return append([]Group(nil), s.groups...), nil
}

type modelSquareAccountRepoStub struct {
	AccountRepository
	byGroup map[int64][]*Account
}

func (s *modelSquareAccountRepoStub) ListCatalogEligibleByGroupIDs(_ context.Context, ids []int64) (map[int64][]*Account, error) {
	result := make(map[int64][]*Account, len(ids))
	for _, id := range ids {
		result[id] = append([]*Account(nil), s.byGroup[id]...)
	}
	return result, nil
}

type modelSquareUserRepoStub struct {
	UserRepository
	user *User
}

func (s *modelSquareUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type modelSquareRateRepoStub struct {
	UserGroupRateRepository
	rates map[int64]float64
}

func (s *modelSquareRateRepoStub) GetByUserID(context.Context, int64) (map[int64]float64, error) {
	return s.rates, nil
}

type modelSquareCatalogStub struct{ snapshot ModelCatalogSnapshot }

func (s modelSquareCatalogStub) SnapshotForPlatform(string) (ModelCatalogSnapshot, error) {
	return s.snapshot, nil
}

type modelSquarePlatformCatalogStub struct {
	snapshots map[string]ModelCatalogSnapshot
	errors    map[string]error
}

func (s modelSquarePlatformCatalogStub) SnapshotForPlatform(platform string) (ModelCatalogSnapshot, error) {
	if err := s.errors[platform]; err != nil {
		return ModelCatalogSnapshot{}, err
	}
	return s.snapshots[platform], nil
}

func newModelSquareServiceForTest() *ModelSquareService {
	group := Group{ID: 10, Name: "OpenAI", Platform: PlatformOpenAI, Status: StatusActive, RateMultiplier: 1.2, SortOrder: 1}
	account := &Account{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	billing := NewBillingService(&config.Config{}, nil)
	return &ModelSquareService{
		groups:   &modelSquareGroupRepoStub{groups: []Group{group}},
		accounts: &modelSquareAccountRepoStub{byGroup: map[int64][]*Account{10: {account}}},
		users:    &modelSquareUserRepoStub{user: &User{ID: 1}},
		rates:    &modelSquareRateRepoStub{rates: map[int64]float64{10: 1.1}},
		catalog: modelSquareCatalogStub{snapshot: ModelCatalogSnapshot{
			Provider: "openai", Checksum: "v1", UpdatedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
			Models: []string{"gpt-5.4", "gpt-5.5"},
		}},
		resolver: NewModelPricingResolver(nil, billing), cursorSecret: []byte("test-secret"),
		cache: gocache.New(modelSquareCacheTTL, time.Minute),
	}
}

func TestModelSquareListsVisibleGroupsAndUsesEffectiveMultiplier(t *testing.T) {
	svc := newModelSquareServiceForTest()
	result, err := svc.ListGroups(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	require.Equal(t, "1.2000", result.Groups[0].DefaultMultiplier)
	require.Equal(t, "1.1000", result.Groups[0].EffectiveMultiplier)
	require.True(t, result.Groups[0].HasCustomMultiplier)
	require.Equal(t, 2, result.Groups[0].ModelCount)
}

func TestModelSquareUsesPublicModelRestrictionBeforeAccountMapping(t *testing.T) {
	svc := newModelSquareServiceForTest()
	accounts, ok := svc.accounts.(*modelSquareAccountRepoStub)
	require.True(t, ok)
	accounts.byGroup[10][0].Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}
	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "gpt-5.4", result.Items[0].Name)
}

func TestModelSquarePrioritizesAccountModelRestrictionList(t *testing.T) {
	svc := newModelSquareServiceForTest()
	accounts, ok := svc.accounts.(*modelSquareAccountRepoStub)
	require.True(t, ok)
	accounts.byGroup[10][0].Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}

	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "gpt-5.4", result.Items[0].Name)
}

func TestModelSquareRestrictionListDoesNotExpandFromUnrestrictedSibling(t *testing.T) {
	svc := newModelSquareServiceForTest()
	accounts, ok := svc.accounts.(*modelSquareAccountRepoStub)
	require.True(t, ok)
	accounts.byGroup[10] = append(accounts.byGroup[10], &Account{
		ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
	})
	accounts.byGroup[10][0].Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}

	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "gpt-5.4", result.Items[0].Name)
}

func TestModelSquareExactRestrictionTakesPriorityOverWildcardMapping(t *testing.T) {
	svc := newModelSquareServiceForTest()
	accounts, ok := svc.accounts.(*modelSquareAccountRepoStub)
	require.True(t, ok)
	accounts.byGroup[10][0].Credentials = map[string]any{"model_mapping": map[string]any{
		"*":       "provider-gpt-5.4",
		"gpt-5.4": "gpt-5.4",
	}}

	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "gpt-5.4", result.Items[0].Name)
}

func TestModelSquareSignedCursorPaginationAndCatalogChange(t *testing.T) {
	svc := newModelSquareServiceForTest()
	first, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 1})
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	require.Equal(t, "system", first.Items[0].PricingSource)
	require.NotNil(t, first.NextCursor)
	require.Equal(t, "2.75000000", first.Items[0].Prices.Input.MultiplierPrice.StringFixed(8))

	second, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 1, Cursor: *first.NextCursor})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.NotEqual(t, first.Items[0].Name, second.Items[0].Name)

	_, err = svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 1, Cursor: *first.NextCursor + "x"})
	require.ErrorIs(t, err, ErrModelSquareInvalidCursor)

	changedAt := first.CatalogUpdatedAt.Add(time.Second)
	_, err = svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 1, CatalogUpdatedAt: &changedAt})
	require.ErrorIs(t, err, ErrModelSquareCatalogChanged)
}

func TestModelSquareOnlyListsPublicGroupsAndKeepsCatalogFailureVisible(t *testing.T) {
	svc := newModelSquareServiceForTest()
	groups, ok := svc.groups.(*modelSquareGroupRepoStub)
	require.True(t, ok)
	accounts, ok := svc.accounts.(*modelSquareAccountRepoStub)
	require.True(t, ok)
	public := groups.groups[0]
	exclusive := public
	exclusive.ID = 11
	exclusive.Name = "Exclusive"
	exclusive.IsExclusive = true
	unsupported := public
	unsupported.ID = 12
	unsupported.Name = "Unsupported"
	unsupported.Platform = "unsupported"
	groups.groups = []Group{public, exclusive, unsupported}
	accounts.byGroup[12] = accounts.byGroup[10]
	catalog, ok := svc.catalog.(modelSquareCatalogStub)
	require.True(t, ok)
	baseSnapshot := catalog.snapshot
	svc.catalog = modelSquarePlatformCatalogStub{
		snapshots: map[string]ModelCatalogSnapshot{PlatformOpenAI: baseSnapshot},
		errors:    map[string]error{"unsupported": errors.New("unsupported platform")},
	}

	result, err := svc.ListGroups(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, result.Groups, 2)
	require.Equal(t, public.ID, result.Groups[0].ID)
	require.Equal(t, unsupported.ID, result.Groups[1].ID)
	require.Zero(t, result.Groups[1].ModelCount)
}

func TestModelSquareCustomerResponseDoesNotExposeInternalRoutingFields(t *testing.T) {
	svc := newModelSquareServiceForTest()
	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	payload, err := json.Marshal(result)
	require.NoError(t, err)
	for _, forbidden := range []string{"channel_id", "account_id", "wallet_id", "upstream_model", "price_version_id", "credentials"} {
		require.NotContains(t, string(payload), forbidden)
	}
	require.NotContains(t, string(payload), `"fast":`)
	require.NotContains(t, string(payload), `"billing_mode":{"`)
}

func TestChannelPricingCompleteRequiresFullTokenPrice(t *testing.T) {
	input, output := 1e-6, 2e-6
	require.False(t, IsCompleteChannelSalesPricing(ChannelModelPricing{BillingMode: BillingModeToken, InputPrice: &input}))
	require.True(t, IsCompleteChannelSalesPricing(ChannelModelPricing{BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output}))
	require.False(t, IsCompleteChannelSalesPricing(ChannelModelPricing{BillingMode: BillingModeToken, Intervals: []PricingInterval{{InputPrice: &input}}}))
	require.True(t, IsCompleteChannelSalesPricing(ChannelModelPricing{BillingMode: BillingModeToken, Intervals: []PricingInterval{{InputPrice: &input, OutputPrice: &output}}}))
}

func TestModelSquareCacheKeyTracksUserMultiplierAndCatalogVersion(t *testing.T) {
	svc := newModelSquareServiceForTest()
	first, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	second, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, first.Items, second.Items)
	require.Equal(t, uint64(1), svc.SnapshotMetrics().CacheMisses)
	require.Equal(t, uint64(1), svc.SnapshotMetrics().CacheHits)

	rates, ok := svc.rates.(*modelSquareRateRepoStub)
	require.True(t, ok)
	rates.rates[10] = 1.3
	changedMultiplier, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, "3.25000000", changedMultiplier.Items[0].Prices.Input.MultiplierPrice.StringFixed(8))
	require.Equal(t, uint64(2), svc.SnapshotMetrics().CacheMisses)

	catalog, ok := svc.catalog.(modelSquareCatalogStub)
	require.True(t, ok)
	updatedSnapshot := catalog.snapshot
	updatedSnapshot.Checksum = "v2"
	updatedSnapshot.UpdatedAt = updatedSnapshot.UpdatedAt.Add(time.Second)
	svc.catalog = modelSquareCatalogStub{snapshot: updatedSnapshot}
	_, err = svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, uint64(3), svc.SnapshotMetrics().CacheMisses)

	svc.InvalidateCache()
	_, err = svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, uint64(4), svc.SnapshotMetrics().CacheMisses)
	require.Equal(t, uint64(5), svc.SnapshotMetrics().Requests)
}

func TestModelSquareRoutabilityMatrixMatchesFastEligibilityBaseContract(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name    string
		group   Group
		account *Account
		want    bool
	}{
		{name: "active schedulable supported", group: Group{}, account: &Account{Status: StatusActive, Schedulable: true}, want: true},
		{name: "inactive", group: Group{}, account: &Account{Status: StatusDisabled, Schedulable: true}, want: false},
		{name: "manually unschedulable", group: Group{}, account: &Account{Status: StatusActive, Schedulable: false}, want: false},
		{name: "oauth required rejects api key", group: Group{RequireOAuthOnly: true}, account: &Account{Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey}, want: false},
		{name: "oauth required accepts oauth", group: Group{RequireOAuthOnly: true}, account: &Account{Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth}, want: true},
		{name: "privacy required rejects unset", group: Group{RequirePrivacySet: true}, account: &Account{Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Extra: map[string]any{}}, want: false},
		{name: "privacy required accepts set", group: Group{Platform: PlatformOpenAI, RequirePrivacySet: true}, account: &Account{Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Extra: map[string]any{"privacy_mode": PrivacyModeTrainingOff}}, want: true},
		{name: "platform mismatch", group: Group{Platform: PlatformOpenAI}, account: &Account{Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true}, want: false},
		{name: "mixed antigravity accepted", group: Group{Platform: PlatformAnthropic}, account: &Account{Platform: PlatformAntigravity, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": true}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-public": "gpt-public"}}}, want: true},
		{name: "mixed antigravity disabled", group: Group{Platform: PlatformAnthropic}, account: &Account{Platform: PlatformAntigravity, Status: StatusActive, Schedulable: true}, want: false},
		{name: "unsupported model", group: Group{}, account: &Account{Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"other": "other"}}}, want: false},
		{name: "transient rate limit does not hide catalog model", group: Group{}, account: &Account{Status: StatusActive, Schedulable: true, RateLimitResetAt: &future}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, modelSquareAccountRoutable(&tt.group, tt.account, "gpt-public"))
			require.Equal(t, tt.want, modelRoutableForGroup(&tt.group, []*Account{tt.account}, "gpt-public"))
			require.Equal(t, tt.want, modelSquareFastAllowed(DefaultOpenAIFastPolicySettings(), &tt.group, []*Account{tt.account}, "gpt-public"))
		})
	}
}

func TestModelSquareUsesSystemCatalogAndAddsCompleteChannelModels(t *testing.T) {
	input, output := 3e-6, 6e-6
	customInput, customOutput := 9e-6, 12e-6
	incompleteInput := 5e-6
	pricing := NewPricingService(&config.Config{}, nil)
	pricing.pricingData = map[string]*LiteLLMModelPricing{
		"gpt-public": {
			InputCostPerToken: 2e-6, OutputCostPerToken: 4e-6,
			CacheReadInputTokenCost: 0.5e-6, CacheCreationInputTokenCost: 0.75e-6,
			InputCostPerTokenPriority: 4e-6, OutputCostPerTokenPriority: 8e-6,
			CacheReadInputTokenCostPriority: 1e-6, LiteLLMProvider: "openai", SupportsServiceTier: true,
		},
		"gpt-blocked":       {InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, LiteLLMProvider: "openai"},
		"incomplete-public": {InputCostPerToken: 5e-6, OutputCostPerToken: 10e-6, LiteLLMProvider: "openai"},
	}
	pricing.localHash = "matrix-v1"
	pricing.lastUpdated = time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	channel := Channel{
		ID: 30, Name: "sales", Status: StatusActive, GroupIDs: []int64{10},
		ModelMapping: map[string]map[string]string{PlatformOpenAI: {
			"gpt-public": "gpt-upstream", "custom-public": "custom-upstream", "incomplete-public": "incomplete-upstream",
		}},
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-public"}, BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output},
			{Platform: PlatformOpenAI, Models: []string{"custom-public"}, BillingMode: BillingModeToken, InputPrice: &customInput, OutputPrice: &customOutput},
			{Platform: PlatformOpenAI, Models: []string{"incomplete-public"}, BillingMode: BillingModeToken, InputPrice: &incompleteInput},
		},
	}
	channels := NewChannelService(nil, nil, nil, pricing)
	channels.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{10: PlatformOpenAI}))
	account := &Account{
		ID: 20, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"gpt-public": "gpt-upstream", "custom-public": "custom-upstream", "incomplete-public": "incomplete-upstream",
		}},
	}
	svc := &ModelSquareService{
		groups:       &modelSquareGroupRepoStub{groups: []Group{{ID: 10, Name: "OpenAI", Platform: PlatformOpenAI, Status: StatusActive, RateMultiplier: 1.5}}},
		accounts:     &modelSquareAccountRepoStub{byGroup: map[int64][]*Account{10: {account}}},
		users:        &modelSquareUserRepoStub{user: &User{ID: 1}},
		rates:        &modelSquareRateRepoStub{rates: map[int64]float64{}},
		channels:     channels,
		catalog:      NewPricingServiceModelCatalog(pricing),
		resolver:     NewModelPricingResolver(channels, NewBillingService(&config.Config{}, pricing)),
		cursorSecret: []byte("matrix-secret"), cache: gocache.New(modelSquareCacheTTL, time.Minute),
	}

	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 50})
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	require.Equal(t, []string{"custom-public", "gpt-public", "incomplete-public"}, []string{result.Items[0].Name, result.Items[1].Name, result.Items[2].Name})
	require.Equal(t, "channel", result.Items[0].PricingSource)
	require.Equal(t, "13.50000000", result.Items[0].Prices.Input.MultiplierPrice.StringFixed(8))
	require.Equal(t, "channel", result.Items[1].PricingSource)
	require.Equal(t, "4.50000000", result.Items[1].Prices.Input.MultiplierPrice.StringFixed(8))
	require.Equal(t, "0.75000000", result.Items[1].Prices.CacheRead.MultiplierPrice.StringFixed(8))
	require.NotNil(t, result.Items[1].FastPrices)
	require.NotNil(t, result.Items[1].FastPrices.CacheRead)
	require.NotNil(t, result.Items[1].FastPrices.CacheWrite5m)
	require.Equal(t, "system", result.Items[2].PricingSource)
	require.Equal(t, "7.50000000", result.Items[2].Prices.Input.MultiplierPrice.StringFixed(8))
	require.NotContains(t, []string{result.Items[0].Name, result.Items[1].Name, result.Items[2].Name}, "gpt-blocked")

	publicOnly := *account
	publicOnly.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-public": "gpt-public"}}
	svc.accounts = &modelSquareAccountRepoStub{byGroup: map[int64][]*Account{10: {&publicOnly}}}
	svc.cache = nil
	result, err = svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 50})
	require.NoError(t, err)
	require.Len(t, result.Items, 1, "account mappings are checked against the customer-requested model")
	require.Equal(t, "gpt-public", result.Items[0].Name)
}

func TestModelSquareCatalogFallbackRequiresCompleteChannelPricing(t *testing.T) {
	input, output := 3e-6, 6e-6
	complete := Channel{ID: 30, Status: StatusActive, ModelPricing: []ChannelModelPricing{{
		Platform: PlatformOpenAI, Models: []string{"gpt-public"}, BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output,
	}}}
	incomplete := complete
	incomplete.ModelPricing = []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-public"}, BillingMode: BillingModeToken, InputPrice: &input}}
	require.Equal(t, []string{"gpt-public"}, modelSquareCompleteChannelModels(&complete, PlatformOpenAI))
	require.Empty(t, modelSquareCompleteChannelModels(&incomplete, PlatformOpenAI))
}

func TestModelSquareCatalogFallbackExcludesIncompleteChannelModels(t *testing.T) {
	input, output := 3e-6, 6e-6
	channel := Channel{ID: 30, Status: StatusActive, GroupIDs: []int64{10}, ModelPricing: []ChannelModelPricing{
		{Platform: PlatformOpenAI, Models: []string{"complete-model"}, BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output},
		{Platform: PlatformOpenAI, Models: []string{"incomplete-model"}, BillingMode: BillingModeToken, InputPrice: &input},
	}}
	channels := NewChannelService(nil, nil, nil, nil)
	channels.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{10: PlatformOpenAI}))
	svc := &ModelSquareService{
		groups:       &modelSquareGroupRepoStub{groups: []Group{{ID: 10, Name: "OpenAI", Platform: PlatformOpenAI, Status: StatusActive, RateMultiplier: 1}}},
		accounts:     &modelSquareAccountRepoStub{byGroup: map[int64][]*Account{10: {{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}}}},
		users:        &modelSquareUserRepoStub{user: &User{ID: 1}},
		rates:        &modelSquareRateRepoStub{rates: map[int64]float64{}},
		channels:     channels,
		catalog:      modelSquarePlatformCatalogStub{errors: map[string]error{PlatformOpenAI: errors.New("system catalog unavailable")}},
		resolver:     NewModelPricingResolver(channels, NewBillingService(&config.Config{}, nil)),
		cursorSecret: []byte("fallback-secret"),
	}

	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 50})
	require.NoError(t, err)
	require.Equal(t, []string{"complete-model"}, []string{result.Items[0].Name})
}

func TestModelSquareUsesChannelMappedModelForRestrictions(t *testing.T) {
	input, output := 3e-6, 6e-6
	channel := Channel{
		ID: 30, Status: StatusActive, GroupIDs: []int64{10}, RestrictModels: true,
		ModelMapping: map[string]map[string]string{PlatformOpenAI: {"public-model": "provider-model"}},
		ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"public-model"}, BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output}},
	}
	channels := NewChannelService(nil, nil, nil, nil)
	channels.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{10: PlatformOpenAI}))
	account := &Account{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"public-model": "provider-model"}}}
	svc := &ModelSquareService{
		groups:       &modelSquareGroupRepoStub{groups: []Group{{ID: 10, Name: "OpenAI", Platform: PlatformOpenAI, Status: StatusActive, RateMultiplier: 1}}},
		accounts:     &modelSquareAccountRepoStub{byGroup: map[int64][]*Account{10: {account}}},
		users:        &modelSquareUserRepoStub{user: &User{ID: 1}},
		rates:        &modelSquareRateRepoStub{rates: map[int64]float64{}},
		channels:     channels,
		catalog:      modelSquareCatalogStub{snapshot: ModelCatalogSnapshot{Provider: "system", Checksum: "restriction-v1", UpdatedAt: time.Now(), Models: []string{"public-model"}}},
		resolver:     NewModelPricingResolver(channels, NewBillingService(&config.Config{}, nil)),
		cursorSecret: []byte("restriction-secret"),
	}

	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 50})
	require.NoError(t, err)
	require.Empty(t, result.Items, "the gateway restricts the mapped billing model, so it must not be advertised")
}

func TestModelSquareUsesAccountUpstreamModelForRestrictions(t *testing.T) {
	input, output := 3e-6, 6e-6
	channel := Channel{
		ID: 31, Status: StatusActive, GroupIDs: []int64{10}, RestrictModels: true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing:       []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"other-provider-model"}, BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output}},
	}
	channels := NewChannelService(nil, nil, nil, nil)
	channels.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{10: PlatformOpenAI}))
	pricing := NewPricingService(&config.Config{}, nil)
	pricing.pricingData = map[string]*LiteLLMModelPricing{
		"public-model": {InputCostPerToken: input, OutputCostPerToken: output, LiteLLMProvider: "openai"},
	}
	pricing.localHash = "upstream-restriction-v1"
	svc := &ModelSquareService{
		groups: &modelSquareGroupRepoStub{groups: []Group{{ID: 10, Name: "OpenAI", Platform: PlatformOpenAI, Status: StatusActive, RateMultiplier: 1}}},
		accounts: &modelSquareAccountRepoStub{byGroup: map[int64][]*Account{10: {{
			ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{"public-model": "blocked-provider-model"}},
		}}}},
		users:        &modelSquareUserRepoStub{user: &User{ID: 1}},
		rates:        &modelSquareRateRepoStub{rates: map[int64]float64{}},
		channels:     channels,
		catalog:      NewPricingServiceModelCatalog(pricing),
		resolver:     NewModelPricingResolver(channels, NewBillingService(&config.Config{}, pricing)),
		cursorSecret: []byte("upstream-restriction-secret"),
	}

	result, err := svc.ListModels(context.Background(), 1, 10, ModelSquareModelsQuery{PageSize: 50})
	require.NoError(t, err)
	require.Empty(t, result.Items, "upstream billing restrictions must use the account's mapped provider model")
}
