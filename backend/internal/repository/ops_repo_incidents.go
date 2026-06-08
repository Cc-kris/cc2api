package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetIncidentImpact(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsIncidentImpact, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	where, args := buildIncidentImpactWhere(filter, filter.StartTime.UTC(), filter.EndTime.UTC(), 1)
	out := &service.OpsIncidentImpact{
		AffectedModels:   []string{},
		AffectedAccounts: []*service.OpsIncidentAffectedAccount{},
	}

	countSQL := `
SELECT
  COUNT(DISTINCT COALESCE(e.user_id, ak.user_id)) FILTER (WHERE COALESCE(e.user_id, ak.user_id) IS NOT NULL),
  COUNT(DISTINCT e.api_key_id) FILTER (WHERE e.api_key_id IS NOT NULL)
FROM ops_error_logs e
LEFT JOIN api_keys ak ON e.api_key_id = ak.id
` + where
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&out.AffectedUsers, &out.AffectedAPIKeys); err != nil {
		return nil, err
	}

	modelRows, err := r.db.QueryContext(ctx, `
SELECT DISTINCT COALESCE(e.model, '') AS model
FROM ops_error_logs e
`+where+`
  AND COALESCE(e.model, '') <> ''
ORDER BY model ASC
LIMIT 20`, args...)
	if err != nil {
		return nil, err
	}
	for modelRows.Next() {
		var model string
		if err := modelRows.Scan(&model); err != nil {
			_ = modelRows.Close()
			return nil, err
		}
		out.AffectedModels = append(out.AffectedModels, model)
	}
	if err := modelRows.Close(); err != nil {
		return nil, err
	}
	if err := modelRows.Err(); err != nil {
		return nil, err
	}

	accountRows, err := r.db.QueryContext(ctx, `
SELECT DISTINCT e.account_id, COALESCE(a.name, '') AS account_name
FROM ops_error_logs e
LEFT JOIN accounts a ON e.account_id = a.id
`+where+`
  AND e.account_id IS NOT NULL
ORDER BY account_name ASC, e.account_id ASC
LIMIT 20`, args...)
	if err != nil {
		return nil, err
	}
	for accountRows.Next() {
		var id int64
		var name sql.NullString
		if err := accountRows.Scan(&id, &name); err != nil {
			_ = accountRows.Close()
			return nil, err
		}
		out.AffectedAccounts = append(out.AffectedAccounts, &service.OpsIncidentAffectedAccount{ID: id, Name: name.String})
	}
	if err := accountRows.Close(); err != nil {
		return nil, err
	}
	if err := accountRows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func buildIncidentImpactWhere(filter *service.OpsDashboardFilter, start, end time.Time, startIndex int) (string, []any) {
	idx := startIndex
	clauses := []string{
		fmt.Sprintf("e.created_at >= $%d", idx),
	}
	args := []any{start}
	idx++
	clauses = append(clauses, fmt.Sprintf("e.created_at < $%d", idx))
	args = append(args, end)
	idx++
	clauses = append(clauses, "COALESCE(e.is_count_tokens,false) = false")
	clauses = append(clauses, opsPlatformSLAErrorConditionFor("e"))

	if filter != nil {
		if filter.GroupID != nil && *filter.GroupID > 0 {
			args = append(args, *filter.GroupID)
			clauses = append(clauses, fmt.Sprintf("e.group_id = $%d", idx))
			idx++
		}
		if platform := strings.TrimSpace(strings.ToLower(filter.Platform)); platform != "" {
			args = append(args, platform)
			clauses = append(clauses, fmt.Sprintf("e.platform = $%d", idx))
			idx++
		}
		if model := strings.TrimSpace(filter.Model); model != "" {
			args = append(args, model)
			clauses = append(clauses, fmt.Sprintf("COALESCE(e.model,'') = $%d", idx))
		}
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}
