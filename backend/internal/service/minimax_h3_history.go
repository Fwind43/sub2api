package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
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

// GetUserTask returns a task owned by the user, regardless of which API key
// submitted it.
func (s *H3Service) GetUserTask(ctx context.Context, id string, userID int64) (*H3Task, error) {
	if id == "" || userID <= 0 {
		return nil, ErrH3NotFound
	}
	return scanH3(s.db.QueryRowContext(ctx, "SELECT "+h3Columns+" FROM minimax_h3_tasks WHERE id=$1 AND user_id=$2", id, userID))
}

// RefreshUserTask advances one owned, unsettled task immediately instead of
// waiting for the background poller. Tasks that are still submitting, already
// terminal, leased by another worker, or refreshed in the last few seconds are
// returned unchanged.
func (s *H3Service) RefreshUserTask(ctx context.Context, id string, userID int64) (*H3Task, error) {
	if _, err := s.GetUserTask(ctx, id, userID); err != nil {
		return nil, err
	}
	token := uuid.NewString()
	t, err := scanH3(s.db.QueryRowContext(ctx, `UPDATE minimax_h3_tasks SET lease_until=now()+interval '3 minutes',lease_token=$3
		WHERE id=$1 AND user_id=$2 AND billing='pending' AND generation IN ('pending','completed')
		AND (lease_until IS NULL OR lease_until<now()) AND next_poll_at<=now()+interval '15 seconds'
		RETURNING `+h3Columns, id, userID, token))
	if err == nil {
		// Detach from the HTTP request so a client disconnect cannot abandon
		// a claimed lease mid-settlement; the service context still stops it.
		base := s.ctx
		if base == nil {
			base = context.Background()
		}
		s.processClaimed(base, t, token)
	} else if !errors.Is(err, ErrH3NotFound) {
		return nil, err
	}
	return s.GetUserTask(ctx, id, userID)
}

// HistoryItem projects a task for the user-facing history API.
func (t *H3Task) HistoryItem() H3HistoryItem {
	return H3HistoryItem{ID: t.ID, Generation: t.Generation, Delivery: t.Delivery, Billing: t.Billing, CreatedAt: t.CreatedAt}
}
