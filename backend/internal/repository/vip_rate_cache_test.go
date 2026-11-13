package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestVIPWritesInvalidateRateCacheAfterSuccess(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		query     string
		userScope bool
		write     func(*userGroupRateRepository) error
	}{
		{"add", "INSERT INTO vip_users", true, func(r *userGroupRateRepository) error {
			_, err := r.AddVIPUsers(ctx, []int64{11, 12}, "test")
			return err
		}},
		{"remove", "DELETE FROM vip_users", true, func(r *userGroupRateRepository) error {
			_, err := r.RemoveVIPUsers(ctx, []int64{11, 12})
			return err
		}},
		{"set", "INSERT INTO group_vip_rates", false, func(r *userGroupRateRepository) error {
			return r.SetGroupVIPRate(ctx, 21, 0.5)
		}},
		{"clear", "DELETE FROM group_vip_rates", false, func(r *userGroupRateRepository) error {
			return r.ClearGroupVIPRate(ctx, 21)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, result := range []string{"success", "no_rows", "failure"} {
				t.Run(result, func(t *testing.T) {
					db, mock, err := sqlmock.New()
					require.NoError(t, err)
					t.Cleanup(func() { _ = db.Close() })
					repo := &userGroupRateRepository{sql: db}
					expect := mock.ExpectExec(tc.query)
					writeErr := errors.New("write failed")
					if result == "failure" {
						expect.WillReturnError(writeErr)
					} else {
						rows := int64(1)
						if result == "no_rows" {
							rows = 0
						}
						expect.WillReturnResult(sqlmock.NewResult(0, rows))
					}
					err = tc.write(repo)
					if result == "failure" {
						require.ErrorIs(t, err, writeErr)
						require.Zero(t, repo.UserGroupRateCacheVersion(11, 21))
					} else {
						require.NoError(t, err)
						version := repo.UserGroupRateCacheVersion(11, 21)
						require.NotZero(t, version)
						if tc.userScope {
							require.Equal(t, version, repo.UserGroupRateCacheVersion(11, 22))
							require.Equal(t, version, repo.UserGroupRateCacheVersion(12, 22))
							require.Zero(t, repo.UserGroupRateCacheVersion(13, 21))
						} else {
							require.Equal(t, version, repo.UserGroupRateCacheVersion(13, 21))
							require.Zero(t, repo.UserGroupRateCacheVersion(11, 22))
						}
					}
					require.NoError(t, mock.ExpectationsWereMet())
				})
			}
		})
	}
}

func TestVIPRateCacheVersionMonotonicAcrossUserAndGroupWrites(t *testing.T) {
	repo := &userGroupRateRepository{}
	previous := uint64(0)
	for _, invalidate := range []func(){
		func() { repo.invalidateVIPUsers([]int64{11}) },
		func() { repo.invalidateGroupVIPRate(21) },
		func() { repo.invalidateVIPUsers([]int64{11}) },
		func() { repo.invalidateGroupVIPRate(21) },
	} {
		invalidate()
		version := repo.UserGroupRateCacheVersion(11, 21)
		require.Greater(t, version, previous)
		require.Zero(t, repo.UserGroupRateCacheVersion(12, 22))
		previous = version
	}
}

func TestEmptyVIPUserWritesDoNotInvalidate(t *testing.T) {
	repo := &userGroupRateRepository{}
	n, err := repo.AddVIPUsers(context.Background(), nil, "")
	require.NoError(t, err)
	require.Zero(t, n)
	n, err = repo.RemoveVIPUsers(context.Background(), nil)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Zero(t, repo.rateVersion)
}
