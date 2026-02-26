package public

import (
	"errors"
	"strconv"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// CreatePaymentRequest 创建支付请求
type CreatePaymentRequest struct {
	OrderID    uint `json:"order_id" binding:"required"`
	ChannelID  uint `json:"channel_id"`
	UseBalance bool `json:"use_balance"`
}

// LatestPaymentQuery 查询最新待支付记录
type LatestPaymentQuery struct {
	OrderID uint `form:"order_id" binding:"required"`
}

// PaypalWebhookQuery PayPal webhook 查询参数。
type PaypalWebhookQuery struct {
	ChannelID uint `form:"channel_id" binding:"required"`
}

// WechatCallbackQuery 微信支付回调查询参数。
type WechatCallbackQuery struct {
	ChannelID uint `form:"channel_id"`
}

// StripeWebhookQuery Stripe webhook 查询参数。
type StripeWebhookQuery struct {
	ChannelID uint `form:"channel_id"`
}

const callbackLogValueLimit = 4096

// CreatePayment 创建支付单
func (h *Handler) CreatePayment(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var req CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if _, err := h.OrderService.GetOrderByUser(req.OrderID, uid); err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	result, err := h.PaymentService.CreatePayment(service.CreatePaymentInput{
		OrderID:    req.OrderID,
		ChannelID:  req.ChannelID,
		UseBalance: req.UseBalance,
		ClientIP:   c.ClientIP(),
		Context:    c.Request.Context(),
	})
	if err != nil {
		respondPaymentCreateError(c, err)
		return
	}

	resp := gin.H{
		"order_paid":         result.OrderPaid,
		"wallet_paid_amount": result.WalletPaidAmount,
		"online_pay_amount":  result.OnlinePayAmount,
	}
	if result.Payment != nil {
		resp["payment_id"] = result.Payment.ID
		resp["provider_type"] = result.Payment.ProviderType
		resp["channel_type"] = result.Payment.ChannelType
		resp["interaction_mode"] = result.Payment.InteractionMode
		resp["pay_url"] = result.Payment.PayURL
		resp["qr_code"] = result.Payment.QRCode
		resp["expires_at"] = result.Payment.ExpiredAt
	}
	response.Success(c, resp)
}

// CapturePayment 用户捕获支付。
func (h *Handler) CapturePayment(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	paymentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || paymentID == 0 {
		respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		return
	}
	payment, err := h.PaymentService.GetPayment(uint(paymentID))
	if err != nil {
		if errors.Is(err, service.ErrPaymentNotFound) {
			respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}
	if _, err := h.OrderService.GetOrderByUser(payment.OrderID, uid); err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}
	updated, err := h.PaymentService.CapturePayment(service.CapturePaymentInput{
		PaymentID: uint(paymentID),
		Context:   c.Request.Context(),
	})
	if err != nil {
		respondPaymentCaptureError(c, err)
		return
	}
	response.Success(c, gin.H{
		"payment_id": updated.ID,
		"status":     updated.Status,
	})
}

// GetLatestPayment 获取用户最新待支付记录
func (h *Handler) GetLatestPayment(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var query LatestPaymentQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	order, err := h.OrderService.GetOrderByUser(query.OrderID, uid)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	if order.ParentID != nil {
		respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		return
	}
	if order.Status != constants.OrderStatusPendingPayment {
		respondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		return
	}
	if order.ExpiresAt != nil && !order.ExpiresAt.After(time.Now()) {
		respondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		return
	}

	payment, err := h.PaymentRepo.GetLatestPendingByOrder(order.ID, time.Now())
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}
	if payment == nil {
		respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		return
	}

	response.Success(c, gin.H{
		"payment_id":       payment.ID,
		"order_id":         payment.OrderID,
		"channel_id":       payment.ChannelID,
		"provider_type":    payment.ProviderType,
		"channel_type":     payment.ChannelType,
		"interaction_mode": payment.InteractionMode,
		"pay_url":          payment.PayURL,
		"qr_code":          payment.QRCode,
		"expires_at":       payment.ExpiredAt,
	})
}

func respondPaymentCreateError(c *gin.Context, err error) {
	respondWithMappedError(c, err, paymentCreateErrorRules, response.CodeInternal, "error.payment_create_failed")
}

func respondPaymentCaptureError(c *gin.Context, err error) {
	respondWithMappedError(c, err, paymentCaptureErrorRules, response.CodeInternal, "error.payment_callback_failed")
}
