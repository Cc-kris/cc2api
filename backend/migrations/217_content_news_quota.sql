CREATE TABLE IF NOT EXISTS content_news_quota_reservations (
    quota_day DATE NOT NULL,
    quota_kind VARCHAR(16) NOT NULL CHECK (quota_kind IN ('fetch','publish')),
    reserved_count INTEGER NOT NULL DEFAULT 0 CHECK (reserved_count >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (quota_day, quota_kind)
);
