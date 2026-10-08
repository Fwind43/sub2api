package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type clinePassUpstreamProbeRequest struct {
	Model string `json:"model"`
}

// ProbeClinePassUpstreams harvests the upstream provider slugs available for
// one model on a clinepass account, so the admin UI can offer real choices
// for the upstream pin instead of free-text guesses.
// POST /api/v1/admin/accounts/:id/clinepass-upstreams/probe
func (h *AccountHandler) ProbeClinePassUpstreams(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.accountTestService == nil {
		response.BadRequest(c, "Account test service unavailable")
		return
	}
	var req clinePassUpstreamProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil && !strings.Contains(err.Error(), "EOF") {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.accountTestService.ProbeClinePassUpstreams(c.Request.Context(), account, req.Model)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}

// clinePassLastKnownItem is one persisted "actual upstream" observation.
type clinePassLastKnownItem struct {
	Model      string   `json:"model"`
	Provider   string   `json:"provider"`
	Pipeline   string   `json:"pipeline"`
	Canonical  string   `json:"canonical_slug,omitempty"`
	Fallbacks  []string `json:"fallbacks,omitempty"`
	Plan       string   `json:"plan,omitempty"`
	ObservedAt string   `json:"observed_at"`
}

type clinePassLastKnownResponse struct {
	AccountID int64                    `json:"account_id"`
	Items     []clinePassLastKnownItem `json:"items"`
}

// GetClinePassLastKnown returns the most recent actually-hit upstream provider
// per model for one clinepass account, so the admin UI can compare the pin
// expectation with reality.
// GET /api/v1/admin/accounts/:id/clinepass-lastknown
func (h *AccountHandler) GetClinePassLastKnown(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.clinePassLastKnown == nil {
		response.BadRequest(c, "ClinePass last-known store unavailable")
		return
	}
	records, err := h.clinePassLastKnown.ListClinePassLastKnown(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	resp := clinePassLastKnownResponse{AccountID: accountID, Items: make([]clinePassLastKnownItem, 0, len(records))}
	for _, rec := range records {
		resp.Items = append(resp.Items, clinePassLastKnownItem{
			Model:      rec.Model,
			Provider:   rec.Provider,
			Pipeline:   rec.Pipeline,
			Canonical:  rec.Canonical,
			Fallbacks:  rec.Fallbacks,
			Plan:       rec.Plan,
			ObservedAt: rec.ObservedAt.UTC().Format(time.RFC3339),
		})
	}
	response.Success(c, resp)
}
