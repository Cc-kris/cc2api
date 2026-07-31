ALTER TABLE finance_fx_rate_versions
    ADD COLUMN IF NOT EXISTS operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS change_reason VARCHAR(500),
    ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(200);

CREATE UNIQUE INDEX IF NOT EXISTS finance_fx_rate_idempotency_key_uq
    ON finance_fx_rate_versions(idempotency_key)
    WHERE idempotency_key IS NOT NULL;
