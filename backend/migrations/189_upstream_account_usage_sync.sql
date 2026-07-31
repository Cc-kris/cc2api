-- Account usage synchronization records cumulative protocol observations only.
-- Recharge ratios, wallet balances and funding events are not inputs to this job.
ALTER TABLE upstream_finance_sync_runs
    DROP CONSTRAINT IF EXISTS upstream_finance_sync_runs_type_check;

ALTER TABLE upstream_finance_sync_runs
    DROP CONSTRAINT IF EXISTS upstream_finance_sync_runs_sync_type_check;

ALTER TABLE upstream_finance_sync_runs
    ADD CONSTRAINT upstream_finance_sync_runs_sync_type_check
    CHECK (sync_type IN ('probe', 'pricing', 'balance', 'quota', 'bill', 'funding', 'account_usage'));
