-- Persist user video generation chat session history.
CREATE TABLE IF NOT EXISTS seedace_video_session_histories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(64) NOT NULL,
    summary VARCHAR(64) NOT NULL,
    generation_count INTEGER NOT NULL DEFAULT 0,
    messages JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT seedace_video_session_histories_generation_count_nonnegative CHECK (generation_count >= 0),
    CONSTRAINT seedace_video_session_histories_session_id_nonempty CHECK (length(trim(session_id)) > 0),
    CONSTRAINT seedace_video_session_histories_summary_nonempty CHECK (length(trim(summary)) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_seedace_video_session_histories_user_session
    ON seedace_video_session_histories(user_id, session_id);

CREATE INDEX IF NOT EXISTS idx_seedace_video_session_histories_user_updated
    ON seedace_video_session_histories(user_id, updated_at DESC);
