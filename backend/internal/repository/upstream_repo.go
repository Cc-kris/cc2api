package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamRepository struct{ db *sql.DB }

func NewUpstreamRepository(db *sql.DB) service.UpstreamRepository { return &upstreamRepository{db: db} }

func (r *upstreamRepository) ListUpstreams(ctx context.Context) ([]*service.Upstream, error) {
	rows, err := r.db.QueryContext(ctx, upstreamListSQL+" ORDER BY u.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUpstreams(rows)
}

func (r *upstreamRepository) GetUpstream(ctx context.Context, id int64) (*service.Upstream, error) {
	rows, err := r.db.QueryContext(ctx, upstreamListSQL+" WHERE u.id = $1 AND u.deleted_at IS NULL", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanUpstreams(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("upstream not found")
	}
	return items[0], nil
}

func (r *upstreamRepository) CreateUpstream(ctx context.Context, input *service.UpstreamInput) (*service.Upstream, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO upstreams (base_url, normalized_base_url, name, rate_multiplier, initial_balance, balance_alert_enabled, alert_balance, notes, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
RETURNING id`, input.BaseURL, service.NormalizeUpstreamBaseURLForRepo(input.BaseURL), input.Name, input.RateMultiplier, input.InitialBalance, input.BalanceAlertEnabled, input.AlertBalance, input.Notes).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.GetUpstream(ctx, id)
}

func (r *upstreamRepository) UpdateUpstream(ctx context.Context, id int64, input *service.UpstreamInput) (*service.Upstream, error) {
	res, err := r.db.ExecContext(ctx, `
UPDATE upstreams
SET base_url=$2, normalized_base_url=$3, name=$4, rate_multiplier=$5, initial_balance=$6,
    balance_alert_enabled=$7, alert_balance=$8, notes=$9, updated_at=NOW(),
    alert_email_sent_at = CASE WHEN COALESCE(alert_balance, -1) <> COALESCE($8, -1) OR balance_alert_enabled <> $7 THEN NULL ELSE alert_email_sent_at END
WHERE id=$1 AND deleted_at IS NULL`, id, input.BaseURL, service.NormalizeUpstreamBaseURLForRepo(input.BaseURL), input.Name, input.RateMultiplier, input.InitialBalance, input.BalanceAlertEnabled, input.AlertBalance, input.Notes)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("upstream not found")
	}
	return r.GetUpstream(ctx, id)
}

func (r *upstreamRepository) DeleteUpstream(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE upstreams SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("upstream not found")
	}
	return nil
}

func (r *upstreamRepository) SyncFromAccounts(ctx context.Context) (int, error) {
	rows, err := r.db.QueryContext(ctx, `
WITH account_upstreams AS (
  SELECT DISTINCT trim(trailing '/' FROM lower(COALESCE(NULLIF(credentials->>'base_url',''), default_account_base_url(platform, type)))) AS normalized_base_url,
         trim(trailing '/' FROM COALESCE(NULLIF(credentials->>'base_url',''), default_account_base_url(platform, type))) AS base_url
  FROM accounts
  WHERE deleted_at IS NULL
), inserted AS (
  INSERT INTO upstreams (base_url, normalized_base_url, name, rate_multiplier, initial_balance, balance_alert_enabled, notes, created_at, updated_at)
  SELECT base_url, normalized_base_url, regexp_replace(normalized_base_url, '^https?://', ''), 1.0, 0, false, '', NOW(), NOW()
  FROM account_upstreams
  WHERE normalized_base_url <> ''
  ON CONFLICT (normalized_base_url) WHERE deleted_at IS NULL DO NOTHING
  RETURNING id
)
SELECT COUNT(*)::bigint FROM inserted`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int64
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	return int(count), rows.Err()
}

func (r *upstreamRepository) GetUpstreamStats(ctx context.Context, start, end time.Time, granularity string) (*service.UpstreamStatsResponse, error) {
	summary, err := r.upstreamStatsSummary(ctx)
	if err != nil {
		return nil, err
	}
	costBars, err := r.upstreamCostBars(ctx, start, end)
	if err != nil {
		return nil, err
	}
	tokenTrend, err := r.upstreamTokenTrend(ctx, start, end, granularity)
	if err != nil {
		return nil, err
	}
	return &service.UpstreamStatsResponse{Summary: *summary, CostBars: costBars, TokenTrend: tokenTrend, StartDate: start.Format(time.RFC3339), EndDate: end.Format(time.RFC3339), Granularity: granularity, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (r *upstreamRepository) GetFinanceStats(ctx context.Context, start, end time.Time, granularity string) (*service.FinanceStatsResponse, error) {
	summary := service.FinanceStatsSummary{}
	err := r.db.QueryRowContext(ctx, `
WITH user_recharge AS (
  SELECT COALESCE(SUM(rc.value),0)::float8 AS amount
  FROM redeem_codes rc JOIN users u ON u.id=rc.used_by
  WHERE u.role <> $1 AND u.deleted_at IS NULL AND rc.status=$2 AND rc.value > 0 AND rc.type IN ($3,$4) AND rc.used_by IS NOT NULL
), upstream_recharge AS (
  SELECT COALESCE(SUM(initial_balance),0)::float8 AS amount FROM upstreams WHERE deleted_at IS NULL
), consumed AS (
  SELECT COALESCE(SUM(ul.actual_cost),0)::float8 AS user_cost,
         COALESCE(SUM(ul.actual_cost * COALESCE(up.rate_multiplier,1)),0)::float8 AS upstream_cost
  FROM usage_logs ul
  JOIN users u ON u.id=ul.user_id AND u.role <> $1 AND u.deleted_at IS NULL
  LEFT JOIN accounts a ON a.id=ul.account_id AND a.deleted_at IS NULL
  LEFT JOIN upstreams up ON up.normalized_base_url = trim(trailing '/' FROM lower(COALESCE(NULLIF(a.credentials->>'base_url',''), default_account_base_url(a.platform, a.type)))) AND up.deleted_at IS NULL
)
SELECT user_recharge.amount, upstream_recharge.amount, consumed.user_cost, consumed.upstream_cost
FROM user_recharge, upstream_recharge, consumed`, service.RoleAdmin, service.StatusUsed, service.RedeemTypeBalance, service.AdjustmentTypeAdminBalance).Scan(&summary.UserRechargeTotal, &summary.UpstreamRechargeTotal, &summary.UserConsumedAmount, &summary.UpstreamConsumedAmount)
	if err != nil {
		return nil, err
	}
	summary.ConsumedProfit = service.RoundMoneyForRepo(summary.UserConsumedAmount - summary.UpstreamConsumedAmount)
	if summary.UserConsumedAmount > 0 {
		summary.ConsumedProfitRate = service.RoundMoneyForRepo(summary.ConsumedProfit / summary.UserConsumedAmount * 100)
	}
	trend, err := r.financeTrend(ctx, start, end, granularity)
	if err != nil {
		return nil, err
	}
	return &service.FinanceStatsResponse{Summary: summary, Trend: trend, StartDate: start.Format(time.RFC3339), EndDate: end.Format(time.RFC3339), Granularity: granularity, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (r *upstreamRepository) ListBalanceAlertCandidates(ctx context.Context) ([]*service.Upstream, error) {
	rows, err := r.db.QueryContext(ctx, upstreamListSQL+" WHERE u.deleted_at IS NULL AND u.balance_alert_enabled = TRUE AND u.alert_balance IS NOT NULL AND u.alert_email_sent_at IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanUpstreams(rows)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *upstreamRepository) MarkBalanceAlertSent(ctx context.Context, id int64, currentBalance float64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE upstreams SET alert_email_sent_at=NOW(), alert_last_balance=$2, updated_at=NOW() WHERE id=$1`, id, currentBalance)
	return err
}

func (r *upstreamRepository) ResetBalanceAlert(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE upstreams SET alert_email_sent_at=NULL, alert_last_balance=NULL, updated_at=NOW() WHERE id=$1`, id)
	return err
}

const upstreamListSQL = `
SELECT u.id, u.base_url, u.normalized_base_url, u.name, u.rate_multiplier::float8, u.initial_balance::float8,
       COALESCE(consumed.amount,0)::float8 AS consumed_balance,
       (u.initial_balance - COALESCE(consumed.amount,0))::float8 AS current_balance,
       COALESCE(accounts.account_count,0)::bigint AS account_count,
       u.balance_alert_enabled, u.alert_balance::float8, u.alert_email_sent_at, u.alert_last_balance::float8, COALESCE(u.notes,''), u.created_at, u.updated_at
FROM upstreams u
LEFT JOIN LATERAL (
  SELECT COUNT(*)::bigint AS account_count
  FROM accounts a
  WHERE a.deleted_at IS NULL AND trim(trailing '/' FROM lower(COALESCE(NULLIF(a.credentials->>'base_url',''), default_account_base_url(a.platform, a.type)))) = u.normalized_base_url
) accounts ON TRUE
LEFT JOIN LATERAL (
  SELECT COALESCE(SUM(ul.actual_cost * u.rate_multiplier),0)::float8 AS amount
  FROM usage_logs ul
  JOIN users usr ON usr.id=ul.user_id AND usr.role <> 'admin' AND usr.deleted_at IS NULL
  JOIN accounts a ON a.id=ul.account_id AND a.deleted_at IS NULL
  WHERE trim(trailing '/' FROM lower(COALESCE(NULLIF(a.credentials->>'base_url',''), default_account_base_url(a.platform, a.type)))) = u.normalized_base_url
) consumed ON TRUE`

func scanUpstreams(rows *sql.Rows) ([]*service.Upstream, error) {
	var items []*service.Upstream
	for rows.Next() {
		item := &service.Upstream{}
		var alertBalance, alertLast sql.NullFloat64
		var alertSent sql.NullTime
		if err := rows.Scan(&item.ID, &item.BaseURL, &item.NormalizedBaseURL, &item.Name, &item.RateMultiplier, &item.InitialBalance, &item.ConsumedBalance, &item.CurrentBalance, &item.AccountCount, &item.BalanceAlertEnabled, &alertBalance, &alertSent, &alertLast, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.InitialBalance = service.RoundMoneyForRepo(item.InitialBalance)
		item.ConsumedBalance = service.RoundMoneyForRepo(item.ConsumedBalance)
		item.CurrentBalance = service.RoundMoneyForRepo(item.CurrentBalance)
		if alertBalance.Valid {
			v := alertBalance.Float64
			item.AlertBalance = &v
		}
		if alertLast.Valid {
			v := alertLast.Float64
			item.AlertLastBalance = &v
		}
		if alertSent.Valid {
			t := alertSent.Time
			item.AlertEmailSentAt = &t
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *upstreamRepository) upstreamStatsSummary(ctx context.Context) (*service.UpstreamStatsSummary, error) {
	s := &service.UpstreamStatsSummary{}
	err := r.db.QueryRowContext(ctx, `
WITH upstream_balance AS (`+strings.TrimPrefix(upstreamListSQL, "\n")+` WHERE u.deleted_at IS NULL), token_totals AS (
  SELECT COALESCE(SUM(ul.input_tokens),0)::bigint, COALESCE(SUM(ul.output_tokens),0)::bigint,
         COALESCE(SUM(ul.cache_creation_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens),0)::bigint,
         COALESCE(SUM(ul.cache_read_tokens),0)::bigint
  FROM usage_logs ul JOIN users usr ON usr.id=ul.user_id AND usr.role <> $1 AND usr.deleted_at IS NULL
)
SELECT COUNT(*)::bigint, COALESCE(SUM(current_balance),0)::float8, COALESCE(SUM(initial_balance),0)::float8, COALESCE(SUM(consumed_balance),0)::float8,
       token_totals.sum, token_totals.sum_1, token_totals.sum_2, token_totals.sum_3
FROM upstream_balance, token_totals
GROUP BY token_totals.sum, token_totals.sum_1, token_totals.sum_2, token_totals.sum_3`, service.RoleAdmin).Scan(&s.UpstreamCount, &s.TotalCurrentBalance, &s.TotalInitialBalance, &s.TotalConsumedBalance, &s.TotalInputTokens, &s.TotalOutputTokens, &s.TotalCacheWriteTokens, &s.TotalCacheReadTokens)
	if err != nil {
		return nil, err
	}
	s.TotalTokens = s.TotalInputTokens + s.TotalOutputTokens + s.TotalCacheWriteTokens + s.TotalCacheReadTokens
	s.TotalCurrentBalance = service.RoundMoneyForRepo(s.TotalCurrentBalance)
	s.TotalInitialBalance = service.RoundMoneyForRepo(s.TotalInitialBalance)
	s.TotalConsumedBalance = service.RoundMoneyForRepo(s.TotalConsumedBalance)
	return s, nil
}

func (r *upstreamRepository) upstreamCostBars(ctx context.Context, start, end time.Time) ([]service.UpstreamCostPoint, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT u.id, u.name, COALESCE(SUM(ul.actual_cost * u.rate_multiplier),0)::float8,
       COALESCE(SUM(ul.input_tokens),0)::bigint, COALESCE(SUM(ul.output_tokens),0)::bigint,
       COALESCE(SUM(ul.cache_creation_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens),0)::bigint,
       COALESCE(SUM(ul.cache_read_tokens),0)::bigint
FROM upstreams u
LEFT JOIN accounts a ON a.deleted_at IS NULL AND trim(trailing '/' FROM lower(COALESCE(NULLIF(a.credentials->>'base_url',''), default_account_base_url(a.platform, a.type)))) = u.normalized_base_url
LEFT JOIN usage_logs ul ON ul.account_id=a.id AND ul.created_at >= $1 AND ul.created_at < $2
LEFT JOIN users usr ON usr.id=ul.user_id AND usr.role <> $3 AND usr.deleted_at IS NULL
WHERE u.deleted_at IS NULL
GROUP BY u.id, u.name
ORDER BY 3 DESC, u.id`, start, end, service.RoleAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service.UpstreamCostPoint
	for rows.Next() {
		var p service.UpstreamCostPoint
		var id int64
		if err := rows.Scan(&id, &p.UpstreamName, &p.ConsumedBalance, &p.InputTokens, &p.OutputTokens, &p.CacheWriteTokens, &p.CacheReadTokens); err != nil {
			return nil, err
		}
		p.UpstreamID = &id
		p.TotalTokens = p.InputTokens + p.OutputTokens + p.CacheWriteTokens + p.CacheReadTokens
		p.ConsumedBalance = service.RoundMoneyForRepo(p.ConsumedBalance)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *upstreamRepository) upstreamTokenTrend(ctx context.Context, start, end time.Time, granularity string) ([]service.UpstreamCostPoint, error) {
	bucket := bucketExpr(granularity, "ul.created_at")
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s AS bucket, COALESCE(SUM(ul.actual_cost * COALESCE(u.rate_multiplier,1)),0)::float8,
       COALESCE(SUM(ul.input_tokens),0)::bigint, COALESCE(SUM(ul.output_tokens),0)::bigint,
       COALESCE(SUM(ul.cache_creation_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens),0)::bigint,
       COALESCE(SUM(ul.cache_read_tokens),0)::bigint
FROM usage_logs ul
JOIN users usr ON usr.id=ul.user_id AND usr.role <> $3 AND usr.deleted_at IS NULL
LEFT JOIN accounts a ON a.id=ul.account_id AND a.deleted_at IS NULL
LEFT JOIN upstreams u ON u.normalized_base_url = trim(trailing '/' FROM lower(COALESCE(NULLIF(a.credentials->>'base_url',''), default_account_base_url(a.platform, a.type)))) AND u.deleted_at IS NULL
WHERE ul.created_at >= $1 AND ul.created_at < $2
GROUP BY 1 ORDER BY 1`, bucket), start, end, service.RoleAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service.UpstreamCostPoint
	for rows.Next() {
		var p service.UpstreamCostPoint
		var t time.Time
		if err := rows.Scan(&t, &p.ConsumedBalance, &p.InputTokens, &p.OutputTokens, &p.CacheWriteTokens, &p.CacheReadTokens); err != nil {
			return nil, err
		}
		p.Bucket = t.Format(time.RFC3339)
		p.TotalTokens = p.InputTokens + p.OutputTokens + p.CacheWriteTokens + p.CacheReadTokens
		p.ConsumedBalance = service.RoundMoneyForRepo(p.ConsumedBalance)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *upstreamRepository) financeTrend(ctx context.Context, start, end time.Time, granularity string) ([]service.FinanceTrendPoint, error) {
	bucketUsage := bucketExpr(granularity, "ul.created_at")
	bucketRedeem := bucketExpr(granularity, "rc.used_at")
	query := fmt.Sprintf(`
WITH usage_points AS (
  SELECT %s AS bucket, COALESCE(SUM(ul.actual_cost),0)::float8 AS user_cost, COALESCE(SUM(ul.actual_cost * COALESCE(up.rate_multiplier,1)),0)::float8 AS upstream_cost
  FROM usage_logs ul
  JOIN users u ON u.id=ul.user_id AND u.role <> $3 AND u.deleted_at IS NULL
  LEFT JOIN accounts a ON a.id=ul.account_id AND a.deleted_at IS NULL
  LEFT JOIN upstreams up ON up.normalized_base_url = trim(trailing '/' FROM lower(COALESCE(NULLIF(a.credentials->>'base_url',''), default_account_base_url(a.platform, a.type)))) AND up.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $2
  GROUP BY 1
), recharge_points AS (
  SELECT %s AS bucket, COALESCE(SUM(rc.value),0)::float8 AS user_recharge
  FROM redeem_codes rc JOIN users u ON u.id=rc.used_by AND u.role <> $3 AND u.deleted_at IS NULL
  WHERE rc.used_at >= $1 AND rc.used_at < $2 AND rc.status=$4 AND rc.value > 0 AND rc.type IN ($5,$6)
  GROUP BY 1
), buckets AS (SELECT bucket FROM usage_points UNION SELECT bucket FROM recharge_points)
SELECT b.bucket, COALESCE(up.user_cost,0)::float8, COALESCE(up.upstream_cost,0)::float8, COALESCE(rp.user_recharge,0)::float8
FROM buckets b LEFT JOIN usage_points up ON up.bucket=b.bucket LEFT JOIN recharge_points rp ON rp.bucket=b.bucket ORDER BY b.bucket`, bucketUsage, bucketRedeem)
	rows, err := r.db.QueryContext(ctx, query, start, end, service.RoleAdmin, service.StatusUsed, service.RedeemTypeBalance, service.AdjustmentTypeAdminBalance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service.FinanceTrendPoint
	for rows.Next() {
		var p service.FinanceTrendPoint
		var t time.Time
		if err := rows.Scan(&t, &p.UserConsumedAmount, &p.UpstreamConsumedAmount, &p.UserRecharge); err != nil {
			return nil, err
		}
		p.Bucket = t.Format(time.RFC3339)
		p.UpstreamCost = service.RoundMoneyForRepo(p.UpstreamConsumedAmount)
		p.Profit = service.RoundMoneyForRepo(p.UserConsumedAmount - p.UpstreamConsumedAmount)
		out = append(out, p)
	}
	return out, rows.Err()
}

func bucketExpr(granularity, column string) string {
	switch granularity {
	case "hour":
		return "date_trunc('hour', " + column + ")"
	case "month":
		return "date_trunc('month', " + column + ")"
	default:
		return "date_trunc('day', " + column + ")"
	}
}
