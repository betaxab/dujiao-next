package admin

import (
	"errors"
	"strconv"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// CreatePaymentChannelRequest 创建支付渠道请求
type CreatePaymentChannelRequest struct {
	Name            string                 `json:"name" binding:"required"`
	ProviderType    string                 `json:"provider_type" binding:"required"`
	ChannelType     string                 `json:"channel_type" binding:"required"`
	InteractionMode string                 `json:"interaction_mode" binding:"required"`
	FeeRate         *models.Money          `json:"fee_rate"`
	ConfigJSON      map[string]interface{} `json:"config_json"`
	IsActive        *bool                  `json:"is_active"`
	SortOrder       int                    `json:"sort_order"`
}

// CreatePaymentChannel 创建支付渠道
func (h *Handler) CreatePaymentChannel(c *gin.Context) {
	var req CreatePaymentChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	channel := &models.PaymentChannel{
		Name:            req.Name,
		ProviderType:    req.ProviderType,
		ChannelType:     req.ChannelType,
		InteractionMode: req.InteractionMode,
		ConfigJSON:      models.JSON(req.ConfigJSON),
		SortOrder:       req.SortOrder,
		IsActive:        true,
	}
	if req.IsActive != nil {
		channel.IsActive = *req.IsActive
	}
	if req.FeeRate != nil {
		channel.FeeRate = *req.FeeRate
	}

	if err := h.PaymentService.ValidateChannel(channel); err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentProviderNotSupported):
			respondError(c, response.CodeBadRequest, "error.payment_provider_not_supported", nil)
		case errors.Is(err, service.ErrPaymentChannelConfigInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_channel_config_invalid", nil)
		default:
			respondError(c, response.CodeBadRequest, "error.payment_channel_invalid", nil)
		}
		return
	}

	if err := h.PaymentChannelRepo.Create(channel); err != nil {
		respondError(c, response.CodeInternal, "error.payment_channel_create_failed", err)
		return
	}
	_ = cache.Del(c.Request.Context(), publicConfigCacheKey)

	response.Success(c, channel)
}

// UpdatePaymentChannelRequest 更新支付渠道请求
type UpdatePaymentChannelRequest struct {
	Name            string                 `json:"name"`
	ProviderType    string                 `json:"provider_type"`
	ChannelType     string                 `json:"channel_type"`
	InteractionMode string                 `json:"interaction_mode"`
	FeeRate         *models.Money          `json:"fee_rate"`
	ConfigJSON      map[string]interface{} `json:"config_json"`
	IsActive        *bool                  `json:"is_active"`
	SortOrder       *int                   `json:"sort_order"`
}

// UpdatePaymentChannel 更新支付渠道
func (h *Handler) UpdatePaymentChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, response.CodeBadRequest, "error.payment_channel_invalid", nil)
		return
	}

	var req UpdatePaymentChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	channel, err := h.PaymentService.GetChannel(uint(id))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentChannelNotFound):
			respondError(c, response.CodeNotFound, "error.payment_channel_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_channel_update_failed", err)
		}
		return
	}

	if req.Name != "" {
		channel.Name = req.Name
	}
	if req.ProviderType != "" {
		channel.ProviderType = req.ProviderType
	}
	if req.ChannelType != "" {
		channel.ChannelType = req.ChannelType
	}
	if req.InteractionMode != "" {
		channel.InteractionMode = req.InteractionMode
	}
	if req.FeeRate != nil {
		channel.FeeRate = *req.FeeRate
	}
	if req.ConfigJSON != nil {
		channel.ConfigJSON = models.JSON(req.ConfigJSON)
	}
	if req.IsActive != nil {
		channel.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		channel.SortOrder = *req.SortOrder
	}

	if err := h.PaymentService.ValidateChannel(channel); err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentProviderNotSupported):
			respondError(c, response.CodeBadRequest, "error.payment_provider_not_supported", nil)
		case errors.Is(err, service.ErrPaymentChannelConfigInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_channel_config_invalid", nil)
		default:
			respondError(c, response.CodeBadRequest, "error.payment_channel_invalid", nil)
		}
		return
	}

	if err := h.PaymentChannelRepo.Update(channel); err != nil {
		respondError(c, response.CodeInternal, "error.payment_channel_update_failed", err)
		return
	}
	_ = cache.Del(c.Request.Context(), publicConfigCacheKey)

	response.Success(c, channel)
}

// DeletePaymentChannel 删除支付渠道
func (h *Handler) DeletePaymentChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, response.CodeBadRequest, "error.payment_channel_invalid", nil)
		return
	}

	if err := h.PaymentChannelRepo.Delete(uint(id)); err != nil {
		respondError(c, response.CodeInternal, "error.payment_channel_delete_failed", err)
		return
	}
	_ = cache.Del(c.Request.Context(), publicConfigCacheKey)

	response.Success(c, gin.H{"deleted": true})
}

// GetPaymentChannel 获取支付渠道详情
func (h *Handler) GetPaymentChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, response.CodeBadRequest, "error.payment_channel_invalid", nil)
		return
	}

	channel, err := h.PaymentService.GetChannel(uint(id))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentChannelNotFound):
			respondError(c, response.CodeNotFound, "error.payment_channel_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_channel_fetch_failed", err)
		}
		return
	}

	response.Success(c, channel)
}

// GetPaymentChannels 获取支付渠道列表
func (h *Handler) GetPaymentChannels(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	providerType := c.Query("provider_type")
	channelType := c.Query("channel_type")
	activeOnly := c.DefaultQuery("active_only", "")
	activeOnlyBool := false
	if activeOnly != "" {
		parsed, err := strconv.ParseBool(activeOnly)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		activeOnlyBool = parsed
	}

	channels, total, err := h.PaymentService.ListChannels(repository.PaymentChannelListFilter{
		Page:         page,
		PageSize:     pageSize,
		ProviderType: providerType,
		ChannelType:  channelType,
		ActiveOnly:   activeOnlyBool,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_channel_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, channels, pagination)
}
