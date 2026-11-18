CREATE INDEX CONCURRENTLY IF NOT EXISTS minimax_h3_tasks_user_history
 ON minimax_h3_tasks(user_id, created_at DESC, id DESC);
