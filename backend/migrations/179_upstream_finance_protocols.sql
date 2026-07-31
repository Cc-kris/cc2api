-- Generic upstream finance protocol catalog and immutable versions.

CREATE TABLE IF NOT EXISTS upstream_finance_protocols (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(80) NOT NULL UNIQUE,
    name VARCHAR(120) NOT NULL,
    protocol_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    current_version_id BIGINT,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_finance_protocol_type_check CHECK (protocol_type IN ('builtin','http_json','plugin')),
    CONSTRAINT upstream_finance_protocol_status_check CHECK (status IN ('draft','published','disabled'))
);

CREATE TABLE IF NOT EXISTS upstream_finance_protocol_versions (
    id BIGSERIAL PRIMARY KEY,
    protocol_id BIGINT NOT NULL REFERENCES upstream_finance_protocols(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    config JSONB NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    validation_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    validation_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    published_at TIMESTAMPTZ,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_finance_protocol_version_unique UNIQUE (protocol_id, version),
    CONSTRAINT upstream_finance_protocol_validation_check CHECK (validation_status IN ('pending','valid','invalid'))
);

DO $$ BEGIN
    ALTER TABLE upstream_finance_protocols
        ADD CONSTRAINT upstream_finance_protocol_current_version_fk
        FOREIGN KEY (current_version_id) REFERENCES upstream_finance_protocol_versions(id) ON DELETE SET NULL;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_upstream_finance_protocol_status_type
    ON upstream_finance_protocols(status, protocol_type);
CREATE INDEX IF NOT EXISTS idx_upstream_finance_protocol_updated_id
    ON upstream_finance_protocols(updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_upstream_finance_protocol_version_created
    ON upstream_finance_protocol_versions(protocol_id, created_at DESC);
