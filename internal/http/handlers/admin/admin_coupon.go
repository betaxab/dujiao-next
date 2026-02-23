package admin

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// CreateCouponRequest 创建优惠券请求
type CreateCouponRequest struct {
	Code         string  `json:"code" binding:"required"`
	Type         string  `json:"type" binding:"required"`
	Value        float64 `json:"value" binding:"required"`
	MinAmount    float64 `json:"min_amount"`
	MaxDiscount  float64 `json:"max_discount"`
	UsageLimit   int     `json:"usage_limit"`
	PerUserLimit int     `json:"per_user_limit"`
	ScopeRefIDs  []uint  `json:"scope_ref_ids" binding:"required"`
	StartsAt     string  `json:"starts_at"`
	EndsAt       string  `json:"ends_at"`
	IsActive     *bool   `json:"is_active"`
}

// CreateCoupon 创建优惠券
func (h *Handler) CreateCoupon(c *gin.Context) {
	var req CreateCouponRequest
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

	coupon, err := h.CouponAdminService.Create(service.CreateCouponInput{
		Code:         req.Code,
		Type:         req.Type,
		Value:        models.NewMoneyFromDecimal(decimal.NewFromFloat(req.Value)),
		MinAmount:    models.NewMoneyFromDecimal(decimal.NewFromFloat(req.MinAmount)),
		MaxDiscount:  models.NewMoneyFromDecimal(decimal.NewFromFloat(req.MaxDiscount)),
		UsageLimit:   req.UsageLimit,
		PerUserLimit: req.PerUserLimit,
		ScopeRefIDs:  req.ScopeRefIDs,
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		IsActive:     req.IsActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCouponInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_invalid", nil)
		case errors.Is(err, service.ErrCouponScopeInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_scope_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.coupon_create_failed", err)
		}
		return
	}

	response.Success(c, coupon)
}

// UpdateCoupon 更新优惠券
func (h *Handler) UpdateCoupon(c *gin.Context) {
	couponID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || couponID == 0 {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	var req CreateCouponRequest
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

	coupon, err := h.CouponAdminService.Update(uint(couponID), service.UpdateCouponInput{
		Code:         req.Code,
		Type:         req.Type,
		Value:        models.NewMoneyFromDecimal(decimal.NewFromFloat(req.Value)),
		MinAmount:    models.NewMoneyFromDecimal(decimal.NewFromFloat(req.MinAmount)),
		MaxDiscount:  models.NewMoneyFromDecimal(decimal.NewFromFloat(req.MaxDiscount)),
		UsageLimit:   req.UsageLimit,
		PerUserLimit: req.PerUserLimit,
		ScopeRefIDs:  req.ScopeRefIDs,
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		IsActive:     req.IsActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCouponNotFound):
			respondError(c, response.CodeNotFound, "error.coupon_not_found", nil)
		case errors.Is(err, service.ErrCouponInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_invalid", nil)
		case errors.Is(err, service.ErrCouponScopeInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_scope_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.coupon_update_failed", err)
		}
		return
	}

	response.Success(c, coupon)
}

// DeleteCoupon 删除优惠券
func (h *Handler) DeleteCoupon(c *gin.Context) {
	couponID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || couponID == 0 {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if err := h.CouponAdminService.Delete(uint(couponID)); err != nil {
		switch {
		case errors.Is(err, service.ErrCouponNotFound):
			respondError(c, response.CodeNotFound, "error.coupon_not_found", nil)
		case errors.Is(err, service.ErrCouponInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.coupon_delete_failed", err)
		}
		return
	}
	response.Success(c, gin.H{
		"deleted": true,
	})
}

// GetAdminCoupons 获取优惠券列表
func (h *Handler) GetAdminCoupons(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	code := c.Query("code")
	var id uint
	if rawID := strings.TrimSpace(c.Query("id")); rawID != "" {
		parsed, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		id = uint(parsed)
	}
	var scopeRefID uint
	if rawScopeRefID := strings.TrimSpace(c.Query("scope_ref_id")); rawScopeRefID != "" {
		parsed, err := strconv.ParseUint(rawScopeRefID, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		scopeRefID = uint(parsed)
	}
	var isActive *bool
	if raw := c.Query("is_active"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		isActive = &parsed
	}

	coupons, total, err := h.CouponAdminService.List(repository.CouponListFilter{
		ID:         id,
		Code:       code,
		ScopeRefID: scopeRefID,
		IsActive:   isActive,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.coupon_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, coupons, pagination)
}

func parseTimeNullable(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
