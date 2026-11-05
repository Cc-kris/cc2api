-- V02-T21：为已持久化的 Antigravity model_mapping 补 gemini-4-argon 及三档。
--
-- gemini-4-argon 的简称要映射到 gemini-4-argon-high，不能走运行时自映射透传，所以需要回填。
-- gemini-3.6/3.7/3.8-flash（含档位）走 antigravityDefaultPassthroughModels 运行时透传，不进迁移，
-- 否则写进 credentials 后会变成「管理员配置」，默认映射再变也跟不上。
--
-- 写法同 220（照上游 144）：每个键一条 UPDATE，只补缺失的键，排除 model_mapping 为 {} 的账号。
-- 幂等：重复执行无副作用。

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,gemini-4-argon}',
    '"gemini-4-argon-high"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'gemini-4-argon' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,gemini-4-argon-high}',
    '"gemini-4-argon-high"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'gemini-4-argon-high' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,gemini-4-argon-medium}',
    '"gemini-4-argon-medium"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'gemini-4-argon-medium' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,gemini-4-argon-low}',
    '"gemini-4-argon-low"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'gemini-4-argon-low' IS NULL;
