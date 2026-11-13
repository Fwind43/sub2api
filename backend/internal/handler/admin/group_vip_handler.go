package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *GroupHandler) vipAdmin(c *gin.Context) (service.VIPAdmin, bool) {
	v, ok := h.adminService.(service.VIPAdmin)
	if !ok {
		response.InternalError(c, "VIP not supported")
		return nil, false
	}
	return v, true
}

// GetGroupVIPRate GET /api/v1/admin/groups/:id/vip-rate
func (h *GroupHandler) GetGroupVIPRate(c *gin.Context) {
	gid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	v, ok := h.vipAdmin(c)
	if !ok {
		return
	}
	r, err := v.GetGroupVIPRate(c.Request.Context(), gid)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"group_id": gid, "vip_rate_multiplier": r})
}

type setGroupVIPRateRequest struct {
	RateMultiplier *float64 `json:"rate_multiplier"`
}

// SetGroupVIPRate PUT /api/v1/admin/groups/:id/vip-rate ；rate_multiplier 为 null 表示清除
func (h *GroupHandler) SetGroupVIPRate(c *gin.Context) {
	gid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	var req setGroupVIPRateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	v, ok := h.vipAdmin(c)
	if !ok {
		return
	}
	if req.RateMultiplier == nil {
		err = v.ClearGroupVIPRate(c.Request.Context(), gid)
	} else {
		if *req.RateMultiplier < 0 {
			response.BadRequest(c, "rate_multiplier must be >= 0")
			return
		}
		err = v.SetGroupVIPRate(c.Request.Context(), gid, *req.RateMultiplier)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"group_id": gid, "vip_rate_multiplier": req.RateMultiplier})
}

// ListVIPUsers GET /api/v1/admin/vip-users
func (h *GroupHandler) ListVIPUsers(c *gin.Context) {
	v, ok := h.vipAdmin(c)
	if !ok {
		return
	}
	items, err := v.ListVIPUsers(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

type vipUsersRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
	Notes   string  `json:"notes"`
}

// AddVIPUsers POST /api/v1/admin/vip-users
func (h *GroupHandler) AddVIPUsers(c *gin.Context) {
	var req vipUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	v, ok := h.vipAdmin(c)
	if !ok {
		return
	}
	n, err := v.AddVIPUsers(c.Request.Context(), req.UserIDs, req.Notes)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"affected": n})
}

// RemoveVIPUsers DELETE /api/v1/admin/vip-users
func (h *GroupHandler) RemoveVIPUsers(c *gin.Context) {
	var req vipUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	v, ok := h.vipAdmin(c)
	if !ok {
		return
	}
	n, err := v.RemoveVIPUsers(c.Request.Context(), req.UserIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"affected": n})
}
