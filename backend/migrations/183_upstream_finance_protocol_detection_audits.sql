CREATE TABLE IF NOT EXISTS upstream_finance_protocol_detection_audits (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    protocol_id BIGINT REFERENCES upstream_finance_protocols(id) ON DELETE SET NULL,
    protocol_version_id BIGINT REFERENCES upstream_finance_protocol_versions(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL,
    reason VARCHAR(120) NOT NULL DEFAULT '',
    platform VARCHAR(50) NOT NULL DEFAULT '',
    account_type VARCHAR(50) NOT NULL DEFAULT '',
    base_url_hash CHAR(64) NOT NULL,
    candidates JSONB NOT NULL DEFAULT '[]'::jsonb,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_finance_detection_audit_status_check CHECK (status IN ('matched','not_found','conflict','error'))
);

CREATE INDEX IF NOT EXISTS idx_upstream_finance_detection_audit_account_created
    ON upstream_finance_protocol_detection_audits(account_id, created_at DESC);
