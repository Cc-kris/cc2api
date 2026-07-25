CREATE TABLE IF NOT EXISTS grok_video_task_bindings (
  api_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  group_id BIGINT,
  task_id VARCHAR(128) NOT NULL,
  account_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (api_key_id, user_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_grok_video_task_bindings_account_id
  ON grok_video_task_bindings(account_id);

COMMENT ON TABLE grok_video_task_bindings IS 'Grok 异步视频任务的持久账号归属；客户端查询必须命中同一 API Key、用户和分组';
