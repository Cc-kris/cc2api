-- Ops cache/admin vNext: create AI analysis task table.
-- This migration is intentionally idempotent and does not modify existing rows.

CREATE TABLE IF NOT EXISTS ops_ai_analysis_tasks (
    id BIGSERIAL PRIMARY KEY,

    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT,
    trigger_type VARCHAR(16) NOT NULL DEFAULT 'manual',
    trigger_user_id BIGINT,

    time_start TIMESTAMPTZ NOT NULL,
    time_end TIMESTAMPTZ NOT NULL,
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,

    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    sample_count INT NOT NULL DEFAULT 0,

    provider VARCHAR(32),
    model VARCHAR(128),
    error_message TEXT,

    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_status_created
    ON ops_ai_analysis_tasks (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_source
    ON ops_ai_analysis_tasks (source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_trigger_user
    ON ops_ai_analysis_tasks (trigger_user_id, created_at DESC)
    WHERE trigger_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_time_range
    ON ops_ai_analysis_tasks (time_start, time_end);

COMMENT ON TABLE ops_ai_analysis_tasks IS 'Ops AI analysis task queue and lifecycle records.';
COMMENT ON COLUMN ops_ai_analysis_tasks.source_type IS 'Task source: alert_event/unified_errors/manual_filter.';
COMMENT ON COLUMN ops_ai_analysis_tasks.source_id IS 'Optional source object id, such as alert event id.';
COMMENT ON COLUMN ops_ai_analysis_tasks.trigger_type IS 'Trigger type: auto/manual.';
COMMENT ON COLUMN ops_ai_analysis_tasks.trigger_user_id IS 'User id for manually triggered analysis.';
COMMENT ON COLUMN ops_ai_analysis_tasks.time_start IS 'Analysis source window start time.';
COMMENT ON COLUMN ops_ai_analysis_tasks.time_end IS 'Analysis source window end time.';
COMMENT ON COLUMN ops_ai_analysis_tasks.filters IS 'JSON filters used to select errors or events for analysis.';
COMMENT ON COLUMN ops_ai_analysis_tasks.status IS 'Task status: pending/running/completed/failed/expired.';
COMMENT ON COLUMN ops_ai_analysis_tasks.sample_count IS 'Number of sampled records for the analysis task.';
COMMENT ON COLUMN ops_ai_analysis_tasks.provider IS 'AI provider or interface type used by the task.';
COMMENT ON COLUMN ops_ai_analysis_tasks.model IS 'AI model used by the task.';
COMMENT ON COLUMN ops_ai_analysis_tasks.error_message IS 'Failure reason when task status is failed or expired.';
COMMENT ON COLUMN ops_ai_analysis_tasks.started_at IS 'Task worker start time.';
COMMENT ON COLUMN ops_ai_analysis_tasks.finished_at IS 'Task completion or terminal failure time.';
COMMENT ON COLUMN ops_ai_analysis_tasks.expires_at IS 'Task expiration deadline.';
