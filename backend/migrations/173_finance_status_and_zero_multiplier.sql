-- Align finance cost states and upstream multiplier constraints with the approved accounting contract.

ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_upstream_cost_multiplier_check;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_upstream_cost_multiplier_check
    CHECK (
        upstream_cost_multiplier IS NULL
        OR upstream_cost_multiplier BETWEEN 0 AND 9999.9999
    ) NOT VALID;

ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_upstream_cost_multiplier_check;
ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_upstream_cost_multiplier_check
    CHECK (
        upstream_cost_multiplier IS NULL
        OR upstream_cost_multiplier BETWEEN 0 AND 9999.9999
    ) NOT VALID;

ALTER TABLE usage_upstream_attempts
    DROP CONSTRAINT IF EXISTS usage_upstream_attempts_multiplier_check;
ALTER TABLE usage_upstream_attempts
    ADD CONSTRAINT usage_upstream_attempts_multiplier_check
    CHECK (
        upstream_cost_multiplier IS NULL
        OR upstream_cost_multiplier BETWEEN 0 AND 9999.9999
    ) NOT VALID;

ALTER TABLE account_upstream_multiplier_changes
    DROP CONSTRAINT IF EXISTS account_upstream_multiplier_changes_value_check;
ALTER TABLE account_upstream_multiplier_changes
    ADD CONSTRAINT account_upstream_multiplier_changes_value_check CHECK (
        new_multiplier BETWEEN 0 AND 9999.9999
        AND (old_multiplier IS NULL OR old_multiplier BETWEEN 0 AND 9999.9999)
    ) NOT VALID;

ALTER TABLE usage_finance_records
    DROP CONSTRAINT IF EXISTS usage_finance_records_cost_status_check,
    DROP CONSTRAINT IF EXISTS usage_finance_records_cost_value_check;
ALTER TABLE usage_finance_records
    ADD CONSTRAINT usage_finance_records_cost_status_check CHECK (
        cost_status IN (
            'exact', 'estimated', 'missing_profile', 'missing_price',
            'missing_multiplier', 'missing_usage', 'unsupported_usage',
            'non_billable', 'excluded'
        )
    ),
    ADD CONSTRAINT usage_finance_records_cost_value_check CHECK (
        (cost_status IN ('exact', 'estimated', 'non_billable', 'excluded') AND upstream_cost IS NOT NULL)
        OR (
            cost_status IN (
                'missing_profile', 'missing_price', 'missing_multiplier',
                'missing_usage', 'unsupported_usage'
            )
            AND upstream_cost IS NULL
        )
    );

ALTER TABLE usage_finance_cost_segments
    DROP CONSTRAINT IF EXISTS usage_finance_cost_segments_status_check;
ALTER TABLE usage_finance_cost_segments
    ADD CONSTRAINT usage_finance_cost_segments_status_check CHECK (
        cost_status IN (
            'exact', 'estimated', 'missing_profile', 'missing_price',
            'missing_multiplier', 'missing_usage', 'unsupported_usage',
            'non_billable', 'excluded'
        )
    );
