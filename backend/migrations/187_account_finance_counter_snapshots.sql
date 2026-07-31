CREATE TABLE IF NOT EXISTS account_finance_counter_snapshots (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    scope_key VARCHAR(200) NOT NULL,
    idempotency_key VARCHAR(200) NOT NULL,
    upstream_counter_id VARCHAR(200),
    counter_period VARCHAR(100),
    list_cost_total DECIMAL(20,10),
    actual_cost_total DECIMAL(20,10),
    unit_code VARCHAR(30) NOT NULL,
    unit_semantics VARCHAR(30) NOT NULL,
    currency VARCHAR(3),
    upstream_observed_at TIMESTAMPTZ,
    collected_at TIMESTAMPTZ NOT NULL,
    safe_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    checksum VARCHAR(64) NOT NULL,
    previous_snapshot_id BIGINT REFERENCES account_finance_counter_snapshots(id) ON DELETE RESTRICT,
    list_cost_delta DECIMAL(20,10),
    actual_cost_delta DECIMAL(20,10),
    observed_multiplier DECIMAL(20,10),
    derivation_status VARCHAR(40) NOT NULL,
    anomaly_code VARCHAR(40),
    multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    multiplier_effective_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_finance_counter_semantics_check
        CHECK (unit_semantics IN ('fiat_currency','platform_credit')),
    CONSTRAINT account_finance_counter_currency_check
        CHECK (
            (unit_semantics = 'fiat_currency' AND currency IS NOT NULL AND char_length(currency) = 3)
            OR (unit_semantics = 'platform_credit' AND currency IS NULL)
        ),
    CONSTRAINT account_finance_counter_values_check
        CHECK (
            (list_cost_total IS NULL OR list_cost_total >= 0)
            AND (actual_cost_total IS NULL OR actual_cost_total >= 0)
            AND (list_cost_delta IS NULL OR list_cost_delta >= 0)
            AND (actual_cost_delta IS NULL OR actual_cost_delta >= 0)
            AND (observed_multiplier IS NULL OR observed_multiplier >= 0)
        ),
    CONSTRAINT account_finance_counter_derivation_check
        CHECK (derivation_status IN (
            'baseline','raw_only','missing_values','boundary_changed','time_reversed',
            'counter_reset','no_activity','invalid_list_delta','candidate','applied',
            'unchanged','conflict','inactive_account','invalid_multiplier'
        )),
    CONSTRAINT account_finance_counter_previous_check
        CHECK (previous_snapshot_id IS NULL OR previous_snapshot_id <> id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_account_finance_counter_snapshot_idempotency
    ON account_finance_counter_snapshots(account_id, scope_key, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_account_finance_counter_snapshot_latest
    ON account_finance_counter_snapshots(account_id, scope_key, collected_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_account_finance_counter_snapshot_status
    ON account_finance_counter_snapshots(account_id, derivation_status, collected_at DESC);

COMMENT ON TABLE account_finance_counter_snapshots IS
    'Immutable cumulative upstream cost observations. platform_credit rows are evidence only and never update account multipliers.';
