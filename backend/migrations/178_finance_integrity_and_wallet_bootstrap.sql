-- Finance classification integrity, legacy cut-over audit, and wallet bootstrap.

ALTER TABLE usage_billing_dedup
    ADD COLUMN IF NOT EXISTS finance_classification_recorded BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE usage_billing_dedup_archive
    ADD COLUMN IF NOT EXISTS finance_classification_recorded BOOLEAN NOT NULL DEFAULT FALSE;

-- Rows written after the classification migration were recorded by the new billing path.
UPDATE usage_billing_dedup d
SET finance_classification_recorded = TRUE
WHERE d.finance_classification_recorded = FALSE
  AND d.created_at >= COALESCE(
      (SELECT applied_at FROM schema_migrations WHERE filename='177_finance_usage_classification_facts.sql'),
      'infinity'::timestamptz
  );
UPDATE usage_billing_dedup_archive d
SET finance_classification_recorded = TRUE
WHERE d.finance_classification_recorded = FALSE
  AND d.created_at >= COALESCE(
      (SELECT applied_at FROM schema_migrations WHERE filename='177_finance_usage_classification_facts.sql'),
      'infinity'::timestamptz
  );

-- Pre-cutover billing_type can identify subscription charging, but cannot prove
-- the historical admin/test exclusion state. Keep those rows unrecorded so the
-- finance scanner classifies them as legacy_unknown/excluded.

CREATE TABLE IF NOT EXISTS user_promotion_credit_reconciliations (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    detected_historical_bonus DECIMAL(20,10) NOT NULL DEFAULT 0,
    status VARCHAR(30) NOT NULL DEFAULT 'requires_reconciliation',
    cutover_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    resolved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_remaining_amount DECIMAL(20,10),
    notes TEXT,
    CONSTRAINT user_promotion_credit_reconciliations_status_check
        CHECK (status IN ('requires_reconciliation','resolved')),
    CONSTRAINT user_promotion_credit_reconciliations_confirmed_nonnegative
        CHECK (confirmed_remaining_amount IS NULL OR confirmed_remaining_amount >= 0)
);

INSERT INTO user_promotion_credit_reconciliations(user_id,detected_historical_bonus,status,cutover_at)
SELECT p.user_id,SUM(GREATEST(p.bonus_amount,0)),'requires_reconciliation',m.applied_at
FROM promo_code_usages p
CROSS JOIN LATERAL (
    SELECT applied_at FROM schema_migrations WHERE filename='177_finance_usage_classification_facts.sql'
) m
WHERE p.used_at < m.applied_at
GROUP BY p.user_id,m.applied_at
ON CONFLICT(user_id) DO UPDATE
SET detected_historical_bonus=EXCLUDED.detected_historical_bonus,
    cutover_at=EXCLUDED.cutover_at
WHERE user_promotion_credit_reconciliations.status='requires_reconciliation';

CREATE TABLE IF NOT EXISTS upstream_wallet_assignment_pending (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    normalized_base_url TEXT NOT NULL DEFAULT '',
    reason VARCHAR(60) NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT upstream_wallet_assignment_pending_reason_check
        CHECK (reason IN ('empty_base_url','unresolved_upstream'))
);

-- Bootstrap upstreams from active accounts before creating wallets. This mirrors
-- upstreamRepository.SyncFromAccounts and is safe on databases never manually synced.
WITH account_upstreams AS (
    SELECT DISTINCT normalized_account_base_url(credentials,extra,platform,type) AS normalized_base_url
    FROM accounts
    WHERE deleted_at IS NULL AND status='active'
)
INSERT INTO upstreams(
    base_url,normalized_base_url,name,rate_multiplier,initial_balance,
    balance_alert_enabled,notes,created_at,updated_at
)
SELECT normalized_base_url,normalized_base_url,
       LEFT(regexp_replace(normalized_base_url,'^https?://',''),120),
       1.0,0,FALSE,'',NOW(),NOW()
FROM account_upstreams
WHERE normalized_base_url<>''
ON CONFLICT (normalized_base_url) WHERE deleted_at IS NULL DO NOTHING;

-- Every existing active upstream/account group receives one explicit manual wallet.
INSERT INTO upstream_wallets(
    upstream_id,name,base_url,pricing_adapter,balance_adapter,quota_adapter,
    currency,balance_kind,enabled
)
SELECT DISTINCT u.id,'系统默认钱包',u.base_url,'manual','manual','none','USD','wallet_cash',TRUE
FROM upstreams u
JOIN accounts a
  ON normalized_account_base_url(a.credentials,a.extra,a.platform,a.type)=u.normalized_base_url
WHERE u.deleted_at IS NULL AND a.deleted_at IS NULL AND a.status='active'
ON CONFLICT DO NOTHING;

WITH default_wallets AS (
    SELECT DISTINCT ON (upstream_id) id,upstream_id
    FROM upstream_wallets
    WHERE deleted_at IS NULL AND name='系统默认钱包'
    ORDER BY upstream_id,id
), resolvable AS (
    SELECT a.id AS account_id,a.created_at,w.id AS wallet_id
    FROM accounts a
    JOIN upstreams u
      ON u.normalized_base_url=normalized_account_base_url(a.credentials,a.extra,a.platform,a.type)
     AND u.deleted_at IS NULL
    JOIN default_wallets w ON w.upstream_id=u.id
    WHERE a.deleted_at IS NULL AND a.status='active'
)
INSERT INTO upstream_wallet_accounts(wallet_id,account_id,effective_from,reason)
SELECT r.wallet_id,r.account_id,r.created_at,'migration_178_wallet_bootstrap'
FROM resolvable r
WHERE NOT EXISTS (
    SELECT 1 FROM upstream_wallet_accounts uwa
    WHERE uwa.account_id=r.account_id AND uwa.effective_to IS NULL
)
ON CONFLICT DO NOTHING;

INSERT INTO upstream_wallet_assignment_pending(account_id,normalized_base_url,reason,detected_at,resolved_at)
SELECT a.id,
       normalized_account_base_url(a.credentials,a.extra,a.platform,a.type),
       CASE WHEN normalized_account_base_url(a.credentials,a.extra,a.platform,a.type)=''
            THEN 'empty_base_url' ELSE 'unresolved_upstream' END,
       NOW(),NULL
FROM accounts a
WHERE a.deleted_at IS NULL AND a.status='active'
  AND NOT EXISTS (
      SELECT 1 FROM upstreams u
      WHERE u.deleted_at IS NULL
        AND u.normalized_base_url=normalized_account_base_url(a.credentials,a.extra,a.platform,a.type)
  )
ON CONFLICT(account_id) DO UPDATE
SET normalized_base_url=EXCLUDED.normalized_base_url,
    reason=EXCLUDED.reason,
    detected_at=EXCLUDED.detected_at,
    resolved_at=NULL;

UPDATE upstream_wallet_assignment_pending p
SET resolved_at=NOW()
WHERE p.resolved_at IS NULL
  AND EXISTS (
      SELECT 1 FROM upstream_wallet_accounts uwa
      WHERE uwa.account_id=p.account_id AND uwa.effective_to IS NULL
  );
