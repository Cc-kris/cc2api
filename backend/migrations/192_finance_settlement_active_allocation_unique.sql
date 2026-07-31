CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_cost_settlement_one_active_attempt
    ON usage_cost_settlement_allocations(usage_log_id, attempt_no)
    WHERE invalidated_at IS NULL;

COMMENT ON INDEX idx_usage_cost_settlement_one_active_attempt IS
    'A billable upstream attempt can belong to only one active cumulative settlement allocation.';
