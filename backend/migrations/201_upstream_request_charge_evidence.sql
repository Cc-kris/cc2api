ALTER TABLE usage_upstream_attempts
    ADD COLUMN IF NOT EXISTS upstream_actual_charge DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS upstream_actual_charge_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS upstream_standard_charge DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS upstream_charge_currency VARCHAR(30),
    ADD COLUMN IF NOT EXISTS upstream_charge_unit_semantics VARCHAR(30),
    ADD COLUMN IF NOT EXISTS upstream_billing_request_id VARCHAR(200),
    ADD COLUMN IF NOT EXISTS upstream_charge_snapshot JSONB;

ALTER TABLE usage_upstream_attempts
    ALTER COLUMN upstream_charge_currency TYPE VARCHAR(30),
    DROP CONSTRAINT IF EXISTS usage_upstream_attempts_charge_currency_check,
    DROP CONSTRAINT IF EXISTS usage_upstream_attempts_charge_semantics_check;

ALTER TABLE usage_upstream_attempts
    ADD CONSTRAINT usage_upstream_attempts_charge_currency_check
    CHECK (upstream_charge_currency IS NULL OR upstream_charge_currency ~ '^[A-Z][A-Z0-9_-]{2,29}$');

ALTER TABLE usage_upstream_attempts
    DROP CONSTRAINT IF EXISTS usage_upstream_attempts_charge_semantics_check;
ALTER TABLE usage_upstream_attempts
    ADD CONSTRAINT usage_upstream_attempts_charge_semantics_check
    CHECK (upstream_charge_unit_semantics IS NULL OR upstream_charge_unit_semantics IN ('fiat_currency','platform_credit'));
