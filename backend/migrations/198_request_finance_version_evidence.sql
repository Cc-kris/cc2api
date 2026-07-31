ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS upstream_cost_multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS upstream_cost_multiplier_source VARCHAR(30) NOT NULL DEFAULT 'account_config',
    ADD COLUMN IF NOT EXISTS current_finance_profile_id BIGINT REFERENCES account_finance_profiles(id) ON DELETE RESTRICT;

UPDATE accounts a SET
    upstream_cost_multiplier_change_id=(SELECT id FROM account_upstream_multiplier_changes c WHERE c.account_id=a.id ORDER BY c.effective_at DESC,c.id DESC LIMIT 1),
    current_finance_profile_id=(SELECT id FROM account_finance_profiles p WHERE p.account_id=a.id AND p.effective_to IS NULL ORDER BY p.version DESC LIMIT 1)
WHERE a.upstream_cost_multiplier_change_id IS NULL OR a.current_finance_profile_id IS NULL;

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS upstream_multiplier_source VARCHAR(30),
    ADD COLUMN IF NOT EXISTS upstream_multiplier_effective_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS account_finance_profile_id BIGINT REFERENCES account_finance_profiles(id) ON DELETE RESTRICT;

ALTER TABLE usage_upstream_attempts
    ADD COLUMN IF NOT EXISTS upstream_multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS upstream_multiplier_source VARCHAR(30),
    ADD COLUMN IF NOT EXISTS upstream_multiplier_effective_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS account_finance_profile_id BIGINT REFERENCES account_finance_profiles(id) ON DELETE RESTRICT;

ALTER TABLE usage_finance_records
    ADD COLUMN IF NOT EXISTS upstream_multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS upstream_multiplier_source VARCHAR(30),
    ADD COLUMN IF NOT EXISTS upstream_multiplier_effective_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS account_finance_profile_id BIGINT REFERENCES account_finance_profiles(id) ON DELETE RESTRICT;

ALTER TABLE usage_finance_cost_segments
    ADD COLUMN IF NOT EXISTS upstream_multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS upstream_multiplier_source VARCHAR(30),
    ADD COLUMN IF NOT EXISTS upstream_multiplier_effective_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS account_finance_profile_id BIGINT REFERENCES account_finance_profiles(id) ON DELETE RESTRICT;

COMMENT ON COLUMN usage_logs.upstream_multiplier_change_id IS 'Immutable account multiplier version selected for this request.';
COMMENT ON COLUMN usage_logs.account_finance_profile_id IS 'Immutable account finance profile selected for this request.';
