-- Ops cache/admin vNext: extend alert events with lifecycle, merge, and AI linkage fields.
-- This migration is intentionally idempotent and preserves existing alert event rows.

ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS event_key VARCHAR(255),
    ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(32),
    ADD COLUMN IF NOT EXISTS merged_count INT,
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS recovered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS acknowledged_by BIGINT,
    ADD COLUMN IF NOT EXISTS acknowledged_note TEXT,
    ADD COLUMN IF NOT EXISTS processing_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS processing_by BIGINT,
    ADD COLUMN IF NOT EXISTS processing_note TEXT,
    ADD COLUMN IF NOT EXISTS processing_action TEXT,
    ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS closed_by BIGINT,
    ADD COLUMN IF NOT EXISTS closed_reason TEXT,
    ADD COLUMN IF NOT EXISTS trigger_snapshot JSONB,
    ADD COLUMN IF NOT EXISTS score_snapshot JSONB,
    ADD COLUMN IF NOT EXISTS ai_task_id BIGINT;

UPDATE ops_alert_events
SET lifecycle_status = CASE
    WHEN status = 'resolved' THEN 'recovered'
    WHEN status = 'manual_resolved' THEN 'closed'
    WHEN status = 'firing' THEN 'firing'
    ELSE status
END
WHERE lifecycle_status IS NULL;

UPDATE ops_alert_events
SET merged_count = 0
WHERE merged_count IS NULL;

UPDATE ops_alert_events
SET last_seen_at = fired_at
WHERE last_seen_at IS NULL;

UPDATE ops_alert_events
SET recovered_at = resolved_at
WHERE recovered_at IS NULL
  AND resolved_at IS NOT NULL;

UPDATE ops_alert_events
SET closed_at = resolved_at
WHERE closed_at IS NULL
  AND status = 'manual_resolved'
  AND resolved_at IS NOT NULL;

ALTER TABLE ops_alert_events
    ALTER COLUMN lifecycle_status SET DEFAULT 'firing',
    ALTER COLUMN lifecycle_status SET NOT NULL,
    ALTER COLUMN merged_count SET DEFAULT 0,
    ALTER COLUMN merged_count SET NOT NULL,
    ALTER COLUMN last_seen_at SET DEFAULT NOW(),
    ALTER COLUMN last_seen_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_event_key_status
    ON ops_alert_events (event_key, lifecycle_status);

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_lifecycle_last_seen
    ON ops_alert_events (lifecycle_status, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_ai_task_id
    ON ops_alert_events (ai_task_id)
    WHERE ai_task_id IS NOT NULL;

COMMENT ON COLUMN ops_alert_events.event_key IS 'Alert deduplication key for merging repeated events in silence windows.';
COMMENT ON COLUMN ops_alert_events.lifecycle_status IS 'Lifecycle status: firing/acknowledged/processing/recovered/closed/silenced.';
COMMENT ON COLUMN ops_alert_events.merged_count IS 'Number of repeated matches merged into this alert event.';
COMMENT ON COLUMN ops_alert_events.last_seen_at IS 'Most recent time the alert condition matched this event key.';
COMMENT ON COLUMN ops_alert_events.recovered_at IS 'Time when the alert condition recovered.';
COMMENT ON COLUMN ops_alert_events.acknowledged_at IS 'Time when an operator acknowledged the alert.';
COMMENT ON COLUMN ops_alert_events.acknowledged_by IS 'Operator user id that acknowledged the alert.';
COMMENT ON COLUMN ops_alert_events.acknowledged_note IS 'Operator acknowledgement note.';
COMMENT ON COLUMN ops_alert_events.processing_at IS 'Time when an operator marked the alert as processing.';
COMMENT ON COLUMN ops_alert_events.processing_by IS 'Operator user id that marked the alert as processing.';
COMMENT ON COLUMN ops_alert_events.processing_note IS 'Operator processing note.';
COMMENT ON COLUMN ops_alert_events.processing_action IS 'Expected or planned processing action.';
COMMENT ON COLUMN ops_alert_events.closed_at IS 'Time when the alert was closed.';
COMMENT ON COLUMN ops_alert_events.closed_by IS 'Operator user id that closed the alert.';
COMMENT ON COLUMN ops_alert_events.closed_reason IS 'Reason recorded when the alert was closed.';
COMMENT ON COLUMN ops_alert_events.trigger_snapshot IS 'Metric and impact snapshot captured when the alert fired.';
COMMENT ON COLUMN ops_alert_events.score_snapshot IS 'Health risk score snapshot captured around the alert event.';
COMMENT ON COLUMN ops_alert_events.ai_task_id IS 'Linked AI analysis task id.';
