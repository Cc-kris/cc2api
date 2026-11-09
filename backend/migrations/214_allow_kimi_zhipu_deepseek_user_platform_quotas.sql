-- 214: user_platform_quotas.platform 约束补齐 kimi / zhipu / deepseek
--
-- 背景：ent schema（backend/ent/schema/user_platform_quota.go）与
-- service.AllowedQuotaPlatforms 早已允许 kimi / zhipu / deepseek，
-- 但 DB 侧 CHECK 约束最后一次变更停留在 169_grok_platform.sql（仅 6 个平台）。
-- 结果：kimi 平台的用户配额行插入被 DB 拒绝（每请求一条 ERROR），
--       kimi 用量无法累计、限额不生效。
--
-- 幂等：DROP IF EXISTS + 重新 ADD。
ALTER TABLE user_platform_quotas DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;
ALTER TABLE user_platform_quotas ADD CONSTRAINT user_platform_quotas_platform_check
  CHECK (platform::text = ANY (ARRAY[
    'anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'seedace',
    'kimi', 'zhipu', 'deepseek'
  ]::text[]));
