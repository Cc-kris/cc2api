ALTER TABLE upstream_cost_settlement_intervals
    ALTER COLUMN list_cost_delta DROP NOT NULL,
    ALTER COLUMN observed_multiplier DROP NOT NULL;

ALTER TABLE upstream_cost_settlement_intervals
    DROP CONSTRAINT IF EXISTS upstream_cost_settlement_intervals_list_cost_delta_check,
    DROP CONSTRAINT IF EXISTS upstream_cost_settlement_intervals_observed_multiplier_check;

ALTER TABLE upstream_cost_settlement_intervals
    ADD CONSTRAINT upstream_cost_settlement_intervals_list_cost_delta_check
        CHECK (list_cost_delta IS NULL OR list_cost_delta >= 0),
    ADD CONSTRAINT upstream_cost_settlement_intervals_observed_multiplier_check
        CHECK (observed_multiplier IS NULL OR observed_multiplier >= 0);

COMMENT ON COLUMN upstream_cost_settlement_intervals.list_cost_delta IS
    'Optional upstream list-cost delta. NULL for cumulative_actual protocols.';
COMMENT ON COLUMN upstream_cost_settlement_intervals.observed_multiplier IS
    'Optional derived observation. NULL when the upstream only exposes cumulative actual cost.';

ALTER TABLE account_finance_counter_snapshots
    DROP CONSTRAINT IF EXISTS account_finance_counter_derivation_check;

ALTER TABLE account_finance_counter_snapshots
    ADD CONSTRAINT account_finance_counter_derivation_check
        CHECK (derivation_status IN (
            'baseline','raw_only','missing_values','boundary_changed','time_reversed',
            'counter_reset','no_activity','invalid_list_delta','candidate','applied',
            'settlement_ready','unchanged','conflict','inactive_account','invalid_multiplier'
        ));
