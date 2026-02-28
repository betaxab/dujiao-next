package public

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// AffiliateTrackClickRequest 推广点击记录请求
type AffiliateTrackClickRequest struct {
	AffiliateCode string `json:"affiliate_code" binding:"required"`
	VisitorKey    string `json:"visitor_key"`
	LandingPath   string `json:"landing_path"`
	Referrer      string `json:"referrer"`
}

// TrackAffiliateClick 记录推广点击
func (h *Handler) TrackAffiliateClick(c *gin.Context) {
	var req AffiliateTrackClickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if h.AffiliateService != nil {
		if err := h.AffiliateService.TrackClick(service.AffiliateTrackClickInput{
			AffiliateCode: req.AffiliateCode,
			VisitorKey:    req.VisitorKey,
			LandingPath:   req.LandingPath,
			Referrer:      req.Referrer,
			ClientIP:      c.ClientIP(),
			UserAgent:     c.GetHeader("User-Agent"),
		}); err != nil {
			respondError(c, response.CodeInternal, "error.save_failed", err)
			return
		}
	}
	response.Success(c, gin.H{"ok": true})
}

// OpenAffiliate 开通推广返利
func (h *Handler) OpenAffiliate(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	if h.AffiliateService == nil {
		respondError(c, response.CodeInternal, "error.save_failed", nil)
		return
	}

	profile, err := h.AffiliateService.OpenAffiliate(uid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAffiliateDisabled):
			respondError(c, response.CodeBadRequest, "error.forbidden", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}
	response.Success(c, profile)
}

// GetAffiliateDashboard 获取推广返利看板
func (h *Handler) GetAffiliateDashboard(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	if h.AffiliateService == nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	data, err := h.AffiliateService.GetUserDashboard(uid)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, data)
}

// ListAffiliateCommissions 查询我的推广佣金记录
func (h *Handler) ListAffiliateCommissions(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	if h.AffiliateService == nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)
	status := strings.TrimSpace(c.Query("status"))

	rows, total, err := h.AffiliateService.ListUserCommissions(uid, page, pageSize, status)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// ListAffiliateWithdraws 查询我的提现申请记录
func (h *Handler) ListAffiliateWithdraws(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	if h.AffiliateService == nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)
	status := strings.TrimSpace(c.Query("status"))

	rows, total, err := h.AffiliateService.ListUserWithdraws(uid, page, pageSize, status)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// AffiliateWithdrawApplyRequest 提现申请请求
type AffiliateWithdrawApplyRequest struct {
	Amount  string `json:"amount" binding:"required"`
	Channel string `json:"channel" binding:"required"`
	Account string `json:"account" binding:"required"`
}

// ApplyAffiliateWithdraw 提交提现申请
func (h *Handler) ApplyAffiliateWithdraw(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	if h.AffiliateService == nil {
		respondError(c, response.CodeInternal, "error.save_failed", nil)
		return
	}

	var req AffiliateWithdrawApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	row, err := h.AffiliateService.ApplyWithdraw(uid, service.AffiliateWithdrawApplyInput{
		Amount:  amount,
		Channel: req.Channel,
		Account: req.Account,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAffiliateDisabled):
			respondError(c, response.CodeBadRequest, "error.forbidden", nil)
		case errors.Is(err, service.ErrAffiliateNotOpened):
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		case errors.Is(err, service.ErrAffiliateWithdrawAmountInvalid):
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		case errors.Is(err, service.ErrAffiliateWithdrawChannelInvalid):
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		case errors.Is(err, service.ErrAffiliateWithdrawInsufficient):
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			respondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}
	response.Success(c, row)
}
