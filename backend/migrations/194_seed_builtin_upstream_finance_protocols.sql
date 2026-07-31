WITH builtin(code,name,config,checksum) AS (
    VALUES
    ('sub2api','Sub2API', '{
      "capabilities":["account_usage"],
      "recognition":[
        {"method":"GET","path":"/health","match":{"status":200}},
        {"method":"GET","path":"/api/v1/settings/public","match":{"path":"$.data","exists":true}}
      ],
      "authentication":{"type":"bearer","credential_source":"account_api_key"},
      "operations":{"account_usage":{"method":"GET","path":"/v1/usage","mapping":{"list_cost":"$.usage.total.cost","actual_cost":"$.usage.total.actual_cost","currency":"$.unit"}}},
      "cost_mode":"cumulative_list_and_actual","unit_semantics":"platform_credit","counter_scope":"account",
      "redact_paths":["$.token","$.api_key"]
    }'::jsonb, 'builtin:sub2api:v1'),
    ('newapi','NewAPI', '{
      "capabilities":["pricing","balance"],
      "recognition":[{"method":"GET","path":"/api/status","match":{"path":"$.data","exists":true}}],
      "authentication":{"type":"bearer","credential_source":"account_api_key"},
      "operations":{
        "pricing":{"method":"GET","path":"/api/pricing","mapping":{"models":"$.data"}},
        "balance":{"method":"GET","path":"/api/user/self","mapping":{"balance":"$.data.quota","used":"$.data.used_quota"}}
      },
      "cost_mode":"manual","unit_semantics":"platform_credit"
    }'::jsonb, 'builtin:newapi:v1'),
    ('legacy_openai_billing','Legacy OpenAI Billing', '{
      "capabilities":["quota"],
      "recognition":[{"method":"GET","path":"/dashboard/billing/subscription","match":{"path":"$.hard_limit_usd","exists":true}}],
      "authentication":{"type":"bearer","credential_source":"account_api_key"},
      "operations":{"quota":{"method":"GET","path":"/dashboard/billing/subscription","mapping":{"quota":"$.hard_limit_usd"}}},
      "cost_mode":"manual","unit_semantics":"fiat_currency"
    }'::jsonb, 'builtin:legacy-openai:v1'),
    ('manual_contract','Manual Contract', '{
      "capabilities":["account_usage"],
      "authentication":{"type":"none"},"operations":{},
      "cost_mode":"manual","unit_semantics":"none"
    }'::jsonb, 'builtin:manual-contract:v1')
), inserted_protocols AS (
    INSERT INTO upstream_finance_protocols(code,name,protocol_type,status,created_at,updated_at)
    SELECT code,name,'builtin','draft',NOW(),NOW() FROM builtin
    ON CONFLICT(code) DO NOTHING
    RETURNING id
), inserted_versions AS (
    INSERT INTO upstream_finance_protocol_versions(
        protocol_id,version,config,checksum,validation_status,validation_result,published_at,created_at
    )
    SELECT p.id,1,b.config,b.checksum,'valid',
           jsonb_build_object('valid',true,'issues','[]'::jsonb,'checksum',b.checksum),NOW(),NOW()
    FROM builtin b
    JOIN upstream_finance_protocols p ON p.code=b.code AND p.protocol_type='builtin'
    WHERE NOT EXISTS (
        SELECT 1 FROM upstream_finance_protocol_versions v WHERE v.protocol_id=p.id
    )
    RETURNING id,protocol_id
)
UPDATE upstream_finance_protocols p
SET current_version_id=v.id,status='published',updated_at=NOW()
FROM upstream_finance_protocol_versions v
WHERE v.protocol_id=p.id AND v.version=1 AND p.protocol_type='builtin' AND p.current_version_id IS NULL;

COMMENT ON COLUMN upstream_finance_protocols.protocol_type IS
    'builtin protocols are seeded and published idempotently; new sites bind immutable versions without domain-specific code.';
