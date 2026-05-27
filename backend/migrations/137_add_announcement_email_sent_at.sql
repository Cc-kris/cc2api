-- Add one-time email notification marker for announcements.
ALTER TABLE announcements
  ADD COLUMN IF NOT EXISTS email_sent_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_announcements_email_sent_at
  ON announcements (email_sent_at);
