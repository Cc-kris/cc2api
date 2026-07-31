-- Finance cost ledger, immutable pricing snapshots, upstream wallets, and reconciliation.
-- This migration is additive. Existing finance fields and endpoints remain available during rollout.

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS upstream_cost_multiplier DECIMAL(10,4),
    ADD COLUMN IF NOT EXISTS upstream_cost_multiplier_updated_at TIMESTAMPTZ;

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_cost_multiplier DECIMAL(10,4),
    ADD COLUMN IF NOT EXISTS sales_model VARCHAR(100),
    ADD COLUMN IF NOT EXISTS sales_pricing_effective_model VARCHAR(100),
    ADD COLUMN IF NOT EXISTS sales_pricing_legacy_source VARCHAR(20),
    ADD COLUMN IF NOT EXISTS sales_pricing_version VARCHAR(10),
    ADD COLUMN IF NOT EXISTS sales_pricing_source VARCHAR(30),
    ADD COLUMN IF NOT EXISTS sales_pricing_checksum VARCHAR(128),
    ADD COLUMN IF NOT EXISTS sales_pricing_snapshot JSONB,
    ADD COLUMN IF NOT EXISTS sales_pricing_shadow_snapshot JSONB,
    ADD COLUMN IF NOT EXISTS sales_pricing_shadow_delta DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS usage_list_value DECIMAL(20,10);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'accounts_upstream_cost_multiplier_check'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_upstream_cost_multiplier_check
            CHECK (
                upstream_cost_multiplier IS NULL
                OR upstream_cost_multiplier BETWEEN 0.0001 AND 9999.9999
            ) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_upstream_cost_multiplier_check'
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_upstream_cost_multiplier_check
            CHECK (
                upstream_cost_multiplier IS NULL
                OR upstream_cost_multiplier BETWEEN 0.0001 AND 9999.9999
            ) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_sales_pricing_version_check'
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_sales_pricing_version_check
            CHECK (
                sales_pricing_version IS NULL
                OR sales_pricing_version IN ('legacy', 'shadow', 'v2')
            ) NOT VALID;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_accounts_upstream_cost_multiplier
    ON accounts (upstream_cost_multiplier)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_sales_pricing_version_created
    ON usage_logs (sales_pricing_version, created_at DESC);

CREATE TABLE IF NOT EXISTS system_model_price_versions (
    id BIGSERIAL PRIMARY KEY,
    catalog_checksum VARCHAR(128) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    model_name VARCHAR(100) NOT NULL,
    billing_mode VARCHAR(20) NOT NULL,
    price_detail JSONB NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT system_model_price_versions_billing_mode_check
        CHECK (billing_mode IN ('token', 'per_request', 'image', 'per_second')),
    CONSTRAINT system_model_price_versions_range_check
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_system_model_price_versions_unique
    ON system_model_price_versions (catalog_checksum, provider, model_name, billing_mode, effective_from);
CREATE INDEX IF NOT EXISTS idx_system_model_price_versions_lookup
    ON system_model_price_versions (provider, model_name, effective_from DESC, effective_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_system_model_price_versions_current
    ON system_model_price_versions (provider, model_name, billing_mode)
    WHERE effective_to IS NULL;

CREATE TABLE IF NOT EXISTS upstream_wallets (
    id BIGSERIAL PRIMARY KEY,
    upstream_id BIGINT NOT NULL REFERENCES upstreams(id) ON DELETE RESTRICT,
    name VARCHAR(120) NOT NULL,
    base_url TEXT,
    pricing_adapter VARCHAR(30) NOT NULL DEFAULT 'manual',
    pricing_group VARCHAR(100),
    balance_adapter VARCHAR(30) NOT NULL DEFAULT 'manual',
    quota_adapter VARCHAR(30) NOT NULL DEFAULT 'none',
    balance_scope_key VARCHAR(200),
    finance_access_token_encrypted BYTEA,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    balance_kind VARCHAR(20) NOT NULL DEFAULT 'wallet_cash',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_pricing_sync_at TIMESTAMPTZ,
    pricing_sync_status VARCHAR(20) NOT NULL DEFAULT 'idle',
    pricing_sync_error TEXT,
    last_balance_sync_at TIMESTAMPTZ,
    balance_sync_status VARCHAR(20) NOT NULL DEFAULT 'idle',
    balance_sync_error TEXT,
    last_quota_sync_at TIMESTAMPTZ,
    quota_sync_status VARCHAR(20) NOT NULL DEFAULT 'idle',
    quota_sync_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT upstream_wallets_pricing_adapter_check
        CHECK (pricing_adapter IN ('manual', 'newapi', 'none')),
    CONSTRAINT upstream_wallets_balance_adapter_check
        CHECK (balance_adapter IN ('manual', 'newapi_user', 'none')),
    CONSTRAINT upstream_wallets_quota_adapter_check
        CHECK (quota_adapter IN ('legacy_openai', 'newapi', 'none')),
    CONSTRAINT upstream_wallets_balance_kind_check
        CHECK (balance_kind IN ('wallet_cash', 'token_quota')),
    CONSTRAINT upstream_wallets_currency_check
        CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_wallets_name_active
    ON upstream_wallets (upstream_id, lower(name))
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_upstream_wallets_active
    ON upstream_wallets (upstream_id, enabled)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_upstream_wallets_balance_scope
    ON upstream_wallets (balance_scope_key)
    WHERE deleted_at IS NULL AND balance_scope_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS upstream_wallet_accounts (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    reason TEXT NOT NULL,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_wallet_accounts_range_check
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_wallet_accounts_current
    ON upstream_wallet_accounts (account_id)
    WHERE effective_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_upstream_wallet_accounts_wallet_range
    ON upstream_wallet_accounts (wallet_id, effective_from DESC, effective_to);
CREATE INDEX IF NOT EXISTS idx_upstream_wallet_accounts_account_range
    ON upstream_wallet_accounts (account_id, effective_from DESC, effective_to);

CREATE TABLE IF NOT EXISTS upstream_model_price_versions (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    model_pattern VARCHAR(150) NOT NULL,
    is_wildcard BOOLEAN NOT NULL DEFAULT FALSE,
    billing_mode VARCHAR(20) NOT NULL,
    service_tier VARCHAR(50),
    price_detail JSONB NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    source VARCHAR(30) NOT NULL,
    source_snapshot JSONB,
    checksum VARCHAR(128) NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_model_price_versions_billing_mode_check
        CHECK (billing_mode IN ('token', 'per_request', 'image', 'per_second')),
    CONSTRAINT upstream_model_price_versions_source_check
        CHECK (source IN ('upstream_exact', 'manual')),
    CONSTRAINT upstream_model_price_versions_range_check
        CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT upstream_model_price_versions_currency_check
        CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_model_price_versions_unique
    ON upstream_model_price_versions (
        wallet_id, model_pattern, billing_mode, COALESCE(service_tier, ''), effective_from, checksum
    );
CREATE INDEX IF NOT EXISTS idx_upstream_model_price_versions_lookup
    ON upstream_model_price_versions (wallet_id, model_pattern, effective_from DESC, effective_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_model_price_versions_current
    ON upstream_model_price_versions (wallet_id, model_pattern, billing_mode, COALESCE(service_tier, ''))
    WHERE effective_to IS NULL;

CREATE TABLE IF NOT EXISTS usage_upstream_attempts (
    id BIGSERIAL PRIMARY KEY,
    usage_log_id BIGINT NOT NULL REFERENCES usage_logs(id) ON DELETE RESTRICT,
    request_id VARCHAR(64) NOT NULL,
    attempt_no INTEGER NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    channel_id BIGINT,
    upstream_model VARCHAR(100) NOT NULL,
    service_tier VARCHAR(50),
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_5m_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_1h_tokens BIGINT NOT NULL DEFAULT 0,
    request_count BIGINT NOT NULL DEFAULT 0,
    image_count BIGINT NOT NULL DEFAULT 0,
    video_seconds BIGINT NOT NULL DEFAULT 0,
    upstream_cost_multiplier DECIMAL(10,4),
    billable BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_upstream_attempts_attempt_check CHECK (attempt_no > 0),
    CONSTRAINT usage_upstream_attempts_usage_check CHECK (
        input_tokens >= 0 AND output_tokens >= 0 AND cache_read_tokens >= 0
        AND cache_creation_5m_tokens >= 0 AND cache_creation_1h_tokens >= 0
        AND request_count >= 0 AND image_count >= 0 AND video_seconds >= 0
    ),
    CONSTRAINT usage_upstream_attempts_multiplier_check CHECK (
        upstream_cost_multiplier IS NULL
        OR upstream_cost_multiplier BETWEEN 0.0001 AND 9999.9999
    ),
    UNIQUE (usage_log_id, attempt_no)
);

CREATE INDEX IF NOT EXISTS idx_usage_upstream_attempts_request
    ON usage_upstream_attempts (request_id, attempt_no);
CREATE INDEX IF NOT EXISTS idx_usage_upstream_attempts_account_created
    ON usage_upstream_attempts (account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS usage_finance_records (
    id BIGSERIAL PRIMARY KEY,
    usage_log_id BIGINT NOT NULL UNIQUE REFERENCES usage_logs(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    channel_id BIGINT,
    account_id BIGINT REFERENCES accounts(id) ON DELETE RESTRICT,
    wallet_id BIGINT REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    upstream_id BIGINT REFERENCES upstreams(id) ON DELETE RESTRICT,
    usage_created_at TIMESTAMPTZ NOT NULL,
    requested_model VARCHAR(100) NOT NULL,
    upstream_model VARCHAR(100),
    service_tier VARCHAR(50),
    billing_type VARCHAR(30) NOT NULL,
    business_type VARCHAR(30) NOT NULL,
    usage_list_value DECIMAL(20,10),
    upstream_cost DECIMAL(20,10),
    cost_status VARCHAR(30) NOT NULL,
    pricing_source VARCHAR(30),
    price_version_id BIGINT,
    upstream_cost_multiplier_snapshot DECIMAL(10,4),
    current_revision INTEGER NOT NULL DEFAULT 1,
    calculation_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    calculated_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_finance_records_cost_status_check CHECK (
        cost_status IN (
            'exact', 'estimated', 'missing_price', 'missing_multiplier',
            'missing_usage', 'non_billable'
        )
    ),
    CONSTRAINT usage_finance_records_revision_check CHECK (current_revision > 0),
    CONSTRAINT usage_finance_records_cost_value_check CHECK (
        (cost_status IN ('exact', 'estimated', 'non_billable') AND upstream_cost IS NOT NULL)
        OR (cost_status IN ('missing_price', 'missing_multiplier', 'missing_usage') AND upstream_cost IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_usage_finance_records_created
    ON usage_finance_records (usage_created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_usage_finance_records_status_created
    ON usage_finance_records (cost_status, usage_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_finance_records_dimensions
    ON usage_finance_records (group_id, channel_id, upstream_id, wallet_id, account_id, usage_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_finance_records_requested_model
    ON usage_finance_records (requested_model, usage_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_finance_records_upstream_model
    ON usage_finance_records (upstream_model, usage_created_at DESC);

CREATE TABLE IF NOT EXISTS usage_finance_cost_segments (
    id BIGSERIAL PRIMARY KEY,
    usage_finance_record_id BIGINT NOT NULL REFERENCES usage_finance_records(id) ON DELETE RESTRICT,
    attempt_no INTEGER NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    wallet_id BIGINT REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    upstream_id BIGINT REFERENCES upstreams(id) ON DELETE RESTRICT,
    channel_id BIGINT,
    upstream_model VARCHAR(100) NOT NULL,
    service_tier VARCHAR(50),
    usage_detail JSONB NOT NULL,
    upstream_cost_multiplier_snapshot DECIMAL(10,4),
    price_version_id BIGINT,
    pricing_source VARCHAR(30),
    cost_status VARCHAR(30) NOT NULL,
    cost_amount DECIMAL(20,10),
    calculation_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_finance_cost_segments_attempt_check CHECK (attempt_no > 0),
    CONSTRAINT usage_finance_cost_segments_status_check CHECK (
        cost_status IN (
            'exact', 'estimated', 'missing_price', 'missing_multiplier',
            'missing_usage', 'non_billable'
        )
    ),
    UNIQUE (usage_finance_record_id, attempt_no)
);

CREATE INDEX IF NOT EXISTS idx_usage_finance_cost_segments_account
    ON usage_finance_cost_segments (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_finance_cost_segments_wallet
    ON usage_finance_cost_segments (wallet_id, created_at DESC);

CREATE TABLE IF NOT EXISTS finance_calculation_revisions (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    revision INTEGER NOT NULL,
    old_result JSONB,
    new_result JSONB NOT NULL,
    reason TEXT NOT NULL,
    job_id BIGINT,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT finance_calculation_revisions_revision_check CHECK (revision > 0),
    UNIQUE (entity_type, entity_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_finance_calculation_revisions_job
    ON finance_calculation_revisions (job_id, created_at DESC);

CREATE TABLE IF NOT EXISTS subscription_revenue_recognitions (
    id BIGSERIAL PRIMARY KEY,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    recognition_date DATE NOT NULL,
    recognized_revenue DECIMAL(20,10) NOT NULL,
    refund_reduction DECIMAL(20,10) NOT NULL DEFAULT 0,
    allocated_revenue DECIMAL(20,10) NOT NULL DEFAULT 0,
    unallocated_revenue DECIMAL(20,10) NOT NULL DEFAULT 0,
    allocation_status VARCHAR(30) NOT NULL,
    calculation_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    current_revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subscription_revenue_recognitions_status_check CHECK (
        allocation_status IN ('pending', 'allocated', 'unallocated', 'partial')
    ),
    UNIQUE (payment_order_id, recognition_date)
);

CREATE INDEX IF NOT EXISTS idx_subscription_revenue_recognitions_date
    ON subscription_revenue_recognitions (recognition_date DESC, user_id, group_id);

CREATE TABLE IF NOT EXISTS usage_revenue_allocations (
    id BIGSERIAL PRIMARY KEY,
    usage_log_id BIGINT NOT NULL REFERENCES usage_logs(id) ON DELETE RESTRICT,
    source_type VARCHAR(40) NOT NULL,
    source_id BIGINT,
    allocated_amount DECIMAL(20,10) NOT NULL,
    allocation_method VARCHAR(30) NOT NULL,
    recognition_date DATE NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    invalidated_at TIMESTAMPTZ,
    audit_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_revenue_allocations_source_type_check CHECK (
        source_type IN ('balance_usage', 'subscription_recognition', 'refund_adjustment')
    ),
    CONSTRAINT usage_revenue_allocations_revision_check CHECK (revision > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_revenue_allocations_active_revision
    ON usage_revenue_allocations (usage_log_id, source_type, COALESCE(source_id, 0), revision);
CREATE INDEX IF NOT EXISTS idx_usage_revenue_allocations_date
    ON usage_revenue_allocations (recognition_date DESC, usage_log_id)
    WHERE invalidated_at IS NULL;

CREATE TABLE IF NOT EXISTS finance_daily_aggregates (
    id BIGSERIAL PRIMARY KEY,
    aggregate_date DATE NOT NULL,
    timezone VARCHAR(100) NOT NULL,
    dimension_type VARCHAR(50) NOT NULL,
    dimension_key VARCHAR(200) NOT NULL,
    metric_detail JSONB NOT NULL,
    source_revision BIGINT NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (aggregate_date, timezone, dimension_type, dimension_key)
);

CREATE INDEX IF NOT EXISTS idx_finance_daily_aggregates_dimension
    ON finance_daily_aggregates (dimension_type, dimension_key, aggregate_date DESC);

CREATE TABLE IF NOT EXISTS upstream_balance_snapshots (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    dedupe_key VARCHAR(200) NOT NULL,
    balance_kind VARCHAR(20) NOT NULL,
    balance_amount DECIMAL(20,10),
    total_quota DECIMAL(20,10),
    used_quota DECIMAL(20,10),
    currency VARCHAR(3) NOT NULL,
    source VARCHAR(30) NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    sync_status VARCHAR(20) NOT NULL,
    safe_snapshot JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_balance_snapshots_kind_check
        CHECK (balance_kind IN ('wallet_cash', 'token_quota')),
    CONSTRAINT upstream_balance_snapshots_value_check CHECK (
        (balance_kind = 'wallet_cash' AND balance_amount IS NOT NULL AND total_quota IS NULL AND used_quota IS NULL)
        OR (balance_kind = 'token_quota' AND balance_amount IS NULL AND total_quota IS NOT NULL AND used_quota IS NOT NULL)
    ),
    UNIQUE (wallet_id, dedupe_key)
);

CREATE INDEX IF NOT EXISTS idx_upstream_balance_snapshots_latest
    ON upstream_balance_snapshots (wallet_id, balance_kind, collected_at DESC);

CREATE TABLE IF NOT EXISTS upstream_fund_events (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    event_type VARCHAR(30) NOT NULL,
    original_amount DECIMAL(20,10) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    fx_rate_to_usd DECIMAL(20,10) NOT NULL,
    usd_amount DECIMAL(20,10) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    reference_no VARCHAR(200),
    note TEXT NOT NULL,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    idempotency_key VARCHAR(200) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_fund_events_type_check
        CHECK (event_type IN ('opening_balance', 'topup', 'refund', 'adjustment')),
    UNIQUE (wallet_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_upstream_fund_events_occurred
    ON upstream_fund_events (wallet_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS payment_provider_fee_events (
    id BIGSERIAL PRIMARY KEY,
    payment_order_id BIGINT REFERENCES payment_orders(id) ON DELETE RESTRICT,
    provider VARCHAR(50) NOT NULL,
    bill_event_id VARCHAR(200) NOT NULL,
    gross_amount DECIMAL(20,10),
    fee_amount DECIMAL(20,10),
    net_amount DECIMAL(20,10),
    currency VARCHAR(3) NOT NULL,
    fx_rate_to_usd DECIMAL(20,10),
    gross_usd_amount DECIMAL(20,10),
    fee_usd_amount DECIMAL(20,10),
    net_usd_amount DECIMAL(20,10),
    fee_status VARCHAR(20) NOT NULL,
    source VARCHAR(30) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_provider_fee_events_status_check
        CHECK (fee_status IN ('confirmed', 'uncollected')),
    UNIQUE (provider, bill_event_id)
);

CREATE INDEX IF NOT EXISTS idx_payment_provider_fee_events_order
    ON payment_provider_fee_events (payment_order_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_payment_provider_fee_events_status
    ON payment_provider_fee_events (fee_status, occurred_at DESC);

CREATE TABLE IF NOT EXISTS account_upstream_multiplier_changes (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    old_multiplier DECIMAL(10,4),
    new_multiplier DECIMAL(10,4) NOT NULL,
    change_type VARCHAR(20) NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_upstream_multiplier_changes_type_check
        CHECK (change_type IN ('created', 'updated', 'imported')),
    CONSTRAINT account_upstream_multiplier_changes_value_check CHECK (
        new_multiplier BETWEEN 0.0001 AND 9999.9999
        AND (old_multiplier IS NULL OR old_multiplier BETWEEN 0.0001 AND 9999.9999)
    )
);

CREATE INDEX IF NOT EXISTS idx_account_upstream_multiplier_changes_history
    ON account_upstream_multiplier_changes (account_id, effective_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS finance_alerts (
    id BIGSERIAL PRIMARY KEY,
    alert_type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    aggregation_key VARCHAR(300) NOT NULL,
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL,
    dimension_type VARCHAR(50),
    dimension_id BIGINT,
    impact_amount DECIMAL(20,10),
    request_count BIGINT NOT NULL DEFAULT 0,
    occurrence_count BIGINT NOT NULL DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'open',
    first_occurred_at TIMESTAMPTZ NOT NULL,
    last_occurred_at TIMESTAMPTZ NOT NULL,
    assignee_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    handled_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    handled_note TEXT,
    handled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT finance_alerts_severity_check CHECK (severity IN ('info', 'warning', 'critical')),
    CONSTRAINT finance_alerts_status_check CHECK (status IN ('open', 'acknowledged', 'resolved', 'ignored'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_alerts_open_aggregation
    ON finance_alerts (aggregation_key)
    WHERE status IN ('open', 'acknowledged');
CREATE INDEX IF NOT EXISTS idx_finance_alerts_status_last
    ON finance_alerts (status, severity, last_occurred_at DESC);

CREATE TABLE IF NOT EXISTS finance_alert_status_audits (
    id BIGSERIAL PRIMARY KEY,
    alert_id BIGINT NOT NULL REFERENCES finance_alerts(id) ON DELETE CASCADE,
    from_status VARCHAR(20) NOT NULL,
    to_status VARCHAR(20) NOT NULL,
    note TEXT NOT NULL,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT finance_alert_status_audits_transition_check CHECK (from_status <> to_status)
);

CREATE INDEX IF NOT EXISTS idx_finance_alert_status_audits_alert
    ON finance_alert_status_audits (alert_id, created_at DESC);

CREATE TABLE IF NOT EXISTS upstream_bill_reconciliations (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    upstream_bill_amount DECIMAL(20,10) NOT NULL,
    system_cost_amount DECIMAL(20,10) NOT NULL,
    difference_amount DECIMAL(20,10) NOT NULL,
    difference_rate DECIMAL(20,10),
    currency VARCHAR(3) NOT NULL,
    source_reference VARCHAR(200),
    source_file_checksum VARCHAR(128) NOT NULL,
    status VARCHAR(20) NOT NULL,
    handled_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    handled_note TEXT,
    handled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_bill_reconciliations_range_check CHECK (period_end > period_start),
    CONSTRAINT upstream_bill_reconciliations_status_check CHECK (
        status IN ('pending', 'matched', 'difference', 'confirmed', 'ignored')
    ),
    UNIQUE (wallet_id, period_start, period_end, source_file_checksum)
);

CREATE INDEX IF NOT EXISTS idx_upstream_bill_reconciliations_status
    ON upstream_bill_reconciliations (status, period_start DESC);

CREATE TABLE IF NOT EXISTS finance_async_jobs (
    id BIGSERIAL PRIMARY KEY,
    job_type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    idempotency_key VARCHAR(200),
    request_checksum VARCHAR(128),
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    cursor JSONB,
    progress DECIMAL(10,4) NOT NULL DEFAULT 0,
    processed_count BIGINT NOT NULL DEFAULT 0,
    success_count BIGINT NOT NULL DEFAULT 0,
    failed_count BIGINT NOT NULL DEFAULT 0,
    lease_owner VARCHAR(200),
    lease_expires_at TIMESTAMPTZ,
    error_summary TEXT,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT finance_async_jobs_status_check CHECK (
        status IN ('queued', 'running', 'paused', 'completed', 'failed', 'cancelled')
    ),
    CONSTRAINT finance_async_jobs_progress_check CHECK (progress BETWEEN 0 AND 1)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_async_jobs_idempotency
    ON finance_async_jobs (job_type, operator_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_finance_async_jobs_queue
    ON finance_async_jobs (job_type, status, created_at)
    WHERE status IN ('queued', 'running', 'paused');

CREATE TABLE IF NOT EXISTS finance_backfill_jobs (
    id BIGSERIAL PRIMARY KEY,
    async_job_id BIGINT NOT NULL UNIQUE REFERENCES finance_async_jobs(id) ON DELETE RESTRICT,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    mode VARCHAR(30) NOT NULL,
    pricing_policy VARCHAR(30) NOT NULL,
    preview_token_hash VARCHAR(128) NOT NULL,
    preview_expires_at TIMESTAMPTZ NOT NULL,
    scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    reason TEXT NOT NULL,
    CONSTRAINT finance_backfill_jobs_range_check CHECK (end_date >= start_date)
);

CREATE TABLE IF NOT EXISTS finance_export_jobs (
    id BIGSERIAL PRIMARY KEY,
    async_job_id BIGINT NOT NULL UNIQUE REFERENCES finance_async_jobs(id) ON DELETE RESTRICT,
    report VARCHAR(50) NOT NULL,
    format VARCHAR(20) NOT NULL DEFAULT 'csv',
    filters JSONB NOT NULL,
    timezone VARCHAR(100) NOT NULL,
    storage_key TEXT,
    file_size BIGINT,
    row_count BIGINT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT finance_export_jobs_format_check CHECK (format IN ('csv'))
);

CREATE TABLE IF NOT EXISTS upstream_finance_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    async_job_id BIGINT REFERENCES finance_async_jobs(id) ON DELETE RESTRICT,
    wallet_id BIGINT NOT NULL REFERENCES upstream_wallets(id) ON DELETE RESTRICT,
    sync_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    collected_count BIGINT NOT NULL DEFAULT 0,
    skipped_count BIGINT NOT NULL DEFAULT 0,
    upstream_status INTEGER,
    duration_ms BIGINT,
    error_summary TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_finance_sync_runs_type_check
        CHECK (sync_type IN ('probe', 'pricing', 'balance', 'quota', 'bill')),
    CONSTRAINT upstream_finance_sync_runs_status_check
        CHECK (status IN ('queued', 'running', 'success', 'partial', 'failed', 'unsupported'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_finance_sync_runs_active
    ON upstream_finance_sync_runs (wallet_id, sync_type)
    WHERE status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS idx_upstream_finance_sync_runs_history
    ON upstream_finance_sync_runs (wallet_id, sync_type, created_at DESC);

COMMENT ON COLUMN accounts.upstream_cost_multiplier IS
    'Local fallback procurement multiplier. NULL means not configured for a historical account.';
COMMENT ON COLUMN usage_logs.upstream_cost_multiplier IS
    'Immutable procurement multiplier captured when the upstream account was selected.';
COMMENT ON TABLE usage_upstream_attempts IS
    'Immutable per-attempt upstream billing facts. Billable retry attempts are retained.';
COMMENT ON TABLE usage_finance_records IS
    'Current finance projection for each usage log; original usage facts remain immutable.';
COMMENT ON TABLE finance_calculation_revisions IS
    'Append-only audit trail for finance recalculation and correction.';
COMMENT ON TABLE upstream_balance_snapshots IS
    'Wallet cash and token quota snapshots are separate balance kinds and must never be summed.';
