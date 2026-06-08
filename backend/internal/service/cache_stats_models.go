package service

import (
	"context"
	"time"
)

type CacheStatsFilter struct {
	StartTime    time.Time
	EndTime      time.Time
	Platform     string
	Model        string
	APIKeyID     *int64
	GroupID      *int64
	HotspotLimit int
	// ViewerRole is the current admin-side viewer role. It keeps field-level
	// filtering in the service layer instead of relying only on route guards.
	ViewerRole string
}

type CacheStatsRawRow struct {
	Platform             string
	Model                string
	TotalRequests        int64
	CandidateRequests    int64
	HitRequests          int64
	BypassRequests       int64
	StoreSuccess         int64
	StoreSkip            int64
	InputTokens          int64
	OutputTokens         int64
	HitTokens            int64
	CandidateTokens      int64
	AllRequestTokens     int64
	BypassReasonsJSON    string
	StoreSkipReasonsJSON string
	EstimatedSavedAmount string
}

type CacheStatsSummary struct {
	TotalRequests         int64   `json:"total_requests"`
	CandidateRequests     int64   `json:"candidate_requests"`
	HitRequests           int64   `json:"hit_requests"`
	MissRequests          int64   `json:"miss_requests"`
	BypassRequests        int64   `json:"bypass_requests"`
	StoreSuccess          int64   `json:"store_success"`
	StoreSkip             int64   `json:"store_skip"`
	RequestHitRate        float64 `json:"request_hit_rate"`
	InputTokens           int64   `json:"input_tokens"`
	OutputTokens          int64   `json:"output_tokens"`
	HitTokens             int64   `json:"hit_tokens"`
	CandidateTokens       int64   `json:"candidate_tokens"`
	TokensHitRate         float64 `json:"tokens_hit_rate"`
	OverallTokensCoverage float64 `json:"overall_tokens_coverage"`
	EstimatedSavedAmount  string  `json:"estimated_saved_amount"`
}

type CacheStatsModelRow struct {
	Platform             string  `json:"platform"`
	Model                string  `json:"model"`
	TotalRequests        int64   `json:"total_requests"`
	CandidateRequests    int64   `json:"candidate_requests"`
	HitRequests          int64   `json:"hit_requests"`
	MissRequests         int64   `json:"miss_requests"`
	BypassRequests       int64   `json:"bypass_requests"`
	StoreSuccess         int64   `json:"store_success"`
	StoreSkip            int64   `json:"store_skip"`
	InputTokens          int64   `json:"input_tokens"`
	OutputTokens         int64   `json:"output_tokens"`
	HitTokens            int64   `json:"hit_tokens"`
	CandidateTokens      int64   `json:"candidate_tokens"`
	AllRequestTokens     int64   `json:"all_request_tokens"`
	RequestHitRate       float64 `json:"request_hit_rate"`
	TokensHitRate        float64 `json:"tokens_hit_rate"`
	TopBypassReason      string  `json:"top_bypass_reason,omitempty"`
	TopStoreSkipReason   string  `json:"top_store_skip_reason,omitempty"`
	EstimatedSavedAmount string  `json:"estimated_saved_amount"`
}

type CacheStatsReasonRow struct {
	Reason  string  `json:"reason"`
	Count   int64   `json:"count"`
	Percent float64 `json:"percent"`
}

type CacheStatsResponse struct {
	Summary          CacheStatsSummary     `json:"summary"`
	ModelRows        []CacheStatsModelRow  `json:"model_rows"`
	BypassReasons    []CacheStatsReasonRow `json:"bypass_reasons"`
	StoreSkipReasons []CacheStatsReasonRow `json:"store_skip_reasons"`
}

type AdvancedCacheStatsResponse struct {
	Capacity    AdvancedCacheCapacityStats    `json:"capacity"`
	Compression AdvancedCacheCompressionStats `json:"compression"`
	Hotspots    []AdvancedCacheHotspot        `json:"hotspots"`
	Savings     AdvancedCacheSavings          `json:"savings"`
	EmptyStates AdvancedCacheEmptyStates      `json:"empty_states"`
	Fallback    AdvancedCacheFallback         `json:"fallback"`
	UpdatedAt   time.Time                     `json:"updated_at"`
}

type AdvancedCacheCapacityStats struct {
	CurrentUsedBytes     int64      `json:"current_used_bytes"`
	CapacityLimitBytes   int64      `json:"capacity_limit_bytes"`
	CapacityUsageRate    float64    `json:"capacity_usage_rate"`
	MemorySafeLimitBytes int64      `json:"memory_safe_limit_bytes"`
	EvictionPolicy       string     `json:"eviction_policy"`
	RecentEvictionCount  int64      `json:"recent_eviction_count"`
	LastEvictedAt        *time.Time `json:"last_evicted_at"`
}

type AdvancedCacheCompressionStats struct {
	Enabled                  bool    `json:"enabled"`
	RawResponseBytes         int64   `json:"raw_response_bytes"`
	StoredResponseBytes      int64   `json:"stored_response_bytes"`
	CompressionSavedBytes    int64   `json:"compression_saved_bytes"`
	CompressionSavedRate     float64 `json:"compression_saved_rate"`
	CompressedEntryCount     int64   `json:"compressed_entry_count"`
	CompressionFailedCount   int64   `json:"compression_failed_count"`
	DecompressionFailedCount int64   `json:"decompression_failed_count"`
}

type AdvancedCacheHotspot struct {
	Rank      int                  `json:"rank"`
	Platform  string               `json:"platform"`
	Model     string               `json:"model"`
	Group     AdvancedCacheNameRef `json:"group"`
	APIKey    AdvancedCacheNameRef `json:"api_key"`
	HitCount  int64                `json:"hit_count"`
	HitTokens int64                `json:"hit_tokens"`
	LastHitAt time.Time            `json:"last_hit_at"`
}

type AdvancedCacheNameRef struct {
	ID      int64  `json:"id"`
	Name    string `json:"name,omitempty"`
	Display string `json:"display,omitempty"`
}

type AdvancedCacheSavings struct {
	LocalResponseCacheSavedTokens  int64    `json:"local_response_cache_saved_tokens"`
	LocalResponseCacheSavedAmount  *string  `json:"local_response_cache_saved_amount"`
	UpstreamPromptCacheReadTokens  int64    `json:"upstream_prompt_cache_read_tokens"`
	UpstreamPromptCacheWriteTokens int64    `json:"upstream_prompt_cache_write_tokens"`
	UpstreamPromptCacheSavedAmount *string  `json:"upstream_prompt_cache_saved_amount"`
	TotalEstimatedSavedAmount      *string  `json:"total_estimated_saved_amount"`
	PriceMissing                   bool     `json:"price_missing"`
	PriceMissingModels             []string `json:"price_missing_models"`
}

type AdvancedCacheEmptyStates struct {
	Hotspots    bool `json:"hotspots"`
	PromptCache bool `json:"prompt_cache"`
	Price       bool `json:"price"`
}

type AdvancedCacheFallback struct {
	AdvancedCacheFallbackActive bool       `json:"advanced_cache_fallback_active"`
	FallbackReason              *string    `json:"fallback_reason"`
	LastFallbackAt              *time.Time `json:"last_fallback_at"`
}

type CacheStatsRepository interface {
	ListCacheStatsRows(ctx context.Context, filter *CacheStatsFilter) ([]*CacheStatsRawRow, error)
}

type PromptCacheStatsRepository interface {
	ListPromptCacheStats(ctx context.Context, filter *CacheStatsFilter) (*PromptCacheStatsRaw, error)
}
