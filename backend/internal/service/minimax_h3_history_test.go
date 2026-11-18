package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestH3ListUserTasks(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &H3Service{db: db}
	const userID int64 = 41
	const countQuery = `SELECT COUNT(*) FROM minimax_h3_tasks WHERE user_id = $1`
	const pageQuery = `SELECT id, generation, delivery, billing, created_at
		FROM minimax_h3_tasks WHERE user_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(countQuery)).WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery(regexp.QuoteMeta(pageQuery)).WithArgs(userID, 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "generation", "delivery", "billing", "created_at"}).
			AddRow("h3-reconcile", "submission_unknown", "unverified", "pending", createdAt))
	items, total, err := s.ListUserTasks(context.Background(), userID, 2, 20)
	require.NoError(t, err)
	require.Equal(t, int64(21), total)
	require.Equal(t, []H3HistoryItem{{
		ID: "h3-reconcile", Generation: "submission_unknown", Delivery: "unverified", Billing: "pending", CreatedAt: createdAt,
	}}, items)

	mock.ExpectQuery(regexp.QuoteMeta(countQuery)).WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	items, total, err = s.ListUserTasks(context.Background(), 42, 1, 20)
	require.NoError(t, err)
	require.NotNil(t, items)
	require.Empty(t, items)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
