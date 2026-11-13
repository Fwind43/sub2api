-- Dedicated H3 lifecycle; no Redis ownership or billing claims.
CREATE TABLE IF NOT EXISTS minimax_h3_tasks (
 id text PRIMARY KEY,
 user_id bigint NOT NULL,
 api_key_id bigint NOT NULL,
 group_id bigint NOT NULL,
 account_id bigint NOT NULL,
 upstream_id text NOT NULL DEFAULT '',
 generation text NOT NULL DEFAULT 'submitting',
 delivery text NOT NULL DEFAULT 'unverified',
 billing text NOT NULL DEFAULT 'pending',
 snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 next_poll_at timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz,
 lease_token text NOT NULL DEFAULT '',
 idempotency_key text NOT NULL,
 payload_hash text NOT NULL,
 UNIQUE(api_key_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS minimax_h3_tasks_pending ON minimax_h3_tasks(next_poll_at)
 WHERE generation IN ('submitting','pending','completed') AND billing = 'pending';
