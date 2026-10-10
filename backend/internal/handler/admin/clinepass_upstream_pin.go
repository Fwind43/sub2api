package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetClinePassUpstreamPin returns the platform-level upstream pin applied to all
// clinepass accounts (null when unset).
func (h *ClinePassOAuthHandler) GetClinePassUpstreamPin(c *gin.Context) {
	if h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "setting service not available")
		return
	}
	cfg, err := h.settingService.GetClinePassUpstreamPin(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"config": cfg})
}

// UpdateClinePassUpstreamPin replaces the platform-level upstream pin.
func (h *ClinePassOAuthHandler) UpdateClinePassUpstreamPin(c *gin.Context) {
	if h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "setting service not available")
		return
	}
	var req service.ClinePassUpstreamPinConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid clinepass upstream pin payload")
		return
	}
	cfg, err := h.settingService.SetClinePassUpstreamPin(c.Request.Context(), &req)
	if err != nil {
		if !response.ErrorFrom(c, err) {
			response.Error(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	response.Success(c, gin.H{"config": cfg})
}
