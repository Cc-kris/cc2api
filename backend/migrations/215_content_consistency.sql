-- 215_content_consistency.sql
-- Enforce the invariant used by public projections: published records carry a
-- publication timestamp. NOT VALID keeps an existing legacy row from blocking
-- deployment while enforcing the invariant for every new or updated row.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'content_articles_published_at_required'
    ) THEN
        ALTER TABLE content_articles
            ADD CONSTRAINT content_articles_published_at_required
            CHECK (status <> 'published' OR published_at IS NOT NULL) NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'content_translations_published_at_required'
    ) THEN
        ALTER TABLE content_translations
            ADD CONSTRAINT content_translations_published_at_required
            CHECK (status <> 'published' OR published_at IS NOT NULL) NOT VALID;
    END IF;
END $$;
