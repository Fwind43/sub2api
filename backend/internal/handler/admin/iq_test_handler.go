package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// IQTestHandler handles admin IQ-test bank/plan/run management.
type IQTestHandler struct {
	iqTestSvc *service.IQTestService
}

// NewIQTestHandler creates a new IQTestHandler.
func NewIQTestHandler(iqTestSvc *service.IQTestService) *IQTestHandler {
	return &IQTestHandler{iqTestSvc: iqTestSvc}
}

type iqTestQuestionRequest struct {
	Type     string   `json:"type"`
	Prompt   string   `json:"prompt"`
	Options  []string `json:"options"`
	Answer   string   `json:"answer"`
	Keywords []string `json:"keywords"`
	Weight   int      `json:"weight"`
}

type createIQTestBankRequest struct {
	Name        string                   `json:"name" binding:"required"`
	Description string                   `json:"description"`
	Enabled     *bool                    `json:"enabled"`
	Questions   []iqTestQuestionRequest  `json:"questions"`
}

type updateIQTestBankRequest struct {
	Name             string                  `json:"name"`
	Description      string                  `json:"description"`
	Enabled          *bool                   `json:"enabled"`
	Questions        []iqTestQuestionRequest `json:"questions"`
	ReplaceQuestions bool                    `json:"replace_questions"`
}

type createIQTestPlanRequest struct {
	BankID          int64  `json:"bank_id" binding:"required"`
	QuestionID      int64  `json:"question_id" binding:"required"`
	AccountID       int64  `json:"account_id" binding:"required"`
	ModelID         string `json:"model_id"`
	CronExpression  string `json:"cron_expression"`
	Enabled         *bool  `json:"enabled"`
	MaxResults      int    `json:"max_results"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type updateIQTestPlanRequest struct {
	QuestionID      *int64  `json:"question_id"`
	ModelID         string  `json:"model_id"`
	CronExpression  string  `json:"cron_expression"`
	Enabled         *bool   `json:"enabled"`
	MaxResults      int     `json:"max_results"`
	ReasoningEffort *string `json:"reasoning_effort"`
}

type runIQTestRequest struct {
	BankID          int64  `json:"bank_id" binding:"required"`
	QuestionID      int64  `json:"question_id" binding:"required"`
	AccountID       int64  `json:"account_id" binding:"required"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort"`
}

func toIQTestQuestions(reqs []iqTestQuestionRequest) []*service.IQTestQuestion {
	questions := make([]*service.IQTestQuestion, 0, len(reqs))
	for _, r := range reqs {
		questions = append(questions, &service.IQTestQuestion{
			Type:     r.Type,
			Prompt:   r.Prompt,
			Options:  r.Options,
			Answer:   r.Answer,
			Keywords: r.Keywords,
			Weight:   r.Weight,
		})
	}
	return questions
}

// ListBanks GET /admin/iq-test/banks
func (h *IQTestHandler) ListBanks(c *gin.Context) {
	banks, err := h.iqTestSvc.ListBanks(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, banks)
}

// GetBank GET /admin/iq-test/banks/:id
func (h *IQTestHandler) GetBank(c *gin.Context) {
	bankID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid bank id")
		return
	}
	bank, err := h.iqTestSvc.GetBank(c.Request.Context(), bankID)
	if err != nil {
		response.NotFound(c, "bank not found")
		return
	}
	c.JSON(http.StatusOK, bank)
}

// CreateBank POST /admin/iq-test/banks
func (h *IQTestHandler) CreateBank(c *gin.Context) {
	var req createIQTestBankRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	bank := &service.IQTestBank{Name: req.Name, Description: req.Description, Enabled: true}
	if req.Enabled != nil {
		bank.Enabled = *req.Enabled
	}
	created, err := h.iqTestSvc.CreateBank(c.Request.Context(), bank, toIQTestQuestions(req.Questions))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, created)
}

// UpdateBank PUT /admin/iq-test/banks/:id
func (h *IQTestHandler) UpdateBank(c *gin.Context) {
	bankID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid bank id")
		return
	}
	existing, err := h.iqTestSvc.GetBank(c.Request.Context(), bankID)
	if err != nil {
		response.NotFound(c, "bank not found")
		return
	}

	var req updateIQTestBankRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	existing.Description = req.Description
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	updated, err := h.iqTestSvc.UpdateBank(c.Request.Context(), existing, toIQTestQuestions(req.Questions), req.ReplaceQuestions)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DeleteBank DELETE /admin/iq-test/banks/:id
func (h *IQTestHandler) DeleteBank(c *gin.Context) {
	bankID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid bank id")
		return
	}
	if err := h.iqTestSvc.DeleteBank(c.Request.Context(), bankID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListPlansByAccount GET /admin/accounts/:id/iq-test-plans
func (h *IQTestHandler) ListPlansByAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}
	plans, err := h.iqTestSvc.ListPlansByAccount(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, plans)
}

// ListPlansByBank GET /admin/iq-test/plans
func (h *IQTestHandler) ListPlansByBank(c *gin.Context) {
	bankID, err := strconv.ParseInt(c.Query("bank_id"), 10, 64)
	if err != nil || bankID <= 0 {
		response.BadRequest(c, "bank_id is required")
		return
	}
	plans, err := h.iqTestSvc.ListPlansByBank(c.Request.Context(), bankID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, plans)
}

// CreatePlan POST /admin/iq-test/plans
func (h *IQTestHandler) CreatePlan(c *gin.Context) {
	var req createIQTestPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	plan := &service.IQTestPlan{
		BankID:         req.BankID,
		QuestionID:     req.QuestionID,
		AccountID:      req.AccountID,
		ModelID:        req.ModelID,
		CronExpression:  req.CronExpression,
		Enabled:         true,
		MaxResults:      req.MaxResults,
		ReasoningEffort: req.ReasoningEffort,
	}
	if req.Enabled != nil {
		plan.Enabled = *req.Enabled
	}
	created, err := h.iqTestSvc.CreatePlan(c.Request.Context(), plan)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, created)
}

// UpdatePlan PUT /admin/iq-test/plans/:id
func (h *IQTestHandler) UpdatePlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plan id")
		return
	}
	existing, err := h.iqTestSvc.GetPlan(c.Request.Context(), planID)
	if err != nil {
		response.NotFound(c, "plan not found")
		return
	}

	var req updateIQTestPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.QuestionID != nil && *req.QuestionID > 0 {
		existing.QuestionID = *req.QuestionID
	}
	if req.ModelID != "" {
		existing.ModelID = req.ModelID
	}
	if req.CronExpression != "" {
		existing.CronExpression = req.CronExpression
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.MaxResults > 0 {
		existing.MaxResults = req.MaxResults
	}
	if req.ReasoningEffort != nil {
		existing.ReasoningEffort = *req.ReasoningEffort
	}

	updated, err := h.iqTestSvc.UpdatePlan(c.Request.Context(), existing)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DeletePlan DELETE /admin/iq-test/plans/:id
func (h *IQTestHandler) DeletePlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plan id")
		return
	}
	if err := h.iqTestSvc.DeletePlan(c.Request.Context(), planID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// Run POST /admin/iq-test/runs
func (h *IQTestHandler) Run(c *gin.Context) {
	var req runIQTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	run, err := h.iqTestSvc.RunQuestion(c.Request.Context(), req.BankID, req.QuestionID, req.AccountID, req.ModelID, service.IQTestTriggerManual, 0, req.ReasoningEffort)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, run)
}

// ListRuns GET /admin/iq-test/runs
func (h *IQTestHandler) ListRuns(c *gin.Context) {
	filter := service.IQTestRunFilter{}
	if v, err := strconv.ParseInt(c.Query("bank_id"), 10, 64); err == nil {
		filter.BankID = v
	}
	if v, err := strconv.ParseInt(c.Query("account_id"), 10, 64); err == nil {
		filter.AccountID = v
	}
	if v, err := strconv.ParseInt(c.Query("plan_id"), 10, 64); err == nil {
		filter.PlanID = v
	}
	if v, err := strconv.Atoi(c.Query("limit")); err == nil {
		filter.Limit = v
	}

	runs, err := h.iqTestSvc.ListRuns(c.Request.Context(), filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, runs)
}

// GetRun GET /admin/iq-test/runs/:id
func (h *IQTestHandler) GetRun(c *gin.Context) {
	runID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid run id")
		return
	}
	run, err := h.iqTestSvc.GetRun(c.Request.Context(), runID)
	if err != nil {
		response.NotFound(c, "run not found")
		return
	}
	c.JSON(http.StatusOK, run)
}

// RunPlanNow POST /admin/iq-test/plans/:id/run
func (h *IQTestHandler) RunPlanNow(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plan id")
		return
	}
	plan, err := h.iqTestSvc.GetPlan(c.Request.Context(), planID)
	if err != nil {
		response.NotFound(c, "plan not found")
		return
	}
	run, err := h.iqTestSvc.RunQuestion(c.Request.Context(), plan.BankID, plan.QuestionID, plan.AccountID, plan.ModelID, service.IQTestTriggerManual, plan.ID, plan.ReasoningEffort)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, run)
}

// RunForAccount POST /admin/accounts/:id/iq-test/run
func (h *IQTestHandler) RunForAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}
	var req struct {
		BankID          int64  `json:"bank_id" binding:"required"`
		QuestionID      int64  `json:"question_id" binding:"required"`
		ModelID         string `json:"model_id"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	run, err := h.iqTestSvc.RunQuestion(c.Request.Context(), req.BankID, req.QuestionID, accountID, req.ModelID, service.IQTestTriggerManual, 0, req.ReasoningEffort)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, run)
}

// ListRunsByAccount GET /admin/accounts/:id/iq-test/runs
func (h *IQTestHandler) ListRunsByAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}
	limit := 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = v
	}
	runs, err := h.iqTestSvc.ListRuns(c.Request.Context(), service.IQTestRunFilter{AccountID: accountID, Limit: limit})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, runs)
}

// ListRunsByPlan GET /admin/iq-test/plans/:id/runs
func (h *IQTestHandler) ListRunsByPlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plan id")
		return
	}
	limit := 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = v
	}
	runs, err := h.iqTestSvc.ListRuns(c.Request.Context(), service.IQTestRunFilter{PlanID: planID, Limit: limit})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, runs)
}
