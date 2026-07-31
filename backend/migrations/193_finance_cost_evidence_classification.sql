UPDATE usage_finance_cost_segments
SET pricing_source='upstream_catalog', cost_status='estimated'
WHERE pricing_source='upstream_exact'
  AND NOT (COALESCE(calculation_detail,'{}'::jsonb) ? 'settlement_interval_id');

WITH aggregate AS (
    SELECT usage_finance_record_id,
           COUNT(*) FILTER (WHERE cost_amount IS NULL) AS unknown_count,
           COUNT(*) FILTER (WHERE cost_status='estimated') AS estimated_count,
           COUNT(*) FILTER (WHERE cost_status='exact') AS exact_count,
           COALESCE(SUM(cost_amount),0) AS known_cost
    FROM usage_finance_cost_segments
    GROUP BY usage_finance_record_id
)
UPDATE usage_finance_records ufr
SET upstream_cost=CASE WHEN aggregate.unknown_count=0 THEN aggregate.known_cost ELSE NULL END,
    cost_status=CASE
        WHEN aggregate.unknown_count>0 THEN ufr.cost_status
        WHEN aggregate.estimated_count>0 THEN 'estimated'
        WHEN aggregate.exact_count>0 THEN 'exact'
        ELSE ufr.cost_status END,
    pricing_source=CASE
        WHEN aggregate.unknown_count=0 AND aggregate.estimated_count>0 THEN 'upstream_catalog'
        WHEN aggregate.unknown_count=0 AND aggregate.exact_count>0 THEN 'upstream_exact'
        ELSE ufr.pricing_source END,
    updated_at=NOW()
FROM aggregate
WHERE ufr.id=aggregate.usage_finance_record_id;

COMMENT ON COLUMN usage_finance_records.pricing_source IS
    'Evidence category: upstream_exact is real settlement; upstream_catalog/manual/system are calculated or contract evidence.';
