package handler

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type localResponseCacheStatsResponse struct {
	Enabled          bool             `json:"enabled"`
	Entries          int64            `json:"entries"`
	Bytes            int64            `json:"bytes"`
	LookupHit        int64            `json:"lookup_hit"`
	LookupMiss       int64            `json:"lookup_miss"`
	LookupError      int64            `json:"lookup_error"`
	StoreSuccess     int64            `json:"store_success"`
	StoreFailed      int64            `json:"store_failed"`
	BypassTotal      int64            `json:"bypass_total"`
	StoreSkipTotal   int64            `json:"store_skip_total"`
	HitRate          float64          `json:"hit_rate"`
	BypassReasons    map[string]int64 `json:"bypass_reasons"`
	StoreSkipReasons map[string]int64 `json:"store_skip_reasons"`
	Counters         map[string]int64 `json:"counters"`
}

func (h *OpenAIGatewayHandler) LocalResponseCacheStats(c *gin.Context) {
	if h == nil || h.gatewayService == nil {
		response.Error(c, http.StatusServiceUnavailable, "OpenAI gateway service is unavailable")
		return
	}
	ctx := c.Request.Context()
	cfg := h.gatewayService.LocalResponseCacheConfig(ctx)
	stats, err := h.gatewayService.GetLocalResponseCacheStats(ctx)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get local response cache stats")
		return
	}

	counters := map[string]int64{}
	for k, v := range stats.Counters {
		counters[k] = v
	}
	bypassReasons, bypassTotal := splitLocalResponseCacheReasonCounters(counters, "lookup_bypass:")
	storeSkipReasons, storeSkipTotal := splitLocalResponseCacheReasonCounters(counters, "store_skip:")
	hit := counters["lookup_hit"]
	miss := counters["lookup_miss"]
	hitRate := 0.0
	if total := hit + miss; total > 0 {
		hitRate = float64(hit) / float64(total)
	}

	response.Success(c, localResponseCacheStatsResponse{
		Enabled:          cfg.Enabled,
		Entries:          stats.Entries,
		Bytes:            stats.Bytes,
		LookupHit:        hit,
		LookupMiss:       miss,
		LookupError:      counters["lookup_error"],
		StoreSuccess:     counters["store_success"],
		StoreFailed:      counters["store_failed"],
		BypassTotal:      bypassTotal,
		StoreSkipTotal:   storeSkipTotal,
		HitRate:          hitRate,
		BypassReasons:    bypassReasons,
		StoreSkipReasons: storeSkipReasons,
		Counters:         counters,
	})
}

func splitLocalResponseCacheReasonCounters(counters map[string]int64, prefix string) (map[string]int64, int64) {
	reasons := map[string]int64{}
	var total int64
	keys := make([]string, 0, len(counters))
	for key := range counters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		value := counters[key]
		reason := strings.TrimPrefix(key, prefix)
		reasons[reason] = value
		total += value
	}
	return reasons, total
}
