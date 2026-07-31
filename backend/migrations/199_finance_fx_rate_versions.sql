CREATE TABLE IF NOT EXISTS finance_fx_rate_versions (
    id BIGSERIAL PRIMARY KEY,
    currency VARCHAR(3) NOT NULL,
    rate_to_usd DECIMAL(20,10) NOT NULL CHECK (rate_to_usd > 0),
    source VARCHAR(80) NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    checksum VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT finance_fx_rate_period_check CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT finance_fx_rate_currency_check CHECK (char_length(currency) = 3),
    CONSTRAINT finance_fx_rate_identity_key UNIQUE(currency, rate_to_usd, source, effective_from)
);
CREATE INDEX IF NOT EXISTS finance_fx_rate_effective_idx ON finance_fx_rate_versions(currency, effective_from DESC, id DESC);

ALTER TABLE upstream_fund_events
    ADD COLUMN IF NOT EXISTS fx_rate_version_id BIGINT REFERENCES finance_fx_rate_versions(id) ON DELETE RESTRICT;
ALTER TABLE usage_finance_records
    ADD COLUMN IF NOT EXISTS fx_rate_version_id BIGINT REFERENCES finance_fx_rate_versions(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS source_currency VARCHAR(3),
    ADD COLUMN IF NOT EXISTS fx_rate_to_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS fx_source VARCHAR(80),
    ADD COLUMN IF NOT EXISTS fx_observed_at TIMESTAMPTZ;
ALTER TABLE usage_finance_cost_segments
    ADD COLUMN IF NOT EXISTS fx_rate_version_id BIGINT REFERENCES finance_fx_rate_versions(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS source_currency VARCHAR(3),
    ADD COLUMN IF NOT EXISTS fx_rate_to_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS fx_source VARCHAR(80),
    ADD COLUMN IF NOT EXISTS fx_observed_at TIMESTAMPTZ;
ALTER TABLE upstream_cost_settlement_intervals
    ADD COLUMN IF NOT EXISTS fx_rate_version_id BIGINT REFERENCES finance_fx_rate_versions(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS fx_rate_to_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS fx_source VARCHAR(80),
    ADD COLUMN IF NOT EXISTS fx_observed_at TIMESTAMPTZ;
COMMENT ON COLUMN finance_fx_rate_versions.id IS 'Immutable FX evidence version used by historical finance records.';
