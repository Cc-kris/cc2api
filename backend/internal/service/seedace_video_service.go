package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultSeedaceVideoDurationSeconds = 4
	minSeedaceVideoDurationSeconds     = 4
	seedaceVideoGenerationsEndpoint    = "/video/generations"
)

// SeedaceVideoService provides URL-relay forwarding for Seedace video APIs.
type SeedaceVideoService struct {
	accountRepo           AccountRepository
	usageLogRepo          UsageLogRepository
	usageBillingRepo      UsageBillingRepository
	userRepo              UserRepository
	userSubRepo           UserSubscriptionRepository
	channelService        *ChannelService
	billingService        *BillingService
	modelPricingResolver  *ModelPricingResolver
	apiKeyService         *APIKeyService
	billingCacheService   *BillingCacheService
	deferredService       *DeferredService
	balanceNotifyService  *BalanceNotifyService
	userPlatformQuotaRepo UserPlatformQuotaRepository
	httpUpstream          HTTPUpstream
	cfg                   *config.Config
}

type SeedaceVideoCreateInput struct {
	APIKey       *APIKey
	Subscription *UserSubscription
	Body         []byte
	Headers      http.Header
	UserAgent    string
	IPAddress    string
}

type SeedaceVideoPollInput struct {
	APIKey    *APIKey
	TaskID    string
	Headers   http.Header
	UserAgent string
	IPAddress string
}

type SeedaceVideoResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type seedaceVideoTaskUsageLookup interface {
	GetSeedaceVideoByTaskID(ctx context.Context, apiKeyID int64, taskID string) (*UsageLog, error)
}

// NewSeedaceVideoService creates the Seedace video relay service.
func NewSeedaceVideoService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	usageBillingRepo UsageBillingRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	channelService *ChannelService,
	billingService *BillingService,
	modelPricingResolver *ModelPricingResolver,
	apiKeyService *APIKeyService,
	billingCacheService *BillingCacheService,
	deferredService *DeferredService,
	balanceNotifyService *BalanceNotifyService,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *SeedaceVideoService {
	return &SeedaceVideoService{
		accountRepo:           accountRepo,
		usageLogRepo:          usageLogRepo,
		usageBillingRepo:      usageBillingRepo,
		userRepo:              userRepo,
		userSubRepo:           userSubRepo,
		channelService:        channelService,
		billingService:        billingService,
		modelPricingResolver:  modelPricingResolver,
		apiKeyService:         apiKeyService,
		billingCacheService:   billingCacheService,
		deferredService:       deferredService,
		balanceNotifyService:  balanceNotifyService,
		userPlatformQuotaRepo: userPlatformQuotaRepo,
		httpUpstream:          httpUpstream,
		cfg:                   cfg,
	}
}

func (s *SeedaceVideoService) Create(ctx context.Context, input SeedaceVideoCreateInput) (*SeedaceVideoResult, error) {
	if s == nil {
		return nil, errors.New("seedace video service is nil")
	}
	if len(input.Body) == 0 {
		return nil, errors.New("request body is empty")
	}
	request, err := parseSeedaceVideoRequest(input.Body)
	if err != nil {
		return nil, err
	}
	account, err := s.selectAccount(ctx, input.APIKey)
	if err != nil {
		return nil, err
	}
	if err := s.ensureCreatePricingConfigured(ctx, input.APIKey, request); err != nil {
		return nil, err
	}
	resp, err := s.forward(ctx, account, http.MethodPost, seedaceVideoGenerationsEndpoint, input.Headers, input.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusBadRequest {
		if err := s.recordCreateUsage(ctx, account, input, request, resp.Body); err != nil {
			slog.Error("seedace video usage billing failed", "account_id", account.ID, "error", err)
			s.writeUnbilledCreateUsage(ctx, account, input, request, resp.Body, err)
		}
	}
	return resp, nil
}

func (s *SeedaceVideoService) Poll(ctx context.Context, input SeedaceVideoPollInput) (*SeedaceVideoResult, error) {
	if s == nil {
		return nil, errors.New("seedace video service is nil")
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	account, err := s.selectPollAccount(ctx, input.APIKey, taskID)
	if err != nil {
		return nil, err
	}
	return s.forward(ctx, account, http.MethodGet, seedaceVideoGenerationsEndpoint+"/"+url.PathEscape(taskID), input.Headers, nil)
}

func (s *SeedaceVideoService) selectPollAccount(ctx context.Context, apiKey *APIKey, taskID string) (*Account, error) {
	if apiKey != nil && s.usageLogRepo != nil {
		if lookup, ok := s.usageLogRepo.(seedaceVideoTaskUsageLookup); ok {
			log, err := lookup.GetSeedaceVideoByTaskID(ctx, apiKey.ID, taskID)
			if err == nil && log != nil && log.AccountID > 0 && s.accountRepo != nil {
				account, accountErr := s.accountRepo.GetByID(ctx, log.AccountID)
				if accountErr == nil && account != nil && account.IsSeedaceAPIKey() && account.IsSeedaceURLRelayEnabled() && account.GetSeedaceBaseURL() != "" && account.GetSeedaceAPIKey() != "" {
					return account, nil
				}
				if accountErr != nil {
					slog.Warn("seedace video poll sticky account lookup failed", "task_id", taskID, "account_id", log.AccountID, "error", accountErr)
				}
				return nil, errors.New("seedace video task upstream account is no longer available")
			} else if err != nil && !errors.Is(err, ErrUsageLogNotFound) {
				slog.Warn("seedace video poll task usage lookup failed", "task_id", taskID, "api_key_id", apiKey.ID, "error", err)
			}
		}
	}
	return s.selectAccount(ctx, apiKey)
}

func (s *SeedaceVideoService) selectAccount(ctx context.Context, apiKey *APIKey) (*Account, error) {
	if s.accountRepo == nil {
		return nil, errors.New("account repository is nil")
	}
	var (
		accounts []Account
		err      error
	)
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformSeedace)
	} else if apiKey != nil && apiKey.GroupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *apiKey.GroupID, PlatformSeedace)
	} else {
		accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, PlatformSeedace)
	}
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		account := accounts[i]
		if account.IsSeedaceAPIKey() && account.IsSeedaceURLRelayEnabled() && account.GetSeedaceBaseURL() != "" && account.GetSeedaceAPIKey() != "" {
			return &account, nil
		}
	}
	return nil, errors.New("no available seedace url relay account")
}

func (s *SeedaceVideoService) ensureCreatePricingConfigured(ctx context.Context, apiKey *APIKey, req seedaceVideoRequest) error {
	if apiKey == nil {
		return errors.New("api key is required")
	}
	model, _ := normalizeSeedaceVideoMeter(req)
	resolved := s.resolvePricing(ctx, apiKey.GroupID, model)
	if resolved == nil || resolved.Mode != BillingModePerSecond || resolved.DefaultPerRequestPrice <= 0 {
		return fmt.Errorf("seedace per-second pricing is not configured for model %s", model)
	}
	return nil
}

func (s *SeedaceVideoService) forward(ctx context.Context, account *Account, method, path string, headers http.Header, body []byte) (*SeedaceVideoResult, error) {
	if account == nil {
		return nil, errors.New("account is nil")
	}
	if s.httpUpstream == nil {
		return nil, errors.New("http upstream is nil")
	}
	upstreamURL, err := joinSeedaceURL(account.GetSeedaceBaseURL(), path)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, upstreamURL, reader)
	if err != nil {
		return nil, err
	}
	copySeedaceForwardHeaders(req.Header, headers)
	req.Header.Set("Authorization", "Bearer "+account.GetSeedaceAPIKey())
	if len(body) > 0 && strings.TrimSpace(req.Header.Get("Content-Type")) == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	return &SeedaceVideoResult{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}, nil
}

func (s *SeedaceVideoService) recordCreateUsage(ctx context.Context, account *Account, input SeedaceVideoCreateInput, req seedaceVideoRequest, upstreamBody []byte) error {
	if account == nil || input.APIKey == nil || input.APIKey.User == nil {
		return errors.New("seedace usage context is incomplete")
	}
	model, durationSeconds := normalizeSeedaceVideoMeter(req)
	groupID := input.APIKey.GroupID
	resolved := s.resolvePricing(ctx, groupID, model)
	unitPrice := 0.0
	if resolved != nil {
		unitPrice = resolved.DefaultPerRequestPrice
	}
	if unitPrice < 0 {
		unitPrice = 0
	}
	groupMultiplier := 1.0
	if input.APIKey.Group != nil {
		groupMultiplier = input.APIKey.Group.RateMultiplier
	}
	if groupMultiplier <= 0 {
		groupMultiplier = 1.0
	}
	accountRateMultiplier := account.BillingRateMultiplier()
	totalCost := unitPrice * float64(durationSeconds)
	actualCost := totalCost * groupMultiplier
	cost := &CostBreakdown{
		TotalCost:   totalCost,
		ActualCost:  actualCost,
		BillingMode: string(BillingModePerSecond),
	}

	taskID := extractSeedaceTaskID(upstreamBody)
	requestID := resolveUsageBillingRequestID(ctx, taskID)
	durationMs := 0
	billingType := BillingTypeBalance
	if input.Subscription != nil && input.APIKey.Group != nil && input.APIKey.Group.IsSubscriptionType() {
		billingType = BillingTypeSubscription
	}
	billingMode := string(BillingModePerSecond)
	inboundEndpoint := "/v1/video/generations"
	upstreamEndpoint := seedaceVideoGenerationsEndpoint
	videoDuration := durationSeconds
	videoTaskID := taskID
	accountStatsCost := totalCost * accountRateMultiplier
	usageLog := &UsageLog{
		UserID:                input.APIKey.User.ID,
		APIKeyID:              input.APIKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 model,
		RequestedModel:        model,
		UpstreamModel:         &model,
		GroupID:               groupID,
		SubscriptionID:        subscriptionIDPtr(input.Subscription),
		InputTokens:           0,
		OutputTokens:          0,
		TotalCost:             totalCost,
		ActualCost:            actualCost,
		RateMultiplier:        groupMultiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		RequestType:           RequestTypeSync,
		DurationMs:            &durationMs,
		UserAgent:             stringPtrIfNotEmpty(input.UserAgent),
		IPAddress:             stringPtrIfNotEmpty(input.IPAddress),
		VideoDurationSeconds:  &videoDuration,
		VideoTaskID:           stringPtrIfNotEmpty(videoTaskID),
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		ChannelID:             s.channelID(ctx, groupID),
		BillingMode:           &billingMode,
		AccountStatsCost:      &accountStatsCost,
		CreatedAt:             time.Now(),
	}

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.seedace_video")
		if s.deferredService != nil {
			s.deferredService.ScheduleLastUsedUpdate(account.ID)
		}
		return nil
	}

	_, err := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
		Cost:                  cost,
		User:                  input.APIKey.User,
		APIKey:                input.APIKey,
		Account:               account,
		Subscription:          input.Subscription,
		RequestPayloadHash:    HashUsageRequestPayload(input.Body),
		IsSubscriptionBill:    billingType == BillingTypeSubscription,
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         s.apiKeyService,
		Platform:              PlatformSeedace,
	}, s.billingDeps(), s.usageBillingRepo)
	if err != nil {
		return err
	}
	return nil
}

func (s *SeedaceVideoService) writeUnbilledCreateUsage(ctx context.Context, account *Account, input SeedaceVideoCreateInput, req seedaceVideoRequest, upstreamBody []byte, billingErr error) {
	if s == nil || s.usageLogRepo == nil || account == nil || input.APIKey == nil || input.APIKey.User == nil {
		return
	}
	model, durationSeconds := normalizeSeedaceVideoMeter(req)
	groupID := input.APIKey.GroupID
	taskID := extractSeedaceTaskID(upstreamBody)
	requestID := resolveUsageBillingRequestID(ctx, taskID)
	durationMs := 0
	billingMode := string(BillingModePerSecond)
	inboundEndpoint := "/v1/video/generations"
	upstreamEndpoint := seedaceVideoGenerationsEndpoint
	videoDuration := durationSeconds
	usageLog := &UsageLog{
		UserID:               input.APIKey.User.ID,
		APIKeyID:             input.APIKey.ID,
		AccountID:            account.ID,
		RequestID:            requestID,
		Model:                model,
		RequestedModel:       model,
		UpstreamModel:        &model,
		GroupID:              groupID,
		SubscriptionID:       subscriptionIDPtr(input.Subscription),
		BillingType:          BillingTypeBalance,
		RequestType:          RequestTypeSync,
		DurationMs:           &durationMs,
		UserAgent:            stringPtrIfNotEmpty(input.UserAgent),
		IPAddress:            stringPtrIfNotEmpty(input.IPAddress),
		VideoDurationSeconds: &videoDuration,
		VideoTaskID:          stringPtrIfNotEmpty(taskID),
		InboundEndpoint:      &inboundEndpoint,
		UpstreamEndpoint:     &upstreamEndpoint,
		ChannelID:            s.channelID(ctx, groupID),
		BillingMode:          &billingMode,
		CreatedAt:            time.Now(),
	}
	if input.Subscription != nil && input.APIKey.Group != nil && input.APIKey.Group.IsSubscriptionType() {
		usageLog.BillingType = BillingTypeSubscription
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.seedace_video.unbilled")
	slog.Error("seedace video usage left unbilled after upstream success", "account_id", account.ID, "api_key_id", input.APIKey.ID, "task_id", taskID, "error", billingErr)
}

func (s *SeedaceVideoService) resolvePricing(ctx context.Context, groupID *int64, model string) *ResolvedPricing {
	if s.modelPricingResolver != nil {
		resolved := s.modelPricingResolver.Resolve(ctx, PricingInput{Model: model, GroupID: groupID})
		if resolved != nil && resolved.Mode == BillingModePerSecond {
			return resolved
		}
	}
	if groupID != nil && s.channelService != nil {
		if pricing := s.channelService.GetChannelModelPricing(ctx, *groupID, model); pricing != nil && pricing.PerRequestPrice != nil {
			return &ResolvedPricing{
				Mode:                   BillingModePerSecond,
				Source:                 PricingSourceChannel,
				DefaultPerRequestPrice: *pricing.PerRequestPrice,
			}
		}
	}
	return &ResolvedPricing{Mode: BillingModePerSecond, Source: PricingSourceFallback}
}

func (s *SeedaceVideoService) channelID(ctx context.Context, groupID *int64) *int64 {
	if groupID == nil || s.channelService == nil {
		return nil
	}
	channel, err := s.channelService.GetChannelForGroup(ctx, *groupID)
	if err != nil || channel == nil {
		return nil
	}
	return &channel.ID
}

func (s *SeedaceVideoService) billingDeps() *billingDeps {
	return &billingDeps{
		accountRepo:           s.accountRepo,
		userRepo:              s.userRepo,
		userSubRepo:           s.userSubRepo,
		billingCacheService:   s.billingCacheService,
		deferredService:       s.deferredService,
		balanceNotifyService:  s.balanceNotifyService,
		userPlatformQuotaRepo: s.userPlatformQuotaRepo,
	}
}

type seedaceVideoRequest struct {
	Model    string          `json:"model"`
	Duration int             `json:"duration"`
	Seconds  json.RawMessage `json:"seconds"`
}

func parseSeedaceVideoRequest(body []byte) (seedaceVideoRequest, error) {
	var req seedaceVideoRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, fmt.Errorf("invalid json body: %w", err)
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Duration < 0 {
		return req, errors.New("duration must be >= 0")
	}
	if len(req.Seconds) > 0 {
		seconds, ok, err := parseSeedaceVideoSeconds(req.Seconds)
		if err != nil {
			return req, err
		}
		if ok {
			req.Duration = seconds
		}
	}
	return req, nil
}

func normalizeSeedaceVideoMeter(req seedaceVideoRequest) (string, int) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "seedance-2.0-720"
	}
	durationSeconds := req.Duration
	if durationSeconds <= 0 {
		durationSeconds = defaultSeedaceVideoDurationSeconds
	}
	if durationSeconds < minSeedaceVideoDurationSeconds {
		durationSeconds = minSeedaceVideoDurationSeconds
	}
	return model, durationSeconds
}

func parseSeedaceVideoSeconds(raw json.RawMessage) (int, bool, error) {
	if string(bytes.TrimSpace(raw)) == "null" {
		return 0, false, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return 0, false, nil
		}
		seconds, convErr := strconv.Atoi(text)
		if convErr != nil {
			return 0, false, fmt.Errorf("seconds must be an integer string: %w", convErr)
		}
		if seconds < 0 {
			return 0, false, errors.New("seconds must be >= 0")
		}
		return seconds, true, nil
	}

	var seconds int
	if err := json.Unmarshal(raw, &seconds); err != nil {
		return 0, false, fmt.Errorf("seconds must be an integer string or number: %w", err)
	}
	if seconds < 0 {
		return 0, false, errors.New("seconds must be >= 0")
	}
	return seconds, true, nil
}

func joinSeedaceURL(baseURL, path string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("seedace base_url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return "", fmt.Errorf("invalid seedace base_url: %w", err)
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + path, nil
	}
	return baseURL + "/v1" + path, nil
}

func copySeedaceForwardHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if shouldSkipSeedaceForwardHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func shouldSkipSeedaceForwardHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization", "host", "content-length", "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func extractSeedaceTaskID(body []byte) string {
	var payload any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	return firstStringByKeys(payload, "task_id", "id")
}

func firstStringByKeys(value any, keys ...string) string {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if raw, ok := v[key].(string); ok && strings.TrimSpace(raw) != "" {
				return strings.TrimSpace(raw)
			}
		}
		for _, child := range v {
			if found := firstStringByKeys(child, keys...); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range v {
			if found := firstStringByKeys(child, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func subscriptionIDPtr(sub *UserSubscription) *int64 {
	if sub == nil {
		return nil
	}
	return &sub.ID
}

func stringPtrIfNotEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
