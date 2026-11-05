-- V02-T20：为已持久化的 Antigravity model_mapping 补新 Claude 模型
-- （fable-5、fable-5-1、opus-4-8、opus-5-5 及三档、sonnet-5-5 及三档）。
--
-- 写法照上游 144_add_opus48_to_model_mapping.sql：每个键一条 UPDATE，只补缺失的键，
-- 不覆盖管理员已有的同名映射。不要用整份覆盖 model_mapping 的写法（如 060）。
--
-- 范围：
-- - 没有 model_mapping 的账号运行时直接用 DefaultAntigravityModelMapping，不需要回填。
-- - model_mapping 为 {} 的账号表示「不限制模型」，运行时同样走默认映射，必须排除；
--   否则写入这些键后账号只剩这几个模型可用。
-- - 有显式映射（含白名单 {m: m}）的账号会多出这些模型，和上游 144 的取舍一致。
--
-- 幂等：每条 UPDATE 只在键为 NULL 时写，重复执行无副作用。

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-fable-5}',
    '"claude-fable-5"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-fable-5' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-fable-5-1}',
    '"claude-fable-5-1"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-fable-5-1' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-opus-4-8}',
    '"claude-opus-4-8"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-opus-4-8' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-opus-5-5}',
    '"claude-opus-5-5-high"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-opus-5-5' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-opus-5-5-high}',
    '"claude-opus-5-5-high"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-opus-5-5-high' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-opus-5-5-medium}',
    '"claude-opus-5-5-medium"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-opus-5-5-medium' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-opus-5-5-low}',
    '"claude-opus-5-5-low"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-opus-5-5-low' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-sonnet-5-5}',
    '"claude-sonnet-5-5-high"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-sonnet-5-5' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-sonnet-5-5-high}',
    '"claude-sonnet-5-5-high"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-sonnet-5-5-high' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-sonnet-5-5-medium}',
    '"claude-sonnet-5-5-medium"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-sonnet-5-5-medium' IS NULL;

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-sonnet-5-5-low}',
    '"claude-sonnet-5-5-low"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND jsonb_typeof(credentials->'model_mapping') = 'object'
  AND credentials->'model_mapping' <> '{}'::jsonb
  AND credentials->'model_mapping'->>'claude-sonnet-5-5-low' IS NULL;
