ALTER TABLE finance_export_jobs
    ADD COLUMN IF NOT EXISTS download_token_hash VARCHAR(128),
    ADD COLUMN IF NOT EXISTS download_token_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS downloaded_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_finance_export_jobs_expiry
    ON finance_export_jobs (expires_at)
    WHERE storage_key IS NOT NULL;
