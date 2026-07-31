-- Allow protocol-backed wallets to synchronize immutable recharge transactions.
-- This job type only appends upstream_fund_events; it never changes account
-- upstream multipliers or historical usage cost snapshots.
ALTER TABLE upstream_finance_sync_runs
    DROP CONSTRAINT IF EXISTS upstream_finance_sync_runs_sync_type_check;

ALTER TABLE upstream_finance_sync_runs
    ADD CONSTRAINT upstream_finance_sync_runs_sync_type_check
    CHECK (sync_type IN ('probe', 'pricing', 'balance', 'quota', 'bill', 'funding'));
