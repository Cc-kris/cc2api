-- Bind an upstream wallet to one immutable, published finance protocol version.

ALTER TABLE upstream_wallets
    ADD COLUMN IF NOT EXISTS protocol_version_id BIGINT
        REFERENCES upstream_finance_protocol_versions(id) ON DELETE RESTRICT;

ALTER TABLE upstream_wallets DROP CONSTRAINT IF EXISTS upstream_wallets_pricing_adapter_check;
ALTER TABLE upstream_wallets ADD CONSTRAINT upstream_wallets_pricing_adapter_check
    CHECK (pricing_adapter IN ('manual', 'newapi', 'protocol', 'none'));

ALTER TABLE upstream_wallets DROP CONSTRAINT IF EXISTS upstream_wallets_balance_adapter_check;
ALTER TABLE upstream_wallets ADD CONSTRAINT upstream_wallets_balance_adapter_check
    CHECK (balance_adapter IN ('manual', 'newapi_user', 'protocol', 'none'));

ALTER TABLE upstream_wallets DROP CONSTRAINT IF EXISTS upstream_wallets_quota_adapter_check;
ALTER TABLE upstream_wallets ADD CONSTRAINT upstream_wallets_quota_adapter_check
    CHECK (quota_adapter IN ('legacy_openai', 'newapi', 'protocol', 'none'));

ALTER TABLE upstream_wallets DROP CONSTRAINT IF EXISTS upstream_wallets_protocol_binding_check;
ALTER TABLE upstream_wallets ADD CONSTRAINT upstream_wallets_protocol_binding_check CHECK (
    (protocol_version_id IS NULL AND NOT (
        pricing_adapter='protocol' OR balance_adapter='protocol' OR quota_adapter='protocol'
    )) OR
    (protocol_version_id IS NOT NULL AND
        pricing_adapter='protocol' AND balance_adapter='protocol' AND quota_adapter='protocol')
);

CREATE INDEX IF NOT EXISTS idx_upstream_wallets_protocol_version
    ON upstream_wallets(protocol_version_id)
    WHERE protocol_version_id IS NOT NULL;
