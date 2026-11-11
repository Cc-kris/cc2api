-- V03-R11 用量审计字段（只读审计，不参与计费）。
-- 全部可空、无默认值：PostgreSQL 11+ 上只改元数据，不重写 usage_logs 大表。
-- NULL 表示历史行、该路径未观测到对应值，或客户端未显式提供。
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_response_model VARCHAR(200),
    ADD COLUMN IF NOT EXISTS upstream_model_mismatch BOOLEAN,
    ADD COLUMN IF NOT EXISTS session_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS requested_reasoning_effort VARCHAR(20),
    ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(128);
