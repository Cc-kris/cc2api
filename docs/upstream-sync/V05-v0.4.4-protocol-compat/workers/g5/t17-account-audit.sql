-- V05-T17: read-only Gemini default-model impact review for an operator.
-- Returns only account/group identifiers and model_mapping, never API keys,
-- OAuth tokens or service-account credentials. Do not run as a migration.
SELECT
  a.id AS account_id,
  a.name AS account_name,
  a.type AS account_type,
  ag.group_id,
  a.credentials->'model_mapping' AS model_mapping
FROM accounts AS a
LEFT JOIN account_groups AS ag ON ag.account_id = a.id
WHERE a.platform = 'gemini'
  AND a.deleted_at IS NULL
  AND (
    a.credentials IS NULL
    OR a.credentials->'model_mapping' IS NULL
    OR a.credentials->'model_mapping' = '{}'::jsonb
  )
ORDER BY a.id, ag.group_id;
