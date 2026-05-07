-- Persisted image playground tasks.
-- Stores task lifecycle and image payloads so browser disconnects/refreshes do
-- not lose task state. Large image data URLs are kept in JSONB for portability.

CREATE TABLE IF NOT EXISTS model_playground_tasks (
    id                       TEXT PRIMARY KEY,
    user_id                  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id               BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    api_mode                 VARCHAR(20) NOT NULL DEFAULT 'images',
    model                    TEXT NOT NULL,
    prompt                   TEXT NOT NULL,
    params                   JSONB NOT NULL DEFAULT '{}'::jsonb,
    input_images             JSONB NOT NULL DEFAULT '[]'::jsonb,
    output_images            JSONB NOT NULL DEFAULT '[]'::jsonb,
    revised_prompt_by_image  JSONB NOT NULL DEFAULT '{}'::jsonb,
    actual_params            JSONB,
    raw_response             JSONB,
    codex_cli                BOOLEAN NOT NULL DEFAULT FALSE,
    status                   VARCHAR(20) NOT NULL DEFAULT 'queued',
    error                    TEXT,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at               TIMESTAMPTZ,
    finished_at              TIMESTAMPTZ,
    elapsed_ms               INTEGER
);

CREATE INDEX IF NOT EXISTS idx_model_playground_tasks_user_created
    ON model_playground_tasks(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_model_playground_tasks_user_status
    ON model_playground_tasks(user_id, status);
