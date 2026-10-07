package service

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// IQTestService owns question-bank CRUD, scheduled plans and run execution.
type IQTestService struct {
	bankRepo       IQTestBankRepository
	planRepo       IQTestPlanRepository
	runRepo        IQTestRunRepository
	accountTestSvc *AccountTestService
}

// NewIQTestService creates a new IQTestService.
func NewIQTestService(
	bankRepo IQTestBankRepository,
	planRepo IQTestPlanRepository,
	runRepo IQTestRunRepository,
	accountTestSvc *AccountTestService,
) *IQTestService {
	return &IQTestService{
		bankRepo:       bankRepo,
		planRepo:       planRepo,
		runRepo:        runRepo,
		accountTestSvc: accountTestSvc,
	}
}

// --- Bank CRUD ---

// CreateBank validates and persists a bank together with its questions.
func (s *IQTestService) CreateBank(ctx context.Context, bank *IQTestBank, questions []*IQTestQuestion) (*IQTestBank, error) {
	if strings.TrimSpace(bank.Name) == "" {
		return nil, fmt.Errorf("bank name is required")
	}
	normalizeQuestions(questions)

	created, err := s.bankRepo.CreateBank(ctx, bank)
	if err != nil {
		return nil, err
	}
	if len(questions) > 0 {
		saved, err := s.bankRepo.ReplaceQuestions(ctx, created.ID, questions)
		if err != nil {
			return nil, err
		}
		created.Questions = saved
	}
	return created, nil
}

// GetBank returns a bank with its questions loaded.
func (s *IQTestService) GetBank(ctx context.Context, id int64) (*IQTestBank, error) {
	bank, err := s.bankRepo.GetBank(ctx, id)
	if err != nil {
		return nil, err
	}
	questions, err := s.bankRepo.ListQuestions(ctx, id)
	if err != nil {
		return nil, err
	}
	bank.Questions = questions
	return bank, nil
}

// ListBanks returns all banks (without questions).
func (s *IQTestService) ListBanks(ctx context.Context) ([]*IQTestBank, error) {
	return s.bankRepo.ListBanks(ctx)
}

// UpdateBank updates bank metadata; questions are only replaced when provided.
func (s *IQTestService) UpdateBank(ctx context.Context, bank *IQTestBank, questions []*IQTestQuestion, replaceQuestions bool) (*IQTestBank, error) {
	if strings.TrimSpace(bank.Name) == "" {
		return nil, fmt.Errorf("bank name is required")
	}
	updated, err := s.bankRepo.UpdateBank(ctx, bank)
	if err != nil {
		return nil, err
	}
	if replaceQuestions {
		saved, err := s.replaceQuestions(ctx, updated.ID, questions)
		if err != nil {
			return nil, err
		}
		updated.Questions = saved
	}
	return updated, nil
}

// SaveQuestions replaces all questions of a bank. Plan assignments are kept
// stable by position where possible.
func (s *IQTestService) SaveQuestions(ctx context.Context, bankID int64, questions []*IQTestQuestion) ([]*IQTestQuestion, error) {
	if _, err := s.bankRepo.GetBank(ctx, bankID); err != nil {
		return nil, err
	}
	return s.replaceQuestions(ctx, bankID, questions)
}

// replaceQuestions swaps the whole question list of a bank and remaps plan
// assignments by position, so a plan that referenced question N keeps
// referencing the new question at position N when it still exists.
func (s *IQTestService) replaceQuestions(ctx context.Context, bankID int64, questions []*IQTestQuestion) ([]*IQTestQuestion, error) {
	plans, err := s.planRepo.ListByBankID(ctx, bankID)
	if err != nil {
		return nil, err
	}
	oldQuestions, err := s.bankRepo.ListQuestions(ctx, bankID)
	if err != nil {
		return nil, err
	}
	normalizeQuestions(questions)
	saved, err := s.bankRepo.ReplaceQuestions(ctx, bankID, questions)
	if err != nil {
		return nil, err
	}
	s.remapPlanQuestionsByPosition(ctx, plans, oldQuestions, saved)
	return saved, nil
}

func (s *IQTestService) remapPlanQuestionsByPosition(ctx context.Context, plans []*IQTestPlan, oldQuestions, saved []*IQTestQuestion) {
	if len(plans) == 0 {
		return
	}
	positionByQuestionID := make(map[int64]int, len(oldQuestions))
	for _, q := range oldQuestions {
		positionByQuestionID[q.ID] = q.Position
	}
	for _, plan := range plans {
		if plan.QuestionID <= 0 {
			continue
		}
		position, ok := positionByQuestionID[plan.QuestionID]
		if !ok || position < 0 || position >= len(saved) {
			_ = s.planRepo.SetQuestionID(ctx, plan.ID, 0)
			continue
		}
		_ = s.planRepo.SetQuestionID(ctx, plan.ID, saved[position].ID)
	}
}

// DeleteBank removes a bank (questions/plans/runs cascade).
func (s *IQTestService) DeleteBank(ctx context.Context, id int64) error {
	return s.bankRepo.DeleteBank(ctx, id)
}

// --- Plan CRUD ---

// CreatePlan validates the cron expression and persists a plan.
func (s *IQTestService) CreatePlan(ctx context.Context, plan *IQTestPlan) (*IQTestPlan, error) {
	if plan.BankID <= 0 {
		return nil, fmt.Errorf("bank_id is required")
	}
	if plan.QuestionID <= 0 {
		return nil, fmt.Errorf("question_id is required")
	}
	if plan.AccountID <= 0 {
		return nil, fmt.Errorf("account_id is required")
	}
	if _, err := s.bankRepo.GetQuestion(ctx, plan.BankID, plan.QuestionID); err != nil {
		return nil, fmt.Errorf("question %d not found in bank %d", plan.QuestionID, plan.BankID)
	}
	if strings.TrimSpace(plan.CronExpression) == "" {
		plan.CronExpression = "0 3 * * *"
	}
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun
	if plan.MaxResults <= 0 {
		plan.MaxResults = 50
	}
	return s.planRepo.Create(ctx, plan)
}

// GetPlan retrieves a plan by ID.
func (s *IQTestService) GetPlan(ctx context.Context, id int64) (*IQTestPlan, error) {
	return s.planRepo.GetByID(ctx, id)
}

// ListPlansByAccount returns all plans of an account.
func (s *IQTestService) ListPlansByAccount(ctx context.Context, accountID int64) ([]*IQTestPlan, error) {
	return s.planRepo.ListByAccountID(ctx, accountID)
}

// ListPlansByBank returns all plans using a bank.
func (s *IQTestService) ListPlansByBank(ctx context.Context, bankID int64) ([]*IQTestPlan, error) {
	return s.planRepo.ListByBankID(ctx, bankID)
}

// UpdatePlan validates the cron expression and updates a plan.
func (s *IQTestService) UpdatePlan(ctx context.Context, plan *IQTestPlan) (*IQTestPlan, error) {
	if plan.QuestionID > 0 && plan.BankID > 0 {
		if _, err := s.bankRepo.GetQuestion(ctx, plan.BankID, plan.QuestionID); err != nil {
			return nil, fmt.Errorf("question %d not found in bank %d", plan.QuestionID, plan.BankID)
		}
	}
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun
	if plan.MaxResults <= 0 {
		plan.MaxResults = 50
	}
	return s.planRepo.Update(ctx, plan)
}

// DeletePlan removes a plan (its runs cascade).
func (s *IQTestService) DeletePlan(ctx context.Context, id int64) error {
	return s.planRepo.Delete(ctx, id)
}

// --- Runs ---

// ListRuns returns recent runs matching the filter.
func (s *IQTestService) ListRuns(ctx context.Context, filter IQTestRunFilter) ([]*IQTestRun, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	return s.runRepo.List(ctx, filter)
}

// GetRun returns a single run with its per-question details.
func (s *IQTestService) GetRun(ctx context.Context, id int64) (*IQTestRun, error) {
	return s.runRepo.GetByID(ctx, id)
}

// RunQuestion executes a single question of a bank against an account and
// records the scored run. trigger is "manual" or "scheduled". planID may be 0.
//
// The run scores 100 when the reply is graded correct and 0 otherwise. A
// failed model call is recorded with status "failed" and the underlying error.
func (s *IQTestService) RunQuestion(ctx context.Context, bankID, questionID, accountID int64, modelID, trigger string, planID int64) (*IQTestRun, error) {
	if bankID <= 0 {
		return nil, fmt.Errorf("bank_id is required")
	}
	if questionID <= 0 {
		return nil, fmt.Errorf("question_id is required")
	}
	question, err := s.bankRepo.GetQuestion(ctx, bankID, questionID)
	if err != nil {
		return nil, fmt.Errorf("question not found: %w", err)
	}
	if s.accountTestSvc == nil {
		return nil, fmt.Errorf("account test service unavailable")
	}

	weight := question.Weight
	if weight <= 0 {
		weight = 1
	}

	startedAt := time.Now()
	response, _, qErr := s.accountTestSvc.RunQuestionBackground(ctx, accountID, modelID, question.Prompt)
	item := IQTestQuestionResult{
		QuestionID: question.ID,
		Type:       question.Type,
		Prompt:     question.Prompt,
		Response:   response,
		Expected:   question.Answer,
		Weight:     weight,
	}
	run := &IQTestRun{
		PlanID:     planID,
		BankID:     bankID,
		QuestionID: question.ID,
		AccountID:  accountID,
		ModelID:    modelID,
		Trigger:    trigger,
		Status:     IQTestStatusSuccess,
		Total:      1,
		StartedAt:  startedAt,
	}
	if qErr != nil {
		item.ErrorMessage = qErr.Error()
		run.Status = IQTestStatusFailed
		run.ErrorMessage = qErr.Error()
	} else if GradeQuestion(question, response) {
		item.Correct = true
		run.Correct = 1
		run.Score = 100
	}
	run.Details = []IQTestQuestionResult{item}
	run.LatencyMs = time.Since(startedAt).Milliseconds()
	run.FinishedAt = time.Now()

	saved, err := s.runRepo.Create(ctx, run)
	if err != nil {
		return nil, err
	}
	if planID > 0 {
		_ = s.runRepo.PruneOldRuns(ctx, planID, 50)
	}
	return saved, nil
}

// --- grading ---

var iqChoiceLetterPattern = regexp.MustCompile(`(?i)^[\s\p{Pd}]*(?:\()?([a-h])[\).:\s]`)

// GradeQuestion grades a single answer. Exposed for unit testing.
func GradeQuestion(q *IQTestQuestion, response string) bool {
	answer := strings.TrimSpace(q.Answer)
	trimmed := strings.TrimSpace(response)
	if answer == "" {
		// No expected answer configured: treat any non-empty reply as a pass.
		return trimmed != ""
	}
	if q.Type == IQTestQuestionSingleChoice {
		return gradeSingleChoice(answer, trimmed, q.Options)
	}
	return gradeOpen(answer, q.Keywords, trimmed)
}

func gradeSingleChoice(answer, response string, options []string) bool {
	expectedLetter := firstChoiceLetter(answer)
	if expectedLetter != "" {
		if got := firstChoiceLetter(response); got == expectedLetter {
			return true
		}
		// Some models restate the option text instead of the letter.
		if idx := choiceIndex(expectedLetter); idx >= 0 && idx < len(options) {
			option := strings.TrimSpace(stripOptionPrefix(options[idx]))
			if option != "" && strings.Contains(strings.ToLower(response), strings.ToLower(option)) {
				return true
			}
		}
		return false
	}
	// Answer is free text: fall back to containment.
	return strings.Contains(strings.ToLower(response), strings.ToLower(answer))
}

func gradeOpen(answer string, keywords []string, response string) bool {
	lowered := strings.ToLower(response)
	keys := keywords
	if len(keys) == 0 {
		keys = []string{answer}
	}
	matched := 0
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if strings.Contains(lowered, k) {
			matched++
		}
	}
	if len(keys) == 0 {
		return strings.TrimSpace(response) != ""
	}
	// Pass when at least half of the keywords are present (min 1).
	need := int(math.Ceil(float64(len(keys)) / 2))
	if need < 1 {
		need = 1
	}
	return matched >= need
}

func firstChoiceLetter(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := iqChoiceLetterPattern.FindStringSubmatch(s); len(m) == 2 {
		return strings.ToUpper(m[1])
	}
	// Bare letter answer, e.g. "B".
	if len(s) == 1 && strings.ContainsAny(s, "abcdefghABCDEFGH") {
		return strings.ToUpper(s)
	}
	return ""
}

func choiceIndex(letter string) int {
	if letter == "" {
		return -1
	}
	return int(letter[0]) - int('A')
}

func stripOptionPrefix(option string) string {
	return strings.TrimSpace(iqOptionPrefixPattern.ReplaceAllString(option, ""))
}

var iqOptionPrefixPattern = regexp.MustCompile(`(?i)^\s*\(?[a-h][\).:]\s*`)

func normalizeQuestions(questions []*IQTestQuestion) {
	for _, q := range questions {
		if q.Type == "" {
			q.Type = IQTestQuestionSingleChoice
		}
		if q.Weight <= 0 {
			q.Weight = 1
		}
		if q.Options == nil {
			q.Options = []string{}
		}
		if q.Keywords == nil {
			q.Keywords = []string{}
		}
	}
}
