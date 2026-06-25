-- Add Seedace/video usage metadata fields.

ALTER TABLE usage_logs
  ADD COLUMN IF NOT EXISTS video_duration_seconds INTEGER,
  ADD COLUMN IF NOT EXISTS video_task_id VARCHAR(128);
