-- Track users that have already received the current low-balance notification.
-- A row is removed once the user balance recovers to the configured threshold or the user is excluded.
CREATE TABLE IF NOT EXISTS balance_low_notify_states (
    user_id     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    notified_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_balance_low_notify_states_notified_at
    ON balance_low_notify_states(notified_at);
