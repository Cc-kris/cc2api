-- Ops cache/admin vNext: semantic cache entries.
--
-- BE-007 scope:
-- - Store semantic cache candidate entries and their exact-cache binding.
-- - Isolate entries by namespace across API key, user, group, platform, model, system fingerprint and rule version.
-- - Keep the migration idempotent and non-destructive.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS ops_semantic_cache_entries (
    id BIGSERIAL PRIMARY KEY,

    namespace VARCHAR(512) NOT NULL,
    platform VARCHAR(32) NOT NULL,
    model VARCHAR(128) NOT NULL,
    api_key_id BIGINT,
    user_id BIGINT,
    group_id BIGINT,
    system_fingerprint VARCHAR(128) NOT NULL,
    rule_version VARCHAR(64) NOT NULL,
    embedding_model VARCHAR(128) NOT NULL,
    embedding_dimension INTEGER NOT NULL,
    embedding_ref JSONB NOT NULL DEFAULT '{}'::jsonb,
    normalized_prompt_hash VARCHAR(128) NOT NULL,
    response_cache_key VARCHAR(512) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_entries_status_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_entries
            ADD CONSTRAINT ops_semantic_cache_entries_status_check
            CHECK (status IN ('active', 'expired', 'deleted', 'invalidated'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_entries_positive_dimension_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_entries
            ADD CONSTRAINT ops_semantic_cache_entries_positive_dimension_check
            CHECK (embedding_dimension > 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_entries_identity_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_entries
            ADD CONSTRAINT ops_semantic_cache_entries_identity_check
            CHECK (
                (api_key_id IS NULL OR api_key_id >= 0)
                AND (user_id IS NULL OR user_id >= 0)
                AND (group_id IS NULL OR group_id >= 0)
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_entries_required_text_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_entries
            ADD CONSTRAINT ops_semantic_cache_entries_required_text_check
            CHECK (
                length(trim(namespace)) > 0
                AND length(trim(platform)) > 0
                AND length(trim(model)) > 0
                AND length(trim(system_fingerprint)) > 0
                AND length(trim(rule_version)) > 0
                AND length(trim(embedding_model)) > 0
                AND length(trim(normalized_prompt_hash)) > 0
                AND length(trim(response_cache_key)) > 0
            );
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_semantic_cache_entries_active_unique
    ON ops_semantic_cache_entries (
        namespace,
        normalized_prompt_hash,
        rule_version,
        embedding_model,
        embedding_dimension
    )
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_entries_lookup
    ON ops_semantic_cache_entries (namespace, status, expires_at, normalized_prompt_hash);

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_entries_platform_model
    ON ops_semantic_cache_entries (platform, model, status, expires_at);

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_entries_api_key
    ON ops_semantic_cache_entries (api_key_id, status, expires_at)
    WHERE api_key_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_entries_user
    ON ops_semantic_cache_entries (user_id, status, expires_at)
    WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_entries_group
    ON ops_semantic_cache_entries (group_id, status, expires_at)
    WHERE group_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_entries_cleanup
    ON ops_semantic_cache_entries (status, expires_at);

COMMENT ON TABLE ops_semantic_cache_entries IS 'Semantic cache entries isolated by namespace and bound to exact response cache keys.';
COMMENT ON COLUMN ops_semantic_cache_entries.namespace IS 'Isolation namespace built from API key, user, group, platform, model, system fingerprint and semantic rule version.';
COMMENT ON COLUMN ops_semantic_cache_entries.embedding_ref IS 'JSON embedding reference or inline vector payload, depending on semantic provider storage mode.';
COMMENT ON COLUMN ops_semantic_cache_entries.normalized_prompt_hash IS 'Hash of normalized semantic prompt content.';
COMMENT ON COLUMN ops_semantic_cache_entries.response_cache_key IS 'Exact cache key for the reusable response.';
COMMENT ON COLUMN ops_semantic_cache_entries.status IS 'Entry status: active, expired, deleted or invalidated.';
COMMENT ON COLUMN ops_semantic_cache_entries.expires_at IS 'Semantic cache entry expiration time.';
