package admin

import (
	"encoding/json"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// ListGlobalPricing 返回管理页维护的全局模型统一价条目。
// 条目值与 LiteLLM 目录同构（per-token 价格），前端负责 $/MTok 展示换算。
// GET /api/v1/admin/model-pricing/global
func (h *ChannelHandler) ListGlobalPricing(c *gin.Context) {
	entries := h.pricingService.GlobalPricingEntries()
	if entries == nil {
		entries = map[string]json.RawMessage{}
	}
	response.Success(c, gin.H{
		"entries": entries,
		"file":    h.pricingService.GlobalPricingFilePath(),
	})
}

// SaveGlobalPricing 写入/更新单个模型的全局统一价。
// 请求体为价格 JSON 对象（per-token，字段名与 LiteLLM 目录一致）；字段值为 null 表示
// 从目录基线中删除该字段。模型名走 query 参数以免名称中的斜杠污染路径。
// PUT /api/v1/admin/model-pricing/global?model=xxx
func (h *ChannelHandler) SaveGlobalPricing(c *gin.Context) {
	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("MISSING_PARAMETER", "model parameter is required").
			WithMetadata(map[string]string{"param": "model"}))
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_BODY", "failed to read request body"))
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_BODY", "pricing body is required"))
		return
	}

	if err := h.pricingService.SaveGlobalPricingEntry(model, body); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("SAVE_FAILED", err.Error()).
			WithMetadata(map[string]string{"model": model}))
		return
	}
	response.Success(c, gin.H{"model": model})
}

// DeleteGlobalPricing 删除单个模型的全局统一价。
// DELETE /api/v1/admin/model-pricing/global?model=xxx
func (h *ChannelHandler) DeleteGlobalPricing(c *gin.Context) {
	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("MISSING_PARAMETER", "model parameter is required").
			WithMetadata(map[string]string{"param": "model"}))
		return
	}

	if err := h.pricingService.DeleteGlobalPricingEntry(model); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("DELETE_FAILED", err.Error()).
			WithMetadata(map[string]string{"model": model}))
		return
	}
	response.Success(c, gin.H{"model": model})
}

// ReloadGlobalPricing 手动重建价格表（供直接改文件后免重启生效）。
// POST /api/v1/admin/model-pricing/global/reload
func (h *ChannelHandler) ReloadGlobalPricing(c *gin.Context) {
	if err := h.pricingService.ReloadGlobalPricing(); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("RELOAD_FAILED", err.Error()))
		return
	}
	response.Success(c, gin.H{"reloaded": true})
}
