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
	defer func() { _ = db.Close() }()
	s := &H3Service{db: db}
	const userID int64 = 41
	const countQuery = `SELECT COUNT(*) FROM minimax_h3_tasks WHERE user_id = $1`
	const pageQuery = `SELECT id, generation, delivery, billing, (snapshot->'cost'->>'ActualCost')::float8, created_at
		FROM minimax_h3_tasks WHERE user_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	cost := 1.25

	mock.ExpectQuery(regexp.QuoteMeta(countQuery)).WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery(regexp.QuoteMeta(pageQuery)).WithArgs(userID, 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "generation", "delivery", "billing", "cost", "created_at"}).
			AddRow("h3-reconcile", "submission_unknown", "unverified", "pending", cost, createdAt))
	items, total, err := s.ListUserTasks(context.Background(), userID, "", 2, 20)
	require.NoError(t, err)
	require.Equal(t, int64(21), total)
	require.Equal(t, []H3HistoryItem{{
		ID: "h3-reconcile", Generation: "submission_unknown", Delivery: "unverified", Billing: "pending", Cost: &cost, CreatedAt: createdAt,
	}}, items)

	mock.ExpectQuery(regexp.QuoteMeta(countQuery)).WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	items, total, err = s.ListUserTasks(context.Background(), 42, "", 1, 20)
	require.NoError(t, err)
	require.NotNil(t, items)
	require.Empty(t, items)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestH3ListUserTasksGenerationFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	s := &H3Service{db: db}
	createdAt := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM minimax_h3_tasks WHERE user_id = $1 AND generation = $2`)).
		WithArgs(int64(7), "completed").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM minimax_h3_tasks WHERE user_id = $1 AND generation = $2
		ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`)).
		WithArgs(int64(7), "completed", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "generation", "delivery", "billing", "cost", "created_at"}).
			AddRow("h3-done", "completed", "verified", "settled", nil, createdAt))
	items, total, err := s.ListUserTasks(context.Background(), 7, "completed", 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []H3HistoryItem{{
		ID: "h3-done", Generation: "completed", Delivery: "verified", Billing: "settled", CreatedAt: createdAt,
	}}, items)
	require.NoError(t, mock.ExpectationsWereMet())
}
