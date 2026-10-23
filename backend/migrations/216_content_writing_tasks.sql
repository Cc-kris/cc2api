-- Durable AI writing/translation queue. Queue state is separate from the audit log
-- so multiple application instances can claim work with row-level locking.
CREATE TABLE IF NOT EXISTS content_writing_tasks (
    task_id VARCHAR(96) PRIMARY KEY,
    kind VARCHAR(24) NOT NULL,
    article_id BIGINT,
    translation_id BIGINT,
    source_version INTEGER NOT NULL DEFAULT 0,
    source_locale VARCHAR(16),
    target_locale VARCHAR(16),
    topic TEXT NOT NULL,
    audience TEXT,
    language VARCHAR(16),
    content_type VARCHAR(64),
    keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
    word_count INTEGER NOT NULL DEFAULT 0 CHECK (word_count >= 0),
    tone VARCHAR(120),
    facts JSONB NOT NULL DEFAULT '[]'::jsonb,
    internal_links JSONB NOT NULL DEFAULT '[]'::jsonb,
    model VARCHAR(200) NOT NULL,
    provider_key_ref VARCHAR(200),
    status VARCHAR(24) NOT NULL DEFAULT 'queued',
    result TEXT,
    error_code VARCHAR(120),
    lease_until TIMESTAMPTZ,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_writing_tasks_kind_check CHECK (kind IN ('writing', 'translation')),
    CONSTRAINT content_writing_tasks_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT content_writing_tasks_article_fk FOREIGN KEY (article_id) REFERENCES content_articles(id) ON DELETE SET NULL,
    CONSTRAINT content_writing_tasks_translation_fk FOREIGN KEY (translation_id) REFERENCES content_translations(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS content_writing_tasks_claim_idx
    ON content_writing_tasks (status, lease_until, created_at);
