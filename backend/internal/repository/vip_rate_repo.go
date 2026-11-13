package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var _ service.VIPRateStore = (*userGroupRateRepository)(nil)

func (r *userGroupRateRepository) ListVIPUsers(ctx context.Context) ([]service.VIPUser, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT v.user_id, COALESCE(u.email,''), COALESCE(u.username,''), v.notes, v.created_at
		FROM vip_users v LEFT JOIN users u ON u.id = v.user_id ORDER BY v.created_at DESC, v.user_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.VIPUser{}
	for rows.Next() {
		var x service.VIPUser
		if err := rows.Scan(&x.UserID, &x.Email, &x.Username, &x.Notes, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *userGroupRateRepository) AddVIPUsers(ctx context.Context, userIDs []int64, notes string) (int, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	res, err := r.sql.ExecContext(ctx, `INSERT INTO vip_users (user_id, notes)
		SELECT id, $2 FROM users WHERE id = ANY($1) AND deleted_at IS NULL
		ON CONFLICT (user_id) DO UPDATE SET notes = EXCLUDED.notes`, pq.Array(userIDs), notes)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *userGroupRateRepository) RemoveVIPUsers(ctx context.Context, userIDs []int64) (int, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	res, err := r.sql.ExecContext(ctx, `DELETE FROM vip_users WHERE user_id = ANY($1)`, pq.Array(userIDs))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *userGroupRateRepository) GetGroupVIPRate(ctx context.Context, groupID int64) (*float64, error) {
	var v sql.NullFloat64
	err := scanSingleRow(ctx, r.sql, `SELECT rate_multiplier FROM group_vip_rates WHERE group_id = $1`, []any{groupID}, &v)
	if err == sql.ErrNoRows || (err == nil && !v.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f := v.Float64
	return &f, nil
}

func (r *userGroupRateRepository) SetGroupVIPRate(ctx context.Context, groupID int64, rate float64) error {
	_, err := r.sql.ExecContext(ctx, `INSERT INTO group_vip_rates (group_id, rate_multiplier, updated_at)
		VALUES ($1, $2, NOW()) ON CONFLICT (group_id) DO UPDATE SET rate_multiplier = EXCLUDED.rate_multiplier, updated_at = NOW()`, groupID, rate)
	return err
}

func (r *userGroupRateRepository) ClearGroupVIPRate(ctx context.Context, groupID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM group_vip_rates WHERE group_id = $1`, groupID)
	return err
}
