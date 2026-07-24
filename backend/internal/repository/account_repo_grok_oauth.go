package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *accountRepository) ListOAuthRefreshCandidatePage(ctx context.Context, options service.OAuthRefreshPageOptions) (*service.OAuthRefreshCandidatePage, error) {
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	if len(options.Platforms) == 0 {
		return nil, errors.New("oauth refresh candidate platforms cannot be empty")
	}
	if options.Limit <= 0 || options.Limit > 1000 {
		return nil, errors.New("oauth refresh candidate page limit must be between 1 and 1000")
	}
	query := `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND platform = ANY($1)
			AND id > $2`
	if options.ActiveOnly {
		query += ` AND status = 'active'`
	}
	if options.IncludeSetupToken {
		query += ` AND type IN ('oauth', 'setup-token')`
	} else {
		query += ` AND type = 'oauth'`
	}
	if options.RequireRefreshToken {
		query += ` AND credentials ? 'refresh_token' AND btrim(credentials->>'refresh_token') <> ''`
	}
	if options.ExcludeRetryCooldown {
		query += ` AND (temp_unschedulable_until > NOW() AND temp_unschedulable_reason LIKE 'token refresh retry exhausted:%') IS NOT TRUE`
	}
	query += ` ORDER BY id ASC LIMIT $3`

	rows, err := r.sql.QueryContext(ctx, query, pq.Array(options.Platforms), options.AfterID, options.Limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &service.OAuthRefreshCandidatePage{Accounts: []service.Account{}}, nil
	}
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			byID[account.ID] = account
		}
	}
	out := make([]service.Account, 0, len(accounts))
	for _, id := range ids {
		if account := byID[id]; account != nil {
			out = append(out, *account)
		}
	}
	return &service.OAuthRefreshCandidatePage{
		Accounts:    out,
		NextAfterID: ids[len(ids)-1],
		HasMore:     len(ids) == options.Limit,
	}, nil
}

func (r *accountRepository) SetGrokOAuthErrorIfCredentialsUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, errorMsg string) (bool, error) {
	expectedJSON, err := marshalGrokCredentialsForCAS(r, expectedCredentials)
	if err != nil {
		return false, err
	}
	return r.execGrokCredentialCAS(ctx, id, `
		WITH updated AS (
		UPDATE accounts AS a SET status = $1, error_message = $2, schedulable = FALSE, updated_at = NOW()
		WHERE a.id = $3 AND a.deleted_at IS NULL AND a.platform = $4 AND a.type = $5
			AND a.status = $6 AND a.credentials = $7::jsonb
			AND NULLIF(BTRIM(a.credentials->>'refresh_token'), '') IS NULL
		RETURNING a.id)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $8, updated.id, NULL, NULL FROM updated`,
		service.StatusError, errorMsg, id, service.PlatformGrok, service.AccountTypeOAuth,
		service.StatusActive, expectedJSON, service.SchedulerOutboxEventAccountChanged)
}

func (r *accountRepository) UpdateGrokOAuthCredentialsIfUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, expectedProxyID *int64, credentials map[string]any) (bool, error) {
	expectedJSON, err := marshalGrokCredentialsForCAS(r, expectedCredentials)
	if err != nil {
		return false, err
	}
	credentialsJSON, err := marshalGrokCredentialsForCAS(r, credentials)
	if err != nil {
		return false, err
	}
	return r.execGrokCredentialCAS(ctx, id, `
		WITH updated AS (
		UPDATE accounts AS a SET credentials = $1::jsonb, updated_at = NOW()
		WHERE a.id = $2 AND a.deleted_at IS NULL AND a.platform = $3 AND a.type = $4
			AND a.credentials = $5::jsonb AND a.proxy_id IS NOT DISTINCT FROM $6
		RETURNING a.id)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $7, updated.id, NULL, NULL FROM updated`,
		credentialsJSON, id, service.PlatformGrok, service.AccountTypeOAuth, expectedJSON,
		expectedProxyID, service.SchedulerOutboxEventAccountChanged)
}

func (r *accountRepository) SetGrokOAuthRefreshErrorIfCredentialsUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, expectedProxyID *int64, errorMsg string) (bool, error) {
	expectedJSON, err := marshalGrokCredentialsForCAS(r, expectedCredentials)
	if err != nil {
		return false, err
	}
	return r.execGrokCredentialCAS(ctx, id, `
		WITH updated AS (
		UPDATE accounts AS a SET status = $1, error_message = $2, schedulable = FALSE, updated_at = NOW()
		WHERE a.id = $3 AND a.deleted_at IS NULL AND a.platform = $4 AND a.type = $5
			AND a.status = $6 AND a.credentials = $7::jsonb AND a.proxy_id IS NOT DISTINCT FROM $8
		RETURNING a.id)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $9, updated.id, NULL, NULL FROM updated`,
		service.StatusError, errorMsg, id, service.PlatformGrok, service.AccountTypeOAuth,
		service.StatusActive, expectedJSON, expectedProxyID, service.SchedulerOutboxEventAccountChanged)
}

func (r *accountRepository) SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, expectedProxyID *int64, until time.Time, reason string) (bool, error) {
	expectedJSON, err := marshalGrokCredentialsForCAS(r, expectedCredentials)
	if err != nil {
		return false, err
	}
	return r.execGrokCredentialCAS(ctx, id, `
		WITH updated AS (
		UPDATE accounts AS a SET temp_unschedulable_until = $1, temp_unschedulable_reason = $2, updated_at = NOW()
		WHERE a.id = $3 AND a.deleted_at IS NULL AND a.platform = $4 AND a.type = $5
			AND a.status = $6 AND a.credentials = $7::jsonb AND a.proxy_id IS NOT DISTINCT FROM $8
			AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until < $1)
		RETURNING a.id)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $9, updated.id, NULL, NULL FROM updated`,
		until, reason, id, service.PlatformGrok, service.AccountTypeOAuth,
		service.StatusActive, expectedJSON, expectedProxyID, service.SchedulerOutboxEventAccountChanged)
}

func marshalGrokCredentialsForCAS(r *accountRepository, credentials map[string]any) (string, error) {
	if r == nil || r.sql == nil {
		return "", errors.New("account repository SQL executor is not configured")
	}
	body, err := json.Marshal(normalizeJSONMap(credentials))
	return string(body), err
}

func (r *accountRepository) execGrokCredentialCAS(ctx context.Context, id int64, query string, args ...any) (bool, error) {
	result, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return true, nil
}
