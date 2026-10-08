-- 245_clinepass_upstream_lastknown.sql
-- Records the most recent upstream provider that actually served a clinepass
-- request, per (account, model). Written by the gateway when it parses the
-- relay's provider_metadata / provider routing info out of the response.
--
-- Idempotent: safe to re-run.

CREATE TABLE IF NOT EXISTS clinepass_upstream_lastknown (
    id           BIGSERIAL PRIMARY KEY,
    account_id   BIGINT       NOT NULL,
    model        VARCHAR(255) NOT NULL,
    provider     VARCHAR(255) NOT NULL,
    pipeline     VARCHAR(32)  NOT NULL DEFAULT '',
    canonical    VARCHAR(255) NOT NULL DEFAULT '',
    fallbacks    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    plan         TEXT         NOT NULL DEFAULT '',
    observed_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT clinepass_upstream_lastknown_account_model_key UNIQUE (account_id, model)
);

CREATE INDEX IF NOT EXISTS idx_clinepass_upstream_lastknown_account
    ON clinepass_upstream_lastknown (account_id);
