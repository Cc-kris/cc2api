package service

import (
	"context"
	"log/slog"
	"sort"
)

type AccountSchedulerGroupScore struct {
	GroupID *int64  `json:"group_id"`
	Score   float64 `json:"score"`
}

// PreviewOpenAISchedulerScores queries only requested OpenAI pools and performs
// one load batch for their union, regardless of how many groups the page shows.
func (s *RateLimitService) PreviewOpenAISchedulerScores(ctx context.Context, page []Account, concurrency *ConcurrencyService) map[int64][]AccountSchedulerGroupScore {
	result := map[int64][]AccountSchedulerGroupScore{}
	if s == nil || s.accountRepo == nil {
		return result
	}
	groups := map[int64]struct{}{}
	for _, account := range page {
		if !account.IsOpenAI() {
			continue
		}
		ids := append([]int64(nil), account.GroupIDs...)
		if len(ids) == 0 {
			for _, g := range account.AccountGroups {
				ids = append(ids, g.GroupID)
			}
		}
		if len(ids) == 0 {
			groups[0] = struct{}{}
		} else {
			for _, id := range ids {
				groups[id] = struct{}{}
			}
		}
	}
	if len(groups) == 0 {
		return result
	}
	pools := map[int64][]Account{}
	union := map[int64]*Account{}
	for id := range groups {
		var pool []Account
		var err error
		if id == 0 {
			pool, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, PlatformOpenAI)
		} else {
			pool, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, id, PlatformOpenAI)
		}
		if err != nil {
			slog.Warn("openai_scheduler_preview_pool_failed", "group_id", id, "error", err)
			continue
		}
		pools[id] = pool
		for i := range pool {
			if pool[i].IsOpenAI() {
				union[pool[i].ID] = &pool[i]
			}
		}
	}
	loads := make([]AccountWithConcurrency, 0, len(union))
	for _, account := range union {
		loads = append(loads, AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.EffectiveLoadFactor()})
	}
	var loadMap map[int64]*AccountLoadInfo
	if concurrency != nil && len(loads) > 0 {
		var err error
		loadMap, err = concurrency.GetAccountsLoadBatch(ctx, loads)
		if err != nil {
			slog.Warn("openai_scheduler_preview_load_batch_failed", "error", err)
		}
	}
	gateway := &OpenAIGatewayService{cfg: s.cfg, rateLimitService: s}
	controls := OpenAISchedulerSettings{}
	if gateway.isOpenAIAdvancedSchedulerEnabled(ctx) {
		controls = gateway.schedulerControls(ctx)
	}
	scheduler := &defaultOpenAIAccountScheduler{service: gateway}
	groupIDs := make([]int64, 0, len(pools))
	for id := range pools {
		groupIDs = append(groupIDs, id)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	for _, id := range groupIDs {
		pool := pools[id]
		candidates := make([]*Account, 0, len(pool))
		for i := range pool {
			if pool[i].IsOpenAI() {
				candidates = append(candidates, &pool[i])
			}
		}
		plan := scheduler.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{Controls: controls}, candidates, loadMap)
		var groupID *int64
		if id != 0 {
			value := id
			groupID = &value
		}
		for _, candidate := range plan.candidates {
			result[candidate.account.ID] = append(result[candidate.account.ID], AccountSchedulerGroupScore{GroupID: groupID, Score: candidate.score})
		}
	}
	return result
}
