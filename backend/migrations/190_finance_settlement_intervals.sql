CREATE TABLE IF NOT EXISTS upstream_cost_settlement_intervals (
    id BIGSERIAL PRIMARY KEY,
    owner_type VARCHAR(20) NOT NULL CHECK (owner_type IN ('account','wallet')),
    owner_id BIGINT NOT NULL CHECK (owner_id > 0),
    account_id BIGINT REFERENCES accounts(id) ON DELETE RESTRICT,
    wallet_id BIGINT REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    scope_key VARCHAR(240) NOT NULL,
    previous_snapshot_id BIGINT NOT NULL REFERENCES account_finance_counter_snapshots(id) ON DELETE RESTRICT,
    current_snapshot_id BIGINT NOT NULL REFERENCES account_finance_counter_snapshots(id) ON DELETE RESTRICT,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    unit_semantics VARCHAR(30) NOT NULL CHECK (unit_semantics IN ('fiat_currency','platform_credit')),
    currency VARCHAR(3),
    list_cost_delta DECIMAL(20,10) NOT NULL CHECK (list_cost_delta >= 0),
    actual_cost_delta DECIMAL(20,10) NOT NULL CHECK (actual_cost_delta >= 0),
    observed_multiplier DECIMAL(20,10) NOT NULL CHECK (observed_multiplier >= 0),
    status VARCHAR(30) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','settled','needs_review','failed')),
    current_revision INTEGER NOT NULL DEFAULT 1 CHECK (current_revision > 0),
    request_count BIGINT NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    segment_count BIGINT NOT NULL DEFAULT 0 CHECK (segment_count >= 0),
    standard_cost_total DECIMAL(20,10),
    allocated_cost_total DECIMAL(20,10),
    difference_amount DECIMAL(20,10),
    error_summary TEXT NOT NULL DEFAULT '',
    settled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_cost_settlement_interval_period_check CHECK (period_end > period_start),
    CONSTRAINT upstream_cost_settlement_interval_owner_check CHECK (
        (owner_type='account' AND account_id=owner_id AND wallet_id IS NULL) OR
        (owner_type='wallet' AND wallet_id=owner_id AND account_id IS NULL)
    ),
    CONSTRAINT upstream_cost_settlement_interval_snapshot_order_check CHECK (current_snapshot_id <> previous_snapshot_id),
    CONSTRAINT upstream_cost_settlement_interval_scope_snapshots_key UNIQUE (scope_key, previous_snapshot_id, current_snapshot_id)
);

CREATE INDEX IF NOT EXISTS upstream_cost_settlement_intervals_owner_period_idx
    ON upstream_cost_settlement_intervals(owner_type, owner_id, period_end DESC);
CREATE INDEX IF NOT EXISTS upstream_cost_settlement_intervals_status_created_idx
    ON upstream_cost_settlement_intervals(status, created_at);

CREATE TABLE IF NOT EXISTS usage_cost_settlement_allocations (
    id BIGSERIAL PRIMARY KEY,
    settlement_interval_id BIGINT NOT NULL REFERENCES upstream_cost_settlement_intervals(id) ON DELETE RESTRICT,
    usage_log_id BIGINT NOT NULL REFERENCES usage_logs(id) ON DELETE RESTRICT,
    attempt_no INTEGER NOT NULL CHECK (attempt_no > 0),
    revision INTEGER NOT NULL CHECK (revision > 0),
    standard_cost_weight DECIMAL(20,10) NOT NULL CHECK (standard_cost_weight >= 0),
    allocation_rate DECIMAL(24,16) NOT NULL CHECK (allocation_rate >= 0 AND allocation_rate <= 1),
    allocated_cost DECIMAL(20,10) NOT NULL CHECK (allocated_cost >= 0),
    invalidated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_cost_settlement_allocation_revision_key UNIQUE (settlement_interval_id, usage_log_id, attempt_no, revision)
);

CREATE INDEX IF NOT EXISTS usage_cost_settlement_allocations_usage_idx
    ON usage_cost_settlement_allocations(usage_log_id, attempt_no, invalidated_at);

-- Settlement is derived only from upstream cumulative usage evidence and local
-- request facts. Recharge and bonus-income tables are intentionally absent.
