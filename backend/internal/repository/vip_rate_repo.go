package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var _ service.VIPRateStore = (*userGroupRateRepository)(nil)
var _ service.EffectiveUserGroupRateReader = (*userGroupRateRepository)(nil)

// GetEffectiveByUserID 读取用户端展示用的倍率覆盖值：用户专属 > VIP。
// 单次查询保持同一快照；NULL（例如仅设置 RPM）不覆盖 VIP，零倍率仍有效。
// 不填充分组默认倍率，以保留前端的默认/分时倍率展示行为。
func (r *userGroupRateRepository) GetEffectiveByUserID(ctx context.Context, userID int64) (map[int64]float64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT group_id, rate_multiplier
		FROM user_group_rate_multipliers
		WHERE user_id = $1 AND rate_multiplier IS NOT NULL
		UNION ALL
		SELECT gv.group_id, gv.rate_multiplier
		FROM group_vip_rates gv
		JOIN vip_users vu ON vu.user_id = $1
		WHERE NOT EXISTS (
			SELECT 1 FROM user_group_rate_multipliers ugr
			WHERE ugr.user_id = $1 AND ugr.group_id = gv.group_id
				AND ugr.rate_multiplier IS NOT NULL
		)`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	rates := make(map[int64]float64)
	for rows.Next() {
		var groupID int64
		var rate float64
		if err := rows.Scan(&groupID, &rate); err != nil {
			return nil, err
		}
		rates[groupID] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rates, nil
}

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
	r.invalidateVIPUsers(userIDs)
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
	r.invalidateVIPUsers(userIDs)
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
	if err != nil {
		return err
	}
	r.invalidateGroupVIPRate(groupID)
	return nil
}

func (r *userGroupRateRepository) ClearGroupVIPRate(ctx context.Context, groupID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM group_vip_rates WHERE group_id = $1`, groupID)
	if err != nil {
		return err
	}
	r.invalidateGroupVIPRate(groupID)
	return nil
}
