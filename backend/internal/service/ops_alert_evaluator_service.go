package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	opsAlertEvaluatorJobName = "ops_alert_evaluator"

	opsAlertEvaluatorTimeout         = 45 * time.Second
	opsAlertEvaluatorLeaderLockKey   = "ops:alert:evaluator:leader"
	opsAlertEvaluatorLeaderLockTTL   = 90 * time.Second
	opsAlertEvaluatorSkipLogInterval = 1 * time.Minute
)

var opsAlertEvaluatorReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type OpsAlertEvaluatorService struct {
	opsService   *OpsService
	opsRepo      OpsRepository
	emailService *EmailService

	redisClient *redis.Client
	cfg         *config.Config
	instanceID  string

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	mu         sync.Mutex
	ruleStates map[int64]*opsAlertRuleState

	emailLimiter *slidingWindowLimiter

	skipLogMu sync.Mutex
	skipLogAt time.Time

	warnNoRedisOnce sync.Once
}

type opsAlertRuleState struct {
	LastEvaluatedAt     time.Time
	ConsecutiveBreaches int
}

func NewOpsAlertEvaluatorService(
	opsService *OpsService,
	opsRepo OpsRepository,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsAlertEvaluatorService {
	return &OpsAlertEvaluatorService{
		opsService:   opsService,
		opsRepo:      opsRepo,
		emailService: emailService,
		redisClient:  redisClient,
		cfg:          cfg,
		instanceID:   uuid.NewString(),
		ruleStates:   map[int64]*opsAlertRuleState{},
		emailLimiter: newSlidingWindowLimiter(0, time.Hour),
	}
}

func (s *OpsAlertEvaluatorService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		s.wg.Add(1)
		go s.run()
	})
}

func (s *OpsAlertEvaluatorService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
	s.wg.Wait()
}

func (s *OpsAlertEvaluatorService) run() {
	defer s.wg.Done()

	// Start immediately to produce early feedback in ops dashboard.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			interval := s.getInterval()
			s.evaluateOnce(interval)
			timer.Reset(interval)
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpsAlertEvaluatorService) getInterval() time.Duration {
	// Default.
	interval := 60 * time.Second

	if s == nil || s.opsService == nil {
		return interval
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg, err := s.opsService.GetOpsAlertRuntimeSettings(ctx)
	if err != nil || cfg == nil {
		return interval
	}
	if cfg.EvaluationIntervalSeconds <= 0 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds < 1 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds > int((24 * time.Hour).Seconds()) {
		return interval
	}
	return time.Duration(cfg.EvaluationIntervalSeconds) * time.Second
}

func (s *OpsAlertEvaluatorService) evaluateOnce(interval time.Duration) {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAlertEvaluatorTimeout)
	defer cancel()

	if s.opsService != nil && !s.opsService.IsMonitoringEnabled(ctx) {
		return
	}

	runtimeCfg := defaultOpsAlertRuntimeSettings()
	if s.opsService != nil {
		if loaded, err := s.opsService.GetOpsAlertRuntimeSettings(ctx); err == nil && loaded != nil {
			runtimeCfg = loaded
		}
	}

	release, ok := s.tryAcquireLeaderLock(ctx, runtimeCfg.DistributedLock)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	startedAt := time.Now().UTC()
	runAt := startedAt

	rules, err := s.opsRepo.ListAlertRules(ctx)
	if err != nil {
		s.recordHeartbeatError(runAt, time.Since(startedAt), err)
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] list rules failed: %v", err)
		return
	}

	rulesTotal := len(rules)
	rulesEnabled := 0
	rulesEvaluated := 0
	eventsCreated := 0
	eventsMerged := 0
	eventsResolved := 0
	emailsSent := 0

	now := time.Now().UTC()
	safeEnd := now.Truncate(time.Minute)
	if safeEnd.IsZero() {
		safeEnd = now
	}

	systemMetrics, _ := s.opsRepo.GetLatestSystemMetrics(ctx, 1)

	// Cleanup stale state for removed rules.
	s.pruneRuleStates(rules)

	for _, rule := range rules {
		if rule == nil || rule.ID <= 0 {
			continue
		}
		if isMigratedLegacyAlertRule(rule) {
			s.resetRuleState(rule.ID, now)
			continue
		}
		if !rule.Enabled {
			continue
		}
		rulesEnabled++

		scopePlatform, scopeGroupID, scopeRegion := parseOpsAlertRuleScope(rule.Filters)

		windowMinutes := rule.WindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = 1
		}
		windowStart := safeEnd.Add(-time.Duration(windowMinutes) * time.Minute)
		windowEnd := safeEnd

		metricValue, ok := s.computeRuleMetric(ctx, rule, systemMetrics, windowStart, windowEnd, scopePlatform, scopeGroupID)
		if !ok {
			s.resetRuleState(rule.ID, now)
			continue
		}
		rulesEvaluated++

		breachedNow := compareMetric(metricValue, rule.Operator, rule.Threshold)
		required := requiredSustainedBreaches(rule.SustainedMinutes, interval)
		consecutive := s.updateRuleBreaches(rule.ID, now, interval, breachedNow)

		activeEvent, err := s.opsRepo.GetActiveAlertEvent(ctx, rule.ID)
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get active event failed (rule=%d): %v", rule.ID, err)
			continue
		}

		if breachedNow && consecutive >= required {
			if activeEvent != nil && rule.SilenceMinutes <= 0 {
				continue
			}

			// Scoped silencing: if a matching silence exists, skip creating a firing event.
			if s.opsService != nil {
				platform := strings.TrimSpace(scopePlatform)
				region := scopeRegion
				if platform != "" {
					if ok, err := s.opsService.IsAlertSilenced(ctx, rule.ID, platform, scopeGroupID, region, now); err == nil && ok {
						continue
					}
				}
			}

			firedEvent := &OpsAlertEvent{
				RuleID:          rule.ID,
				Severity:        strings.TrimSpace(rule.Severity),
				Status:          OpsAlertStatusFiring,
				LifecycleStatus: OpsAlertStatusFiring,
				Title:           fmt.Sprintf("%s: %s", strings.TrimSpace(rule.Severity), strings.TrimSpace(rule.Name)),
				Description:     buildOpsAlertDescription(rule, metricValue, windowMinutes, scopePlatform, scopeGroupID),
				MetricValue:     float64Ptr(metricValue),
				ThresholdValue:  float64Ptr(rule.Threshold),
				Dimensions:      buildOpsAlertDimensions(scopePlatform, scopeGroupID),
				TriggerSnapshot: buildOpsAlertTriggerSnapshot(rule, metricValue, windowMinutes, scopePlatform, scopeGroupID),
				FiredAt:         now,
				LastSeenAt:      now,
				CreatedAt:       now,
			}
			firedEvent.EventKey = buildOpsAlertEventKey(rule, firedEvent.Dimensions)
			if rule.SilenceMinutes > 0 {
				mergeStart := now.Add(-time.Duration(rule.SilenceMinutes) * time.Minute)
				firedEvent.MergeWindowStart = &mergeStart
			}

			if firedEvent.EventKey != "" && firedEvent.MergeWindowStart != nil {
				existing, err := s.opsRepo.GetMergeableAlertEvent(ctx, firedEvent.EventKey, *firedEvent.MergeWindowStart)
				if err != nil {
					logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get mergeable event failed (rule=%d): %v", rule.ID, err)
					continue
				}
				if existing != nil && existing.ID > 0 {
					if _, err := s.opsRepo.MergeAlertEvent(ctx, existing.ID, firedEvent); err != nil {
						logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] merge event failed (rule=%d event=%d): %v", rule.ID, existing.ID, err)
						continue
					}
					eventsMerged++
					continue
				}
			}

			latestEvent, err := s.opsRepo.GetLatestAlertEvent(ctx, rule.ID)
			if err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get latest event failed (rule=%d): %v", rule.ID, err)
				continue
			}
			if latestEvent != nil && rule.CooldownMinutes > 0 {
				cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
				if now.Sub(latestEvent.FiredAt) < cooldown {
					continue
				}
			}

			created, err := s.createAlertEvent(ctx, firedEvent)
			if err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] create event failed (rule=%d): %v", rule.ID, err)
				continue
			}

			merged := created != nil && created.MergedCount > 0
			if merged {
				eventsMerged++
			} else {
				eventsCreated++
			}
			if created != nil && created.ID > 0 {
				if !merged && s.opsService != nil {
					s.opsService.MaybeCreateAutoAIAnalysisTaskForAlert(ctx, rule, created)
				}
				if !merged && s.maybeSendAlertEmail(ctx, runtimeCfg, rule, created) {
					emailsSent++
				}
			}
			continue
		}

		// Not breached: resolve active event if present.
		if activeEvent != nil {
			resolvedAt := now
			if err := s.opsRepo.UpdateAlertEventStatus(ctx, activeEvent.ID, OpsAlertStatusRecovered, "", "", nil, &resolvedAt); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] resolve event failed (event=%d): %v", activeEvent.ID, err)
			} else {
				eventsResolved++
			}
		}
	}

	result := truncateString(fmt.Sprintf("rules=%d enabled=%d evaluated=%d created=%d merged=%d resolved=%d emails_sent=%d", rulesTotal, rulesEnabled, rulesEvaluated, eventsCreated, eventsMerged, eventsResolved, emailsSent), 2048)
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
}

func (s *OpsAlertEvaluatorService) createAlertEvent(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	if s != nil && s.opsService != nil {
		return s.opsService.CreateAlertEvent(ctx, event)
	}
	if s == nil || s.opsRepo == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	return s.opsRepo.CreateAlertEvent(ctx, event)
}

func (s *OpsAlertEvaluatorService) pruneRuleStates(rules []*OpsAlertRule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	live := map[int64]struct{}{}
	for _, r := range rules {
		if r != nil && r.ID > 0 {
			live[r.ID] = struct{}{}
		}
	}
	for id := range s.ruleStates {
		if _, ok := live[id]; !ok {
			delete(s.ruleStates, id)
		}
	}
}

func (s *OpsAlertEvaluatorService) resetRuleState(ruleID int64, now time.Time) {
	if ruleID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.ruleStates[ruleID]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[ruleID] = state
	}
	state.LastEvaluatedAt = now
	state.ConsecutiveBreaches = 0
}

func (s *OpsAlertEvaluatorService) updateRuleBreaches(ruleID int64, now time.Time, interval time.Duration, breached bool) int {
	if ruleID <= 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.ruleStates[ruleID]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[ruleID] = state
	}

	if !state.LastEvaluatedAt.IsZero() && interval > 0 {
		if now.Sub(state.LastEvaluatedAt) > interval*2 {
			state.ConsecutiveBreaches = 0
		}
	}

	state.LastEvaluatedAt = now
	if breached {
		state.ConsecutiveBreaches++
	} else {
		state.ConsecutiveBreaches = 0
	}
	return state.ConsecutiveBreaches
}

func requiredSustainedBreaches(sustainedMinutes int, interval time.Duration) int {
	if sustainedMinutes <= 0 {
		return 1
	}
	if interval <= 0 {
		return sustainedMinutes
	}
	required := int(math.Ceil(float64(sustainedMinutes*60) / interval.Seconds()))
	if required < 1 {
		return 1
	}
	return required
}

func parseOpsAlertRuleScope(filters map[string]any) (platform string, groupID *int64, region *string) {
	if filters == nil {
		return "", nil, nil
	}
	if v, ok := filters["platform"]; ok {
		if s, ok := v.(string); ok {
			platform = strings.TrimSpace(s)
		}
	}
	if v, ok := filters["group_id"]; ok {
		switch t := v.(type) {
		case float64:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case int64:
			if t > 0 {
				id := t
				groupID = &id
			}
		case int:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err == nil && n > 0 {
				groupID = &n
			}
		}
	}
	if v, ok := filters["region"]; ok {
		if s, ok := v.(string); ok {
			vv := strings.TrimSpace(s)
			if vv != "" {
				region = &vv
			}
		}
	}
	return platform, groupID, region
}

func (s *OpsAlertEvaluatorService) computeRuleMetric(
	ctx context.Context,
	rule *OpsAlertRule,
	systemMetrics *OpsSystemMetricsSnapshot,
	start time.Time,
	end time.Time,
	platform string,
	groupID *int64,
) (float64, bool) {
	if rule == nil {
		return 0, false
	}
	switch strings.TrimSpace(rule.MetricType) {
	case "cpu_usage_percent":
		if systemMetrics != nil && systemMetrics.CPUUsagePercent != nil {
			return *systemMetrics.CPUUsagePercent, true
		}
		return 0, false
	case "memory_usage_percent":
		if systemMetrics != nil && systemMetrics.MemoryUsagePercent != nil {
			return *systemMetrics.MemoryUsagePercent, true
		}
		return 0, false
	case "concurrency_queue_depth":
		if systemMetrics != nil && systemMetrics.ConcurrencyQueueDepth != nil {
			return float64(*systemMetrics.ConcurrencyQueueDepth), true
		}
		return 0, false
	case "group_available_accounts":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil {
			return 0, true
		}
		return float64(availability.Group.AvailableCount), true
	case "group_available_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return computeGroupAvailableRatio(availability.Group), true
	case "account_rate_limited_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})), true
	case "account_error_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})), true
	case "group_rate_limit_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil || availability.Group.TotalAccounts <= 0 {
			return 0, true
		}
		return (float64(availability.Group.RateLimitCount) / float64(availability.Group.TotalAccounts)) * 100, true
	case "account_error_ratio":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		total := int64(len(availability.Accounts))
		if total <= 0 {
			return 0, true
		}
		errorCount := countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})
		return (float64(errorCount) / float64(total)) * 100, true
	case "overload_account_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsOverloaded
		})), true
	}

	overview, err := s.opsRepo.GetDashboardOverview(ctx, &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  platform,
		GroupID:   groupID,
		QueryMode: OpsQueryModeRaw,
	})
	if err != nil {
		return 0, false
	}
	if overview == nil {
		return 0, false
	}

	switch strings.TrimSpace(rule.MetricType) {
	case "compound_rule":
		return s.computeCompoundRuleMetric(ctx, rule, overview, start, end, platform, groupID)
	case "health_score":
		result := computeDashboardHealthScoreResult(time.Now().UTC(), overview)
		if result == nil {
			return 0, false
		}
		return float64(result.Score), true
	case "final_failure_rate":
		if overview.RequestCountSLA <= 0 && overview.RequestCountTotal <= 0 {
			return 0, false
		}
		return finalFailureRate(overview) * 100, true
	case "final_failures":
		return float64(finalFailureCount(overview)), true
	case "success_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.SLA * 100, true
	case "error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.ErrorRate * 100, true
	case "upstream_error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.UpstreamErrorRate * 100, true
	case "p95_latency_ms":
		if overview.Duration.P95 == nil {
			return 0, false
		}
		return float64(*overview.Duration.P95), true
	case "p99_latency_ms":
		if overview.Duration.P99 == nil {
			return 0, false
		}
		return float64(*overview.Duration.P99), true
	default:
		return 0, false
	}
}

func (s *OpsAlertEvaluatorService) computeCompoundRuleMetric(
	ctx context.Context,
	rule *OpsAlertRule,
	overview *OpsDashboardOverview,
	start time.Time,
	end time.Time,
	platform string,
	groupID *int64,
) (float64, bool) {
	if rule == nil || overview == nil {
		return 0, false
	}
	stats, err := s.opsRepo.GetCompoundAlertStats(ctx, &OpsCompoundAlertStatsFilter{
		StartTime:       start,
		EndTime:         end,
		Platform:        platform,
		GroupID:         groupID,
		ErrorCategories: rule.ErrorCategories,
	})
	if err != nil {
		return 0, false
	}
	if stats == nil {
		return 0, false
	}
	finalFailures := stats.FinalFailures
	if rule.MinSampleCount > 0 && overview.RequestCountSLA < int64(rule.MinSampleCount) {
		return 0, true
	}
	if rule.MinFinalFailures > 0 && finalFailures < int64(rule.MinFinalFailures) {
		return 0, true
	}
	if rule.MinFailureRate > 0 {
		failureRate := s.compoundRuleFailureRate(ctx, rule, stats, overview, start, end, platform, groupID)
		if failureRate < rule.MinFailureRate {
			return 0, true
		}
	}
	if !opsAlertImpactScopeSatisfied(rule, stats) {
		return 0, true
	}
	return float64(finalFailures), true
}

func (s *OpsAlertEvaluatorService) compoundRuleFailureRate(
	ctx context.Context,
	rule *OpsAlertRule,
	stats *OpsCompoundAlertStats,
	overview *OpsDashboardOverview,
	start time.Time,
	end time.Time,
	platform string,
	groupID *int64,
) float64 {
	if rule != nil && stats != nil && rule.ImpactScope["affected_models"] == 1 && strings.TrimSpace(stats.DominantModel) != "" && s != nil && s.opsRepo != nil {
		modelOverview, err := s.opsRepo.GetDashboardOverview(ctx, &OpsDashboardFilter{
			StartTime: start,
			EndTime:   end,
			Platform:  platform,
			Model:     strings.TrimSpace(stats.DominantModel),
			GroupID:   groupID,
			QueryMode: OpsQueryModeRaw,
		})
		if err == nil && modelOverview != nil && modelOverview.RequestCountSLA > 0 {
			return modelOverview.ErrorRate * 100
		}
		return 0
	}
	if overview != nil && overview.RequestCountSLA > 0 {
		return overview.ErrorRate * 100
	}
	return 0
}

func opsAlertImpactScopeSatisfied(rule *OpsAlertRule, stats *OpsCompoundAlertStats) bool {
	if rule == nil || len(rule.ImpactScope) == 0 {
		return true
	}
	if stats == nil {
		return false
	}
	for key, minValue := range rule.ImpactScope {
		if minValue <= 0 {
			continue
		}
		var actualDistinct int64
		var actualMax int64
		switch strings.TrimSpace(key) {
		case "affected_users":
			actualDistinct = stats.AffectedUsers
			actualMax = stats.MaxFailuresByUser
		case "affected_api_keys":
			actualDistinct = stats.AffectedAPIKeys
			actualMax = stats.MaxFailuresByAPIKey
		case "affected_groups":
			actualDistinct = stats.AffectedGroups
			actualMax = stats.MaxFailuresByGroup
		case "affected_models":
			actualDistinct = stats.AffectedModels
			actualMax = stats.MaxFailuresByModel
		case "affected_upstream_accounts", "affected_accounts":
			actualDistinct = stats.AffectedUpstreamAccounts
			actualMax = stats.MaxFailuresByUpstreamAccount
		default:
			return false
		}
		if int64(minValue) <= 1 {
			minFailures := int64(rule.MinFinalFailures)
			if minFailures <= 0 {
				minFailures = int64(rule.Threshold)
			}
			if minFailures <= 0 {
				minFailures = 1
			}
			if actualDistinct < 1 || actualMax < minFailures {
				return false
			}
			continue
		}
		if actualDistinct < int64(minValue) {
			return false
		}
	}
	return true
}

func compareMetric(value float64, operator string, threshold float64) bool {
	switch strings.TrimSpace(operator) {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

func buildOpsAlertDimensions(platform string, groupID *int64) map[string]any {
	dims := map[string]any{}
	if strings.TrimSpace(platform) != "" {
		dims["platform"] = strings.TrimSpace(platform)
	}
	if groupID != nil && *groupID > 0 {
		dims["group_id"] = *groupID
	}
	if len(dims) == 0 {
		return nil
	}
	return dims
}

func buildOpsAlertEventKey(rule *OpsAlertRule, dimensions map[string]any) string {
	if rule == nil {
		return ""
	}
	category := "metric"
	if len(rule.ErrorCategories) > 0 {
		cleaned := make([]string, 0, len(rule.ErrorCategories))
		for _, categoryItem := range rule.ErrorCategories {
			if item := strings.TrimSpace(categoryItem); item != "" {
				cleaned = append(cleaned, item)
			}
		}
		if len(cleaned) > 0 {
			category = strings.Join(cleaned, ",")
		}
	}
	subCategory := strings.TrimSpace(rule.MetricType)
	if subCategory == "" {
		subCategory = "unknown"
	}
	triggerLevel := strings.TrimSpace(rule.TriggerLevel)
	if triggerLevel == "" {
		triggerLevel = strings.TrimSpace(rule.Severity)
	}
	if triggerLevel == "" {
		triggerLevel = "unknown"
	}
	return strings.Join([]string{
		opsAlertEventKeyPart(category),
		opsAlertEventKeyPart(subCategory),
		opsAlertEventKeyPart(opsAlertDimensionString(dimensions, "group_id")),
		opsAlertEventKeyPart(opsAlertDimensionString(dimensions, "model")),
		opsAlertEventKeyPart(firstNonEmptyOpsAlertDimensionString(dimensions, "upstream_account_id", "upstream_account", "account_id")),
		opsAlertEventKeyPart(triggerLevel),
	}, "|")
}

func buildOpsAlertTriggerSnapshot(rule *OpsAlertRule, value float64, windowMinutes int, platform string, groupID *int64) map[string]any {
	if rule == nil {
		return nil
	}
	snapshot := map[string]any{
		"rule_id":        rule.ID,
		"rule_name":      strings.TrimSpace(rule.Name),
		"metric_type":    strings.TrimSpace(rule.MetricType),
		"operator":       strings.TrimSpace(rule.Operator),
		"threshold":      rule.Threshold,
		"metric_value":   value,
		"window_minutes": windowMinutes,
	}
	if strings.TrimSpace(rule.TriggerLevel) != "" {
		snapshot["trigger_level"] = strings.TrimSpace(rule.TriggerLevel)
	} else if strings.TrimSpace(rule.Severity) != "" {
		snapshot["trigger_level"] = strings.TrimSpace(rule.Severity)
	}
	if strings.TrimSpace(platform) != "" {
		snapshot["platform"] = strings.TrimSpace(platform)
	}
	if groupID != nil && *groupID > 0 {
		snapshot["group_id"] = *groupID
	}
	return snapshot
}

func opsAlertDimensionString(dimensions map[string]any, key string) string {
	if len(dimensions) == 0 || strings.TrimSpace(key) == "" {
		return ""
	}
	value, ok := dimensions[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstNonEmptyOpsAlertDimensionString(dimensions map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := opsAlertDimensionString(dimensions, key); value != "" {
			return value
		}
	}
	return ""
}

func opsAlertEventKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "all"
	}
	value = strings.ReplaceAll(value, "|", "_")
	return value
}

func buildOpsAlertDescription(rule *OpsAlertRule, value float64, windowMinutes int, platform string, groupID *int64) string {
	if rule == nil {
		return ""
	}
	scope := "overall"
	if strings.TrimSpace(platform) != "" {
		scope = fmt.Sprintf("platform=%s", strings.TrimSpace(platform))
	}
	if groupID != nil && *groupID > 0 {
		scope = fmt.Sprintf("%s group_id=%d", scope, *groupID)
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}
	return fmt.Sprintf("%s %s %.2f (current %.2f) over last %dm (%s)",
		strings.TrimSpace(rule.MetricType),
		strings.TrimSpace(rule.Operator),
		rule.Threshold,
		value,
		windowMinutes,
		strings.TrimSpace(scope),
	)
}

func (s *OpsAlertEvaluatorService) maybeSendAlertEmail(ctx context.Context, runtimeCfg *OpsAlertRuntimeSettings, rule *OpsAlertRule, event *OpsAlertEvent) bool {
	if s == nil || s.emailService == nil || s.opsService == nil || event == nil || rule == nil {
		return false
	}
	if event.EmailSent {
		return false
	}
	if !rule.NotifyEmail {
		return false
	}

	emailCfg, err := s.opsService.GetEmailNotificationConfig(ctx)
	if err != nil || emailCfg == nil || !emailCfg.Alert.Enabled {
		return false
	}

	if len(emailCfg.Alert.Recipients) == 0 {
		return false
	}
	if !shouldSendOpsAlertEmailByMinSeverity(strings.TrimSpace(emailCfg.Alert.MinSeverity), strings.TrimSpace(rule.Severity)) {
		return false
	}

	if runtimeCfg != nil && runtimeCfg.Silencing.Enabled {
		if isOpsAlertSilenced(time.Now().UTC(), rule, event, runtimeCfg.Silencing) {
			return false
		}
	}

	// Apply/update rate limiter.
	s.emailLimiter.SetLimit(emailCfg.Alert.RateLimitPerHour)

	subject := fmt.Sprintf("[Ops Alert][%s] %s", strings.TrimSpace(rule.Severity), strings.TrimSpace(rule.Name))
	body := buildOpsAlertEmailBody(rule, event)

	anySent := false
	for _, to := range emailCfg.Alert.Recipients {
		addr := strings.TrimSpace(to)
		if addr == "" {
			continue
		}
		if !s.emailLimiter.Allow(time.Now().UTC()) {
			continue
		}
		if s.emailService.notificationEmailService != nil {
			if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
				Event:          NotificationEmailEventOpsAlert,
				RecipientEmail: addr,
				RecipientName:  emailRecipientName(addr),
				SourceType:     "ops_alert",
				SourceID:       fmt.Sprintf("%d", event.ID),
				Variables:      opsAlertEmailVariables(rule, event),
			}); err == nil {
				anySent = true
				continue
			} else if !shouldFallbackNotificationEmail(err) {
				if isNotificationEmailDeliveryError(err) {
					logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] alert email delivery failed (rule=%d event=%d to=%s): %v", rule.ID, event.ID, addr, err)
				}
				continue
			}
		}
		if err := s.emailService.SendEmail(ctx, addr, subject, body); err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] alert email send failed (rule=%d event=%d to=%s): %v", rule.ID, event.ID, addr, err)
			continue
		}
		anySent = true
	}

	if anySent {
		_ = s.opsRepo.UpdateAlertEventEmailSent(context.Background(), event.ID, true)
	}
	return anySent
}

func opsAlertEmailVariables(rule *OpsAlertRule, event *OpsAlertEvent) map[string]string {
	variables := map[string]string{
		"rule_name":         "-",
		"severity":          "-",
		"alert_status":      "-",
		"metric_type":       "-",
		"operator":          "-",
		"metric_value":      "-",
		"threshold_value":   "-",
		"triggered_at":      time.Now().UTC().Format(time.RFC3339),
		"alert_description": "-",
	}
	if rule != nil {
		variables["rule_name"] = strings.TrimSpace(rule.Name)
		variables["severity"] = strings.TrimSpace(rule.Severity)
		variables["metric_type"] = strings.TrimSpace(rule.MetricType)
		variables["operator"] = strings.TrimSpace(rule.Operator)
		variables["threshold_value"] = fmt.Sprintf("%.2f", rule.Threshold)
		if strings.TrimSpace(rule.Description) != "" {
			variables["alert_description"] = strings.TrimSpace(rule.Description)
		}
	}
	if event != nil {
		variables["alert_status"] = strings.TrimSpace(event.Status)
		if event.MetricValue != nil {
			variables["metric_value"] = fmt.Sprintf("%.2f", *event.MetricValue)
		}
		if event.ThresholdValue != nil {
			variables["threshold_value"] = fmt.Sprintf("%.2f", *event.ThresholdValue)
		}
		if !event.FiredAt.IsZero() {
			variables["triggered_at"] = event.FiredAt.UTC().Format(time.RFC3339)
		}
		if strings.TrimSpace(event.Description) != "" {
			variables["alert_description"] = strings.TrimSpace(event.Description)
		}
	}
	return variables
}

func buildOpsAlertEmailBody(rule *OpsAlertRule, event *OpsAlertEvent) string {
	if rule == nil || event == nil {
		return ""
	}
	metric := strings.TrimSpace(rule.MetricType)
	value := "-"
	threshold := fmt.Sprintf("%.2f", rule.Threshold)
	if event.MetricValue != nil {
		value = fmt.Sprintf("%.2f", *event.MetricValue)
	}
	if event.ThresholdValue != nil {
		threshold = fmt.Sprintf("%.2f", *event.ThresholdValue)
	}
	return fmt.Sprintf(`
<h2>Ops Alert</h2>
<p><b>Rule</b>: %s</p>
<p><b>Severity</b>: %s</p>
<p><b>Status</b>: %s</p>
<p><b>Metric</b>: %s %s %s</p>
<p><b>Fired at</b>: %s</p>
<p><b>Description</b>: %s</p>
`,
		htmlEscape(rule.Name),
		htmlEscape(rule.Severity),
		htmlEscape(event.Status),
		htmlEscape(metric),
		htmlEscape(rule.Operator),
		htmlEscape(fmt.Sprintf("%s (threshold %s)", value, threshold)),
		event.FiredAt.Format(time.RFC3339),
		htmlEscape(event.Description),
	)
}

func shouldSendOpsAlertEmailByMinSeverity(minSeverity string, ruleSeverity string) bool {
	minSeverity = strings.ToLower(strings.TrimSpace(minSeverity))
	if minSeverity == "" {
		return true
	}

	eventLevel := opsEmailSeverityForOps(ruleSeverity)
	minLevel := strings.ToLower(minSeverity)

	rank := func(level string) int {
		switch level {
		case "critical":
			return 3
		case "warning":
			return 2
		case "info":
			return 1
		default:
			return 0
		}
	}
	return rank(eventLevel) >= rank(minLevel)
}

func opsEmailSeverityForOps(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0":
		return "critical"
	case "P1":
		return "warning"
	default:
		return "info"
	}
}

func isOpsAlertSilenced(now time.Time, rule *OpsAlertRule, event *OpsAlertEvent, silencing OpsAlertSilencingSettings) bool {
	if !silencing.Enabled {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(silencing.GlobalUntilRFC3339) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(silencing.GlobalUntilRFC3339)); err == nil {
			if now.Before(t) {
				return true
			}
		}
	}

	for _, entry := range silencing.Entries {
		untilRaw := strings.TrimSpace(entry.UntilRFC3339)
		if untilRaw == "" {
			continue
		}
		until, err := time.Parse(time.RFC3339, untilRaw)
		if err != nil {
			continue
		}
		if now.After(until) {
			continue
		}
		if entry.RuleID != nil && rule != nil && rule.ID > 0 && *entry.RuleID != rule.ID {
			continue
		}
		if len(entry.Severities) > 0 {
			match := false
			for _, s := range entry.Severities {
				if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(event.Severity)) || strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(rule.Severity)) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		return true
	}

	return false
}

func (s *OpsAlertEvaluatorService) tryAcquireLeaderLock(ctx context.Context, lock OpsDistributedLockSettings) (func(), bool) {
	if !lock.Enabled {
		return nil, true
	}
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] redis not configured; running without distributed lock")
		})
		return nil, true
	}
	key := strings.TrimSpace(lock.Key)
	if key == "" {
		key = opsAlertEvaluatorLeaderLockKey
	}
	ttl := time.Duration(lock.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = opsAlertEvaluatorLeaderLockTTL
	}

	ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
	if err != nil {
		// Prefer fail-closed to avoid duplicate evaluators stampeding the DB when Redis is flaky.
		// Single-node deployments can disable the distributed lock via runtime settings.
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock SetNX failed; skipping this cycle: %v", err)
		})
		return nil, false
	}
	if !ok {
		s.maybeLogSkip(key)
		return nil, false
	}
	return func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		_, _ = opsAlertEvaluatorReleaseScript.Run(releaseCtx, s.redisClient, []string{key}, s.instanceID).Result()
	}, true
}

func (s *OpsAlertEvaluatorService) maybeLogSkip(key string) {
	s.skipLogMu.Lock()
	defer s.skipLogMu.Unlock()

	now := time.Now()
	if !s.skipLogAt.IsZero() && now.Sub(s.skipLogAt) < opsAlertEvaluatorSkipLogInterval {
		return
	}
	s.skipLogAt = now
	logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock held by another instance; skipping (key=%q)", key)
}

func (s *OpsAlertEvaluatorService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, result string) {
	if s == nil || s.opsRepo == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg := strings.TrimSpace(result)
	if msg == "" {
		msg = "ok"
	}
	msg = truncateString(msg, 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &msg,
	})
}

func (s *OpsAlertEvaluatorService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsRepo == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
	})
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

type slidingWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	sent   []time.Time
}

func newSlidingWindowLimiter(limit int, window time.Duration) *slidingWindowLimiter {
	if window <= 0 {
		window = time.Hour
	}
	return &slidingWindowLimiter{
		limit:  limit,
		window: window,
		sent:   []time.Time{},
	}
}

func (l *slidingWindowLimiter) SetLimit(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = limit
}

func (l *slidingWindowLimiter) Allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.limit <= 0 {
		return true
	}
	cutoff := now.Add(-l.window)
	keep := l.sent[:0]
	for _, t := range l.sent {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	l.sent = keep
	if len(l.sent) >= l.limit {
		return false
	}
	l.sent = append(l.sent, now)
	return true
}

// computeGroupAvailableRatio returns the available percentage for a group.
// Formula: (AvailableCount / TotalAccounts) * 100.
// Returns 0 when TotalAccounts is 0.
func computeGroupAvailableRatio(group *GroupAvailability) float64 {
	if group == nil || group.TotalAccounts <= 0 {
		return 0
	}
	return (float64(group.AvailableCount) / float64(group.TotalAccounts)) * 100
}

// countAccountsByCondition counts accounts that satisfy the given condition.
func countAccountsByCondition(accounts map[int64]*AccountAvailability, condition func(*AccountAvailability) bool) int64 {
	if len(accounts) == 0 || condition == nil {
		return 0
	}
	var count int64
	for _, account := range accounts {
		if account != nil && condition(account) {
			count++
		}
	}
	return count
}
