CREATE OR REPLACE FUNCTION default_account_base_url(platform TEXT, account_type TEXT)
RETURNS TEXT AS $$
BEGIN
    IF platform = 'openai' THEN
        RETURN 'https://api.openai.com';
    ELSIF platform = 'gemini' THEN
        RETURN 'https://generativelanguage.googleapis.com';
    ELSIF platform = 'antigravity' AND account_type = 'api_key' THEN
        RETURN 'https://api.anthropic.com/antigravity';
    ELSIF account_type = 'api_key' THEN
        RETURN 'https://api.anthropic.com';
    END IF;
    RETURN '';
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Upstream management and finance statistics.
CREATE TABLE IF NOT EXISTS upstreams (
    id BIGSERIAL PRIMARY KEY,
    base_url TEXT NOT NULL,
    normalized_base_url TEXT NOT NULL,
    name VARCHAR(120) NOT NULL,
    rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0000,
    initial_balance DECIMAL(20,10) NOT NULL DEFAULT 0,
    balance_alert_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    alert_balance DECIMAL(20,10),
    alert_email_sent_at TIMESTAMPTZ,
    alert_last_balance DECIMAL(20,10),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstreams_normalized_base_url_active
    ON upstreams (normalized_base_url)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_upstreams_deleted_at
    ON upstreams (deleted_at);

COMMENT ON TABLE upstreams IS 'Upstream providers grouped by normalized account base_url.';
COMMENT ON COLUMN upstreams.initial_balance IS 'Manually maintained upstream recharge/starting balance; current balance is initial_balance minus upstream cost.';
COMMENT ON COLUMN upstreams.rate_multiplier IS 'Cost multiplier applied to user actual_cost for this upstream.';
