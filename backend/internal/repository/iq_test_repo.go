package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// --- Bank Repository ---

type iqTestBankRepository struct {
	db *sql.DB
}

// NewIQTestBankRepository creates the IQ test bank repository.
func NewIQTestBankRepository(db *sql.DB) service.IQTestBankRepository {
	return &iqTestBankRepository{db: db}
}

func (r *iqTestBankRepository) CreateBank(ctx context.Context, bank *service.IQTestBank) (*service.IQTestBank, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO iq_test_banks (name, description, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, name, description, enabled, created_at, updated_at
	`, bank.Name, bank.Description, bank.Enabled)
	return scanIQTestBank(row)
}

func (r *iqTestBankRepository) GetBank(ctx context.Context, id int64) (*service.IQTestBank, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, enabled, created_at, updated_at
		FROM iq_test_banks WHERE id = $1
	`, id)
	return scanIQTestBank(row)
}

func (r *iqTestBankRepository) ListBanks(ctx context.Context) ([]*service.IQTestBank, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.name, b.description, b.enabled, b.created_at, b.updated_at,
		       (SELECT COUNT(*) FROM iq_test_questions q WHERE q.bank_id = b.id)
		FROM iq_test_banks b
		ORDER BY b.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	banks := []*service.IQTestBank{}
	for rows.Next() {
		b := &service.IQTestBank{}
		var questionCount int
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Enabled, &b.CreatedAt, &b.UpdatedAt, &questionCount); err != nil {
			return nil, err
		}
		banks = append(banks, b)
	}
	return banks, rows.Err()
}

func (r *iqTestBankRepository) UpdateBank(ctx context.Context, bank *service.IQTestBank) (*service.IQTestBank, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE iq_test_banks
		SET name = $2, description = $3, enabled = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, description, enabled, created_at, updated_at
	`, bank.ID, bank.Name, bank.Description, bank.Enabled)
	return scanIQTestBank(row)
}

func (r *iqTestBankRepository) DeleteBank(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM iq_test_banks WHERE id = $1`, id)
	return err
}

func (r *iqTestBankRepository) ReplaceQuestions(ctx context.Context, bankID int64, questions []*service.IQTestQuestion) ([]*service.IQTestQuestion, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM iq_test_questions WHERE bank_id = $1`, bankID); err != nil {
		return nil, err
	}

	saved := []*service.IQTestQuestion{}
	for i, q := range questions {
		options, err := json.Marshal(nonNilStrings(q.Options))
		if err != nil {
			return nil, err
		}
		keywords, err := json.Marshal(nonNilStrings(q.Keywords))
		if err != nil {
			return nil, err
		}
		row := tx.QueryRowContext(ctx, `
			INSERT INTO iq_test_questions
				(bank_id, position, type, prompt, options, answer, keywords, weight, max_tokens, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7::jsonb, $8, $9, NOW(), NOW())
			RETURNING id, bank_id, position, type, prompt, options, answer, keywords, weight, max_tokens
		`, bankID, i, q.Type, q.Prompt, string(options), q.Answer, string(keywords), q.Weight, q.MaxTokens)
		item, err := scanIQTestQuestion(row)
		if err != nil {
			return nil, err
		}
		saved = append(saved, item)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return saved, nil
}

func (r *iqTestBankRepository) GetQuestion(ctx context.Context, bankID, questionID int64) (*service.IQTestQuestion, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, bank_id, position, type, prompt, options, answer, keywords, weight, max_tokens
		FROM iq_test_questions WHERE bank_id = $1 AND id = $2
	`, bankID, questionID)
	return scanIQTestQuestion(row)
}

func (r *iqTestBankRepository) ListQuestions(ctx context.Context, bankID int64) ([]*service.IQTestQuestion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, bank_id, position, type, prompt, options, answer, keywords, weight, max_tokens
		FROM iq_test_questions WHERE bank_id = $1
		ORDER BY position ASC, id ASC
	`, bankID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := []*service.IQTestQuestion{}
	for rows.Next() {
		q, err := scanIQTestQuestion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, q)
	}
	return items, rows.Err()
}

// --- Plan Repository ---

type iqTestPlanRepository struct {
	db *sql.DB
}

// NewIQTestPlanRepository creates the IQ test plan repository.
func NewIQTestPlanRepository(db *sql.DB) service.IQTestPlanRepository {
	return &iqTestPlanRepository{db: db}
}

const iqTestPlanColumns = `id, bank_id, question_id, account_id, model_id, cron_expression, enabled, max_results, last_run_at, next_run_at, created_at, updated_at`

func (r *iqTestPlanRepository) Create(ctx context.Context, plan *service.IQTestPlan) (*service.IQTestPlan, error) {
	var questionID any
	if plan.QuestionID > 0 {
		questionID = plan.QuestionID
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO iq_test_plans (bank_id, question_id, account_id, model_id, cron_expression, enabled, max_results, next_run_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING `+iqTestPlanColumns,
		plan.BankID, questionID, plan.AccountID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.NextRunAt)
	return scanIQTestPlan(row)
}

func (r *iqTestPlanRepository) GetByID(ctx context.Context, id int64) (*service.IQTestPlan, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+iqTestPlanColumns+` FROM iq_test_plans WHERE id = $1`, id)
	return scanIQTestPlan(row)
}

func (r *iqTestPlanRepository) ListByAccountID(ctx context.Context, accountID int64) ([]*service.IQTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+iqTestPlanColumns+` FROM iq_test_plans WHERE account_id = $1 ORDER BY id DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanIQTestPlans(rows)
}

func (r *iqTestPlanRepository) ListByBankID(ctx context.Context, bankID int64) ([]*service.IQTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+iqTestPlanColumns+` FROM iq_test_plans WHERE bank_id = $1 ORDER BY id DESC`, bankID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanIQTestPlans(rows)
}

func (r *iqTestPlanRepository) ListDue(ctx context.Context, now time.Time) ([]*service.IQTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+iqTestPlanColumns+`
		FROM iq_test_plans
		WHERE enabled = true AND (next_run_at IS NULL OR next_run_at <= $1)
		ORDER BY next_run_at ASC NULLS FIRST
		LIMIT 200
	`, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanIQTestPlans(rows)
}

func (r *iqTestPlanRepository) Update(ctx context.Context, plan *service.IQTestPlan) (*service.IQTestPlan, error) {
	var questionID any
	if plan.QuestionID > 0 {
		questionID = plan.QuestionID
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE iq_test_plans
		SET bank_id = $2, question_id = $3, account_id = $4, model_id = $5, cron_expression = $6,
		    enabled = $7, max_results = $8, next_run_at = $9, updated_at = NOW()
		WHERE id = $1
		RETURNING `+iqTestPlanColumns,
		plan.ID, plan.BankID, questionID, plan.AccountID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.NextRunAt)
	return scanIQTestPlan(row)
}

func (r *iqTestPlanRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM iq_test_plans WHERE id = $1`, id)
	return err
}

func (r *iqTestPlanRepository) UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE iq_test_plans SET last_run_at = $2, next_run_at = $3, updated_at = NOW() WHERE id = $1
	`, id, lastRunAt, nextRunAt)
	return err
}

// SetQuestionID points a plan at a question (or clears it when questionID <= 0).
func (r *iqTestPlanRepository) SetQuestionID(ctx context.Context, planID, questionID int64) error {
	var value any
	if questionID > 0 {
		value = questionID
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE iq_test_plans SET question_id = $2, updated_at = NOW() WHERE id = $1
	`, planID, value)
	return err
}

// --- Run Repository ---

type iqTestRunRepository struct {
	db *sql.DB
}

// NewIQTestRunRepository creates the IQ test run repository.
func NewIQTestRunRepository(db *sql.DB) service.IQTestRunRepository {
	return &iqTestRunRepository{db: db}
}

func (r *iqTestRunRepository) Create(ctx context.Context, run *service.IQTestRun) (*service.IQTestRun, error) {
	details, err := json.Marshal(nonNilQuestionResults(run.Details))
	if err != nil {
		return nil, err
	}
	var planID any
	if run.PlanID > 0 {
		planID = run.PlanID
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO iq_test_runs
			(plan_id, bank_id, question_id, account_id, model_id, trigger, status, score, total, correct,
			 latency_ms, error_message, details, started_at, finished_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14, $15, NOW())
		RETURNING id, plan_id, bank_id, question_id, account_id, model_id, trigger, status, score, total, correct,
		          latency_ms, error_message, details, started_at, finished_at, created_at
	`, planID, run.BankID, run.QuestionID, run.AccountID, run.ModelID, run.Trigger, run.Status, run.Score, run.Total, run.Correct,
		run.LatencyMs, run.ErrorMessage, string(details), run.StartedAt, run.FinishedAt)
	return scanIQTestRun(row)
}

func (r *iqTestRunRepository) GetByID(ctx context.Context, id int64) (*service.IQTestRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, plan_id, bank_id, question_id, account_id, model_id, trigger, status, score, total, correct,
		       latency_ms, error_message, details, started_at, finished_at, created_at
		FROM iq_test_runs WHERE id = $1
	`, id)
	return scanIQTestRun(row)
}

func (r *iqTestRunRepository) List(ctx context.Context, filter service.IQTestRunFilter) ([]*service.IQTestRun, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	conditions := []string{}
	args := []any{}
	if filter.BankID > 0 {
		args = append(args, filter.BankID)
		conditions = append(conditions, fmt.Sprintf("bank_id = $%d", len(args)))
	}
	if filter.AccountID > 0 {
		args = append(args, filter.AccountID)
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", len(args)))
	}
	if filter.PlanID > 0 {
		args = append(args, filter.PlanID)
		conditions = append(conditions, fmt.Sprintf("plan_id = $%d", len(args)))
	}

	query := `
		SELECT id, plan_id, bank_id, question_id, account_id, model_id, trigger, status, score, total, correct,
		       latency_ms, error_message, details, started_at, finished_at, created_at
		FROM iq_test_runs`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runs := []*service.IQTestRun{}
	for rows.Next() {
		run, err := scanIQTestRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *iqTestRunRepository) PruneOldRuns(ctx context.Context, planID int64, keepCount int) error {
	if planID <= 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM iq_test_runs
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY plan_id ORDER BY created_at DESC) AS rn
				FROM iq_test_runs
				WHERE plan_id = $1
			) ranked
			WHERE rn > $2
		)
	`, planID, keepCount)
	return err
}

// --- scan helpers ---

func scanIQTestBank(row scannable) (*service.IQTestBank, error) {
	b := &service.IQTestBank{}
	if err := row.Scan(&b.ID, &b.Name, &b.Description, &b.Enabled, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return b, nil
}

func scanIQTestQuestion(row scannable) (*service.IQTestQuestion, error) {
	q := &service.IQTestQuestion{}
	var options, keywords []byte
	if err := row.Scan(&q.ID, &q.BankID, &q.Position, &q.Type, &q.Prompt, &options, &q.Answer, &keywords, &q.Weight, &q.MaxTokens); err != nil {
		return nil, err
	}
	q.Options = decodeStringList(options)
	q.Keywords = decodeStringList(keywords)
	return q, nil
}

func scanIQTestPlan(row scannable) (*service.IQTestPlan, error) {
	p := &service.IQTestPlan{}
	var questionID sql.NullInt64
	if err := row.Scan(&p.ID, &p.BankID, &questionID, &p.AccountID, &p.ModelID, &p.CronExpression, &p.Enabled,
		&p.MaxResults, &p.LastRunAt, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	if questionID.Valid {
		p.QuestionID = questionID.Int64
	}
	return p, nil
}

func scanIQTestPlans(rows *sql.Rows) ([]*service.IQTestPlan, error) {
	plans := []*service.IQTestPlan{}
	for rows.Next() {
		p, err := scanIQTestPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func scanIQTestRun(row scannable) (*service.IQTestRun, error) {
	run := &service.IQTestRun{}
	var planID sql.NullInt64
	var details []byte
	if err := row.Scan(&run.ID, &planID, &run.BankID, &run.QuestionID, &run.AccountID, &run.ModelID, &run.Trigger, &run.Status,
		&run.Score, &run.Total, &run.Correct, &run.LatencyMs, &run.ErrorMessage, &details,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt); err != nil {
		return nil, err
	}
	if planID.Valid {
		run.PlanID = planID.Int64
	}
	run.Details = decodeQuestionResults(details)
	return run, nil
}

func decodeStringList(raw []byte) []string {
	items := []string{}
	if len(raw) == 0 {
		return items
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return []string{}
	}
	if items == nil {
		return []string{}
	}
	return items
}

func decodeQuestionResults(raw []byte) []service.IQTestQuestionResult {
	items := []service.IQTestQuestionResult{}
	if len(raw) == 0 {
		return items
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return []service.IQTestQuestionResult{}
	}
	if items == nil {
		return []service.IQTestQuestionResult{}
	}
	return items
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func nonNilQuestionResults(in []service.IQTestQuestionResult) []service.IQTestQuestionResult {
	if in == nil {
		return []service.IQTestQuestionResult{}
	}
	return in
}
