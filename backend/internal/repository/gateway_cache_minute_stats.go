package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (c *gatewayCache) RecordLocalResponseCacheMinuteStats(ctx context.Context, entries []*service.LocalResponseCacheMinuteStatEvent) error {
	if c == nil || c.rdb == nil || c.db == nil || len(entries) == 0 {
		return nil
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if err := upsertLocalResponseCacheMinuteStat(ctx, tx, entry); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertLocalResponseCacheMinuteStat(ctx context.Context, tx *sql.Tx, entry *service.LocalResponseCacheMinuteStatEvent) error {
	if tx == nil || entry == nil {
		return nil
	}
	platform := strings.TrimSpace(strings.ToLower(entry.Platform))
	model := strings.TrimSpace(entry.Model)
	cacheType := strings.TrimSpace(strings.ToLower(entry.CacheType))
	minuteAt := entry.At.UTC().Truncate(time.Minute)
	if minuteAt.IsZero() {
		return nil
	}
	totalRequests := entry.TotalRequests
	if totalRequests <= 0 {
		totalRequests = 1
	}
	if platform == "" || model == "" || cacheType == "" {
		return nil
	}
	bypassRequests := int64(0)
	bypassReasons := map[string]int64{}
	if entry.BypassReason != "" {
		bypassRequests = 1
		bypassReasons[entry.BypassReason] = 1
	}
	storeSuccess := int64(0)
	if entry.StoreSuccess {
		storeSuccess = 1
	}
	storeSkip := int64(0)
	storeSkipReasons := map[string]int64{}
	if entry.StoreSkipReason != "" {
		storeSkip = 1
		storeSkipReasons[entry.StoreSkipReason] = 1
	}
	candidateRequests := int64(0)
	candidateTokens := int64(0)
	if entry.Candidate {
		candidateRequests = 1
		candidateTokens = entry.InputTokens + entry.OutputTokens
	}
	hitRequests := int64(0)
	if entry.Hit {
		hitRequests = 1
	}
	allRequestTokens := entry.InputTokens + entry.OutputTokens
	bypassJSON, err := json.Marshal(bypassReasons)
	if err != nil {
		return err
	}
	storeSkipJSON, err := json.Marshal(storeSkipReasons)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_cache_minute_stats (
  minute_at, platform, model, group_id, api_key_id, cache_type,
  total_requests, candidate_requests, hit_requests, bypass_requests, store_success, store_skip,
  input_tokens, output_tokens, hit_tokens, candidate_tokens, all_request_tokens,
  bypass_reasons, store_skip_reasons, updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,$10,$11,$12,
  $13,$14,$15,$16,$17,
  $18::jsonb,$19::jsonb,NOW()
)
ON CONFLICT (
  minute_at, platform, model, (COALESCE(group_id, -1)), (COALESCE(api_key_id, -1)), cache_type
) DO UPDATE SET
  total_requests = ops_cache_minute_stats.total_requests + EXCLUDED.total_requests,
  candidate_requests = ops_cache_minute_stats.candidate_requests + EXCLUDED.candidate_requests,
  hit_requests = ops_cache_minute_stats.hit_requests + EXCLUDED.hit_requests,
  bypass_requests = ops_cache_minute_stats.bypass_requests + EXCLUDED.bypass_requests,
  store_success = ops_cache_minute_stats.store_success + EXCLUDED.store_success,
  store_skip = ops_cache_minute_stats.store_skip + EXCLUDED.store_skip,
  input_tokens = ops_cache_minute_stats.input_tokens + EXCLUDED.input_tokens,
  output_tokens = ops_cache_minute_stats.output_tokens + EXCLUDED.output_tokens,
  hit_tokens = ops_cache_minute_stats.hit_tokens + EXCLUDED.hit_tokens,
  candidate_tokens = ops_cache_minute_stats.candidate_tokens + EXCLUDED.candidate_tokens,
  all_request_tokens = ops_cache_minute_stats.all_request_tokens + EXCLUDED.all_request_tokens,
  bypass_reasons = (
    SELECT COALESCE(jsonb_object_agg(key, to_jsonb(total)), '{}'::jsonb)
    FROM (
      SELECT key, SUM(value::bigint) AS total
      FROM (
        SELECT key, value FROM jsonb_each_text(ops_cache_minute_stats.bypass_reasons)
        UNION ALL
        SELECT key, value FROM jsonb_each_text(EXCLUDED.bypass_reasons)
      ) merged
      GROUP BY key
    ) summed
  ),
  store_skip_reasons = (
    SELECT COALESCE(jsonb_object_agg(key, to_jsonb(total)), '{}'::jsonb)
    FROM (
      SELECT key, SUM(value::bigint) AS total
      FROM (
        SELECT key, value FROM jsonb_each_text(ops_cache_minute_stats.store_skip_reasons)
        UNION ALL
        SELECT key, value FROM jsonb_each_text(EXCLUDED.store_skip_reasons)
      ) merged
      GROUP BY key
    ) summed
  ),
  updated_at = NOW()`,
		minuteAt,
		platform,
		model,
		opsNullInt64(entry.GroupID),
		opsNullInt64(entry.APIKeyID),
		cacheType,
		totalRequests,
		candidateRequests,
		hitRequests,
		bypassRequests,
		storeSuccess,
		storeSkip,
		entry.InputTokens,
		entry.OutputTokens,
		entry.HitTokens,
		candidateTokens,
		allRequestTokens,
		string(bypassJSON),
		string(storeSkipJSON),
	)
	if err != nil {
		return fmt.Errorf("record local response cache minute stats: %w", err)
	}
	return nil
}
