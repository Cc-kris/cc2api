-- CCAI v0.4.0 content center baseline.
-- This migration is intentionally forward-only and keeps source URLs internal.

CREATE TABLE IF NOT EXISTS content_news_sources (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(80) NOT NULL,
    source_type VARCHAR(20) NOT NULL DEFAULT 'rss',
    url TEXT,
    language VARCHAR(16) NOT NULL DEFAULT 'zh-CN',
    category VARCHAR(80),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    interval_minutes INTEGER NOT NULL DEFAULT 15 CHECK (interval_minutes >= 15),
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 60 CHECK (rate_limit_per_minute > 0),
    terms_confirmed_at TIMESTAMPTZ,
    authorization_note TEXT,
    robots_policy TEXT,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    last_error TEXT,
    created_by BIGINT,
    updated_by BIGINT,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_news_sources_type_check CHECK (source_type IN ('rss', 'atom', 'public_json', 'manual')),
    CONSTRAINT content_news_sources_url_check CHECK (
        source_type = 'manual' OR (
            NULLIF(BTRIM(url), '') IS NOT NULL
            AND url ~* '^https?://[^[:space:]]+$'
        )
    ),
    CONSTRAINT content_news_sources_enabled_requirements_check CHECK (
        NOT enabled OR (
            terms_confirmed_at IS NOT NULL
            AND NULLIF(BTRIM(authorization_note), '') IS NOT NULL
            AND NULLIF(BTRIM(robots_policy), '') IS NOT NULL
        )
    ),
    CONSTRAINT content_news_sources_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT content_news_sources_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS content_news_sources_enabled_idx ON content_news_sources (enabled);
CREATE INDEX IF NOT EXISTS content_news_sources_language_idx ON content_news_sources (language);
CREATE INDEX IF NOT EXISTS content_news_sources_category_idx ON content_news_sources (category);
CREATE INDEX IF NOT EXISTS content_news_sources_next_run_idx ON content_news_sources (next_run_at);
CREATE INDEX IF NOT EXISTS content_news_sources_last_run_idx ON content_news_sources (last_run_at);

CREATE TABLE IF NOT EXISTS content_news_items (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL REFERENCES content_news_sources(id),
    external_id VARCHAR(512),
    source_canonical_url TEXT NOT NULL,
    slug VARCHAR(80) NOT NULL,
    locale VARCHAR(16) NOT NULL DEFAULT 'zh-CN',
    title VARCHAR(300) NOT NULL,
    summary TEXT,
    body_html TEXT,
    body_text TEXT,
    body_source_type VARCHAR(24),
    body_fetched_at TIMESTAMPTZ,
    body_hash VARCHAR(64),
    media_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    image_url TEXT,
    source_name VARCHAR(120),
    original_published_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    status VARCHAR(24) NOT NULL DEFAULT 'draft',
    publish_mode VARCHAR(24) NOT NULL DEFAULT 'manual',
    seo_title VARCHAR(300),
    seo_description TEXT,
    fetched_at TIMESTAMPTZ,
    content_hash VARCHAR(64),
    created_by BIGINT,
    updated_by BIGINT,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_news_items_status_check CHECK (status IN ('draft', 'pending_review', 'published', 'unpublished', 'deleted')),
    CONSTRAINT content_news_items_publish_mode_check CHECK (publish_mode IN ('manual', 'auto')),
    CONSTRAINT content_news_items_body_source_type_check CHECK (
        body_source_type IS NULL OR body_source_type IN ('feed_content', 'public_api', 'html_extract')
    ),
    CONSTRAINT content_news_items_published_fields_check CHECK (
        status <> 'published' OR (
            NULLIF(BTRIM(title), '') IS NOT NULL
            AND
            NULLIF(BTRIM(summary), '') IS NOT NULL
            AND NULLIF(BTRIM(body_html), '') IS NOT NULL
            AND NULLIF(BTRIM(body_text), '') IS NOT NULL
            AND NULLIF(BTRIM(source_name), '') IS NOT NULL
            AND published_at IS NOT NULL
        )
    ),
    CONSTRAINT content_news_items_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT content_news_items_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS content_news_items_locale_slug_uq
    ON content_news_items (locale, slug);
CREATE UNIQUE INDEX IF NOT EXISTS content_news_items_source_external_uq
    ON content_news_items (source_id, external_id) WHERE external_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS content_news_items_source_url_idx ON content_news_items (source_canonical_url);
CREATE INDEX IF NOT EXISTS content_news_items_body_hash_idx ON content_news_items (body_hash);
CREATE INDEX IF NOT EXISTS content_news_items_content_hash_idx ON content_news_items (content_hash);
CREATE INDEX IF NOT EXISTS content_news_items_deleted_at_idx ON content_news_items (deleted_at);
CREATE INDEX IF NOT EXISTS content_news_items_status_locale_published_idx ON content_news_items (status, locale, published_at DESC);

CREATE TABLE IF NOT EXISTS content_articles (
    id BIGSERIAL PRIMARY KEY,
    locale VARCHAR(16) NOT NULL DEFAULT 'zh-CN',
    slug VARCHAR(80) NOT NULL,
    title VARCHAR(300) NOT NULL,
    excerpt TEXT,
    body_markdown TEXT,
    body_html TEXT,
    cover_image TEXT,
    cover_alt VARCHAR(300),
    category VARCHAR(80),
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(24) NOT NULL DEFAULT 'draft',
    author_id BIGINT,
    model VARCHAR(200),
    seo_title VARCHAR(300),
    seo_description TEXT,
    canonical_url TEXT,
    og_image TEXT,
    published_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_articles_status_check CHECK (status IN ('draft', 'pending_review', 'published', 'unpublished', 'deleted')),
    CONSTRAINT content_articles_author_fk FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS content_articles_locale_slug_uq
    ON content_articles (locale, slug);
CREATE INDEX IF NOT EXISTS content_articles_status_locale_published_idx ON content_articles (status, locale, published_at DESC);
CREATE INDEX IF NOT EXISTS content_articles_category_idx ON content_articles (category);

CREATE TABLE IF NOT EXISTS content_translations (
    id BIGSERIAL PRIMARY KEY,
    content_type VARCHAR(24) NOT NULL,
    content_id BIGINT NOT NULL,
    source_version INTEGER NOT NULL CHECK (source_version > 0),
    source_locale VARCHAR(16) NOT NULL,
    locale VARCHAR(16) NOT NULL,
    slug VARCHAR(80) NOT NULL,
    title VARCHAR(300) NOT NULL,
    excerpt TEXT,
    body_markdown TEXT,
    body_html TEXT,
    seo_title VARCHAR(300),
    seo_description TEXT,
    canonical_url TEXT,
    translation_model VARCHAR(200),
    failure_code VARCHAR(64),
    status VARCHAR(24) NOT NULL DEFAULT 'translation_draft',
    reviewed_by BIGINT,
    reviewed_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_translations_status_check CHECK (status IN ('translation_draft', 'translation_review', 'published', 'unpublished', 'deleted')),
    CONSTRAINT content_translations_reviewed_by_fk FOREIGN KEY (reviewed_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS content_translations_identity_uq
    ON content_translations (content_type, content_id, locale, source_version);
CREATE INDEX IF NOT EXISTS content_translations_status_locale_published_idx
    ON content_translations (status, locale, published_at DESC);

CREATE TABLE IF NOT EXISTS content_automation_settings (
    id BIGINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    fetch_cron VARCHAR(120) NOT NULL DEFAULT '0 */15 * * * *',
    publish_mode VARCHAR(24) NOT NULL DEFAULT 'manual_review',
    daily_fetch_limit INTEGER NOT NULL DEFAULT 0 CHECK (daily_fetch_limit >= 0),
    daily_publish_limit INTEGER NOT NULL DEFAULT 0 CHECK (daily_publish_limit >= 0),
    publish_start_at TIME,
    publish_end_at TIME,
    dedupe_window_hours INTEGER NOT NULL DEFAULT 24 CHECK (dedupe_window_hours BETWEEN 1 AND 168),
    target_locales JSONB NOT NULL DEFAULT '["zh-CN"]'::jsonb,
    updated_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_automation_settings_publish_mode_check CHECK (publish_mode IN ('manual_review', 'auto_publish')),
    CONSTRAINT content_automation_settings_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS content_translation_settings (
    id BIGINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    default_model VARCHAR(200),
    source_locale VARCHAR(16) NOT NULL DEFAULT 'zh-CN',
    target_locales JSONB NOT NULL DEFAULT '["en-US"]'::jsonb,
    glossary JSONB NOT NULL DEFAULT '{}'::jsonb,
    auto_create_draft BOOLEAN NOT NULL DEFAULT TRUE,
    updated_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_translation_settings_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS content_fetch_runs (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL REFERENCES content_news_sources(id),
    trigger_type VARCHAR(24) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'queued',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    fetched_count INTEGER NOT NULL DEFAULT 0 CHECK (fetched_count >= 0),
    created_count INTEGER NOT NULL DEFAULT 0 CHECK (created_count >= 0),
    duplicate_count INTEGER NOT NULL DEFAULT 0 CHECK (duplicate_count >= 0),
    failed_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    error_summary TEXT,
    task_key VARCHAR(200) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_fetch_runs_trigger_check CHECK (trigger_type IN ('cron', 'manual')),
    CONSTRAINT content_fetch_runs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'partial', 'failed', 'cancelled', 'interrupted'))
);

CREATE UNIQUE INDEX IF NOT EXISTS content_fetch_runs_source_task_uq ON content_fetch_runs (source_id, task_key);
CREATE INDEX IF NOT EXISTS content_fetch_runs_status_created_idx ON content_fetch_runs (status, created_at DESC);

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
        IF key NOT IN ('scroll_percent', 'read_seconds', 'target', 'position', 'section') THEN
            RETURN FALSE;
        END IF;
        IF jsonb_typeof(value) NOT IN ('null', 'string', 'boolean', 'number') THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    RETURN TRUE;
END;
$$;

CREATE TABLE IF NOT EXISTS content_analytics_events (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(128) NOT NULL UNIQUE,
    event_name VARCHAR(32) NOT NULL,
    content_type VARCHAR(24),
    content_id BIGINT,
    locale VARCHAR(16),
    placement VARCHAR(64),
    page_path VARCHAR(512) NOT NULL,
    session_hash VARCHAR(128) NOT NULL,
    referrer_host VARCHAR(255),
    event_at TIMESTAMPTZ NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_analytics_events_name_check CHECK (event_name IN ('page_view', 'content_impression', 'content_click', 'content_read', 'toc_click')),
    CONSTRAINT content_analytics_events_payload_check CHECK (content_analytics_payload_is_allowed(payload_json))
);

CREATE INDEX IF NOT EXISTS content_analytics_events_name_at_idx ON content_analytics_events (event_name, event_at);
CREATE INDEX IF NOT EXISTS content_analytics_events_content_at_idx ON content_analytics_events (content_type, content_id, event_at);
CREATE INDEX IF NOT EXISTS content_analytics_events_session_at_idx ON content_analytics_events (session_hash, event_at);

CREATE TABLE IF NOT EXISTS content_analytics_daily (
    id BIGSERIAL PRIMARY KEY,
    date DATE NOT NULL,
    locale VARCHAR(16) NOT NULL,
    content_type VARCHAR(24) NOT NULL,
    content_id BIGINT NOT NULL DEFAULT 0,
    page_path VARCHAR(512) NOT NULL,
    placement VARCHAR(64) NOT NULL DEFAULT '',
    unique_visitors INTEGER NOT NULL DEFAULT 0 CHECK (unique_visitors >= 0),
    page_views INTEGER NOT NULL DEFAULT 0 CHECK (page_views >= 0),
    reads INTEGER NOT NULL DEFAULT 0 CHECK (reads >= 0),
    impressions INTEGER NOT NULL DEFAULT 0 CHECK (impressions >= 0),
    clicks INTEGER NOT NULL DEFAULT 0 CHECK (clicks >= 0),
    avg_read_seconds INTEGER NOT NULL DEFAULT 0 CHECK (avg_read_seconds >= 0),
    ctr DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (ctr >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS content_analytics_daily_identity_uq
    ON content_analytics_daily (date, locale, content_type, content_id, page_path, placement);
CREATE INDEX IF NOT EXISTS content_analytics_daily_rank_idx
    ON content_analytics_daily (date, content_type, reads DESC);

CREATE TABLE IF NOT EXISTS content_slug_redirects (
    id BIGSERIAL PRIMARY KEY,
    content_type VARCHAR(24) NOT NULL,
    content_id BIGINT NOT NULL,
    locale VARCHAR(16) NOT NULL,
    old_slug VARCHAR(80) NOT NULL,
    new_slug VARCHAR(80) NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 301 CHECK (status_code IN (301, 308)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS content_slug_redirects_identity_uq
    ON content_slug_redirects (content_type, locale, old_slug);

CREATE TABLE IF NOT EXISTS content_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id BIGINT,
    content_type VARCHAR(24) NOT NULL,
    content_id BIGINT,
    action VARCHAR(64) NOT NULL,
    before_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_audit_logs_actor_fk FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS content_audit_logs_content_created_idx
    ON content_audit_logs (content_type, content_id, created_at DESC);
CREATE INDEX IF NOT EXISTS content_audit_logs_actor_created_idx
    ON content_audit_logs (actor_id, created_at DESC);


INSERT INTO content_automation_settings (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO content_translation_settings (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;
