-- IQ test plans: per-plan reasoning effort for the executed test request.
--
-- Plans schedule one question of a bank against a single account. This column
-- stores the reasoning effort level forwarded to the upstream request; the
-- empty string keeps the upstream default. Idempotent: safe to re-run.

ALTER TABLE iq_test_plans ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(20) NOT NULL DEFAULT '';
