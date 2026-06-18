-- Backfill historical announcements that had already sent email before
-- asynchronous email progress tracking was introduced.
UPDATE announcements
SET email_status = 'sent',
    email_total = CASE WHEN email_total = 0 THEN 1 ELSE email_total END,
    email_sent = CASE WHEN email_sent = 0 THEN 1 ELSE email_sent END,
    email_failed = 0
WHERE email_sent_at IS NOT NULL
  AND (email_status IS NULL OR email_status = '' OR email_status = 'not_requested');
