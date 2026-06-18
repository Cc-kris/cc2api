CREATE TABLE IF NOT EXISTS upstream_platform_rates (
    id BIGSERIAL PRIMARY KEY,
    upstream_id BIGINT NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL,
    rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0000,
    image_unit_price DECIMAL(20,10) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_platform_rates_upstream_platform
    ON upstream_platform_rates (upstream_id, lower(platform));

CREATE INDEX IF NOT EXISTS idx_upstream_platform_rates_upstream_id
    ON upstream_platform_rates (upstream_id);

COMMENT ON TABLE upstream_platform_rates IS 'Per-upstream platform cost rules.';
COMMENT ON COLUMN upstream_platform_rates.platform IS 'Account platform such as anthropic, openai, gemini, antigravity.';
COMMENT ON COLUMN upstream_platform_rates.rate_multiplier IS 'Token cost multiplier for this platform. Falls back to upstream default multiplier when not configured.';
COMMENT ON COLUMN upstream_platform_rates.image_unit_price IS 'Fixed upstream cost per image request/image count for this platform. 0 means disabled.';
