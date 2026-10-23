-- Configurable field mapping for public JSON news adapters.
ALTER TABLE content_news_sources
    ADD COLUMN IF NOT EXISTS mapping_json JSONB NOT NULL DEFAULT '{}'::jsonb;
