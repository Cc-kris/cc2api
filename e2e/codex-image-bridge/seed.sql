-- Codex image bridge E2E —— 种子数据（PostgreSQL）
-- 前置：sub2api 已在目标库完成首次启动建表（AutoMigrate）。
-- 用法：psql -h localhost -U <user> -d <e2e_db> -f seed.sql

BEGIN;

-- 用户
INSERT INTO users (id,email,password_hash,role,balance,concurrency,status,created_at,updated_at)
VALUES (1,'e2e@local','x','admin',1000,10,'active',now(),now())
ON CONFLICT (id) DO NOTHING;
SELECT setval(pg_get_serial_sequence('users','id'), GREATEST((SELECT max(id) FROM users),1));

-- 分组：11=开桥接(允许生图), 12=关桥接(允许生图), 13=开桥接但分组禁止生图,
--       14=仅账号级开启（渠道关闭，用于验证账号→渠道优先级）,
--       15=渠道开桥接但账号 strip 策略（验证 explicit_tool_policy）
-- 注意：每个场景都由种子数据静态描述，用例只读不改库。
--       改库触发的是账号快照重建，生效时机不可控（实测 0s~30s 抖动），
--       会让用例互相污染，因此不在用例里 UPDATE accounts。
INSERT INTO groups (id,name,description,rate_multiplier,is_exclusive,status,created_at,updated_at,platform,allow_image_generation,sort_order)
VALUES
 (11,'E2E-bridge-on','bridge enabled group',1,false,'active',now(),now(),'openai',true,11),
 (12,'E2E-bridge-off','bridge disabled group',1,false,'active',now(),now(),'openai',true,12),
 (13,'E2E-bridge-on-imgdenied','bridge on but group denies image',1,false,'active',now(),now(),'openai',false,13),
 (14,'E2E-account-bridge','account-level bridge override',1,false,'active',now(),now(),'openai',true,14),
 (15,'E2E-strip-policy','account strip policy over channel-on',1,false,'active',now(),now(),'openai',true,15)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  allow_image_generation = EXCLUDED.allow_image_generation,
  status = EXCLUDED.status,
  updated_at = now();
SELECT setval(pg_get_serial_sequence('groups','id'), GREATEST((SELECT max(id) FROM groups),15));

-- 账号：指向 mock 上游 19001
-- 关键：openai_apikey_responses_websockets_v2_enabled=true 才会走 WS 上游；
--       缺失时网关 reason=account_disabled 静默回退 HTTP，WS 用例会全部失真。
INSERT INTO accounts (id,name,platform,type,credentials,extra,concurrency,priority,status,created_at,updated_at,schedulable,rate_multiplier)
VALUES
 (101,'e2e-mock','openai','apikey',
  '{"api_key":"sk-mock-upstream","base_url":"http://127.0.0.1:19001"}'::jsonb,
  '{"openai_responses_supported":true,"openai_apikey_responses_websockets_v2_enabled":true}'::jsonb,
  5,1,'active',now(),now(),true,1),
 (102,'e2e-mock-accountbridge','openai','apikey',
  '{"api_key":"sk-mock-upstream","base_url":"http://127.0.0.1:19001"}'::jsonb,
  '{"openai_responses_supported":true,"openai_apikey_responses_websockets_v2_enabled":true,"codex_image_generation_bridge":true}'::jsonb,
  5,1,'active',now(),now(),true,1),
 (103,'e2e-mock-strip','openai','apikey',
  '{"api_key":"sk-mock-upstream","base_url":"http://127.0.0.1:19001"}'::jsonb,
  '{"openai_responses_supported":true,"openai_apikey_responses_websockets_v2_enabled":true,"codex_image_generation_explicit_tool_policy":"strip"}'::jsonb,
  5,1,'active',now(),now(),true,1)
-- 必须 DO UPDATE：否则手工改过的 extra（如 strip 策略）会长期残留，污染下一轮全量跑
ON CONFLICT (id) DO UPDATE SET
  platform = EXCLUDED.platform,
  type = EXCLUDED.type,
  credentials = EXCLUDED.credentials,
  extra = EXCLUDED.extra,
  status = EXCLUDED.status,
  updated_at = now();
SELECT setval(pg_get_serial_sequence('accounts','id'), GREATEST((SELECT max(id) FROM accounts),102));

-- 先清掉这两个 E2E 账号在本套件分组里的历史挂载，保证每个分组的上游账号唯一
-- （否则账号级 extra 会跨组污染"渠道关闭不注入"这类用例的结论）
DELETE FROM account_groups
 WHERE account_id IN (101,102,103) AND group_id IN (11,12,13,14,15);

INSERT INTO account_groups (account_id,group_id,priority,created_at) VALUES
 (101,11,1,now()), (101,12,1,now()), (101,13,1,now()), (102,14,1,now()), (103,15,1,now())
ON CONFLICT DO NOTHING;

-- 渠道特性：21/23/25 开桥接，22/24 关桥接
INSERT INTO channels (id,name,status,created_at,updated_at,features_config)
VALUES
 (21,'e2e-bridge-on','active',now(),now(),'{"codex_image_generation_bridge":{"openai":true}}'::jsonb),
 (22,'e2e-bridge-off','active',now(),now(),'{"codex_image_generation_bridge":{"openai":false}}'::jsonb),
 (23,'e2e-bridge-on-imgdenied','active',now(),now(),'{"codex_image_generation_bridge":{"openai":true}}'::jsonb),
 (24,'e2e-bridge-off-account','active',now(),now(),'{"codex_image_generation_bridge":{"openai":false}}'::jsonb),
 (25,'e2e-strip-policy','active',now(),now(),'{"codex_image_generation_bridge":{"openai":true}}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  features_config = EXCLUDED.features_config,
  status = EXCLUDED.status,
  updated_at = now();
SELECT setval(pg_get_serial_sequence('channels','id'), GREATEST((SELECT max(id) FROM channels),25));

INSERT INTO channel_groups (id,channel_id,group_id,created_at) VALUES
 (31,21,11,now()), (32,22,12,now()), (33,23,13,now()), (34,24,14,now()), (35,25,15,now())
ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('channel_groups','id'), GREATEST((SELECT max(id) FROM channel_groups),35));

-- API keys（长度故意保留，勿改：脚本按原样引用）
INSERT INTO api_keys (id,user_id,key,name,group_id,status,created_at,updated_at)
VALUES
 (201,1,'sk-e2e-bridge-on-00000000000000000000000000000000','bridge-on',11,'active',now(),now()),
 (202,1,'sk-e2e-bridge-off-0000000000000000000000000000000','bridge-off',12,'active',now(),now()),
 (203,1,'sk-e2e-img-denied-0000000000000000000000000000000','img-denied',13,'active',now(),now()),
 (204,1,'sk-e2e-account-bridge-000000000000000000000000000','account-bridge',14,'active',now(),now()),
 (205,1,'sk-e2e-strip-policy-0000000000000000000000000000','strip-policy',15,'active',now(),now())
ON CONFLICT (id) DO UPDATE SET
  key = EXCLUDED.key,
  group_id = EXCLUDED.group_id,
  status = EXCLUDED.status,
  updated_at = now();
SELECT setval(pg_get_serial_sequence('api_keys','id'), GREATEST((SELECT max(id) FROM api_keys),205));

COMMIT;
