-- Ops Monitoring (vNext): AI analysis task queue.
--
-- BE-003 scope:
-- - Persist AI analysis task source, trigger, filter, time range, status and execution metadata.
-- - Keep the migration idempotent and non-destructive.
-- - Do not create AI analysis reports here; reports are covered by BE-004.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS ops_ai_analysis_tasks (
    id BIGSERIAL PRIMARY KEY,

    -- alert_event | unified_errors | manual_filter
    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT,

    -- auto | manual
    trigger_type VARCHAR(16) NOT NULL,
    trigger_user_id BIGINT,

    time_start TIMESTAMPTZ NOT NULL,
    time_end TIMESTAMPTZ NOT NULL,
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- pending | running | completed | failed | expired
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    sample_count INT NOT NULL DEFAULT 0,

    provider VARCHAR(32),
    model VARCHAR(100),
    error_message TEXT,

    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_ai_analysis_tasks_source_type_check'
    ) THEN
        ALTER TABLE ops_ai_analysis_tasks
            ADD CONSTRAINT ops_ai_analysis_tasks_source_type_check
            CHECK (source_type IN ('alert_event', 'unified_errors', 'manual_filter'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_ai_analysis_tasks_trigger_type_check'
    ) THEN
        ALTER TABLE ops_ai_analysis_tasks
            ADD CONSTRAINT ops_ai_analysis_tasks_trigger_type_check
            CHECK (trigger_type IN ('auto', 'manual'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_ai_analysis_tasks_status_check'
    ) THEN
        ALTER TABLE ops_ai_analysis_tasks
            ADD CONSTRAINT ops_ai_analysis_tasks_status_check
            CHECK (status IN ('pending', 'running', 'completed', 'failed', 'expired'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_ai_analysis_tasks_sample_count_check'
    ) THEN
        ALTER TABLE ops_ai_analysis_tasks
            ADD CONSTRAINT ops_ai_analysis_tasks_sample_count_check
            CHECK (sample_count >= 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_ai_analysis_tasks_time_range_check'
    ) THEN
        ALTER TABLE ops_ai_analysis_tasks
            ADD CONSTRAINT ops_ai_analysis_tasks_time_range_check
            CHECK (time_end >= time_start);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_status_created_at
    ON ops_ai_analysis_tasks (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_source
    ON ops_ai_analysis_tasks (source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_time_range
    ON ops_ai_analysis_tasks (time_start, time_end);

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_trigger_user_id
    ON ops_ai_analysis_tasks (trigger_user_id)
    WHERE trigger_user_id IS NOT NULL;

COMMENT ON TABLE ops_ai_analysis_tasks IS 'Ops AI analysis task queue. Stores source, filters, time range, status, provider/model and execution metadata.';
COMMENT ON COLUMN ops_ai_analysis_tasks.source_type IS 'Task source: alert_event, unified_errors, or manual_filter.';
COMMENT ON COLUMN ops_ai_analysis_tasks.source_id IS 'Optional source object id, such as ops_alert_events.id.';
COMMENT ON COLUMN ops_ai_analysis_tasks.trigger_type IS 'Task trigger type: auto or manual.';
COMMENT ON COLUMN ops_ai_analysis_tasks.filters IS 'Snapshot of filters used for sampling errors.';
COMMENT ON COLUMN ops_ai_analysis_tasks.status IS 'Task status: pending, running, completed, failed, or expired.';
