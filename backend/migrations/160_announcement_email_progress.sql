-- Track asynchronous announcement email sending progress.
ALTER TABLE announcements
  ADD COLUMN IF NOT EXISTS email_status VARCHAR(20) NOT NULL DEFAULT 'not_requested',
  ADD COLUMN IF NOT EXISTS email_total INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS email_sent INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS email_failed INTEGER NOT NULL DEFAULT 0;

UPDATE announcements
SET email_status = CASE
    WHEN email_sent_at IS NOT NULL THEN 'sent'
    ELSE 'not_requested'
  END
WHERE email_status IS NULL OR email_status = '';

CREATE INDEX IF NOT EXISTS idx_announcements_email_status
  ON announcements (email_status);

