ALTER TABLE announcements
    ADD COLUMN IF NOT EXISTS source_locale VARCHAR(10) NOT NULL DEFAULT 'zh',
    ADD COLUMN IF NOT EXISTS source_version INTEGER NOT NULL DEFAULT 1 CHECK (source_version > 0),
    ADD COLUMN IF NOT EXISTS translations JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE announcements
SET translations = jsonb_build_object(
    COALESCE(NULLIF(source_locale, ''), 'zh'),
    jsonb_build_object(
        'title', title,
        'content', content,
        'source_version', source_version,
        'status', 'source',
        'updated_at', updated_at
    )
)
WHERE translations = '{}'::jsonb;
