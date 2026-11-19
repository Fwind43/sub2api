package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// H3HistoryItem is the user-facing projection of a persisted H3 task.
type H3HistoryItem struct {
	QueuePosition *int       `json:"queue_position,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`

	ID         string `json:"id"`
	Generation string `json:"status"`
	Delivery   string `json:"delivery_status"`
	Billing    string `json:"billing_status"`
	// Cost is the price locked at submission (balance/quota charge on
	// settlement). Only settled tasks were actually charged.
	Cost      *float64  `json:"cost"`
	CreatedAt time.Time `json:"created_at"`
}

// H3GenerationStates lists the generation values a history filter may select.
var H3GenerationStates = []string{"submitting", "submission_unknown", "pending", "completed", "failed"}

// ListUserTasks includes historical tasks even if their API key was deleted.
// A non-empty generation restricts results to that generation state.
func (s *H3Service) ListUserTasks(ctx context.Context, userID int64, generation string, page, pageSize int) ([]H3HistoryItem, int64, error) {
	items := make([]H3HistoryItem, 0, pageSize)
	where, args := "user_id = $1", []any{userID}
	if generation != "" {
		where, args = where+" AND generation = $2", append(args, generation)
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM minimax_h3_tasks WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if int64(offset) >= total {
		return items, total, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, generation, delivery, billing, (snapshot->'cost'->>'ActualCost')::float8, created_at, queue_position, CASE WHEN billing IN ('settled','not_billable') THEN updated_at END
		FROM minimax_h3_tasks WHERE `+where+fmt.Sprintf(`
		ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2), append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item H3HistoryItem
		var cost sql.NullFloat64
		if err := rows.Scan(&item.ID, &item.Generation, &item.Delivery, &item.Billing, &cost, &item.CreatedAt, &item.QueuePosition, &item.FinishedAt); err != nil {
			return nil, 0, err
		}
		if cost.Valid {
			item.Cost = &cost.Float64
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
	item := H3HistoryItem{ID: t.ID, Generation: t.Generation, Delivery: t.Delivery, Billing: t.Billing, CreatedAt: t.CreatedAt, QueuePosition: t.QueuePosition, FinishedAt: t.FinishedAt}
	if t.Snapshot.Cost != nil {
		c := t.Snapshot.Cost.ActualCost
		item.Cost = &c
	}
	return item
}
