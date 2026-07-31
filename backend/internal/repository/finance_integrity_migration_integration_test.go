//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestFinanceIntegrityMigrationBackfillsOnlyProvableFactsAndBootstrapsWallets(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	suffix := fmt.Sprintf("migration-178-%d", time.Now().UnixNano())
	legacyTime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	baseURL := "https://" + suffix + ".example.test"
	var userID, accountID, unresolvedAccountID, apiKeyID, upstreamID int64
	require.NoError(t, tx.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash,role) VALUES($1,'test','admin') RETURNING id`, suffix+"@example.test").Scan(&userID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts(name,platform,type,credentials,status,created_at)
VALUES($1,'openai','apikey',jsonb_build_object('base_url',$2::text),'active',$3) RETURNING id`,
		suffix, baseURL, legacyTime).Scan(&accountID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts(name,platform,type,credentials,status,created_at)
VALUES($1,'anthropic','oauth','{}'::jsonb,'active',$2) RETURNING id`,
		suffix+"-unresolved", legacyTime).Scan(&unresolvedAccountID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO upstreams(base_url,normalized_base_url,name)
VALUES($1,$1,$2) RETURNING id`, baseURL, suffix).Scan(&upstreamID))
	require.NoError(t, tx.QueryRowContext(ctx,
		`INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'migration') RETURNING id`, userID, "sk-"+suffix).Scan(&apiKeyID))

	_, err := tx.ExecContext(ctx, `
INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,billing_type,subscription_id,created_at)
VALUES($1,$2,$3,$4,'migration-model',1,NULL,$6),
      ($1,$2,$3,$5,'migration-model',0,NULL,$6)`,
		userID, apiKeyID, accountID, "subscription-"+suffix, "unknown-"+suffix, legacyTime)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
INSERT INTO usage_billing_dedup(request_id,api_key_id,request_fingerprint,created_at)
VALUES($1,$3,repeat('a',64),$4),($2,$3,repeat('b',64),$4)`,
		"subscription-"+suffix, "unknown-"+suffix, apiKeyID, legacyTime)
	require.NoError(t, err)
	var promoCodeID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO promo_codes(code,bonus_amount) VALUES($1,12.5) RETURNING id`, "M178"+fmt.Sprint(time.Now().UnixNano())).Scan(&promoCodeID))
	_, err = tx.ExecContext(ctx, `
INSERT INTO promo_code_usages(promo_code_id,user_id,bonus_amount,used_at)
VALUES($1,$2,12.5,$3)`, promoCodeID, userID, legacyTime)
	require.NoError(t, err)

	content, err := migrations.FS.ReadFile("178_finance_integrity_and_wallet_bootstrap.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err)

	var businessType string
	var recorded bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT finance_business_type,finance_classification_recorded
FROM usage_billing_dedup WHERE request_id=$1 AND api_key_id=$2`,
		"subscription-"+suffix, apiKeyID).Scan(&businessType, &recorded))
	require.Equal(t, "balance", businessType)
	require.False(t, recorded, "pre-cutover subscription facts cannot prove admin/test exclusion state")
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT finance_business_type,finance_classification_recorded
FROM usage_billing_dedup WHERE request_id=$1 AND api_key_id=$2`,
		"unknown-"+suffix, apiKeyID).Scan(&businessType, &recorded))
	require.Equal(t, "balance", businessType)
	require.False(t, recorded, "unprovable legacy balance facts must remain explicitly unrecorded")

	var historicalBonus, reconciliationStatus string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT detected_historical_bonus::text,status
FROM user_promotion_credit_reconciliations WHERE user_id=$1`, userID).Scan(&historicalBonus, &reconciliationStatus))
	require.Equal(t, "12.5000000000", historicalBonus)
	require.Equal(t, "requires_reconciliation", reconciliationStatus)

	var walletID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT id FROM upstream_wallets WHERE upstream_id=$1 AND name='系统默认钱包' AND deleted_at IS NULL`, upstreamID).Scan(&walletID))
	var assignmentCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM upstream_wallet_accounts
WHERE wallet_id=$1 AND account_id=$2 AND effective_to IS NULL`, walletID, accountID).Scan(&assignmentCount))
	require.Equal(t, 1, assignmentCount)
	var pendingReason string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT reason FROM upstream_wallet_assignment_pending WHERE account_id=$1 AND resolved_at IS NULL`,
		unresolvedAccountID).Scan(&pendingReason))
	require.Equal(t, "empty_base_url", pendingReason)

	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "migration must remain idempotent")
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM upstream_wallet_accounts
WHERE wallet_id=$1 AND account_id=$2 AND effective_to IS NULL`, walletID, accountID).Scan(&assignmentCount))
	require.Equal(t, 1, assignmentCount)
}

func TestFinanceIntegrityMigrationCreatesUpstreamAndWalletFromActiveAccount(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	suffix := fmt.Sprintf("migration-178-empty-upstreams-%d", time.Now().UnixNano())
	baseURL := "https://" + suffix + ".example.test"
	var accountID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts(name,platform,type,credentials,status,created_at)
VALUES($1,'openai','apikey',jsonb_build_object('base_url',$2::text),'active',$3) RETURNING id`,
		suffix, baseURL, time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)).Scan(&accountID))
	_, err := tx.ExecContext(ctx, `DELETE FROM upstreams WHERE normalized_base_url=$1`, baseURL)
	require.NoError(t, err)
	content, err := migrations.FS.ReadFile("178_finance_integrity_and_wallet_bootstrap.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err)
	var upstreamID, walletID int64
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT id FROM upstreams WHERE normalized_base_url=$1 AND deleted_at IS NULL`, baseURL).Scan(&upstreamID))
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT id FROM upstream_wallets WHERE upstream_id=$1 AND name='系统默认钱包'`, upstreamID).Scan(&walletID))
	var count int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_wallet_accounts WHERE wallet_id=$1 AND account_id=$2 AND effective_to IS NULL`, walletID, accountID).Scan(&count))
	require.Equal(t, 1, count)
}
