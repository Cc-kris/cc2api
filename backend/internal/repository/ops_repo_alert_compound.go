package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetCompoundAlertStats(ctx context.Context, filter *service.OpsCompoundAlertStatsFilter) (*service.OpsCompoundAlertStats, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil || filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	args := []any{filter.StartTime.UTC(), filter.EndTime.UTC()}
	clauses := []string{
		"e.created_at >= $1",
		"e.created_at < $2",
		"COALESCE(e.is_count_tokens,false) = false",
		opsPlatformSLAErrorConditionFor("e"),
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, *filter.GroupID)
		clauses = append(clauses, fmt.Sprintf("e.group_id = $%d", len(args)))
	}
	if platform := strings.TrimSpace(strings.ToLower(filter.Platform)); platform != "" {
		args = append(args, platform)
		clauses = append(clauses, fmt.Sprintf("LOWER(COALESCE(e.platform,'')) = $%d", len(args)))
	}
	if categoryWhere := buildCompoundAlertCategoryWhere(filter.ErrorCategories); categoryWhere != "" {
		clauses = append(clauses, "("+categoryWhere+")")
	}

	q := `
WITH base AS (
  SELECT
    e.*,
    COALESCE(e.user_id, ak.user_id) AS effective_user_id
  FROM ops_error_logs e
  LEFT JOIN api_keys ak ON e.api_key_id = ak.id
  WHERE ` + strings.Join(clauses, " AND ") + `
)
SELECT
  COUNT(*),
  COUNT(DISTINCT effective_user_id) FILTER (WHERE effective_user_id IS NOT NULL),
  COUNT(DISTINCT api_key_id) FILTER (WHERE api_key_id IS NOT NULL),
  COUNT(DISTINCT group_id) FILTER (WHERE group_id IS NOT NULL),
  COUNT(DISTINCT COALESCE(model, '')) FILTER (WHERE COALESCE(model, '') <> ''),
  COUNT(DISTINCT account_id) FILTER (WHERE account_id IS NOT NULL),
  COALESCE((SELECT MAX(c) FROM (SELECT effective_user_id, COUNT(*) AS c FROM base WHERE effective_user_id IS NOT NULL GROUP BY effective_user_id) s), 0),
  COALESCE((SELECT MAX(c) FROM (SELECT api_key_id, COUNT(*) AS c FROM base WHERE api_key_id IS NOT NULL GROUP BY api_key_id) s), 0),
  COALESCE((SELECT MAX(c) FROM (SELECT group_id, COUNT(*) AS c FROM base WHERE group_id IS NOT NULL GROUP BY group_id) s), 0),
  COALESCE((SELECT MAX(c) FROM (SELECT COALESCE(model, '') AS model_key, COUNT(*) AS c FROM base WHERE COALESCE(model, '') <> '' GROUP BY COALESCE(model, '')) s), 0),
  COALESCE((SELECT MAX(c) FROM (SELECT account_id, COUNT(*) AS c FROM base WHERE account_id IS NOT NULL GROUP BY account_id) s), 0),
  COALESCE((SELECT model_key FROM (SELECT COALESCE(model, '') AS model_key, COUNT(*) AS c FROM base WHERE COALESCE(model, '') <> '' GROUP BY COALESCE(model, '') ORDER BY c DESC, model_key ASC LIMIT 1) s), '')
FROM base`

	out := &service.OpsCompoundAlertStats{}
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(
		&out.FinalFailures,
		&out.AffectedUsers,
		&out.AffectedAPIKeys,
		&out.AffectedGroups,
		&out.AffectedModels,
		&out.AffectedUpstreamAccounts,
		&out.MaxFailuresByUser,
		&out.MaxFailuresByAPIKey,
		&out.MaxFailuresByGroup,
		&out.MaxFailuresByModel,
		&out.MaxFailuresByUpstreamAccount,
		&out.DominantModel,
	); err != nil {
		return nil, err
	}
	return out, nil
}

func buildCompoundAlertCategoryWhere(categories []string) string {
	cleaned := map[string]struct{}{}
	for _, category := range categories {
		category = strings.TrimSpace(strings.ToLower(category))
		if category != "" {
			cleaned[category] = struct{}{}
		}
	}
	if len(cleaned) == 0 || len(cleaned) >= len(service.AllOpsErrorCategories) {
		return ""
	}

	parts := []string{}
	for category := range cleaned {
		switch category {
		case service.OpsErrorCategoryClient:
			parts = append(parts, "(LOWER(COALESCE(e.error_source,'')) = 'client' OR LOWER(COALESCE(e.error_owner,'')) = 'client' OR (COALESCE(e.status_code,0) BETWEEN 400 AND 499 AND COALESCE(e.upstream_status_code,0) = 0))")
		case service.OpsErrorCategoryPlatform:
			parts = append(parts, "(LOWER(COALESCE(e.error_source,'')) = 'platform' OR LOWER(COALESCE(e.error_owner,'')) = 'platform')")
		case service.OpsErrorCategoryUpstream:
			parts = append(parts, "(LOWER(COALESCE(e.error_source,'')) IN ('upstream','upstream_http') OR LOWER(COALESCE(e.error_owner,'')) IN ('upstream','provider') OR e.upstream_status_code IS NOT NULL)")
		case service.OpsErrorCategoryAccountPool:
			parts = append(parts, "(LOWER(COALESCE(e.error_message,'')) LIKE '%account%' OR LOWER(COALESCE(e.error_type,'')) LIKE '%account%')")
		case service.OpsErrorCategoryRateLimit:
			parts = append(parts, "(COALESCE(e.status_code,0) = 429 OR COALESCE(e.upstream_status_code,0) = 429 OR LOWER(COALESCE(e.error_type,'')) LIKE '%rate%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%rate%')")
		case service.OpsErrorCategoryPermission:
			parts = append(parts, "(COALESCE(e.status_code,0) IN (401,403) OR COALESCE(e.upstream_status_code,0) IN (401,403) OR LOWER(COALESCE(e.error_type,'')) LIKE '%permission%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%permission%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%auth%')")
		case service.OpsErrorCategoryBalance:
			parts = append(parts, "(LOWER(COALESCE(e.error_type,'')) LIKE '%balance%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%balance%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%quota%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%credit%')")
		case service.OpsErrorCategoryConfig:
			parts = append(parts, "(LOWER(COALESCE(e.error_type,'')) LIKE '%config%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%config%')")
		case service.OpsErrorCategorySlowRequest:
			parts = append(parts, "(LOWER(COALESCE(e.error_type,'')) LIKE '%slow%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%slow%' OR LOWER(COALESCE(e.error_message,'')) LIKE '%timeout%')")
		case service.OpsErrorCategoryUnknown:
			parts = append(parts, "(COALESCE(e.error_type,'') = '' AND COALESCE(e.error_source,'') = '' AND COALESCE(e.error_owner,'') = '')")
		}
	}
	return strings.Join(parts, " OR ")
}
