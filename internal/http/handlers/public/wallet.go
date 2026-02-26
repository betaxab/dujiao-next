package public

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// WalletRechargeRequest 用户充值请求
type WalletRechargeRequest struct {
	Amount    string `json:"amount" binding:"required"`
	ChannelID uint   `json:"channel_id" binding:"required"`
	Currency  string `json:"currency"`
	Remark    string `json:"remark"`
}

// GetMyWallet 获取当前用户钱包信息
func (h *Handler) GetMyWallet(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	account, err := h.WalletService.GetAccount(uid)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, account)
}

// GetMyWalletTransactions 获取当前用户钱包流水
func (h *Handler) GetMyWalletTransactions(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	transactions, total, err := h.WalletService.ListTransactions(repository.WalletTransactionListFilter{
		Page:     page,
		PageSize: pageSize,
		UserID:   uid,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, transactions, pagination)
}

// RechargeWallet 用户充值钱包余额
func (h *Handler) RechargeWallet(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	var req WalletRechargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" && h.SettingService != nil {
		siteCurrency, currencyErr := h.SettingService.GetSiteCurrency(constants.SiteCurrencyDefault)
		if currencyErr == nil {
			currency = siteCurrency
		}
	}
	result, err := h.PaymentService.CreateWalletRechargePayment(service.CreateWalletRechargePaymentInput{
		UserID:    uid,
		ChannelID: req.ChannelID,
		Amount:    models.NewMoneyFromDecimal(amount),
		Currency:  currency,
		Remark:    strings.TrimSpace(req.Remark),
		ClientIP:  c.ClientIP(),
		Context:   c.Request.Context(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWalletInvalidAmount):
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		case errors.Is(err, service.ErrWalletNotSupportedForGuest):
			respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		default:
			respondPaymentCreateError(c, err)
		}
		return
	}
	account, err := h.WalletService.GetAccount(uid)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, buildWalletRechargePaymentPayload(result.Recharge, result.Payment, account))
}

// GetMyWalletRecharge 获取当前用户充值单详情
func (h *Handler) GetMyWalletRecharge(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	rechargeNo := strings.TrimSpace(c.Param("recharge_no"))
	if rechargeNo == "" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	recharge, err := h.WalletService.GetRechargeOrderByRechargeNo(uid, rechargeNo)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWalletRechargeNotFound):
			respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		}
		return
	}
	payment, err := h.PaymentService.GetPayment(recharge.PaymentID)
	if err != nil {
		respondPaymentCaptureError(c, err)
		return
	}
	account, err := h.WalletService.GetAccount(uid)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, buildWalletRechargePaymentPayload(recharge, payment, account))
}

// CaptureMyWalletRechargePayment 主动检查当前用户充值支付状态
func (h *Handler) CaptureMyWalletRechargePayment(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	paymentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || paymentID == 0 {
		respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		return
	}
	recharge, err := h.WalletService.GetRechargeOrderByPaymentIDAndUser(uint(paymentID), uid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWalletRechargeNotFound):
			respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		}
		return
	}
	updatedPayment, err := h.PaymentService.CapturePayment(service.CapturePaymentInput{
		PaymentID: uint(paymentID),
		Context:   c.Request.Context(),
	})
	if err != nil {
		// 部分渠道不支持主动捕获时，回退为返回当前支付状态，避免前端“检查支付状态”直接报错。
		if !errors.Is(err, service.ErrPaymentProviderNotSupported) {
			respondPaymentCaptureError(c, err)
			return
		}
		updatedPayment, err = h.PaymentService.GetPayment(uint(paymentID))
		if err != nil {
			respondPaymentCaptureError(c, err)
			return
		}
	}
	updatedRecharge, err := h.WalletService.GetRechargeOrderByRechargeNo(uid, recharge.RechargeNo)
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}
	account, err := h.WalletService.GetAccount(uid)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, buildWalletRechargePaymentPayload(updatedRecharge, updatedPayment, account))
}

func buildWalletRechargePaymentPayload(recharge *models.WalletRechargeOrder, payment *models.Payment, account *models.WalletAccount) gin.H {
	payload := gin.H{
		"recharge": recharge,
		"payment":  payment,
	}
	if account != nil {
		payload["account"] = account
	}
	if payment != nil {
		payload["payment_id"] = payment.ID
		payload["provider_type"] = payment.ProviderType
		payload["channel_type"] = payment.ChannelType
		payload["interaction_mode"] = payment.InteractionMode
		payload["pay_url"] = payment.PayURL
		payload["qr_code"] = payment.QRCode
		payload["expires_at"] = payment.ExpiredAt
		payload["status"] = payment.Status
	}
	if recharge != nil {
		payload["recharge_no"] = recharge.RechargeNo
		payload["recharge_status"] = recharge.Status
	}
	return payload
}
