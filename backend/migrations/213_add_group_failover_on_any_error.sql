-- Per-group switch: fail over to other accounts in the group on ANY upstream error
-- (any >=400 status, transport errors, mid-stream disconnects with continuation),
-- returning an error to the client only after every account in the group has failed.
-- Default ON for both existing and new groups.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS failover_on_any_error boolean NOT NULL DEFAULT true;

COMMENT ON COLUMN groups.failover_on_any_error IS '任意上游错误都切换分组内其他账号（含流式中途断开续写），直到全部账号失败才返回错误；默认开启。';
