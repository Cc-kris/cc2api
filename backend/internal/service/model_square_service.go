package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	gocache "github.com/patrickmn/go-cache"
	"github.com/shopspring/decimal"
)

const modelSquareCacheTTL = time.Minute

var (
	ErrModelSquareGroupNotVisible = infraerrors.NotFound("GROUP_NOT_VISIBLE", "group is not visible")
	ErrModelSquareInvalidQuery    = infraerrors.BadRequest("MODEL_SQUARE_QUERY_INVALID", "invalid model square query")
	ErrModelSquareInvalidCursor   = infraerrors.BadRequest("INVALID_CURSOR", "invalid cursor")
	ErrModelSquareCatalogChanged  = infraerrors.Conflict("CATALOG_CHANGED", "model catalog changed")
)

type ModelSquareGroupItem struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Platform            string `json:"platform"`
	SubscriptionType    string `json:"subscription_type"`
	DefaultMultiplier   string `json:"default_multiplier"`
	EffectiveMultiplier string `json:"effective_multiplier"`
	HasCustomMultiplier bool   `json:"has_custom_multiplier"`
	ModelCount          int    `json:"model_count"`
}

type ModelSquareGroupsResult struct {
	Groups           []ModelSquareGroupItem `json:"groups"`
	CatalogUpdatedAt time.Time              `json:"catalog_updated_at"`
}

type ModelSquareModelItem struct {
	Name          string                 `json:"name"`
	BillingMode   BillingMode            `json:"billing_mode"`
	PricingSource string                 `json:"pricing_source"`
	Prices        ModelSquarePriceFields `json:"prices"`
	FastPrices    *FastPriceView         `json:"fast_prices"`
	Tiers         []TierPriceView        `json:"tiers"`
}

// ModelSquarePriceFields is the customer-facing regular service-tier price
// whitelist. Fast and tier prices are emitted as sibling fields.
type ModelSquarePriceFields struct {
	Input        *DecimalUnitPrice `json:"input"`
	Output       *DecimalUnitPrice `json:"output"`
	CacheRead    *DecimalUnitPrice `json:"cache_read"`
	CacheWrite5m *DecimalUnitPrice `json:"cache_write_5m"`
	CacheWrite1h *DecimalUnitPrice `json:"cache_write_1h"`
	ImageOutput  *DecimalUnitPrice `json:"image_output,omitempty"`
	PerRequest   *DecimalUnitPrice `json:"per_request,omitempty"`
	PerSecond    *DecimalUnitPrice `json:"per_second,omitempty"`
}

type ModelSquareModelsResult struct {
	GroupID             int64                  `json:"group_id"`
	GroupName           string                 `json:"group_name"`
	EffectiveMultiplier string                 `json:"effective_multiplier"`
	Items               []ModelSquareModelItem `json:"items"`
	NextCursor          *string                `json:"next_cursor"`
	CatalogUpdatedAt    time.Time              `json:"catalog_updated_at"`
}

type ModelSquareModelsQuery struct {
	Search           string
	Cursor           string
	PageSize         int
	CatalogUpdatedAt *time.Time
}

type modelSquareCursor struct {
	GroupID  int64  `json:"g"`
	Offset   int    `json:"o"`
	Search   string `json:"q"`
	Checksum string `json:"v"`
}

type modelSquareCacheValue struct {
	items    []ModelSquareModelItem
	snapshot ModelCatalogSnapshot
}

type modelSquareMetrics struct {
	requests           atomic.Uint64
	errors             atomic.Uint64
	cacheHits          atomic.Uint64
	cacheMisses        atomic.Uint64
	totalLatencyMicros atomic.Uint64
}

type ModelSquareMetricsSnapshot struct {
	Requests           uint64
	Errors             uint64
	CacheHits          uint64
	CacheMisses        uint64
	TotalLatencyMicros uint64
}

type ModelSquareService struct {
	groups       GroupRepository
	accounts     AccountRepository
	users        UserRepository
	rates        UserGroupRateRepository
	channels     *ChannelService
	catalog      ModelCatalogProvider
	resolver     *ModelPricingResolver
	settings     *SettingService
	cursorSecret []byte
	cache        *gocache.Cache
	metrics      modelSquareMetrics
}

func NewModelSquareService(
	groups GroupRepository,
	accounts AccountRepository,
	users UserRepository,
	rates UserGroupRateRepository,
	channels *ChannelService,
	pricing *PricingService,
	resolver *ModelPricingResolver,
	settings *SettingService,
	cfg *config.Config,
) *ModelSquareService {
	secret := []byte("model-square-local-cursor")
	if cfg != nil && strings.TrimSpace(cfg.JWT.Secret) != "" {
		secret = []byte(cfg.JWT.Secret)
	}
	return &ModelSquareService{
		groups: groups, accounts: accounts, users: users, rates: rates,
		channels: channels, catalog: NewPricingServiceModelCatalog(pricing),
		resolver: resolver, settings: settings, cursorSecret: secret,
		cache: gocache.New(modelSquareCacheTTL, time.Minute),
	}
}

func (s *ModelSquareService) ListGroups(ctx context.Context, userID int64) (result *ModelSquareGroupsResult, err error) {
	startedAt := time.Now()
	s.metrics.requests.Add(1)
	defer func() {
		s.metrics.totalLatencyMicros.Add(uint64(time.Since(startedAt).Microseconds()))
		if err != nil {
			s.metrics.errors.Add(1)
		}
	}()
	return s.listGroups(ctx, userID)
}

func (s *ModelSquareService) listGroups(ctx context.Context, userID int64) (*ModelSquareGroupsResult, error) {
	groups, rates, _, err := s.visibleGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]int64, 0, len(groups))
	for i := range groups {
		groupIDs = append(groupIDs, groups[i].ID)
	}
	accountRepo, ok := s.accounts.(ModelSquareAccountRepository)
	if !ok {
		return nil, errors.New("model square account repository is unavailable")
	}
	accounts, err := accountRepo.ListCatalogEligibleByGroupIDs(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	result := &ModelSquareGroupsResult{Groups: make([]ModelSquareGroupItem, 0, len(groups))}
	for i := range groups {
		group := &groups[i]
		multiplier, custom := effectiveGroupMultiplier(group, rates)
		models, snapshot, modelErr := s.listSellableModels(ctx, userID, group, accounts[group.ID], multiplier)
		if modelErr == nil && snapshot.UpdatedAt.After(result.CatalogUpdatedAt) {
			result.CatalogUpdatedAt = snapshot.UpdatedAt
		}
		modelCount := 0
		if modelErr == nil {
			modelCount = len(models)
		}
		result.Groups = append(result.Groups, ModelSquareGroupItem{
			ID: group.ID, Name: group.Name, Platform: group.Platform,
			SubscriptionType:    group.SubscriptionType,
			DefaultMultiplier:   decimal.NewFromFloat(group.RateMultiplier).StringFixed(4),
			EffectiveMultiplier: multiplier.StringFixed(4), HasCustomMultiplier: custom,
			ModelCount: modelCount,
		})
	}
	return result, nil
}

func (s *ModelSquareService) ListModels(ctx context.Context, userID, groupID int64, query ModelSquareModelsQuery) (result *ModelSquareModelsResult, err error) {
	startedAt := time.Now()
	s.metrics.requests.Add(1)
	defer func() {
		s.metrics.totalLatencyMicros.Add(uint64(time.Since(startedAt).Microseconds()))
		if err != nil {
			s.metrics.errors.Add(1)
		}
	}()
	return s.listModels(ctx, userID, groupID, query)
}

func (s *ModelSquareService) listModels(ctx context.Context, userID, groupID int64, query ModelSquareModelsQuery) (*ModelSquareModelsResult, error) {
	if len([]rune(query.Search)) > 100 {
		return nil, ErrModelSquareInvalidQuery
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize < 1 || query.PageSize > 200 {
		return nil, ErrModelSquareInvalidQuery
	}
	groups, rates, _, err := s.visibleGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	var group *Group
	for i := range groups {
		if groups[i].ID == groupID {
			group = &groups[i]
			break
		}
	}
	if group == nil {
		return nil, ErrModelSquareGroupNotVisible
	}
	accountRepo, ok := s.accounts.(ModelSquareAccountRepository)
	if !ok {
		return nil, errors.New("model square account repository is unavailable")
	}
	byGroup, err := accountRepo.ListCatalogEligibleByGroupIDs(ctx, []int64{groupID})
	if err != nil {
		return nil, err
	}
	multiplier, _ := effectiveGroupMultiplier(group, rates)
	models, snapshot, err := s.listSellableModels(ctx, userID, group, byGroup[groupID], multiplier)
	if err != nil {
		return nil, err
	}
	if query.CatalogUpdatedAt != nil && !query.CatalogUpdatedAt.Equal(snapshot.UpdatedAt) {
		return nil, ErrModelSquareCatalogChanged
	}
	search := strings.ToLower(strings.TrimSpace(query.Search))
	filtered := make([]ModelSquareModelItem, 0, len(models))
	for _, model := range models {
		if search == "" || strings.Contains(strings.ToLower(model.Name), search) {
			filtered = append(filtered, model)
		}
	}
	offset := 0
	if query.Cursor != "" {
		cursor, decodeErr := s.decodeCursor(query.Cursor)
		if decodeErr != nil || cursor.GroupID != groupID || cursor.Search != search {
			return nil, ErrModelSquareInvalidCursor
		}
		if cursor.Checksum != snapshot.Checksum {
			return nil, ErrModelSquareCatalogChanged
		}
		offset = cursor.Offset
	}
	if offset < 0 || offset > len(filtered) {
		return nil, ErrModelSquareInvalidCursor
	}
	end := offset + query.PageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	items := append([]ModelSquareModelItem(nil), filtered[offset:end]...)
	var next *string
	if end < len(filtered) {
		encoded, encodeErr := s.encodeCursor(modelSquareCursor{GroupID: groupID, Offset: end, Search: search, Checksum: snapshot.Checksum})
		if encodeErr != nil {
			return nil, encodeErr
		}
		next = &encoded
	}
	return &ModelSquareModelsResult{GroupID: group.ID, GroupName: group.Name, EffectiveMultiplier: multiplier.StringFixed(4), Items: items, NextCursor: next, CatalogUpdatedAt: snapshot.UpdatedAt}, nil
}

func (s *ModelSquareService) visibleGroups(ctx context.Context, userID int64) ([]Group, map[int64]float64, map[int64]struct{}, error) {
	groups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	rates, err := s.rates.GetByUserID(ctx, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	allowed := make(map[int64]struct{})
	visible := make([]Group, 0, len(groups))
	for i := range groups {
		if !groups[i].IsExclusive {
			visible = append(visible, groups[i])
		}
	}
	sort.SliceStable(visible, func(i, j int) bool {
		if visible[i].SortOrder != visible[j].SortOrder {
			return visible[i].SortOrder < visible[j].SortOrder
		}
		if visible[i].Name != visible[j].Name {
			return visible[i].Name < visible[j].Name
		}
		return visible[i].ID < visible[j].ID
	})
	return visible, rates, allowed, nil
}

func effectiveGroupMultiplier(group *Group, rates map[int64]float64) (decimal.Decimal, bool) {
	if value, ok := rates[group.ID]; ok {
		return decimal.NewFromFloat(value), true
	}
	return decimal.NewFromFloat(group.RateMultiplier), false
}

func (s *ModelSquareService) listSellableModels(ctx context.Context, userID int64, group *Group, accounts []*Account, multiplier decimal.Decimal) ([]ModelSquareModelItem, ModelCatalogSnapshot, error) {
	var channel *Channel
	var err error
	if s.channels != nil {
		channel, err = s.channels.GetChannelForGroup(ctx, group.ID)
		if err != nil {
			return nil, ModelCatalogSnapshot{}, err
		}
	}
	snapshot, catalogErr := s.catalog.SnapshotForPlatform(group.Platform)
	usingCompleteChannelFallback := false
	if catalogErr != nil {
		// System catalog failure can be masked only when the channel itself has
		// complete sell-side prices for every returned model. Incomplete channel
		// prices require the system catalog as a pricing dependency and must keep
		// the original failure visible to callers.
		configuredModels := modelSquareCompleteChannelModels(channel, group.Platform)
		if len(configuredModels) == 0 {
			return nil, snapshot, catalogErr
		}
		slog.Warn("model_square.catalog_fallback_to_complete_channel_pricing",
			"group_id", group.ID,
			"platform", group.Platform,
			"channel_id", channel.ID,
			"error", catalogErr,
		)
		snapshot = ModelCatalogSnapshot{
			Provider: "channel", Checksum: fmt.Sprintf("channel:%d:%d", channel.ID, channel.UpdatedAt.UnixNano()),
			UpdatedAt: channel.UpdatedAt, Models: configuredModels,
		}
		usingCompleteChannelFallback = true
	}
	fastPolicy := DefaultOpenAIFastPolicySettings()
	if s.settings != nil {
		loadedPolicy, policyErr := s.settings.GetOpenAIFastPolicySettings(ctx)
		if policyErr != nil {
			fastPolicy = nil
		} else {
			fastPolicy = loadedPolicy
		}
	}
	cacheKey, err := modelSquareCacheKey(userID, group, accounts, channel, multiplier, snapshot.Checksum, fastPolicy)
	if err != nil {
		return nil, snapshot, err
	}
	if s.cache != nil {
		if cached, found := s.cache.Get(cacheKey); found {
			value, valid := cached.(modelSquareCacheValue)
			if valid {
				s.metrics.cacheHits.Add(1)
				return append([]ModelSquareModelItem(nil), value.items...), value.snapshot, nil
			}
			s.cache.Delete(cacheKey)
		}
		s.metrics.cacheMisses.Add(1)
	}
	accountRestrictionConfigured := modelSquareAccountRestrictionConfigured(accounts)
	candidates := append([]string(nil), snapshot.Models...)
	if accountRestrictionConfigured {
		// Once any account in the group has an explicit model restriction list,
		// that list is the customer-visible boundary. Do not let an unrestricted
		// sibling account expand the model square back to the whole system catalog.
		candidates = modelSquareFilterByAccountRestrictions(candidates, accounts)
	}
	// Account model mappings are the first source of customer-visible model
	// names. Exact mapping keys can represent a restricted model that is not in
	// the synchronized upstream catalog; wildcard keys are evaluated against
	// the finite catalog below by modelRoutableForGroup.
	candidates = appendUniqueModelSquareNames(candidates, modelSquareAccountMappedModels(accounts))
	if !usingCompleteChannelFallback && channel != nil && channel.IsActive() {
		// The synchronized system catalog remains the primary model directory.
		// Complete channel-priced models may extend it with site-specific models,
		// but an explicit channel model list must never replace system models.
		channelModels := modelSquareCompleteChannelModels(channel, group.Platform)
		if accountRestrictionConfigured {
			channelModels = modelSquareFilterByAccountRestrictions(channelModels, accounts)
		}
		candidates = appendUniqueModelSquareNames(candidates, channelModels)
	}
	seen := make(map[string]struct{}, len(candidates))
	items := make([]ModelSquareModelItem, 0, len(candidates))
	for _, name := range candidates {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		target := name
		billingModelSource := BillingModelSourceRequested
		if s.channels != nil {
			mapping := s.channels.ResolveChannelMapping(ctx, group.ID, name)
			if mapping.Mapped {
				target = mapping.MappedModel
			}
			billingModelSource = mapping.BillingModelSource
			if billingModel := billingModelForRestriction(billingModelSource, name, target); billingModel != "" && s.channels.IsModelRestricted(ctx, group.ID, billingModel) {
				continue
			}
		}
		// Account selection is based on the customer-requested model. The channel
		// mapping only determines the provider route and billing restriction
		// target; it must not replace the account restriction-list key here.
		if !modelRoutableForGroup(group, accounts, name) {
			continue
		}
		if billingModelSource == BillingModelSourceUpstream && s.channels != nil && !modelSquareUpstreamRestrictionAllows(ctx, s.channels, group, accounts, name) {
			continue
		}
		gid := group.ID
		resolved, resolveErr := ResolveUnifiedSalesPricing(ctx, s.resolver, s.channels, &gid, name)
		if resolveErr != nil {
			continue
		}
		view, viewErr := BuildModelPriceView(resolved, multiplier)
		if viewErr != nil || !modelPriceViewHasPrice(view) {
			continue
		}
		if !modelSquareFastAllowed(fastPolicy, group, accounts, name) {
			view.Fast = nil
		}
		items = append(items, ModelSquareModelItem{
			Name:          name,
			BillingMode:   view.BillingMode,
			PricingSource: normalizeSalesPricingSource(resolved.Source),
			Prices: ModelSquarePriceFields{
				Input:        view.Input,
				Output:       view.Output,
				CacheRead:    view.CacheRead,
				CacheWrite5m: view.CacheWrite5m,
				CacheWrite1h: view.CacheWrite1h,
				ImageOutput:  view.ImageOutput,
				PerRequest:   view.PerRequest,
				PerSecond:    view.PerSecond,
			},
			FastPrices: view.Fast,
			Tiers:      view.Tiers,
		})
	}
	sort.Slice(items, func(i, j int) bool { return naturalModelLess(items[i].Name, items[j].Name) })
	if s.cache != nil {
		s.cache.Set(cacheKey, modelSquareCacheValue{items: append([]ModelSquareModelItem(nil), items...), snapshot: snapshot}, modelSquareCacheTTL)
	}
	return items, snapshot, nil
}

func modelSquareCompleteChannelModels(channel *Channel, platform string) []string {
	if channel == nil || !channel.IsActive() {
		return nil
	}
	models := make([]string, 0)
	for _, pricing := range channel.ModelPricing {
		if !isPlatformPricingMatch(platform, pricing.Platform) || !IsCompleteChannelSalesPricing(pricing) {
			continue
		}
		for _, name := range pricing.Models {
			name = strings.TrimSpace(name)
			if name != "" && !strings.Contains(name, "*") {
				models = append(models, name)
			}
		}
	}
	return models
}

func appendUniqueModelSquareNames(existing, additional []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(additional))
	for _, name := range existing {
		key := strings.ToLower(strings.TrimSpace(name))
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, name := range additional {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, strings.TrimSpace(name))
	}
	return existing
}

func modelSquareAccountMappedModels(accounts []*Account) []string {
	models := make([]string, 0)
	for _, account := range accounts {
		if account == nil {
			continue
		}
		for name := range account.GetModelMapping() {
			name = strings.TrimSpace(name)
			if name == "" || strings.ContainsAny(name, "*?") {
				continue
			}
			models = append(models, name)
		}
	}
	return models
}

func modelSquareAccountRestrictionConfigured(accounts []*Account) bool {
	for _, account := range accounts {
		if account != nil && len(account.GetModelMapping()) > 0 {
			return true
		}
	}
	return false
}

func modelSquareFilterByAccountRestrictions(models []string, accounts []*Account) []string {
	exactRestrictionsConfigured := modelSquareExactRestrictionsConfigured(accounts)
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		for _, account := range accounts {
			if account == nil {
				continue
			}
			mapping := account.GetModelMapping()
			if len(mapping) == 0 {
				continue
			}
			if exactRestrictionsConfigured {
				if modelSquareAccountHasExactModel(mapping, model) {
					filtered = append(filtered, model)
					break
				}
				continue
			}
			if account.IsModelSupported(model) {
				filtered = append(filtered, model)
				break
			}
		}
	}
	return filtered
}

func modelSquareExactRestrictionsConfigured(accounts []*Account) bool {
	for _, account := range accounts {
		if account == nil {
			continue
		}
		for model := range account.GetModelMapping() {
			if model != "" && !strings.ContainsAny(model, "*?") {
				return true
			}
		}
	}
	return false
}

func modelSquareAccountHasExactModel(mapping map[string]string, model string) bool {
	for configuredModel := range mapping {
		if strings.ContainsAny(configuredModel, "*?") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(configuredModel), strings.TrimSpace(model)) {
			return true
		}
	}
	return false
}

func modelSquareCacheKey(userID int64, group *Group, accounts []*Account, channel *Channel, multiplier decimal.Decimal, catalogChecksum string, fastPolicy *OpenAIFastPolicySettings) (string, error) {
	type accountIdentity struct {
		ID           int64             `json:"id"`
		UpdatedAt    int64             `json:"updated_at"`
		Status       string            `json:"status"`
		Type         string            `json:"type"`
		Schedulable  bool              `json:"schedulable"`
		ExpiresAt    int64             `json:"expires_at"`
		ModelMapping map[string]string `json:"model_mapping"`
	}
	accountVersions := make([]accountIdentity, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		expiresAt := int64(0)
		if account.ExpiresAt != nil {
			expiresAt = account.ExpiresAt.UnixNano()
		}
		accountVersions = append(accountVersions, accountIdentity{
			ID: account.ID, UpdatedAt: account.UpdatedAt.UnixNano(), Status: account.Status,
			Type: account.Type, Schedulable: account.Schedulable, ExpiresAt: expiresAt,
			ModelMapping: account.GetModelMapping(),
		})
	}
	sort.Slice(accountVersions, func(i, j int) bool { return accountVersions[i].ID < accountVersions[j].ID })
	type groupIdentity struct {
		ID               int64  `json:"id"`
		Platform         string `json:"platform"`
		UpdatedAt        int64  `json:"updated_at"`
		RequireOAuthOnly bool   `json:"require_oauth_only"`
		RequirePrivacy   bool   `json:"require_privacy"`
	}
	type channelIdentity struct {
		ID                 int64                        `json:"id"`
		Status             string                       `json:"status"`
		UpdatedAt          int64                        `json:"updated_at"`
		RestrictModels     bool                         `json:"restrict_models"`
		BillingModelSource string                       `json:"billing_model_source"`
		ModelMapping       map[string]map[string]string `json:"model_mapping"`
		ModelPricing       []ChannelModelPricing        `json:"model_pricing"`
	}
	var channelVersion *channelIdentity
	if channel != nil {
		channelVersion = &channelIdentity{
			ID: channel.ID, Status: channel.Status, UpdatedAt: channel.UpdatedAt.UnixNano(),
			RestrictModels: channel.RestrictModels, BillingModelSource: channel.BillingModelSource,
			ModelMapping: channel.ModelMapping, ModelPricing: channel.ModelPricing,
		}
	}
	material := struct {
		UserID          int64                     `json:"user_id"`
		Group           groupIdentity             `json:"group"`
		Multiplier      string                    `json:"multiplier"`
		CatalogChecksum string                    `json:"catalog_checksum"`
		Accounts        []accountIdentity         `json:"accounts"`
		Channel         *channelIdentity          `json:"channel"`
		FastPolicy      *OpenAIFastPolicySettings `json:"fast_policy"`
	}{
		UserID:     userID,
		Group:      groupIdentity{ID: group.ID, Platform: group.Platform, UpdatedAt: group.UpdatedAt.UnixNano(), RequireOAuthOnly: group.RequireOAuthOnly, RequirePrivacy: group.RequirePrivacySet},
		Multiplier: multiplier.StringFixed(4), CatalogChecksum: catalogChecksum,
		Accounts: accountVersions, Channel: channelVersion, FastPolicy: fastPolicy,
	}
	payload, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return "model-square:" + base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func modelSquareFastAllowed(settings *OpenAIFastPolicySettings, group *Group, accounts []*Account, model string) bool {
	if settings == nil || group == nil {
		return false
	}
	for _, account := range accounts {
		if !modelSquareAccountRoutable(group, account, model) {
			continue
		}
		action, _ := evaluateOpenAIFastPolicyWithSettings(settings, account, model, OpenAIFastTierPriority)
		if action == BetaPolicyActionPass {
			return true
		}
	}
	return false
}

func (s *ModelSquareService) InvalidateCache() {
	if s != nil && s.cache != nil {
		s.cache.Flush()
	}
}

func (s *ModelSquareService) SnapshotMetrics() ModelSquareMetricsSnapshot {
	if s == nil {
		return ModelSquareMetricsSnapshot{}
	}
	return ModelSquareMetricsSnapshot{
		Requests: s.metrics.requests.Load(), Errors: s.metrics.errors.Load(),
		CacheHits: s.metrics.cacheHits.Load(), CacheMisses: s.metrics.cacheMisses.Load(),
		TotalLatencyMicros: s.metrics.totalLatencyMicros.Load(),
	}
}

func modelRoutableForGroup(group *Group, accounts []*Account, model string) bool {
	for _, account := range accounts {
		if modelSquareAccountRoutable(group, account, model) {
			return true
		}
	}
	return false
}

func modelSquareUpstreamRestrictionAllows(ctx context.Context, channels *ChannelService, group *Group, accounts []*Account, requestedModel string) bool {
	if channels == nil || group == nil {
		return true
	}
	for _, account := range accounts {
		if !modelSquareAccountRoutable(group, account, requestedModel) {
			continue
		}
		upstreamModel := resolveAccountUpstreamModel(account, requestedModel)
		if upstreamModel == "" || !channels.IsModelRestricted(ctx, group.ID, upstreamModel) {
			return true
		}
	}
	return false
}

// modelSquareAccountRoutable is the request-shape-independent subset of the
// gateway's account eligibility contract. Transient runtime blocks such as
// short rate limits and overloads intentionally do not remove a model from the
// catalog; the live scheduler rechecks those conditions when a request starts.
func modelSquareAccountRoutable(group *Group, account *Account, model string) bool {
	if group == nil || account == nil || !account.IsActive() || !account.Schedulable {
		return false
	}
	if group.RequireOAuthOnly && account.Type == AccountTypeAPIKey {
		return false
	}
	if group.RequirePrivacySet && !account.IsPrivacySet() {
		return false
	}
	useMixed := group.Platform == PlatformAnthropic || group.Platform == PlatformGemini
	if !accountAllowedForPlatform(account, group.Platform, useMixed) {
		return false
	}
	return account.IsModelSupported(model)
}

func IsCompleteChannelSalesPricing(pricing ChannelModelPricing) bool {
	mode := pricing.BillingMode
	if mode == "" {
		mode = BillingModeToken
	}
	intervals := filterValidIntervals(pricing.Intervals)
	if len(intervals) > 0 {
		for _, interval := range intervals {
			switch mode {
			case BillingModeToken:
				if interval.InputPrice == nil || interval.OutputPrice == nil {
					return false
				}
			case BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond:
				if interval.PerRequestPrice == nil {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	switch mode {
	case BillingModeToken:
		return pricing.InputPrice != nil && pricing.OutputPrice != nil
	case BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond:
		return pricing.PerRequestPrice != nil
	default:
		return false
	}
}

func modelPriceViewHasPrice(view *ModelPriceView) bool {
	return view != nil && (view.Input != nil || view.Output != nil || view.CacheRead != nil || view.CacheWrite5m != nil || view.CacheWrite1h != nil || view.ImageOutput != nil || view.PerRequest != nil || view.PerSecond != nil || len(view.Tiers) > 0)
}

func (s *ModelSquareService) encodeCursor(cursor modelSquareCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.cursorSecret)
	_, _ = mac.Write(payload)
	data := append(payload, mac.Sum(nil)...)
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func (s *ModelSquareService) decodeCursor(raw string) (modelSquareCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(data) <= sha256.Size {
		return modelSquareCursor{}, errors.New("invalid cursor")
	}
	payload, signature := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	mac := hmac.New(sha256.New, s.cursorSecret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return modelSquareCursor{}, errors.New("invalid cursor")
	}
	var cursor modelSquareCursor
	if err = json.Unmarshal(payload, &cursor); err != nil {
		return modelSquareCursor{}, errors.New("invalid cursor")
	}
	return cursor, nil
}
