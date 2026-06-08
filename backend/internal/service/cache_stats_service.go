package service

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

type CacheStatsService struct {
	repo CacheStatsRepository
}

func NewCacheStatsService(repo CacheStatsRepository) *CacheStatsService {
	return &CacheStatsService{repo: repo}
}

func (s *CacheStatsService) GetStats(ctx context.Context, filter *CacheStatsFilter) (*CacheStatsResponse, error) {
	resp := &CacheStatsResponse{
		ModelRows:        []CacheStatsModelRow{},
		BypassReasons:    []CacheStatsReasonRow{},
		StoreSkipReasons: []CacheStatsReasonRow{},
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
		platform:             strings.TrimSpace(row.Platform),
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
	return CacheStatsSummary{
		TotalRequests:         a.totalRequests,
		CandidateRequests:     a.candidateRequests,
		HitRequests:           a.hitRequests,
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
