CREATE TABLE IF NOT EXISTS account_finance_profiles (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    wallet_id BIGINT REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    protocol_version_id BIGINT REFERENCES upstream_finance_protocol_versions(id) ON DELETE RESTRICT,
    cost_mode VARCHAR(40) NOT NULL CHECK (cost_mode IN ('request_charge','cumulative_list_and_actual','cumulative_actual','contract_multiplier','manual')),
    pricing_group VARCHAR(100),
    endpoint_source VARCHAR(30) NOT NULL DEFAULT 'account_base_url' CHECK (endpoint_source IN ('account_base_url','wallet_base_url')),
    endpoint_base_url_snapshot TEXT NOT NULL DEFAULT '',
    credential_source VARCHAR(40) NOT NULL DEFAULT 'account_api_key',
    counter_scope VARCHAR(30) NOT NULL DEFAULT 'account' CHECK (counter_scope IN ('account','wallet','organization')),
    counter_scope_key VARCHAR(200),
    balance_unit_semantics VARCHAR(30) NOT NULL DEFAULT 'none' CHECK (balance_unit_semantics IN ('fiat_currency','platform_credit','none')),
    recharge_owner_type VARCHAR(20) CHECK (recharge_owner_type IN ('account','wallet')),
    recharge_owner_id BIGINT,
    account_multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    account_multiplier_snapshot DECIMAL(20,10) CHECK (account_multiplier_snapshot IS NULL OR account_multiplier_snapshot >= 0),
    raw_upstream_multiplier DECIMAL(20,10) CHECK (raw_upstream_multiplier IS NULL OR raw_upstream_multiplier >= 0),
    contract_type VARCHAR(30) CHECK (contract_type IN ('multiplier','model_price')),
    contract_multiplier DECIMAL(20,10) CHECK (contract_multiplier IS NULL OR contract_multiplier >= 0),
    contract_multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    readiness_status VARCHAR(30) NOT NULL CHECK (readiness_status IN ('ready_exact','ready_priced','ready_contract','pending_settlement','sync_error','unconfigured')),
    readiness_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    version INTEGER NOT NULL CHECK (version > 0),
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reason TEXT NOT NULL CHECK (char_length(reason) BETWEEN 5 AND 500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_finance_profile_period_check CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT account_finance_profile_recharge_owner_check CHECK ((recharge_owner_type IS NULL AND recharge_owner_id IS NULL) OR (recharge_owner_type IS NOT NULL AND recharge_owner_id IS NOT NULL)),
    CONSTRAINT account_finance_profile_contract_check CHECK (contract_type IS DISTINCT FROM 'multiplier' OR contract_multiplier IS NOT NULL),
    CONSTRAINT account_finance_profile_version_key UNIQUE(account_id, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS account_finance_profiles_current_account_key
    ON account_finance_profiles(account_id) WHERE effective_to IS NULL;
CREATE INDEX IF NOT EXISTS account_finance_profiles_account_effective_idx
    ON account_finance_profiles(account_id, effective_from DESC);

COMMENT ON TABLE account_finance_profiles IS
    'Immutable effective-dated account finance configuration. Recharge economics never change request multiplier snapshots.';
