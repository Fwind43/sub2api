-- VIP 名单与分组 VIP 倍率（单档 VIP）
-- 优先级：user_group_rate_multipliers(用户专属) > group_vip_rates(VIP 倍率) > groups.rate_multiplier

CREATE TABLE IF NOT EXISTS vip_users (
    user_id    BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    notes      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS group_vip_rates (
    group_id        BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    rate_multiplier DOUBLE PRECISION NOT NULL CHECK (rate_multiplier >= 0),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
