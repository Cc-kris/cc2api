-- Prevent the same upstream top-up/refund transaction from being recorded twice
-- with different HTTP idempotency keys. Historical rows without a reference are
-- preserved; new service writes require a reference for these event types.
CREATE UNIQUE INDEX IF NOT EXISTS upstream_fund_events_wallet_type_reference_unique
    ON upstream_fund_events (wallet_id, event_type, reference_no)
    WHERE reference_no IS NOT NULL AND event_type IN ('topup', 'refund');
