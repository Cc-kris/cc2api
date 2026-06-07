-- Ops Monitoring (vNext): AI analysis reports.
--
-- BE-004 scope:
-- - Persist one report per AI analysis task.
-- - Store conclusion, root cause, impact, evidence, suggested actions, error breakdown and feedback.
-- - Keep the migration idempotent and non-destructive.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS ops_ai_analysis_reports (
    task_id BIGINT PRIMARY KEY,

    summary TEXT NOT NULL DEFAULT '',
    root_cause TEXT,
    impact_scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    suggested_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_breakdown JSONB NOT NULL DEFAULT '{}'::jsonb,

    confidence VARCHAR(16) NOT NULL DEFAULT 'medium',
    feedback_status VARCHAR(32) NOT NULL DEFAULT 'none',
    feedback_note TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_ai_analysis_reports_confidence_check'
    ) THEN
        ALTER TABLE ops_ai_analysis_reports
            ADD CONSTRAINT ops_ai_analysis_reports_confidence_check
            CHECK (confidence IN ('high', 'medium', 'low'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ops_ai_analysis_reports_feedback_status_check'
    ) THEN
        ALTER TABLE ops_ai_analysis_reports
            ADD CONSTRAINT ops_ai_analysis_reports_feedback_status_check
            CHECK (feedback_status IN ('none', 'useful', 'not_useful', 'wrong_category'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_reports_confidence
    ON ops_ai_analysis_reports (confidence);

CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_reports_feedback_status
    ON ops_ai_analysis_reports (feedback_status);

COMMENT ON TABLE ops_ai_analysis_reports IS 'Ops AI analysis report content and operator feedback.';
COMMENT ON COLUMN ops_ai_analysis_reports.task_id IS 'AI analysis task id. One report per task.';
COMMENT ON COLUMN ops_ai_analysis_reports.summary IS 'Short conclusion of the issue.';
COMMENT ON COLUMN ops_ai_analysis_reports.root_cause IS 'Root cause judgment.';
COMMENT ON COLUMN ops_ai_analysis_reports.impact_scope IS 'JSON impact scope summary.';
COMMENT ON COLUMN ops_ai_analysis_reports.evidence IS 'JSON evidence summary.';
COMMENT ON COLUMN ops_ai_analysis_reports.suggested_actions IS 'JSON suggested actions.';
COMMENT ON COLUMN ops_ai_analysis_reports.error_breakdown IS 'JSON error distribution and category breakdown.';
COMMENT ON COLUMN ops_ai_analysis_reports.confidence IS 'Report confidence: high, medium, or low.';
COMMENT ON COLUMN ops_ai_analysis_reports.feedback_status IS 'Operator feedback: none, useful, not_useful, or wrong_category.';
