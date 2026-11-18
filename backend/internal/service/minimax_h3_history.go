package service

import (
	"context"
	"time"
)

// H3HistoryItem is the user-facing projection of a persisted H3 task.
type H3HistoryItem struct {
	ID         string    `json:"id"`
	Generation string    `json:"status"`
	Delivery   string    `json:"delivery_status"`
	Billing    string    `json:"billing_status"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListUserTasks includes historical tasks even if their API key was deleted.
func (s *H3Service) ListUserTasks(ctx context.Context, userID int64, page, pageSize int) ([]H3HistoryItem, int64, error) {
	items := make([]H3HistoryItem, 0, pageSize)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM minimax_h3_tasks WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if int64(offset) >= total {
		return items, total, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, generation, delivery, billing, created_at
		FROM minimax_h3_tasks WHERE user_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var item H3HistoryItem
		if err := rows.Scan(&item.ID, &item.Generation, &item.Delivery, &item.Billing, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
