package repository

import (
	"context"
	"database/sql"
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

func (r *opsRepository) ListPromptCacheStats(ctx context.Context, filter *service.CacheStatsFilter) (*service.PromptCacheStatsRaw, error) {
	out := &service.PromptCacheStatsRaw{PriceMissingModels: []string{}}
	if r == nil || r.db == nil {
		return out, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.CacheStatsFilter{}
	}
	join, where, args := buildPromptCacheStatsWhere(filter)
	query := `
SELECT
  COALESCE(SUM(ul.cache_read_tokens), 0)::bigint AS read_tokens,
  COALESCE(SUM(ul.cache_creation_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens), 0)::bigint AS write_tokens,
  COALESCE(SUM(ul.cache_read_cost + ul.cache_creation_cost), 0)::text AS estimated_saved_amount
FROM usage_logs ul
` + join + `
WHERE ` + strings.Join(where, " AND ")
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&out.ReadTokens, &out.WriteTokens, &out.EstimatedSavedAmount); err != nil {
		return nil, fmt.Errorf("list prompt cache stats: %w", err)
	}
	missingQuery := `
SELECT DISTINCT COALESCE(NULLIF(TRIM(ul.model), ''), 'unknown') AS model
FROM usage_logs ul
` + join + `
WHERE ` + strings.Join(where, " AND ") + `
  AND (ul.cache_read_tokens + ul.cache_creation_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens) > 0
GROUP BY COALESCE(NULLIF(TRIM(ul.model), ''), 'unknown')
HAVING COALESCE(SUM(ul.cache_read_cost + ul.cache_creation_cost), 0) = 0
ORDER BY model ASC`
	rows, err := r.db.QueryContext(ctx, missingQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list prompt cache price missing models: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, fmt.Errorf("scan prompt cache price missing model: %w", err)
		}
		if strings.TrimSpace(model) != "" {
			out.PriceMissingModels = append(out.PriceMissingModels, model)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt cache price missing models: %w", err)
	}
	return out, nil
}

func buildPromptCacheStatsWhere(filter *service.CacheStatsFilter) (string, []string, []any) {
	where := []string{"ul.created_at >= $1", "ul.created_at < $2"}
	args := []any{filter.StartTime, filter.EndTime}
	join := ""
	if platform := normalizeCacheStatsPlatform(filter.Platform); platform != "" {
		join = "LEFT JOIN groups g ON g.id = ul.group_id LEFT JOIN accounts a ON a.id = ul.account_id"
		args = append(args, platform)
		where = append(where, fmt.Sprintf("COALESCE(NULLIF(g.platform,''), a.platform) = $%d", len(args)))
	}
	if model := strings.TrimSpace(filter.Model); model != "" {
		args = append(args, model)
		where = append(where, fmt.Sprintf("COALESCE(ul.model,'') = $%d", len(args)))
	}
	if filter.APIKeyID != nil {
		args = append(args, *filter.APIKeyID)
		where = append(where, fmt.Sprintf("ul.api_key_id = $%d", len(args)))
	}
	if filter.GroupID != nil {
		args = append(args, *filter.GroupID)
		where = append(where, fmt.Sprintf("ul.group_id = $%d", len(args)))
	}
	return join, where, args
}

func scanNullDecimalText(value sql.NullString) string {
	if !value.Valid {
		return "0"
	}
	return value.String
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
