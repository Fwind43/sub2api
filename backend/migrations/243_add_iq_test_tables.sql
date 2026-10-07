-- Account IQ test: custom question banks, scheduled runs, and scored results.
--
-- Three concerns:
--   1. iq_test_banks / iq_test_questions  - persistent, editable question banks
--   2. iq_test_plans                       - cron-scheduled runs of a bank against an account
--   3. iq_test_runs                        - scored outcome of one run (manual or scheduled)
--
-- Idempotent: safe to re-run.

CREATE TABLE IF NOT EXISTS iq_test_banks (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS iq_test_questions (
    id         BIGSERIAL PRIMARY KEY,
    bank_id    BIGINT NOT NULL REFERENCES iq_test_banks(id) ON DELETE CASCADE,
    position   INT NOT NULL DEFAULT 0,
    type       VARCHAR(32) NOT NULL DEFAULT 'single_choice',
    prompt     TEXT NOT NULL DEFAULT '',
    options    JSONB NOT NULL DEFAULT '[]'::jsonb,
    answer     TEXT NOT NULL DEFAULT '',
    keywords   JSONB NOT NULL DEFAULT '[]'::jsonb,
    weight     INT NOT NULL DEFAULT 1,
    max_tokens INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_iqt_questions_bank ON iq_test_questions(bank_id, position);

CREATE TABLE IF NOT EXISTS iq_test_plans (
    id              BIGSERIAL PRIMARY KEY,
    bank_id         BIGINT NOT NULL REFERENCES iq_test_banks(id) ON DELETE CASCADE,
    account_id      BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model_id        VARCHAR(100) NOT NULL DEFAULT '',
    cron_expression VARCHAR(100) NOT NULL DEFAULT '0 3 * * *',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    max_results     INT NOT NULL DEFAULT 50,
    last_run_at     TIMESTAMPTZ,
    next_run_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_iqt_plans_account ON iq_test_plans(account_id);
CREATE INDEX IF NOT EXISTS idx_iqt_plans_bank ON iq_test_plans(bank_id);
CREATE INDEX IF NOT EXISTS idx_iqt_plans_enabled_next_run ON iq_test_plans(enabled, next_run_at) WHERE enabled = true;

CREATE TABLE IF NOT EXISTS iq_test_runs (
    id            BIGSERIAL PRIMARY KEY,
    plan_id       BIGINT REFERENCES iq_test_plans(id) ON DELETE CASCADE,
    bank_id       BIGINT NOT NULL REFERENCES iq_test_banks(id) ON DELETE CASCADE,
    account_id    BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model_id      VARCHAR(100) NOT NULL DEFAULT '',
    trigger       VARCHAR(20) NOT NULL DEFAULT 'manual',
    status        VARCHAR(20) NOT NULL DEFAULT 'success',
    score         DOUBLE PRECISION NOT NULL DEFAULT 0,
    total         INT NOT NULL DEFAULT 0,
    correct       INT NOT NULL DEFAULT 0,
    latency_ms    BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    details       JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_iqt_runs_bank_created ON iq_test_runs(bank_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_iqt_runs_account_created ON iq_test_runs(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_iqt_runs_plan_created ON iq_test_runs(plan_id, created_at DESC);

-- Single-question runs: plans pick one question out of the bank; runs record
-- which question was exercised. Idempotent for both fresh installs and
-- databases created before this revision.
ALTER TABLE iq_test_plans ADD COLUMN IF NOT EXISTS question_id BIGINT;
ALTER TABLE iq_test_runs ADD COLUMN IF NOT EXISTS question_id BIGINT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'iq_test_plans_question_id_fkey'
    ) THEN
        ALTER TABLE iq_test_plans
            ADD CONSTRAINT iq_test_plans_question_id_fkey
            FOREIGN KEY (question_id) REFERENCES iq_test_questions(id) ON DELETE SET NULL;
    END IF;
END $$;
