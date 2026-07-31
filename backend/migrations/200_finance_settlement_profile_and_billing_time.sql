ALTER TABLE account_finance_counter_snapshots
    ADD COLUMN IF NOT EXISTS account_finance_profile_id BIGINT REFERENCES account_finance_profiles(id) ON DELETE RESTRICT;

UPDATE account_finance_counter_snapshots snapshot
SET account_finance_profile_id = (
    SELECT id
    FROM account_finance_profiles profile
    WHERE profile.account_id = snapshot.account_id
      AND profile.effective_from <= COALESCE(snapshot.upstream_observed_at, snapshot.collected_at)
      AND (profile.effective_to IS NULL OR profile.effective_to > COALESCE(snapshot.upstream_observed_at, snapshot.collected_at))
    ORDER BY profile.version DESC
    LIMIT 1
)
WHERE snapshot.account_finance_profile_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_account_finance_counter_snapshot_profile_time
    ON account_finance_counter_snapshots(account_finance_profile_id, collected_at DESC, id DESC);

ALTER TABLE usage_upstream_attempts
    ADD COLUMN IF NOT EXISTS billing_observed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

UPDATE usage_upstream_attempts
SET completed_at = created_at
WHERE completed_at IS NULL;

ALTER TABLE usage_upstream_attempts
    ALTER COLUMN completed_at SET DEFAULT NOW(),
    ALTER COLUMN completed_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_upstream_attempt_profile_billing_time
    ON usage_upstream_attempts(
        account_finance_profile_id,
        COALESCE(billing_observed_at, completed_at, created_at),
        usage_log_id,
        attempt_no
    );

ALTER TABLE upstream_cost_settlement_intervals
    ADD COLUMN IF NOT EXISTS account_finance_profile_id BIGINT REFERENCES account_finance_profiles(id) ON DELETE RESTRICT;

UPDATE upstream_cost_settlement_intervals interval
SET account_finance_profile_id = current_snapshot.account_finance_profile_id
FROM account_finance_counter_snapshots previous_snapshot,
     account_finance_counter_snapshots current_snapshot
WHERE interval.previous_snapshot_id = previous_snapshot.id
  AND interval.current_snapshot_id = current_snapshot.id
  AND interval.account_finance_profile_id IS NULL
  AND previous_snapshot.account_finance_profile_id IS NOT DISTINCT FROM current_snapshot.account_finance_profile_id;

UPDATE upstream_cost_settlement_intervals interval
SET status = CASE WHEN interval.status = 'settled' THEN interval.status ELSE 'needs_review' END,
    error_summary = CASE
        WHEN interval.error_summary = '' THEN '财务配置版本跨越结算区间，必须人工复核'
        ELSE interval.error_summary || '; 财务配置版本跨越结算区间，必须人工复核'
    END,
    updated_at = NOW()
FROM account_finance_counter_snapshots previous_snapshot,
     account_finance_counter_snapshots current_snapshot
WHERE interval.previous_snapshot_id = previous_snapshot.id
  AND interval.current_snapshot_id = current_snapshot.id
  AND previous_snapshot.account_finance_profile_id IS DISTINCT FROM current_snapshot.account_finance_profile_id;

CREATE INDEX IF NOT EXISTS idx_upstream_settlement_profile_period
    ON upstream_cost_settlement_intervals(account_finance_profile_id, period_end DESC, id DESC);

COMMENT ON COLUMN account_finance_counter_snapshots.account_finance_profile_id IS
    'Immutable account finance profile active when the cumulative upstream observation was collected.';
COMMENT ON COLUMN usage_upstream_attempts.billing_observed_at IS
    'Upstream-provided billing timestamp when available; completed_at is the formal fallback.';
COMMENT ON COLUMN upstream_cost_settlement_intervals.account_finance_profile_id IS
    'Finance profile boundary shared by both cumulative snapshots and every allocated request attempt.';
