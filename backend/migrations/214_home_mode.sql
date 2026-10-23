-- Preserve the existing custom homepage while making the structured CCAI homepage explicit.
-- This migration is intentionally one-way: once applied, later settings changes are owned by the admin UI.

INSERT INTO settings (key, value)
VALUES ('home_mode', 'structured')
ON CONFLICT (key) DO NOTHING;

UPDATE settings
SET value = 'legacy_custom'
WHERE key = 'home_mode'
  AND value = 'structured'
  AND EXISTS (
      SELECT 1
      FROM settings
      WHERE key = 'home_content'
        AND NULLIF(BTRIM(value), '') IS NOT NULL
  );
