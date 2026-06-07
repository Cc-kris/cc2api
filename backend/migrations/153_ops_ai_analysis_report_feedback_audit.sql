-- Ops Monitoring (vNext): AI analysis report feedback audit fields.
--
-- BE-030 scope:
-- - Store who submitted AI analysis report feedback and when it was submitted.
-- - Keep the migration idempotent and non-destructive.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE ops_ai_analysis_reports
    ADD COLUMN IF NOT EXISTS feedback_user_id BIGINT;

ALTER TABLE ops_ai_analysis_reports
    ADD COLUMN IF NOT EXISTS feedback_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_reports_feedback_user_id
    ON ops_ai_analysis_reports (feedback_user_id)
    WHERE feedback_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_reports_feedback_at
    ON ops_ai_analysis_reports (feedback_at DESC)
    WHERE feedback_at IS NOT NULL;

COMMENT ON COLUMN ops_ai_analysis_reports.feedback_user_id IS 'User id of the operator who submitted the latest AI report feedback.';
COMMENT ON COLUMN ops_ai_analysis_reports.feedback_at IS 'Timestamp when the latest AI report feedback was submitted.';
