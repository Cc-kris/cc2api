CREATE TABLE IF NOT EXISTS finance_ledger_retries (
    id BIGSERIAL PRIMARY KEY,
    usage_log_id BIGINT NOT NULL UNIQUE REFERENCES usage_logs(id) ON DELETE CASCADE,
    attempt_count INTEGER NOT NULL DEFAULT 1,
    last_error TEXT NOT NULL,
    next_retry_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    first_failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT finance_ledger_retries_attempt_check CHECK (attempt_count > 0),
    CONSTRAINT finance_ledger_retries_status_check CHECK (status IN ('pending','exhausted','resolved'))
);

CREATE INDEX IF NOT EXISTS idx_finance_ledger_retries_due
    ON finance_ledger_retries (next_retry_at, usage_log_id)
    WHERE status = 'pending';
