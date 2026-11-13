package service

import (
	"context"
	"errors"
	"time"
)

// VIPUser 是 VIP 名单条目。
type VIPUser struct {
	UserID    int64     `json:"user_id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

// VIPRateStore 由仓储层实现，管理 VIP 名单与分组 VIP 倍率。
type VIPRateStore interface {
	ListVIPUsers(ctx context.Context) ([]VIPUser, error)
	AddVIPUsers(ctx context.Context, userIDs []int64, notes string) (int, error)
	RemoveVIPUsers(ctx context.Context, userIDs []int64) (int, error)
	GetGroupVIPRate(ctx context.Context, groupID int64) (*float64, error)
	SetGroupVIPRate(ctx context.Context, groupID int64, rate float64) error
	ClearGroupVIPRate(ctx context.Context, groupID int64) error
}

// VIPAdmin 由 adminServiceImpl 实现，handler 通过可选断言使用，避免扩大 AdminService 接口。
type VIPAdmin = VIPRateStore

var ErrVIPUnsupported = errors.New("vip rate store not available")

func (s *adminServiceImpl) vipStore() (VIPRateStore, error) {
	if st, ok := s.userGroupRateRepo.(VIPRateStore); ok && st != nil {
		return st, nil
	}
	return nil, ErrVIPUnsupported
}

func (s *adminServiceImpl) ListVIPUsers(ctx context.Context) ([]VIPUser, error) {
	st, err := s.vipStore()
	if err != nil {
		return nil, err
	}
	return st.ListVIPUsers(ctx)
}

func (s *adminServiceImpl) AddVIPUsers(ctx context.Context, ids []int64, notes string) (int, error) {
	st, err := s.vipStore()
	if err != nil {
		return 0, err
	}
	return st.AddVIPUsers(ctx, ids, notes)
}

func (s *adminServiceImpl) RemoveVIPUsers(ctx context.Context, ids []int64) (int, error) {
	st, err := s.vipStore()
	if err != nil {
		return 0, err
	}
	return st.RemoveVIPUsers(ctx, ids)
}

func (s *adminServiceImpl) GetGroupVIPRate(ctx context.Context, gid int64) (*float64, error) {
	st, err := s.vipStore()
	if err != nil {
		return nil, err
	}
	return st.GetGroupVIPRate(ctx, gid)
}

func (s *adminServiceImpl) SetGroupVIPRate(ctx context.Context, gid int64, rate float64) error {
	st, err := s.vipStore()
	if err != nil {
		return err
	}
	return st.SetGroupVIPRate(ctx, gid, rate)
}

func (s *adminServiceImpl) ClearGroupVIPRate(ctx context.Context, gid int64) error {
	st, err := s.vipStore()
	if err != nil {
		return err
	}
	return st.ClearGroupVIPRate(ctx, gid)
}
