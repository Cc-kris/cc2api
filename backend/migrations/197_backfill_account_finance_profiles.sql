INSERT INTO account_finance_profiles(
    account_id,cost_mode,endpoint_source,endpoint_base_url_snapshot,credential_source,counter_scope,
    balance_unit_semantics,account_multiplier_change_id,account_multiplier_snapshot,contract_type,
    contract_multiplier,contract_multiplier_change_id,readiness_status,readiness_detail,version,
    effective_from,reason
)
SELECT a.id,'contract_multiplier','account_base_url',
       COALESCE(NULLIF(a.credentials->>'base_url',''),''),'account_api_key','account','none',
       change.id,a.upstream_cost_multiplier,
       CASE WHEN a.upstream_cost_multiplier IS NULL THEN NULL ELSE 'multiplier' END,
       a.upstream_cost_multiplier,
       CASE WHEN a.upstream_cost_multiplier IS NULL THEN NULL ELSE change.id END,
       CASE WHEN a.upstream_cost_multiplier IS NULL THEN 'unconfigured' ELSE 'ready_contract' END,
       CASE WHEN a.upstream_cost_multiplier IS NULL
            THEN '{"issues":["未配置账号上游倍率"],"actions":["在账号详情填写上游倍率"]}'::jsonb
            ELSE '{"issues":[],"actions":[]}'::jsonb END,
       1,COALESCE(a.upstream_cost_multiplier_updated_at,a.created_at,NOW()),'历史账号财务配置初始化'
FROM accounts a
LEFT JOIN LATERAL (
    SELECT id FROM account_upstream_multiplier_changes c
    WHERE c.account_id=a.id
    ORDER BY c.effective_at DESC,c.id DESC LIMIT 1
) change ON TRUE
WHERE NOT EXISTS (SELECT 1 FROM account_finance_profiles p WHERE p.account_id=a.id);
