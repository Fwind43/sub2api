package admin

import (
	"strconv"
	"strings"

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
