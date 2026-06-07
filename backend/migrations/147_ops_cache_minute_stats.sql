-- Ops cache/admin vNext: cache minute statistics.
--
-- BE-005 scope:
-- - Store minute-level cache metrics by platform/model/group/API key/cache type.
-- - Support cache stats summary, model aggregation and reason distributions.
-- - Keep the migration idempotent and non-destructive.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS ops_cache_minute_stats (
    id BIGSERIAL PRIMARY KEY,

    minute_at TIMESTAMPTZ NOT NULL,
    platform VARCHAR(32) NOT NULL,
    model VARCHAR(128) NOT NULL,
    group_id BIGINT,
    api_key_id BIGINT,
    cache_type VARCHAR(16) NOT NULL,

    total_requests BIGINT NOT NULL DEFAULT 0,
    candidate_requests BIGINT NOT NULL DEFAULT 0,
    hit_requests BIGINT NOT NULL DEFAULT 0,
    bypass_requests BIGINT NOT NULL DEFAULT 0,
    store_success BIGINT NOT NULL DEFAULT 0,
    store_skip BIGINT NOT NULL DEFAULT 0,

    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    hit_tokens BIGINT NOT NULL DEFAULT 0,
    candidate_tokens BIGINT NOT NULL DEFAULT 0,
    all_request_tokens BIGINT NOT NULL DEFAULT 0,

    bypass_reasons JSONB NOT NULL DEFAULT '{}'::jsonb,
    store_skip_reasons JSONB NOT NULL DEFAULT '{}'::jsonb,
    estimated_saved_amount DECIMAL(20,8) NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_cache_minute_stats_cache_type_check'
    ) THEN
        ALTER TABLE ops_cache_minute_stats
            ADD CONSTRAINT ops_cache_minute_stats_cache_type_check
            CHECK (cache_type IN ('exact', 'semantic'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_cache_minute_stats_non_negative_check'
    ) THEN
        ALTER TABLE ops_cache_minute_stats
            ADD CONSTRAINT ops_cache_minute_stats_non_negative_check
            CHECK (
                total_requests >= 0
                AND candidate_requests >= 0
                AND hit_requests >= 0
                AND bypass_requests >= 0
                AND store_success >= 0
                AND store_skip >= 0
                AND input_tokens >= 0
                AND output_tokens >= 0
                AND hit_tokens >= 0
                AND candidate_tokens >= 0
                AND all_request_tokens >= 0
                AND (group_id IS NULL OR group_id >= 0)
                AND (api_key_id IS NULL OR api_key_id >= 0)
                AND estimated_saved_amount >= 0
            );
    END IF;

END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_unique_bucket
    ON ops_cache_minute_stats (
        minute_at,
        platform,
        model,
        COALESCE(group_id, -1),
        COALESCE(api_key_id, -1),
        cache_type
    );

CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_time
    ON ops_cache_minute_stats (minute_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_platform_model_time
    ON ops_cache_minute_stats (platform, model, minute_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_group_time
    ON ops_cache_minute_stats (group_id, minute_at DESC)
    WHERE group_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_api_key_time
    ON ops_cache_minute_stats (api_key_id, minute_at DESC)
    WHERE api_key_id IS NOT NULL;

COMMENT ON TABLE ops_cache_minute_stats IS 'Minute-level cache statistics by platform, model, group, API key and cache type.';
COMMENT ON COLUMN ops_cache_minute_stats.minute_at IS 'Minute bucket timestamp.';
COMMENT ON COLUMN ops_cache_minute_stats.cache_type IS 'Cache type: exact or semantic.';
COMMENT ON COLUMN ops_cache_minute_stats.candidate_requests IS 'Requests eligible for cache lookup.';
COMMENT ON COLUMN ops_cache_minute_stats.hit_requests IS 'Requests that hit cache.';
COMMENT ON COLUMN ops_cache_minute_stats.hit_tokens IS 'Tokens served from cache hits.';
COMMENT ON COLUMN ops_cache_minute_stats.candidate_tokens IS 'Tokens from cache candidate requests.';
COMMENT ON COLUMN ops_cache_minute_stats.all_request_tokens IS 'Input plus output tokens for all requests.';
COMMENT ON COLUMN ops_cache_minute_stats.bypass_reasons IS 'JSON distribution of cache bypass reasons.';
COMMENT ON COLUMN ops_cache_minute_stats.store_skip_reasons IS 'JSON distribution of cache store skip reasons.';
COMMENT ON COLUMN ops_cache_minute_stats.estimated_saved_amount IS 'Estimated saved amount from cache hits.';
