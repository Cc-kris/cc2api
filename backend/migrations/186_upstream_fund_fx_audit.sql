ALTER TABLE upstream_fund_events
    ADD COLUMN IF NOT EXISTS fx_source VARCHAR(80) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS fx_observed_at TIMESTAMPTZ;

UPDATE upstream_fund_events
SET fx_observed_at = occurred_at
WHERE fx_observed_at IS NULL;

ALTER TABLE upstream_fund_events
    ALTER COLUMN fx_observed_at SET NOT NULL;
