package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const cacheStatsExportMaxRows = 100000

var ErrCacheStatsExportTooLarge = errors.New("cache stats export row count exceeds limit")
var ErrCacheStatsExportEmpty = errors.New("cache stats export has no rows")

type CacheStatsService struct {
	repo           CacheStatsRepository
	promptRepo     PromptCacheStatsRepository
	statsStore     LocalResponseCacheStatsStore
	hotspotStore   LocalResponseCacheHotspotStore
	settingService *SettingService
}

func NewCacheStatsService(repo CacheStatsRepository) *CacheStatsService {
	return &CacheStatsService{repo: repo}
}

func (s *CacheStatsService) SetAdvancedDependencies(promptRepo PromptCacheStatsRepository, cache GatewayCache, settingService *SettingService) *CacheStatsService {
	if s == nil {
		return s
	}
	s.promptRepo = promptRepo
	if store, ok := cache.(LocalResponseCacheStatsStore); ok {
		s.statsStore = store
	}
	if store, ok := cache.(LocalResponseCacheHotspotStore); ok {
		s.hotspotStore = store
	}
	s.settingService = settingService
	return s
}

func (s *CacheStatsService) GetStats(ctx context.Context, filter *CacheStatsFilter) (*CacheStatsResponse, error) {
	resp := &CacheStatsResponse{
		ModelRows:        []CacheStatsModelRow{},
		BypassReasons:    []CacheStatsReasonRow{},
		StoreSkipReasons: []CacheStatsReasonRow{},
	}
	if !canViewCacheStats(viewerRoleFromCacheStatsFilter(filter)) {
		return resp, nil
	}
	if s == nil || s.repo == nil {
		return resp, nil
	}
	rows, err := s.repo.ListCacheStatsRows(ctx, filter)
	if err != nil {
		return nil, err
	}

	models := map[string]*cacheStatsAccumulator{}
	bypassReasons := map[string]int64{}
	storeSkipReasons := map[string]int64{}
	var summary cacheStatsAccumulator
	for _, row := range rows {
		if row == nil {
			continue
		}
		acc := cacheStatsAccumulatorFromRow(row)
		summary.add(acc)
		modelKey := strings.ToLower(strings.TrimSpace(row.Platform)) + "\x00" + strings.TrimSpace(row.Model)
		modelAcc := models[modelKey]
		if modelAcc == nil {
			modelAcc = &cacheStatsAccumulator{platform: strings.TrimSpace(row.Platform), model: strings.TrimSpace(row.Model), estimatedSavedAmount: "0"}
			models[modelKey] = modelAcc
		}
		modelAcc.add(acc)
		mergeReasonCounters(bypassReasons, acc.bypassReasons)
		mergeReasonCounters(storeSkipReasons, acc.storeSkipReasons)
	}

	resp.Summary = summary.summary()
	modelRows := make([]CacheStatsModelRow, 0, len(models))
	for _, acc := range models {
		modelRows = append(modelRows, acc.modelRow())
	}
	sort.Slice(modelRows, func(i, j int) bool {
		if modelRows[i].HitTokens == modelRows[j].HitTokens {
			if modelRows[i].Platform == modelRows[j].Platform {
				return modelRows[i].Model < modelRows[j].Model
			}
			return modelRows[i].Platform < modelRows[j].Platform
		}
		return modelRows[i].HitTokens > modelRows[j].HitTokens
	})
	resp.ModelRows = modelRows
	resp.BypassReasons = cacheStatsReasonRows(bypassReasons)
	resp.StoreSkipReasons = cacheStatsReasonRows(storeSkipReasons)
	return resp, nil
}

func (s *CacheStatsService) GetAdvancedStats(ctx context.Context, filter *CacheStatsFilter) (*AdvancedCacheStatsResponse, error) {
	if filter == nil {
		filter = &CacheStatsFilter{}
	}
	if !canViewCacheStats(viewerRoleFromCacheStatsFilter(filter)) {
		return &AdvancedCacheStatsResponse{Hotspots: []AdvancedCacheHotspot{}, UpdatedAt: time.Now()}, nil
	}
	baseStats, err := s.GetStats(ctx, filter)
	if err != nil {
		return nil, err
	}
	cfg := DefaultAdvancedCacheConfig()
	if s != nil && s.settingService != nil {
		if loaded, loadErr := s.settingService.GetAdvancedCacheConfig(ctx); loadErr == nil {
			cfg = loaded
		}
	}
	localStats := &LocalResponseCacheStats{Counters: map[string]int64{}}
	if s != nil && s.statsStore != nil {
		loaded, loadErr := s.statsStore.GetLocalResponseCacheStats(ctx)
		if loadErr != nil {
			return nil, loadErr
		}
		if loaded != nil {
			localStats = loaded
		}
	}
	promptStats := &PromptCacheStatsRaw{PriceMissingModels: []string{}}
	if s != nil && s.promptRepo != nil {
		loaded, loadErr := s.promptRepo.ListPromptCacheStats(ctx, filter)
		if loadErr != nil {
			return nil, loadErr
		}
		if loaded != nil {
			promptStats = loaded
		}
	}
	hotspots := []AdvancedCacheHotspot{}
	if s != nil && s.hotspotStore != nil {
		items, listErr := s.hotspotStore.ListLocalResponseCacheHotspots(ctx, LocalResponseCacheHotspotFilter{
			Window:   advancedCacheHotWindowDuration(cfg.HotWindow),
			Limit:    normalizeAdvancedCacheHotspotLimit(filter.HotspotLimit),
			Platform: filter.Platform,
			Model:    filter.Model,
			GroupID:  filter.GroupID,
			APIKeyID: filter.APIKeyID,
		})
		if listErr != nil {
			return nil, listErr
		}
		hotspots = advancedCacheHotspotsFromLocal(items)
	}
	capacityLimitBytes := int64(cfg.RedisCapacityMB) * 1024 * 1024
	memorySafeLimitBytes := int64(cfg.MemorySafeLimitMB) * 1024 * 1024
	rawBytes := nonNegativeInt64(localStats.RawResponseBytes)
	storedBytes := nonNegativeInt64(localStats.StoredResponseBytes)
	savedBytes := rawBytes - storedBytes
	if savedBytes < 0 {
		savedBytes = 0
	}
	localAmount := normalizeDecimalString(baseStats.Summary.EstimatedSavedAmount)
	promptAmount := normalizeDecimalString(promptStats.EstimatedSavedAmount)
	priceMissingModels := mergeUniqueStrings(promptStats.PriceMissingModels, localPriceMissingModels(baseStats.ModelRows))
	priceMissing := len(priceMissingModels) > 0
	var localAmountPtr, promptAmountPtr, totalAmountPtr *string
	if !priceMissing {
		localAmountPtr = stringPtr(localAmount)
		promptAmountPtr = stringPtr(promptAmount)
		totalAmountPtr = stringPtr(addDecimalStrings(localAmount, promptAmount))
	}
	fallback := AdvancedCacheFallback{}
	if !cfg.AdvancedCacheEnabled {
		reason := "advanced_cache_disabled"
		fallback.AdvancedCacheFallbackActive = true
		fallback.FallbackReason = &reason
	}
	return &AdvancedCacheStatsResponse{
		Capacity: AdvancedCacheCapacityStats{
			CurrentUsedBytes:     nonNegativeInt64(localStats.Bytes),
			CapacityLimitBytes:   capacityLimitBytes,
			CapacityUsageRate:    percent(nonNegativeInt64(localStats.Bytes), capacityLimitBytes),
			MemorySafeLimitBytes: memorySafeLimitBytes,
			EvictionPolicy:       strings.TrimSpace(cfg.EvictionPolicy),
			RecentEvictionCount:  nonNegativeInt64(localStats.Counters["eviction_deleted_keys"]),
		},
		Compression: AdvancedCacheCompressionStats{
			Enabled:                  cfg.CompressionEnabled,
			RawResponseBytes:         rawBytes,
			StoredResponseBytes:      storedBytes,
			CompressionSavedBytes:    savedBytes,
			CompressionSavedRate:     percent(savedBytes, rawBytes),
			CompressedEntryCount:     nonNegativeInt64(localStats.CompressedEntryCount),
			CompressionFailedCount:   nonNegativeInt64(localStats.Counters["compression_failed"]),
			DecompressionFailedCount: nonNegativeInt64(localStats.Counters["decompression_failed"]),
		},
		Hotspots: hotspots,
		Savings: AdvancedCacheSavings{
			LocalResponseCacheSavedTokens:  nonNegativeInt64(baseStats.Summary.HitTokens),
			LocalResponseCacheSavedAmount:  localAmountPtr,
			UpstreamPromptCacheReadTokens:  nonNegativeInt64(promptStats.ReadTokens),
			UpstreamPromptCacheWriteTokens: nonNegativeInt64(promptStats.WriteTokens),
			UpstreamPromptCacheSavedAmount: promptAmountPtr,
			TotalEstimatedSavedAmount:      totalAmountPtr,
			PriceMissing:                   priceMissing,
			PriceMissingModels:             priceMissingModels,
		},
		EmptyStates: AdvancedCacheEmptyStates{
			Hotspots:    len(hotspots) == 0,
			PromptCache: promptStats.ReadTokens == 0 && promptStats.WriteTokens == 0,
			Price:       priceMissing,
		},
		Fallback:  fallback,
		UpdatedAt: time.Now(),
	}, nil
}

func (s *CacheStatsService) ExportCSV(ctx context.Context, filter *CacheStatsFilter) ([]byte, error) {
	if !canExportCacheStats(viewerRoleFromCacheStatsFilter(filter)) {
		return nil, ErrCacheStatsExportEmpty
	}
	stats, err := s.GetStats(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(stats.ModelRows) > cacheStatsExportMaxRows {
		return nil, ErrCacheStatsExportTooLarge
	}
	if len(stats.ModelRows) == 0 {
		return nil, ErrCacheStatsExportEmpty
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"平台",
		"模型",
		"请求次数",
		"候选请求数",
		"命中次数",
		"未命中次数",
		"绕过次数",
		"输入 tokens",
		"输出 tokens",
		"命中 tokens",
		"请求命中率",
		"tokens 命中率",
		"主要绕过原因",
	}); err != nil {
		return nil, err
	}
	for _, row := range stats.ModelRows {
		if err := writer.Write([]string{
			row.Platform,
			row.Model,
			strconv.FormatInt(row.TotalRequests, 10),
			strconv.FormatInt(row.CandidateRequests, 10),
			strconv.FormatInt(row.HitRequests, 10),
			strconv.FormatInt(row.MissRequests, 10),
			strconv.FormatInt(row.BypassRequests, 10),
			strconv.FormatInt(row.InputTokens, 10),
			strconv.FormatInt(row.OutputTokens, 10),
			strconv.FormatInt(row.HitTokens, 10),
			formatCacheStatsPercent(row.RequestHitRate),
			formatCacheStatsPercent(row.TokensHitRate),
			row.TopBypassReason,
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("write cache stats export csv: %w", err)
	}
	return buf.Bytes(), nil
}

type cacheStatsAccumulator struct {
	platform             string
	model                string
	totalRequests        int64
	candidateRequests    int64
	hitRequests          int64
	bypassRequests       int64
	storeSuccess         int64
	storeSkip            int64
	inputTokens          int64
	outputTokens         int64
	hitTokens            int64
	candidateTokens      int64
	allRequestTokens     int64
	bypassReasons        map[string]int64
	storeSkipReasons     map[string]int64
	estimatedSavedAmount string
}

func cacheStatsAccumulatorFromRow(row *CacheStatsRawRow) *cacheStatsAccumulator {
	return &cacheStatsAccumulator{
		platform:             displayCacheStatsPlatform(row.Platform),
		model:                strings.TrimSpace(row.Model),
		totalRequests:        nonNegativeInt64(row.TotalRequests),
		candidateRequests:    nonNegativeInt64(row.CandidateRequests),
		hitRequests:          nonNegativeInt64(row.HitRequests),
		bypassRequests:       nonNegativeInt64(row.BypassRequests),
		storeSuccess:         nonNegativeInt64(row.StoreSuccess),
		storeSkip:            nonNegativeInt64(row.StoreSkip),
		inputTokens:          nonNegativeInt64(row.InputTokens),
		outputTokens:         nonNegativeInt64(row.OutputTokens),
		hitTokens:            nonNegativeInt64(row.HitTokens),
		candidateTokens:      nonNegativeInt64(row.CandidateTokens),
		allRequestTokens:     nonNegativeInt64(row.AllRequestTokens),
		bypassReasons:        parseCacheStatsReasonJSON(row.BypassReasonsJSON),
		storeSkipReasons:     parseCacheStatsReasonJSON(row.StoreSkipReasonsJSON),
		estimatedSavedAmount: strings.TrimSpace(row.EstimatedSavedAmount),
	}
}

func (a *cacheStatsAccumulator) add(b *cacheStatsAccumulator) {
	if a == nil || b == nil {
		return
	}
	a.totalRequests += b.totalRequests
	a.candidateRequests += b.candidateRequests
	a.hitRequests += b.hitRequests
	a.bypassRequests += b.bypassRequests
	a.storeSuccess += b.storeSuccess
	a.storeSkip += b.storeSkip
	a.inputTokens += b.inputTokens
	a.outputTokens += b.outputTokens
	a.hitTokens += b.hitTokens
	a.candidateTokens += b.candidateTokens
	a.allRequestTokens += b.allRequestTokens
	if a.bypassReasons == nil {
		a.bypassReasons = map[string]int64{}
	}
	if a.storeSkipReasons == nil {
		a.storeSkipReasons = map[string]int64{}
	}
	mergeReasonCounters(a.bypassReasons, b.bypassReasons)
	mergeReasonCounters(a.storeSkipReasons, b.storeSkipReasons)
	a.estimatedSavedAmount = addDecimalStrings(a.estimatedSavedAmount, b.estimatedSavedAmount)
}

func (a *cacheStatsAccumulator) summary() CacheStatsSummary {
	if a == nil {
		return CacheStatsSummary{EstimatedSavedAmount: "0.00000000"}
	}
	miss := a.candidateRequests - a.hitRequests
	if miss < 0 {
		miss = 0
	}
	return CacheStatsSummary{
		TotalRequests:         a.totalRequests,
		CandidateRequests:     a.candidateRequests,
		HitRequests:           a.hitRequests,
		MissRequests:          miss,
		BypassRequests:        a.bypassRequests,
		StoreSuccess:          a.storeSuccess,
		StoreSkip:             a.storeSkip,
		RequestHitRate:        percent(a.hitRequests, a.candidateRequests),
		InputTokens:           a.inputTokens,
		OutputTokens:          a.outputTokens,
		HitTokens:             a.hitTokens,
		CandidateTokens:       a.candidateTokens,
		TokensHitRate:         percent(a.hitTokens, a.candidateTokens),
		OverallTokensCoverage: percent(a.hitTokens, a.allRequestTokens),
		EstimatedSavedAmount:  normalizeDecimalString(a.estimatedSavedAmount),
	}
}

func (a *cacheStatsAccumulator) modelRow() CacheStatsModelRow {
	miss := a.candidateRequests - a.hitRequests
	if miss < 0 {
		miss = 0
	}
	return CacheStatsModelRow{
		Platform:             a.platform,
		Model:                a.model,
		TotalRequests:        a.totalRequests,
		CandidateRequests:    a.candidateRequests,
		HitRequests:          a.hitRequests,
		MissRequests:         miss,
		BypassRequests:       a.bypassRequests,
		StoreSuccess:         a.storeSuccess,
		StoreSkip:            a.storeSkip,
		InputTokens:          a.inputTokens,
		OutputTokens:         a.outputTokens,
		HitTokens:            a.hitTokens,
		CandidateTokens:      a.candidateTokens,
		AllRequestTokens:     a.allRequestTokens,
		RequestHitRate:       percent(a.hitRequests, a.candidateRequests),
		TokensHitRate:        percent(a.hitTokens, a.candidateTokens),
		TopBypassReason:      topReason(a.bypassReasons),
		TopStoreSkipReason:   topReason(a.storeSkipReasons),
		EstimatedSavedAmount: normalizeDecimalString(a.estimatedSavedAmount),
	}
}

func parseCacheStatsReasonJSON(raw string) map[string]int64 {
	out := map[string]int64{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return out
	}
	for reason, value := range decoded {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		switch v := value.(type) {
		case float64:
			if v > 0 {
				out[reason] += int64(v)
			}
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed > 0 {
				out[reason] += parsed
			}
		}
	}
	return out
}

func mergeReasonCounters(dst, src map[string]int64) {
	for reason, count := range src {
		if strings.TrimSpace(reason) != "" && count > 0 {
			dst[reason] += count
		}
	}
}

func cacheStatsReasonRows(reasons map[string]int64) []CacheStatsReasonRow {
	var total int64
	for _, count := range reasons {
		if count > 0 {
			total += count
		}
	}
	rows := make([]CacheStatsReasonRow, 0, len(reasons))
	for reason, count := range reasons {
		if count <= 0 {
			continue
		}
		rows = append(rows, CacheStatsReasonRow{Reason: reason, Count: count, Percent: percent(count, total)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count == rows[j].Count {
			return rows[i].Reason < rows[j].Reason
		}
		return rows[i].Count > rows[j].Count
	})
	return rows
}

func normalizeAdvancedCacheHotspotLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func advancedCacheHotspotsFromLocal(items []LocalResponseCacheHotspot) []AdvancedCacheHotspot {
	out := make([]AdvancedCacheHotspot, 0, len(items))
	for i, item := range items {
		rank := item.Rank
		if rank <= 0 {
			rank = i + 1
		}
		out = append(out, AdvancedCacheHotspot{
			Rank:      rank,
			Platform:  displayCacheStatsPlatform(item.Platform),
			Model:     strings.TrimSpace(item.Model),
			Group:     advancedCacheGroupRef(item.GroupID),
			APIKey:    advancedCacheAPIKeyRef(item.APIKeyID),
			HitCount:  nonNegativeInt64(item.HitCount),
			HitTokens: nonNegativeInt64(item.HitTokens),
			LastHitAt: item.LastHitAt,
		})
	}
	return out
}

func advancedCacheGroupRef(id *int64) AdvancedCacheNameRef {
	if id == nil || *id <= 0 {
		return AdvancedCacheNameRef{Display: "未分组"}
	}
	return AdvancedCacheNameRef{ID: *id, Display: fmt.Sprintf("group #%d", *id)}
}

func advancedCacheAPIKeyRef(id *int64) AdvancedCacheNameRef {
	if id == nil || *id <= 0 {
		return AdvancedCacheNameRef{Display: "未知 Key"}
	}
	return AdvancedCacheNameRef{ID: *id, Display: fmt.Sprintf("api-key #%d", *id)}
}

func localPriceMissingModels(rows []CacheStatsModelRow) []string {
	out := []string{}
	for _, row := range rows {
		if row.HitTokens <= 0 {
			continue
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(row.EstimatedSavedAmount), 64)
		if err == nil && amount > 0 {
			continue
		}
		if model := strings.TrimSpace(row.Model); model != "" {
			out = append(out, model)
		}
	}
	return out
}

func mergeUniqueStrings(groups ...[]string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, group := range groups {
		for _, item := range group {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			key := strings.ToLower(item)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func stringPtr(v string) *string {
	return &v
}

func topReason(reasons map[string]int64) string {
	rows := cacheStatsReasonRows(reasons)
	if len(rows) == 0 {
		return ""
	}
	return rows[0].Reason
}

func percent(numerator, denominator int64) float64 {
	if denominator <= 0 || numerator <= 0 {
		return 0
	}
	return math.Round((float64(numerator)/float64(denominator))*10000) / 100
}

func nonNegativeInt64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func addDecimalStrings(a, b string) string {
	af, _ := strconv.ParseFloat(strings.TrimSpace(a), 64)
	bf, _ := strconv.ParseFloat(strings.TrimSpace(b), 64)
	return strconv.FormatFloat(af+bf, 'f', 8, 64)
}

func normalizeDecimalString(v string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f < 0 {
		return "0.00000000"
	}
	return strconv.FormatFloat(f, 'f', 8, 64)
}

func displayCacheStatsPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "anthropic", "claude":
		return "claude"
	case "openai", "gemini":
		return strings.ToLower(strings.TrimSpace(platform))
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func formatCacheStatsPercent(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64) + "%"
}
