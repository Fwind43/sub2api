package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// clinePassLastKnownRepository stores the most recent upstream provider that
// actually served a clinepass request, per (account, model).
type clinePassLastKnownRepository struct {
	db *sql.DB
}

// NewClinePassLastKnownRepository creates the clinepass last-known upstream repo.
func NewClinePassLastKnownRepository(db *sql.DB) service.ClinePassLastKnownStore {
	return &clinePassLastKnownRepository{db: db}
}

func clinePassFallbacksJSON(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func (r *clinePassLastKnownRepository) UpsertClinePassLastKnown(ctx context.Context, rec *service.ClinePassLastKnown) error {
	if rec == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO clinepass_upstream_lastknown
			(account_id, model, provider, pipeline, canonical, fallbacks, plan, observed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, NOW(), NOW())
		ON CONFLICT (account_id, model) DO UPDATE SET
			provider = EXCLUDED.provider,
			pipeline = EXCLUDED.pipeline,
			canonical = EXCLUDED.canonical,
			fallbacks = EXCLUDED.fallbacks,
			plan = EXCLUDED.plan,
			observed_at = EXCLUDED.observed_at,
			updated_at = NOW()
	`, rec.AccountID, rec.Model, rec.Provider, rec.Pipeline, rec.Canonical,
		clinePassFallbacksJSON(rec.Fallbacks), rec.Plan, rec.ObservedAt)
	return err
}

func (r *clinePassLastKnownRepository) ListClinePassLastKnown(ctx context.Context, accountID int64) ([]service.ClinePassLastKnown, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT account_id, model, provider, pipeline, canonical, fallbacks, plan, observed_at
		FROM clinepass_upstream_lastknown
		WHERE account_id = $1
		ORDER BY updated_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []service.ClinePassLastKnown{}
	for rows.Next() {
		rec, err := scanClinePassLastKnown(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

func (r *clinePassLastKnownRepository) GetClinePassLastKnown(ctx context.Context, accountID int64, model string) (*service.ClinePassLastKnown, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT account_id, model, provider, pipeline, canonical, fallbacks, plan, observed_at
		FROM clinepass_upstream_lastknown
		WHERE account_id = $1 AND model = $2
	`, accountID, model)
	rec, err := scanClinePassLastKnown(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

func scanClinePassLastKnown(scanner interface {
	Scan(dest ...any) error
}) (*service.ClinePassLastKnown, error) {
	var rec service.ClinePassLastKnown
	var fallbacks string
	if err := scanner.Scan(&rec.AccountID, &rec.Model, &rec.Provider, &rec.Pipeline,
		&rec.Canonical, &fallbacks, &rec.Plan, &rec.ObservedAt); err != nil {
		return nil, err
	}
	if fallbacks != "" {
		_ = json.Unmarshal([]byte(fallbacks), &rec.Fallbacks)
	}
	return &rec, nil
}
