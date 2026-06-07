-- Ops cache/admin vNext: semantic cache audit records.
--
-- BE-008 scope:
-- - Store semantic cache candidate decisions, review state and feedback.
-- - Support semantic audit list filters by time, platform, model, API key, review status, decision and similarity.
-- - Keep the migration idempotent and non-destructive.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS ops_semantic_cache_audits (
    id BIGSERIAL PRIMARY KEY,

    request_id VARCHAR(128) NOT NULL,
    semantic_entry_id BIGINT REFERENCES ops_semantic_cache_entries(id) ON DELETE SET NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    platform VARCHAR(32) NOT NULL,
    model VARCHAR(128) NOT NULL,
    api_key_id BIGINT,
    similarity DECIMAL(8,6) NOT NULL DEFAULT 0,
    decision VARCHAR(32) NOT NULL,
    block_reason VARCHAR(128),
    review_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    feedback_type VARCHAR(32) NOT NULL DEFAULT 'none',
    feedback_note TEXT,
    operator_user_id BIGINT,
    auto_close_reason TEXT,
    source_summary TEXT,
    target_summary TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_audits_decision_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_audits
            ADD CONSTRAINT ops_semantic_cache_audits_decision_check
            CHECK (decision IN ('observe', 'hit', 'miss', 'blocked', 'rollback'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_audits_review_status_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_audits
            ADD CONSTRAINT ops_semantic_cache_audits_review_status_check
            CHECK (review_status IN ('pending', 'reusable', 'not_reusable', 'disputed'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_audits_feedback_type_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_audits
            ADD CONSTRAINT ops_semantic_cache_audits_feedback_type_check
            CHECK (feedback_type IN ('none', 'wrong_hit', 'complaint', 'manual_mark'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_audits_similarity_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_audits
            ADD CONSTRAINT ops_semantic_cache_audits_similarity_check
            CHECK (similarity >= 0 AND similarity <= 1);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_audits_identity_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_audits
            ADD CONSTRAINT ops_semantic_cache_audits_identity_check
            CHECK (
                (api_key_id IS NULL OR api_key_id >= 0)
                AND (operator_user_id IS NULL OR operator_user_id >= 0)
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_semantic_cache_audits_required_text_check'
    ) THEN
        ALTER TABLE ops_semantic_cache_audits
            ADD CONSTRAINT ops_semantic_cache_audits_required_text_check
            CHECK (
                length(trim(request_id)) > 0
                AND length(trim(platform)) > 0
                AND length(trim(model)) > 0
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_audits_occurred_at
    ON ops_semantic_cache_audits (occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_audits_entry
    ON ops_semantic_cache_audits (semantic_entry_id, occurred_at DESC)
    WHERE semantic_entry_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_audits_platform_model
    ON ops_semantic_cache_audits (platform, model, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_audits_api_key
    ON ops_semantic_cache_audits (api_key_id, occurred_at DESC)
    WHERE api_key_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_audits_decision_review
    ON ops_semantic_cache_audits (decision, review_status, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_audits_similarity
    ON ops_semantic_cache_audits (similarity, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_semantic_cache_audits_request_id
    ON ops_semantic_cache_audits (request_id);

COMMENT ON TABLE ops_semantic_cache_audits IS 'Semantic cache decision, review and feedback audit records.';
COMMENT ON COLUMN ops_semantic_cache_audits.request_id IS 'Gateway request identifier.';
COMMENT ON COLUMN ops_semantic_cache_audits.semantic_entry_id IS 'Candidate semantic cache entry, nullable for miss or blocked decisions.';
COMMENT ON COLUMN ops_semantic_cache_audits.similarity IS 'Semantic similarity score between 0 and 1.';
COMMENT ON COLUMN ops_semantic_cache_audits.decision IS 'Decision: observe, hit, miss, blocked or rollback.';
COMMENT ON COLUMN ops_semantic_cache_audits.review_status IS 'Review status: pending, reusable, not_reusable or disputed.';
COMMENT ON COLUMN ops_semantic_cache_audits.feedback_type IS 'Feedback type: none, wrong_hit, complaint or manual_mark.';
COMMENT ON COLUMN ops_semantic_cache_audits.auto_close_reason IS 'Reason for automatic semantic cache closure after quality rollback threshold is reached.';
