-- Ops cache/admin vNext: cache clear audit records.
--
-- BE-006 scope:
-- - Persist every cache clear operation with operator, clear type, scope, result counters and failure reason.
-- - Support cache clear audit listing by time, operator, type and status.
-- - Keep the migration idempotent and non-destructive.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS ops_cache_clear_audits (
    id BIGSERIAL PRIMARY KEY,

    operator_user_id BIGINT,
    clear_type VARCHAR(32) NOT NULL,
    scope JSONB NOT NULL DEFAULT '{}'::jsonb,

    matched_keys BIGINT NOT NULL DEFAULT 0,
    deleted_keys BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL,
    error_message TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_cache_clear_audits_clear_type_check'
    ) THEN
        ALTER TABLE ops_cache_clear_audits
            ADD CONSTRAINT ops_cache_clear_audits_clear_type_check
            CHECK (clear_type IN ('all', 'by_platform', 'by_model', 'by_group', 'by_api_key', 'by_time', 'expired'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_cache_clear_audits_status_check'
    ) THEN
        ALTER TABLE ops_cache_clear_audits
            ADD CONSTRAINT ops_cache_clear_audits_status_check
            CHECK (status IN ('success', 'failed', 'partial_success'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_cache_clear_audits_non_negative_check'
    ) THEN
        ALTER TABLE ops_cache_clear_audits
            ADD CONSTRAINT ops_cache_clear_audits_non_negative_check
            CHECK (
                (operator_user_id IS NULL OR operator_user_id >= 0)
                AND matched_keys >= 0
                AND deleted_keys >= 0
                AND deleted_keys <= matched_keys
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_ops_cache_clear_audits_created_at
    ON ops_cache_clear_audits (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_cache_clear_audits_operator_created_at
    ON ops_cache_clear_audits (operator_user_id, created_at DESC)
    WHERE operator_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_cache_clear_audits_type_status_created_at
    ON ops_cache_clear_audits (clear_type, status, created_at DESC);

COMMENT ON TABLE ops_cache_clear_audits IS 'Cache clear operation audit records.';
COMMENT ON COLUMN ops_cache_clear_audits.operator_user_id IS 'Operator user id that initiated the cache clear.';
COMMENT ON COLUMN ops_cache_clear_audits.clear_type IS 'Clear type: all, by_platform, by_model, by_group, by_api_key, by_time, or expired.';
COMMENT ON COLUMN ops_cache_clear_audits.scope IS 'JSON snapshot of the clear scope.';
COMMENT ON COLUMN ops_cache_clear_audits.matched_keys IS 'Number of cache keys matched by the clear scope.';
COMMENT ON COLUMN ops_cache_clear_audits.deleted_keys IS 'Number of cache keys deleted.';
COMMENT ON COLUMN ops_cache_clear_audits.status IS 'Clear status: success, failed, or partial_success.';
COMMENT ON COLUMN ops_cache_clear_audits.error_message IS 'Failure or partial failure reason.';
