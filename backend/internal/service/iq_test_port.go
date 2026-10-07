package service

import (
	"context"
	"time"
)

// IQ test question types.
const (
	IQTestQuestionSingleChoice = "single_choice"
	IQTestQuestionOpen         = "open"
)

// IQ test run triggers / statuses.
const (
	IQTestTriggerManual    = "manual"
	IQTestTriggerScheduled = "scheduled"

	IQTestStatusSuccess = "success"
	IQTestStatusFailed  = "failed"
)

// IQTestBank is a named, reusable set of questions.
type IQTestBank struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Enabled     bool               `json:"enabled"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Questions   []*IQTestQuestion  `json:"questions,omitempty"`
}

// IQTestQuestion is a single graded item inside a bank.
type IQTestQuestion struct {
	ID       int64    `json:"id"`
	BankID   int64    `json:"bank_id"`
	Position int      `json:"position"`
	Type     string   `json:"type"`
	Prompt   string   `json:"prompt"`
	Options  []string `json:"options"`
	Answer   string   `json:"answer"`
	Keywords []string `json:"keywords"`
	Weight   int      `json:"weight"`
	// MaxTokens is currently advisory metadata kept for forward compatibility;
	// the account test pipeline uses its own fixed max_tokens.
	MaxTokens int `json:"max_tokens"`
}

// IQTestPlan schedules one question of a bank against an account on a cron
// expression. QuestionID is 0 when unset.
type IQTestPlan struct {
	ID             int64      `json:"id"`
	BankID         int64      `json:"bank_id"`
	QuestionID     int64      `json:"question_id"`
	AccountID      int64      `json:"account_id"`
	ModelID        string     `json:"model_id"`
	CronExpression string     `json:"cron_expression"`
	Enabled        bool       `json:"enabled"`
	MaxResults     int        `json:"max_results"`
	LastRunAt      *time.Time `json:"last_run_at"`
	NextRunAt      *time.Time `json:"next_run_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// IQTestQuestionResult is the per-question outcome of a run.
type IQTestQuestionResult struct {
	QuestionID   int64  `json:"question_id"`
	Type         string `json:"type"`
	Prompt       string `json:"prompt"`
	Response     string `json:"response"`
	Expected     string `json:"expected"`
	Correct      bool   `json:"correct"`
	Weight       int    `json:"weight"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// IQTestRun is one scored execution of a single question against an account.
type IQTestRun struct {
	ID           int64                   `json:"id"`
	PlanID       int64                   `json:"plan_id"`
	BankID       int64                   `json:"bank_id"`
	QuestionID   int64                   `json:"question_id"`
	AccountID    int64                   `json:"account_id"`
	ModelID      string                  `json:"model_id"`
	Trigger      string                  `json:"trigger"`
	Status       string                  `json:"status"`
	Score        float64                 `json:"score"`
	Total        int                     `json:"total"`
	Correct      int                     `json:"correct"`
	LatencyMs    int64                   `json:"latency_ms"`
	ErrorMessage string                  `json:"error_message"`
	Details      []IQTestQuestionResult  `json:"details"`
	StartedAt    time.Time               `json:"started_at"`
	FinishedAt   time.Time               `json:"finished_at"`
	CreatedAt    time.Time               `json:"created_at"`
}

// IQTestBankRepository is the data access interface for question banks.
type IQTestBankRepository interface {
	CreateBank(ctx context.Context, bank *IQTestBank) (*IQTestBank, error)
	GetBank(ctx context.Context, id int64) (*IQTestBank, error)
	ListBanks(ctx context.Context) ([]*IQTestBank, error)
	UpdateBank(ctx context.Context, bank *IQTestBank) (*IQTestBank, error)
	DeleteBank(ctx context.Context, id int64) error
	ReplaceQuestions(ctx context.Context, bankID int64, questions []*IQTestQuestion) ([]*IQTestQuestion, error)
	ListQuestions(ctx context.Context, bankID int64) ([]*IQTestQuestion, error)
	GetQuestion(ctx context.Context, bankID, questionID int64) (*IQTestQuestion, error)
}

// IQTestPlanRepository is the data access interface for scheduled plans.
type IQTestPlanRepository interface {
	Create(ctx context.Context, plan *IQTestPlan) (*IQTestPlan, error)
	GetByID(ctx context.Context, id int64) (*IQTestPlan, error)
	ListByAccountID(ctx context.Context, accountID int64) ([]*IQTestPlan, error)
	ListByBankID(ctx context.Context, bankID int64) ([]*IQTestPlan, error)
	ListDue(ctx context.Context, now time.Time) ([]*IQTestPlan, error)
	Update(ctx context.Context, plan *IQTestPlan) (*IQTestPlan, error)
	Delete(ctx context.Context, id int64) error
	UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error
	SetQuestionID(ctx context.Context, planID, questionID int64) error
}

// IQTestRunFilter narrows a run listing.
type IQTestRunFilter struct {
	BankID    int64
	AccountID int64
	PlanID    int64
	Limit     int
}

// IQTestRunRepository is the data access interface for run records.
type IQTestRunRepository interface {
	Create(ctx context.Context, run *IQTestRun) (*IQTestRun, error)
	GetByID(ctx context.Context, id int64) (*IQTestRun, error)
	List(ctx context.Context, filter IQTestRunFilter) ([]*IQTestRun, error)
	PruneOldRuns(ctx context.Context, planID int64, keepCount int) error
}
