-- 219_content_v1.sql
-- v0.4 content contract. Forward-only and idempotent.
-- The featured data backfill is executed by the startup hook because its
-- taxonomy JSON, snapshot, transaction, advisory lock and post-write
-- reconciliation are application-owned responsibilities.

ALTER TABLE content_articles
    ADD COLUMN IF NOT EXISTS featured_at TIMESTAMPTZ NULL;

ALTER TABLE content_articles
    ADD COLUMN IF NOT EXISTS seo_keywords JSONB NULL;

CREATE INDEX IF NOT EXISTS content_articles_featured_idx
    ON content_articles (status, featured_at DESC)
    WHERE featured_at IS NOT NULL;

ALTER TABLE content_news_sources
    ALTER COLUMN interval_minutes DROP NOT NULL;

ALTER TABLE content_news_sources
    ALTER COLUMN interval_minutes DROP DEFAULT;

ALTER TABLE content_news_sources
    DROP CONSTRAINT IF EXISTS content_news_sources_interval_minutes_check;

ALTER TABLE content_news_sources
    ADD CONSTRAINT content_news_sources_interval_minutes_check
    CHECK (interval_minutes IS NULL OR interval_minutes >= 15);

ALTER TABLE content_analytics_events
    DROP CONSTRAINT IF EXISTS content_analytics_events_name_check;

ALTER TABLE content_analytics_events
    ADD CONSTRAINT content_analytics_events_name_check
    CHECK (event_name IN (
        'page_view',
        'content_impression',
        'content_click',
        'content_read',
        'toc_click',
        'page_leave',
        'heartbeat'
    ));

-- Keep this function definition in the v0.4 migration so deployments that
-- apply only the incremental migration receive the new payload contract.
CREATE OR REPLACE FUNCTION content_analytics_payload_is_allowed(payload JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    key TEXT;
    value JSONB;
BEGIN
    FOR key, value IN SELECT * FROM jsonb_each(payload) LOOP
        IF key NOT IN (
            'scroll_percent', 'read_seconds', 'target', 'position', 'section',
            'duration_ms', 'view_id'
        ) THEN
            RETURN FALSE;
        END IF;
        IF jsonb_typeof(value) NOT IN ('null', 'string', 'boolean', 'number') THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    RETURN TRUE;
END;
$$;

-- The startup migration must acquire a transaction-scoped advisory lock,
-- atomically snapshot both legacy sources, and abort on either read failure.
-- It unions taxonomy FeaturedArticleIDs with articles tagged 精选, orders by
-- id DESC, keeps at most ten IDs, assigns strictly decreasing featured_at,
-- clears both legacy sources only after the write, and reconciles every ID
-- and its order before recording the idempotence marker. This post-write
-- reconciliation must verify every ID and order. An empty union must not
-- clear existing featured_at values, and a rerun must not overwrite new
-- manual selections. Failed runs must retain the rollback snapshot and must
-- not record completion.
