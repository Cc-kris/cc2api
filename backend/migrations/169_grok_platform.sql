-- Enable Grok quota accounting and channel monitoring.

ALTER TABLE user_platform_quotas
  DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
  ADD CONSTRAINT user_platform_quotas_platform_check
  CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'seedace'));

ALTER TABLE channel_monitors
  DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;

ALTER TABLE channel_monitors
  ADD CONSTRAINT channel_monitors_provider_check
  CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok'));

ALTER TABLE channel_monitor_request_templates
  DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;

ALTER TABLE channel_monitor_request_templates
  ADD CONSTRAINT channel_monitor_request_templates_provider_check
  CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok'));

-- Grok media routes use the same generation capability gate. Existing Grok
-- groups predate that support, so enable the gate for them during migration.
UPDATE groups
SET allow_image_generation = true
WHERE platform = 'grok'
  AND allow_image_generation = false;
