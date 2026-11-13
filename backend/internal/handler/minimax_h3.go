package handler

import (
	"errors"
	"io"
	"net/http"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) MiniMaxH3Create(c *gin.Context) {
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, 401, "authentication_error", "Invalid API key")
		return
	}
	if h.h3 == nil {
		h.errorResponse(c, 503, "api_error", "H3 unavailable")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, 401, "authentication_error", "User context missing")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		h.errorResponse(c, 400, "invalid_request_error", "Invalid or oversized H3 JSON")
		return
	}
	if _, err = service.ParseH3Request(body); err != nil {
		h.errorResponse(c, 400, "invalid_request_error", err.Error())
		return
	}
	if key.Group == nil ||
		!compositeTargetPlatformAllowed(c, key, service.H3Model, service.PlatformMiniMax) ||
		effectiveAPIKeyPlatform(c, key) != service.PlatformMiniMax {
		h.errorResponse(c, 400, "invalid_request_error", "MiniMax target group required")
		return
	}
	if !service.GroupAllowsImageGeneration(key.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	reqLog := requestLogger(c, "handler.minimax_h3")
	decision := h.checkSecurityAudit(c, reqLog, key, subject, service.ContentModerationProtocolOpenAIImages, service.H3Model, body)
	if decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}
	sub, _ := middleware.GetSubscriptionFromContext(c)
	stream := false
	release, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &stream, requestLogger(c, "handler.minimax_h3"))
	if !acquired {
		return
	}
	if release != nil {
		defer release()
	}
	if err = h.billingCacheService.CheckBillingEligibility(c.Request.Context(), key.User, key, key.Group, sub, "minimax"); err != nil {
		status, code, message, _ := billingErrorDetails(err)
		h.errorResponse(c, status, code, message)
		return
	}
	task, err := h.h3.Create(c.Request.Context(), key, sub, body, c.GetHeader("Idempotency-Key"))
	if errors.Is(err, service.ErrH3Conflict) {
		h.errorResponse(c, 409, "idempotency_conflict", err.Error())
		return
	}
	if err != nil {
		h.errorResponse(c, 502, "h3_error", "H3 submission failed; retry only with the same Idempotency-Key")
		return
	}
	c.Header("Location", "/v1/minimax/videos/"+task.ID)
	c.JSON(http.StatusAccepted, task)
}
func (h *OpenAIGatewayHandler) MiniMaxH3Status(c *gin.Context) {
	task, ok := h.h3OwnedTask(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, task)
}
func (h *OpenAIGatewayHandler) MiniMaxH3Content(c *gin.Context) {
	task, ok := h.h3OwnedTask(c)
	if !ok {
		return
	}
	if task.Delivery != "verified" || task.Billing != "settled" {
		c.Header("Retry-After", "20")
		h.errorResponse(c, 409, "video_not_ready", "Video has not been verified and settled")
		return
	}
	data, err := h.h3.Content(c.Request.Context(), task)
	if err != nil {
		h.errorResponse(c, 502, "h3_delivery_error", "Video download unavailable; task settlement is unchanged")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", "attachment; filename=\""+task.ID+".mp4\"")
	c.Data(http.StatusOK, "video/mp4", data)
}
func (h *OpenAIGatewayHandler) h3OwnedTask(c *gin.Context) (*service.H3Task, bool) {
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, 401, "authentication_error", "Invalid API key")
		return nil, false
	}
	if h.h3 == nil {
		h.errorResponse(c, 503, "api_error", "H3 unavailable")
		return nil, false
	}
	task, err := h.h3.Get(c.Request.Context(), c.Param("request_id"), key)
	if errors.Is(err, service.ErrH3NotFound) {
		h.errorResponse(c, 404, "not_found_error", "H3 task not found")
		return nil, false
	}
	if err != nil {
		h.errorResponse(c, 503, "api_error", "H3 storage unavailable")
		return nil, false
	}
	return task, true
}
