ALTER TABLE users ADD COLUMN IF NOT EXISTS h3_task_limit integer CHECK (h3_task_limit >= 0);
