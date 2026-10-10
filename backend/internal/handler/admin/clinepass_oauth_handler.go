package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ClinePassOAuthHandler exposes the ClinePass device-code login flow.
type ClinePassOAuthHandler struct {
	clinePassOAuthService *service.ClinePassOAuthService
	adminService          service.AdminService
	settingService        *service.SettingService
}

func NewClinePassOAuthHandler(
	clinePassOAuthService *service.ClinePassOAuthService,
	adminService service.AdminService,
	settingService *service.SettingService,
) *ClinePassOAuthHandler {
	return &ClinePassOAuthHandler{
		clinePassOAuthService: clinePassOAuthService,
		adminService:          adminService,
		settingService:        settingService,
	}
}

func (h *ClinePassOAuthHandler) GetCapabilities(c *gin.Context) {
	response.Success(c, h.clinePassOAuthService.GetCapabilities())
}

type clinePassDeviceStartRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

// StartDeviceAuth returns the user code the operator must approve.
func (h *ClinePassOAuthHandler) StartDeviceAuth(c *gin.Context) {
	var req clinePassDeviceStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = clinePassDeviceStartRequest{}
	}
	result, err := h.clinePassOAuthService.StartDeviceAuth(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type clinePassDevicePollRequest struct {
	DeviceCode string `json:"device_code" binding:"required"`
	ProxyID    *int64 `json:"proxy_id"`
}

// PollDeviceAuth performs one poll of the device challenge.
func (h *ClinePassOAuthHandler) PollDeviceAuth(c *gin.Context) {
	var req clinePassDevicePollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.clinePassOAuthService.PollDeviceAuth(c.Request.Context(), req.DeviceCode, req.ProxyID)
	if err != nil {
		if errors.Is(err, service.ClinePassDeviceAuthPending) {
			response.Success(c, gin.H{"status": "pending"})
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"status": "approved", "token_info": tokenInfo})
}

type clinePassCreateFromDeviceRequest struct {
	DeviceCode  string  `json:"device_code" binding:"required"`
	ProxyID     *int64  `json:"proxy_id"`
	Name        string  `json:"name"`
	Concurrency int     `json:"concurrency"`
	Priority    int     `json:"priority"`
	GroupIDs    []int64 `json:"group_ids"`
}

// CreateAccountFromDevice approver-side entry: it polls once and, when the
// operator has approved, creates the account in the same call.
func (h *ClinePassOAuthHandler) CreateAccountFromDevice(c *gin.Context) {
	var req clinePassCreateFromDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.clinePassOAuthService.PollDeviceAuth(c.Request.Context(), req.DeviceCode, req.ProxyID)
	if err != nil {
		if errors.Is(err, service.ClinePassDeviceAuthPending) {
			response.Success(c, gin.H{"status": "pending"})
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	credentials := h.clinePassOAuthService.BuildAccountCredentials(tokenInfo)
	name := strings.TrimSpace(req.Name)
	if name == "" && tokenInfo.Email != "" {
		name = tokenInfo.Email
	}
	if name == "" {
		name = "ClinePass OAuth Account"
	}
	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Platform:    service.PlatformClinePass,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		GroupIDs:    req.GroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"status": "approved", "account": dto.AccountFromService(account)})
}

type clinePassRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
	RT           string `json:"rt"`
	ProxyID      *int64 `json:"proxy_id"`
}

// RefreshToken exchanges a stored Cline refresh token for new credentials.
func (h *ClinePassOAuthHandler) RefreshToken(c *gin.Context) {
	var req clinePassRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(req.RT)
	}
	if refreshToken == "" {
		response.BadRequest(c, "refresh_token is required")
		return
	}
	tokenInfo, err := h.clinePassOAuthService.RefreshToken(c.Request.Context(), refreshToken, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

// RefreshAccountToken refreshes an existing ClinePass account in place.
func (h *ClinePassOAuthHandler) RefreshAccountToken(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account.Platform != service.PlatformClinePass {
		response.BadRequest(c, "Account platform does not match ClinePass OAuth endpoint")
		return
	}
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
	}
	tokenInfo, err := h.clinePassOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	newCredentials := h.clinePassOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = service.MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
	}
	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updatedAccount))
}

type clinePassModelsRequest struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ProxyID      *int64 `json:"proxy_id"`
}

// ListModels proxies the upstream model catalog using a supplied or resolved
// credential, so the admin UI can populate model selectors.
func (h *ClinePassOAuthHandler) ListModels(c *gin.Context) {
	var req clinePassModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = clinePassModelsRequest{}
	}
	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" && strings.TrimSpace(req.RefreshToken) != "" {
		tokenInfo, err := h.clinePassOAuthService.RefreshToken(c.Request.Context(), strings.TrimSpace(req.RefreshToken), req.ProxyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		accessToken = tokenInfo.AccessToken
	}
	models, err := h.clinePassOAuthService.FetchRecommendedModels(c.Request.Context(), accessToken, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"models": models})
}
