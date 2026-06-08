package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) ListCacheStatsRows(ctx context.Context, filter *service.CacheStatsFilter) ([]*service.CacheStatsRawRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.CacheStatsFilter{}
	}
	where := []string{"minute_at >= $1", "minute_at < $2", "cache_type = 'exact'"}
	args := []any{filter.StartTime, filter.EndTime}
	if platform := normalizeCacheStatsPlatform(filter.Platform); platform != "" {
		args = append(args, platform)
		where = append(where, fmt.Sprintf("platform = $%d", len(args)))
	}
	if model := strings.TrimSpace(filter.Model); model != "" {
		args = append(args, model)
		where = append(where, fmt.Sprintf("model = $%d", len(args)))
	}
	if filter.APIKeyID != nil {
		args = append(args, *filter.APIKeyID)
		where = append(where, fmt.Sprintf("api_key_id = $%d", len(args)))
	}
	if filter.GroupID != nil {
		args = append(args, *filter.GroupID)
		where = append(where, fmt.Sprintf("group_id = $%d", len(args)))
	}

	query := fmt.Sprintf(`
WITH filtered AS (
  SELECT *
  FROM ops_cache_minute_stats
  WHERE %s
), metrics AS (
  SELECT
    platform,
    model,
    COALESCE(SUM(total_requests), 0) AS total_requests,
    COALESCE(SUM(candidate_requests), 0) AS candidate_requests,
    COALESCE(SUM(hit_requests), 0) AS hit_requests,
    COALESCE(SUM(bypass_requests), 0) AS bypass_requests,
    COALESCE(SUM(store_success), 0) AS store_success,
    COALESCE(SUM(store_skip), 0) AS store_skip,
    COALESCE(SUM(input_tokens), 0) AS input_tokens,
    COALESCE(SUM(output_tokens), 0) AS output_tokens,
    COALESCE(SUM(hit_tokens), 0) AS hit_tokens,
    COALESCE(SUM(candidate_tokens), 0) AS candidate_tokens,
    COALESCE(SUM(all_request_tokens), 0) AS all_request_tokens,
    COALESCE(SUM(estimated_saved_amount), 0)::text AS estimated_saved_amount
  FROM filtered
  GROUP BY platform, model
), bypass_agg AS (
  SELECT platform, model, COALESCE(jsonb_object_agg(reason, total), '{}'::jsonb)::text AS bypass_reasons
  FROM (
    SELECT f.platform, f.model, reason.key AS reason, SUM(CASE WHEN reason.value ~ '^[0-9]+$' THEN (reason.value)::bigint ELSE 0 END) AS total
    FROM filtered f
    JOIN LATERAL jsonb_each_text(f.bypass_reasons) reason ON TRUE
    GROUP BY f.platform, f.model, reason.key
  ) x
  GROUP BY platform, model
), store_skip_agg AS (
  SELECT platform, model, COALESCE(jsonb_object_agg(reason, total), '{}'::jsonb)::text AS store_skip_reasons
  FROM (
    SELECT f.platform, f.model, reason.key AS reason, SUM(CASE WHEN reason.value ~ '^[0-9]+$' THEN (reason.value)::bigint ELSE 0 END) AS total
    FROM filtered f
    JOIN LATERAL jsonb_each_text(f.store_skip_reasons) reason ON TRUE
    GROUP BY f.platform, f.model, reason.key
  ) x
  GROUP BY platform, model
)
SELECT
  m.platform,
  m.model,
  m.total_requests,
  m.candidate_requests,
  m.hit_requests,
  m.bypass_requests,
  m.store_success,
  m.store_skip,
  m.input_tokens,
  m.output_tokens,
  m.hit_tokens,
  m.candidate_tokens,
  m.all_request_tokens,
  COALESCE(b.bypass_reasons, '{}'::jsonb::text) AS bypass_reasons,
  COALESCE(ss.store_skip_reasons, '{}'::jsonb::text) AS store_skip_reasons,
  m.estimated_saved_amount
FROM metrics m
LEFT JOIN bypass_agg b ON b.platform = m.platform AND b.model = m.model
LEFT JOIN store_skip_agg ss ON ss.platform = m.platform AND ss.model = m.model
ORDER BY m.hit_tokens DESC, m.platform ASC, m.model ASC`, strings.Join(where, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list cache stats rows: %w", err)
	}
	defer rows.Close()

	out := []*service.CacheStatsRawRow{}
	for rows.Next() {
		row := &service.CacheStatsRawRow{}
		if err := rows.Scan(
			&row.Platform,
			&row.Model,
			&row.TotalRequests,
			&row.CandidateRequests,
			&row.HitRequests,
			&row.BypassRequests,
			&row.StoreSuccess,
			&row.StoreSkip,
			&row.InputTokens,
			&row.OutputTokens,
			&row.HitTokens,
			&row.CandidateTokens,
			&row.AllRequestTokens,
			&row.BypassReasonsJSON,
			&row.StoreSkipReasonsJSON,
			&row.EstimatedSavedAmount,
		); err != nil {
			return nil, fmt.Errorf("scan cache stats row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cache stats rows: %w", err)
	}
	return out, nil
}

func normalizeCacheStatsPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "", "all":
		return ""
	case "claude", "anthropic":
		return "anthropic"
	case "openai", "gemini":
		return strings.ToLower(strings.TrimSpace(platform))
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}
