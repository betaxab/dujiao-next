package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// CreatePromotionRequest 创建活动价请求
type CreatePromotionRequest struct {
	Name       string  `json:"name" binding:"required"`
	Type       string  `json:"type" binding:"required"`
	ScopeRefID uint    `json:"scope_ref_id" binding:"required"`
	Value      float64 `json:"value" binding:"required"`
	MinAmount  float64 `json:"min_amount"`
	StartsAt   string  `json:"starts_at"`
	EndsAt     string  `json:"ends_at"`
	IsActive   *bool   `json:"is_active"`
}

// CreatePromotion 创建活动价
func (h *Handler) CreatePromotion(c *gin.Context) {
	var req CreatePromotionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	startsAt, err := parseTimeNullable(req.StartsAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	endsAt, err := parseTimeNullable(req.EndsAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	promotion, err := h.PromotionAdminService.Create(service.CreatePromotionInput{
		Name:       req.Name,
		Type:       req.Type,
		ScopeRefID: req.ScopeRefID,
		Value:      models.NewMoneyFromDecimal(decimal.NewFromFloat(req.Value)),
		MinAmount:  models.NewMoneyFromDecimal(decimal.NewFromFloat(req.MinAmount)),
		StartsAt:   startsAt,
		EndsAt:     endsAt,
		IsActive:   req.IsActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPromotionInvalid):
			respondError(c, response.CodeBadRequest, "error.promotion_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.promotion_create_failed", err)
		}
		return
	}

	response.Success(c, promotion)
}

// UpdatePromotion 更新活动价
func (h *Handler) UpdatePromotion(c *gin.Context) {
	promotionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || promotionID == 0 {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	var req CreatePromotionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	startsAt, err := parseTimeNullable(req.StartsAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	endsAt, err := parseTimeNullable(req.EndsAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	promotion, err := h.PromotionAdminService.Update(uint(promotionID), service.UpdatePromotionInput{
		Name:       req.Name,
		Type:       req.Type,
		ScopeRefID: req.ScopeRefID,
		Value:      models.NewMoneyFromDecimal(decimal.NewFromFloat(req.Value)),
		MinAmount:  models.NewMoneyFromDecimal(decimal.NewFromFloat(req.MinAmount)),
		StartsAt:   startsAt,
		EndsAt:     endsAt,
		IsActive:   req.IsActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPromotionNotFound):
			respondError(c, response.CodeNotFound, "error.promotion_not_found", nil)
		case errors.Is(err, service.ErrPromotionInvalid):
			respondError(c, response.CodeBadRequest, "error.promotion_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.promotion_update_failed", err)
		}
		return
	}

	response.Success(c, promotion)
}

// DeletePromotion 删除活动价
func (h *Handler) DeletePromotion(c *gin.Context) {
	promotionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || promotionID == 0 {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if err := h.PromotionAdminService.Delete(uint(promotionID)); err != nil {
		switch {
		case errors.Is(err, service.ErrPromotionNotFound):
			respondError(c, response.CodeNotFound, "error.promotion_not_found", nil)
		case errors.Is(err, service.ErrPromotionInvalid):
			respondError(c, response.CodeBadRequest, "error.promotion_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.promotion_delete_failed", err)
		}
		return
	}
	response.Success(c, gin.H{
		"deleted": true,
	})
}

// GetAdminPromotions 获取活动价列表
func (h *Handler) GetAdminPromotions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	var id uint
	if rawID := strings.TrimSpace(c.Query("id")); rawID != "" {
		parsed, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		id = uint(parsed)
	}

	scopeRefID, _ := strconv.ParseUint(c.Query("scope_ref_id"), 10, 64)

	var isActive *bool
	if raw := c.Query("is_active"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		isActive = &parsed
	}

	promotions, total, err := h.PromotionAdminService.List(repository.PromotionListFilter{
		ID:         id,
		ScopeRefID: uint(scopeRefID),
		IsActive:   isActive,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.promotion_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, promotions, pagination)
}
