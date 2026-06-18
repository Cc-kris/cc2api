CREATE OR REPLACE FUNCTION default_account_base_url(platform TEXT, account_type TEXT)
RETURNS TEXT AS $$
BEGIN
    IF platform = 'openai' THEN
        RETURN 'https://api.openai.com';
    ELSIF platform = 'gemini' THEN
        RETURN 'https://generativelanguage.googleapis.com';
    ELSIF platform = 'antigravity' AND account_type IN ('api_key', 'apikey') THEN
        RETURN 'https://api.anthropic.com/antigravity';
    ELSIF account_type IN ('api_key', 'apikey') THEN
        RETURN 'https://api.anthropic.com';
    END IF;
    RETURN '';
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION normalized_account_base_url(credentials JSONB, extra JSONB, platform TEXT, account_type TEXT)
RETURNS TEXT AS $$
DECLARE
    raw TEXT;
BEGIN
    raw := NULLIF(BTRIM(COALESCE(credentials->>'base_url', '')), '');

    IF raw IS NULL AND COALESCE((extra->>'custom_base_url_enabled')::BOOLEAN, FALSE) THEN
        raw := NULLIF(BTRIM(COALESCE(extra->>'custom_base_url', '')), '');
    END IF;

    IF raw IS NULL THEN
        raw := default_account_base_url(platform, account_type);
    END IF;

    IF platform = 'antigravity' AND account_type IN ('api_key', 'apikey') AND raw IS NOT NULL AND raw <> '' THEN
        raw := regexp_replace(raw, '/+$', '');
        IF raw !~* '/antigravity$' THEN
            raw := raw || '/antigravity';
        END IF;
    END IF;

    RETURN trim(trailing '/' FROM lower(COALESCE(raw, '')));
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE TABLE IF NOT EXISTS upstream_model_rates (
    id BIGSERIAL PRIMARY KEY,
    upstream_id BIGINT NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    model VARCHAR(120) NOT NULL,
    rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0000,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_model_rates_upstream_model
    ON upstream_model_rates (upstream_id, lower(model));

CREATE INDEX IF NOT EXISTS idx_upstream_model_rates_upstream_id
    ON upstream_model_rates (upstream_id);

COMMENT ON TABLE upstream_model_rates IS 'Per-upstream model cost multiplier overrides.';
COMMENT ON COLUMN upstream_model_rates.model IS 'Requested or upstream model name. Exact match is case-insensitive.';
COMMENT ON COLUMN upstream_model_rates.rate_multiplier IS 'Cost multiplier applied when usage log model matches this model.';
