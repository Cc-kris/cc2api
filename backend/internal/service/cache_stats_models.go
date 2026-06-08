package service

import (
	"context"
	"time"
)

type CacheStatsFilter struct {
	StartTime time.Time
	EndTime   time.Time
	Platform  string
	Model     string
	APIKeyID  *int64
	GroupID   *int64
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
	TotalRequests         int64  `json:"total_requests"`
	CandidateRequests     int64  `json:"candidate_requests"`
	HitRequests           int64  `json:"hit_requests"`
	RequestHitRate        float64 `json:"request_hit_rate"`
	InputTokens           int64  `json:"input_tokens"`
	OutputTokens          int64  `json:"output_tokens"`
	HitTokens             int64  `json:"hit_tokens"`
	CandidateTokens       int64  `json:"candidate_tokens"`
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
	ModelRows        []CacheStatsModelRow   `json:"model_rows"`
	BypassReasons    []CacheStatsReasonRow  `json:"bypass_reasons"`
	StoreSkipReasons []CacheStatsReasonRow  `json:"store_skip_reasons"`
}

type CacheStatsRepository interface {
	ListCacheStatsRows(ctx context.Context, filter *CacheStatsFilter) ([]*CacheStatsRawRow, error)
}
