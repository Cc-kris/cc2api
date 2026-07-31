ALTER TABLE upstream_fund_events
    ADD COLUMN IF NOT EXISTS base_credit_units DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS bonus_credit_units DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS total_credit_units DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS base_recharge_ratio DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS effective_recharge_ratio DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS bonus_income_original DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS bonus_income_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS bonus_status VARCHAR(30) NOT NULL DEFAULT 'not_applicable',
    ADD COLUMN IF NOT EXISTS reversed_event_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'upstream_fund_events_bonus_status_check'
    ) THEN
        ALTER TABLE upstream_fund_events
            ADD CONSTRAINT upstream_fund_events_bonus_status_check
            CHECK (bonus_status IN ('not_applicable', 'confirmed', 'unresolved', 'reversed'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'upstream_fund_events_reversed_event_fk'
    ) THEN
        ALTER TABLE upstream_fund_events
            ADD CONSTRAINT upstream_fund_events_reversed_event_fk
            FOREIGN KEY (reversed_event_id) REFERENCES upstream_fund_events(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'upstream_fund_events_credit_units_check'
    ) THEN
        ALTER TABLE upstream_fund_events
            ADD CONSTRAINT upstream_fund_events_credit_units_check
            CHECK (
                (base_credit_units IS NULL AND bonus_credit_units IS NULL AND total_credit_units IS NULL)
                OR (
                    base_credit_units > 0
                    AND bonus_credit_units >= 0
                    AND total_credit_units = base_credit_units + bonus_credit_units
                )
            );
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_fund_events_reversal_unique
    ON upstream_fund_events (wallet_id, reversed_event_id)
    WHERE reversed_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_upstream_fund_events_bonus_status
    ON upstream_fund_events (bonus_status, occurred_at DESC);
