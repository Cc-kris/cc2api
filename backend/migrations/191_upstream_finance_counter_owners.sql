CREATE TABLE IF NOT EXISTS upstream_finance_counter_owners (
    counter_identity_key VARCHAR(200) PRIMARY KEY,
    wallet_id BIGINT NOT NULL REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    protocol_version_id BIGINT REFERENCES upstream_finance_protocol_versions(id) ON DELETE RESTRICT,
    owner_account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    upstream_counter_id VARCHAR(200),
    counter_period VARCHAR(100),
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_upstream_finance_counter_owner_account
    ON upstream_finance_counter_owners(owner_account_id, last_seen_at DESC);

COMMENT ON TABLE upstream_finance_counter_owners IS
    'Global ownership fence for cumulative upstream counters. One stable counter identity cannot settle multiple accounts.';
