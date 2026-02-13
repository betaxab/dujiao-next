package public

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/alipay"
	"github.com/dujiao-next/internal/payment/epay"
	"github.com/dujiao-next/internal/payment/epusdt"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// CreatePaymentRequest 创建支付请求
type CreatePaymentRequest struct {
	OrderID   uint `json:"order_id" binding:"required"`
	ChannelID uint `json:"channel_id" binding:"required"`
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
		OrderID:   req.OrderID,
		ChannelID: req.ChannelID,
		ClientIP:  c.ClientIP(),
		Context:   c.Request.Context(),
	})
	if err != nil {
		respondPaymentCreateError(c, err)
		return
	}

	response.Success(c, gin.H{
		"payment_id":       result.Payment.ID,
		"provider_type":    result.Payment.ProviderType,
		"channel_type":     result.Payment.ChannelType,
		"interaction_mode": result.Payment.InteractionMode,
		"pay_url":          result.Payment.PayURL,
		"qr_code":          result.Payment.QRCode,
		"expires_at":       result.Payment.ExpiredAt,
		"provider_payload": result.Payment.ProviderPayload,
	})
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
		"provider_payload": payment.ProviderPayload,
	})
}

// PaymentCallbackRequest 支付回调请求
type PaymentCallbackRequest struct {
	PaymentID   uint        `json:"payment_id" form:"payment_id"`
	OrderNo     string      `json:"order_no" form:"order_no"`
	ChannelID   uint        `json:"channel_id" form:"channel_id"`
	Status      string      `json:"status" form:"status"`
	ProviderRef string      `json:"provider_ref" form:"provider_ref"`
	Amount      string      `json:"amount" form:"amount"`
	Currency    string      `json:"currency" form:"currency"`
	PaidAt      string      `json:"paid_at" form:"paid_at"`
	Payload     models.JSON `json:"payload"`
}

// PaymentCallback 支付回调
func (h *Handler) PaymentCallback(c *gin.Context) {
	if handled := h.HandleWechatCallback(c); handled {
		return
	}
	if handled := h.HandleAlipayCallback(c); handled {
		return
	}
	if handled := h.HandleEpayCallback(c); handled {
		return
	}
	if handled := h.HandleEpusdtCallback(c); handled {
		return
	}
	var req PaymentCallbackRequest
	if err := c.ShouldBind(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	var paidAt *time.Time
	if req.PaidAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.PaidAt)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		paidAt = &parsed
	}

	amount := models.Money{}
	if req.Amount != "" {
		parsed, err := decimal.NewFromString(req.Amount)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		amount = models.NewMoneyFromDecimal(parsed)
	}

	payment, err := h.PaymentService.HandleCallback(service.PaymentCallbackInput{
		PaymentID:   req.PaymentID,
		OrderNo:     req.OrderNo,
		ChannelID:   req.ChannelID,
		Status:      req.Status,
		ProviderRef: req.ProviderRef,
		Amount:      amount,
		Currency:    req.Currency,
		PaidAt:      paidAt,
		Payload:     req.Payload,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		case errors.Is(err, service.ErrPaymentNotFound):
			respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		case errors.Is(err, service.ErrPaymentStatusInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_status_invalid", nil)
		case errors.Is(err, service.ErrPaymentAmountMismatch):
			respondError(c, response.CodeBadRequest, "error.payment_amount_mismatch", nil)
		case errors.Is(err, service.ErrPaymentCurrencyMismatch):
			respondError(c, response.CodeBadRequest, "error.payment_currency_mismatch", nil)
		case errors.Is(err, service.ErrOrderNotFound):
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_callback_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"updated":    true,
		"payment_id": payment.ID,
		"status":     payment.Status,
	})
}

// PaypalWebhook PayPal webhook 回调。
func (h *Handler) PaypalWebhook(c *gin.Context) {
	var query PaypalWebhookQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) == 0 {
			continue
		}
		headers[key] = values[0]
	}
	payment, eventType, err := h.PaymentService.HandlePaypalWebhook(service.WebhookCallbackInput{
		ChannelID: query.ChannelID,
		Headers:   headers,
		Body:      body,
		Context:   c.Request.Context(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		case errors.Is(err, service.ErrPaymentNotFound):
			respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		case errors.Is(err, service.ErrPaymentStatusInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_status_invalid", nil)
		case errors.Is(err, service.ErrPaymentAmountMismatch):
			respondError(c, response.CodeBadRequest, "error.payment_amount_mismatch", nil)
		case errors.Is(err, service.ErrPaymentCurrencyMismatch):
			respondError(c, response.CodeBadRequest, "error.payment_currency_mismatch", nil)
		case errors.Is(err, service.ErrPaymentChannelNotFound):
			respondError(c, response.CodeNotFound, "error.payment_channel_not_found", nil)
		case errors.Is(err, service.ErrPaymentProviderNotSupported):
			respondError(c, response.CodeBadRequest, "error.payment_provider_not_supported", nil)
		case errors.Is(err, service.ErrPaymentChannelConfigInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_channel_config_invalid", nil)
		case errors.Is(err, service.ErrPaymentGatewayRequestFailed):
			respondError(c, response.CodeBadRequest, "error.payment_gateway_request_failed", nil)
		case errors.Is(err, service.ErrPaymentGatewayResponseInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_gateway_response_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_callback_failed", err)
		}
		return
	}

	if payment == nil {
		response.Success(c, gin.H{
			"accepted":   true,
			"event_type": eventType,
			"updated":    false,
		})
		return
	}

	response.Success(c, gin.H{
		"accepted":   true,
		"event_type": eventType,
		"updated":    true,
		"payment_id": payment.ID,
		"status":     payment.Status,
	})
}

// StripeWebhook Stripe webhook 回调。
func (h *Handler) StripeWebhook(c *gin.Context) {
	var query StripeWebhookQuery
	_ = c.ShouldBindQuery(&query)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) == 0 {
			continue
		}
		headers[key] = values[0]
	}

	payment, eventType, err := h.PaymentService.HandleStripeWebhook(service.WebhookCallbackInput{
		ChannelID: query.ChannelID,
		Headers:   headers,
		Body:      body,
		Context:   c.Request.Context(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		case errors.Is(err, service.ErrPaymentNotFound):
			respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		case errors.Is(err, service.ErrPaymentStatusInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_status_invalid", nil)
		case errors.Is(err, service.ErrPaymentAmountMismatch):
			respondError(c, response.CodeBadRequest, "error.payment_amount_mismatch", nil)
		case errors.Is(err, service.ErrPaymentCurrencyMismatch):
			respondError(c, response.CodeBadRequest, "error.payment_currency_mismatch", nil)
		case errors.Is(err, service.ErrPaymentChannelNotFound):
			respondError(c, response.CodeNotFound, "error.payment_channel_not_found", nil)
		case errors.Is(err, service.ErrPaymentProviderNotSupported):
			respondError(c, response.CodeBadRequest, "error.payment_provider_not_supported", nil)
		case errors.Is(err, service.ErrPaymentChannelConfigInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_channel_config_invalid", nil)
		case errors.Is(err, service.ErrPaymentGatewayRequestFailed):
			respondError(c, response.CodeBadRequest, "error.payment_gateway_request_failed", nil)
		case errors.Is(err, service.ErrPaymentGatewayResponseInvalid):
			respondError(c, response.CodeBadRequest, "error.payment_gateway_response_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_callback_failed", err)
		}
		return
	}

	if payment == nil {
		response.Success(c, gin.H{
			"accepted":   true,
			"event_type": eventType,
			"updated":    false,
		})
		return
	}

	response.Success(c, gin.H{
		"accepted":   true,
		"event_type": eventType,
		"updated":    true,
		"payment_id": payment.ID,
		"status":     payment.Status,
	})
}

func (h *Handler) HandleWechatCallback(c *gin.Context) bool {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	if !isWechatCallbackRequest(c, body) {
		return false
	}

	var query WechatCallbackQuery
	_ = c.ShouldBindQuery(&query)

	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) == 0 {
			continue
		}
		headers[key] = values[0]
	}

	payment, _, err := h.PaymentService.HandleWechatWebhook(service.WebhookCallbackInput{
		ChannelID: query.ChannelID,
		Headers:   headers,
		Body:      body,
		Context:   c.Request.Context(),
	})
	if err != nil {
		respondWechatCallback(c, false)
		return true
	}
	if payment == nil {
		respondWechatCallback(c, true)
		return true
	}
	respondWechatCallback(c, true)
	return true
}

func isWechatCallbackRequest(c *gin.Context, body []byte) bool {
	if strings.TrimSpace(c.GetHeader("Wechatpay-Signature")) == "" {
		return false
	}
	if strings.TrimSpace(c.GetHeader("Wechatpay-Timestamp")) == "" {
		return false
	}
	if strings.TrimSpace(c.GetHeader("Wechatpay-Nonce")) == "" {
		return false
	}
	if strings.TrimSpace(c.GetHeader("Wechatpay-Serial")) == "" {
		return false
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	resourceRaw, ok := payload["resource"]
	if !ok {
		return false
	}
	_, ok = resourceRaw.(map[string]interface{})
	return ok
}

func respondWechatCallback(c *gin.Context, success bool) {
	if success {
		c.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"message": "成功",
		})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"code":    "FAIL",
		"message": "失败",
	})
}

func parseCallbackForm(c *gin.Context) (map[string][]string, error) {
	if err := c.Request.ParseForm(); err != nil {
		return nil, err
	}
	if len(c.Request.PostForm) > 0 {
		return c.Request.PostForm, nil
	}
	return c.Request.Form, nil
}

func (h *Handler) HandleAlipayCallback(c *gin.Context) bool {
	form, err := parseCallbackForm(c)
	if err != nil {
		return false
	}
	if !isAlipayCallbackForm(form) {
		return false
	}

	payment, channel, err := h.findAlipayCallbackPayment(form)
	if err != nil || payment == nil || channel == nil {
		c.String(200, constants.AlipayCallbackFail)
		return true
	}

	cfg, err := alipay.ParseConfig(channel.ConfigJSON)
	if err != nil {
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	if err := alipay.VerifyCallback(cfg, form); err != nil {
		c.String(200, constants.AlipayCallbackFail)
		return true
	}

	input, err := parseAlipayCallback(form, payment.ID)
	if err != nil {
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	input.ChannelID = channel.ID
	if _, err := h.PaymentService.HandleCallback(*input); err != nil {
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	c.String(200, constants.AlipayCallbackSuccess)
	return true
}

func isAlipayCallbackForm(form map[string][]string) bool {
	if strings.TrimSpace(getFirstValue(form, "sign")) == "" {
		return false
	}
	hasNotifyField := strings.TrimSpace(getFirstValue(form, "notify_id")) != "" ||
		strings.TrimSpace(getFirstValue(form, "notify_type")) != "" ||
		strings.TrimSpace(getFirstValue(form, "buyer_id")) != ""
	if !hasNotifyField {
		return false
	}
	if strings.TrimSpace(getFirstValue(form, "out_trade_no")) == "" && strings.TrimSpace(getFirstValue(form, "trade_no")) == "" {
		return false
	}
	return true
}

func (h *Handler) findAlipayCallbackPayment(form map[string][]string) (*models.Payment, *models.PaymentChannel, error) {
	if paymentID, ok := parseAlipayPaymentID(form); ok {
		payment, channel, err := h.loadAlipayPaymentByID(paymentID)
		if err == nil && payment != nil && channel != nil {
			return payment, channel, nil
		}
	}

	for _, ref := range []string{strings.TrimSpace(getFirstValue(form, "out_trade_no")), strings.TrimSpace(getFirstValue(form, "trade_no"))} {
		if ref == "" {
			continue
		}
		payment, err := h.PaymentRepo.GetLatestByProviderRef(ref)
		if err != nil || payment == nil {
			continue
		}
		channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
		if err != nil || channel == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderOfficial {
			continue
		}
		if strings.ToLower(strings.TrimSpace(channel.ChannelType)) != constants.PaymentChannelTypeAlipay {
			continue
		}
		return payment, channel, nil
	}
	return nil, nil, service.ErrPaymentNotFound
}

func (h *Handler) loadAlipayPaymentByID(paymentID uint) (*models.Payment, *models.PaymentChannel, error) {
	if paymentID == 0 {
		return nil, nil, service.ErrPaymentInvalid
	}
	payment, err := h.PaymentRepo.GetByID(paymentID)
	if err != nil || payment == nil {
		return nil, nil, service.ErrPaymentNotFound
	}
	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		return nil, nil, service.ErrPaymentChannelNotFound
	}
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderOfficial {
		return nil, nil, service.ErrPaymentProviderNotSupported
	}
	if strings.ToLower(strings.TrimSpace(channel.ChannelType)) != constants.PaymentChannelTypeAlipay {
		return nil, nil, service.ErrPaymentProviderNotSupported
	}
	return payment, channel, nil
}

func parseAlipayPaymentID(form map[string][]string) (uint, bool) {
	passback := strings.TrimSpace(getFirstValue(form, "passback_params"))
	if passback == "" {
		return 0, false
	}
	if decoded, err := url.QueryUnescape(passback); err == nil {
		passback = strings.TrimSpace(decoded)
	}
	if strings.Contains(passback, "=") {
		if queryValues, err := url.ParseQuery(passback); err == nil {
			if paymentIDVal := strings.TrimSpace(queryValues.Get("payment_id")); paymentIDVal != "" {
				passback = paymentIDVal
			}
		}
	}
	if strings.HasPrefix(passback, "payment_id:") {
		passback = strings.TrimSpace(strings.TrimPrefix(passback, "payment_id:"))
	}
	parsed, err := strconv.ParseUint(passback, 10, 64)
	if err != nil || parsed == 0 {
		return 0, false
	}
	return uint(parsed), true
}

func parseAlipayCallback(form map[string][]string, paymentID uint) (*service.PaymentCallbackInput, error) {
	if paymentID == 0 {
		return nil, service.ErrPaymentInvalid
	}
	tradeStatus := strings.TrimSpace(getFirstValue(form, "trade_status"))
	status, ok := mapAlipayTradeStatus(tradeStatus)
	if !ok {
		return nil, service.ErrPaymentStatusInvalid
	}
	amount := models.Money{}
	if money := strings.TrimSpace(getFirstValue(form, "total_amount")); money != "" {
		parsed, err := decimal.NewFromString(money)
		if err != nil {
			return nil, service.ErrPaymentInvalid
		}
		amount = models.NewMoneyFromDecimal(parsed)
	}
	providerRef := strings.TrimSpace(getFirstValue(form, "trade_no"))
	if providerRef == "" {
		providerRef = strings.TrimSpace(getFirstValue(form, "out_trade_no"))
	}
	payload := make(map[string]interface{}, len(form))
	for key, values := range form {
		if len(values) > 0 {
			payload[key] = values[0]
		}
	}
	return &service.PaymentCallbackInput{
		PaymentID:   paymentID,
		OrderNo:     strings.TrimSpace(getFirstValue(form, "out_trade_no")),
		Status:      status,
		ProviderRef: providerRef,
		Amount:      amount,
		PaidAt:      parseAlipayPaidAt(getFirstValue(form, "gmt_payment"), getFirstValue(form, "notify_time")),
		Payload:     models.JSON(payload),
	}, nil
}

func parseAlipayPaidAt(values ...string) *time.Time {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
			return &parsed
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return &parsed
		}
	}
	return nil
}

func mapAlipayTradeStatus(tradeStatus string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(tradeStatus)) {
	case constants.AlipayTradeStatusSuccess, constants.AlipayTradeStatusFinished:
		return constants.PaymentStatusSuccess, true
	case constants.AlipayTradeStatusWaitBuyerPay:
		return constants.PaymentStatusPending, true
	case constants.AlipayTradeStatusClosed:
		return constants.PaymentStatusFailed, true
	default:
		return "", false
	}
}

func (h *Handler) HandleEpayCallback(c *gin.Context) bool {
	form, err := parseCallbackForm(c)
	if err != nil {
		return false
	}
	if strings.TrimSpace(getFirstValue(form, "param")) == "" {
		return false
	}
	if strings.TrimSpace(getFirstValue(form, "trade_status")) == "" && strings.TrimSpace(getFirstValue(form, "out_trade_no")) == "" {
		return false
	}
	paymentID, err := parseEpayPaymentID(form)
	if err != nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	payment, err := h.PaymentRepo.GetByID(paymentID)
	if err != nil || payment == nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderEpay {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	cfg, err := epay.ParseConfig(channel.ConfigJSON)
	if err != nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	if err := epay.ValidateConfig(cfg); err != nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	if err := epay.VerifyCallback(cfg, form); err != nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	input, err := parseEpayCallback(form)
	if err != nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	input.ChannelID = channel.ID
	if _, err := h.PaymentService.HandleCallback(*input); err != nil {
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	c.String(200, constants.EpayCallbackSuccess)
	return true
}

func parseEpayPaymentID(form map[string][]string) (uint, error) {
	param := strings.TrimSpace(getFirstValue(form, "param"))
	if param == "" {
		return 0, service.ErrPaymentInvalid
	}
	parsedID, err := strconv.ParseUint(param, 10, 64)
	if err != nil || parsedID == 0 {
		return 0, service.ErrPaymentInvalid
	}
	return uint(parsedID), nil
}

func parseEpayCallback(form map[string][]string) (*service.PaymentCallbackInput, error) {
	orderNo := strings.TrimSpace(getFirstValue(form, "out_trade_no"))
	tradeStatus := strings.TrimSpace(getFirstValue(form, "trade_status"))
	status := constants.PaymentStatusFailed
	if tradeStatus == constants.EpayTradeStatusSuccess {
		status = constants.PaymentStatusSuccess
	}
	amount := models.Money{}
	if money := strings.TrimSpace(getFirstValue(form, "money")); money != "" {
		parsed, err := decimal.NewFromString(money)
		if err != nil {
			return nil, service.ErrPaymentInvalid
		}
		amount = models.NewMoneyFromDecimal(parsed)
	}
	paidAt := parseEpayPaidAt(getFirstValue(form, "endtime"), getFirstValue(form, "addtime"))
	providerRef := strings.TrimSpace(getFirstValue(form, "trade_no"))
	if providerRef == "" {
		providerRef = strings.TrimSpace(getFirstValue(form, "api_trade_no"))
	}
	payload := make(map[string]interface{}, len(form))
	for key, values := range form {
		if len(values) > 0 {
			payload[key] = values[0]
		}
	}
	paymentID, err := parseEpayPaymentID(form)
	if err != nil {
		return nil, err
	}
	return &service.PaymentCallbackInput{
		PaymentID:   paymentID,
		OrderNo:     orderNo,
		Status:      status,
		ProviderRef: providerRef,
		Amount:      amount,
		PaidAt:      paidAt,
		Payload:     models.JSON(payload),
	}, nil
}

func parseEpayPaidAt(endTime, addTime string) *time.Time {
	for _, val := range []string{endTime, addTime} {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			continue
		}
		t := time.Unix(parsed, 0)
		return &t
	}
	return nil
}

func getFirstValue(form map[string][]string, key string) string {
	if values, ok := form[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

func respondPaymentCreateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPaymentInvalid):
		respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
	case errors.Is(err, service.ErrOrderNotFound):
		respondError(c, response.CodeNotFound, "error.order_not_found", nil)
	case errors.Is(err, service.ErrOrderStatusInvalid):
		respondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
	case errors.Is(err, service.ErrPaymentChannelNotFound):
		respondError(c, response.CodeNotFound, "error.payment_channel_not_found", nil)
	case errors.Is(err, service.ErrPaymentChannelInactive):
		respondError(c, response.CodeBadRequest, "error.payment_channel_inactive", nil)
	case errors.Is(err, service.ErrPaymentProviderNotSupported):
		respondError(c, response.CodeBadRequest, "error.payment_provider_not_supported", nil)
	case errors.Is(err, service.ErrPaymentChannelConfigInvalid):
		respondError(c, response.CodeBadRequest, "error.payment_channel_config_invalid", nil)
	case errors.Is(err, service.ErrPaymentGatewayRequestFailed):
		respondError(c, response.CodeBadRequest, "error.payment_gateway_request_failed", nil)
	case errors.Is(err, service.ErrPaymentGatewayResponseInvalid):
		respondError(c, response.CodeBadRequest, "error.payment_gateway_response_invalid", nil)
	default:
		respondError(c, response.CodeInternal, "error.payment_create_failed", err)
	}
}

func respondPaymentCaptureError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPaymentInvalid):
		respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
	case errors.Is(err, service.ErrPaymentNotFound):
		respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
	case errors.Is(err, service.ErrPaymentChannelNotFound):
		respondError(c, response.CodeNotFound, "error.payment_channel_not_found", nil)
	case errors.Is(err, service.ErrPaymentProviderNotSupported):
		respondError(c, response.CodeBadRequest, "error.payment_provider_not_supported", nil)
	case errors.Is(err, service.ErrPaymentChannelConfigInvalid):
		respondError(c, response.CodeBadRequest, "error.payment_channel_config_invalid", nil)
	case errors.Is(err, service.ErrPaymentGatewayRequestFailed):
		respondError(c, response.CodeBadRequest, "error.payment_gateway_request_failed", nil)
	case errors.Is(err, service.ErrPaymentGatewayResponseInvalid):
		respondError(c, response.CodeBadRequest, "error.payment_gateway_response_invalid", nil)
	case errors.Is(err, service.ErrPaymentStatusInvalid):
		respondError(c, response.CodeBadRequest, "error.payment_status_invalid", nil)
	case errors.Is(err, service.ErrPaymentAmountMismatch):
		respondError(c, response.CodeBadRequest, "error.payment_amount_mismatch", nil)
	case errors.Is(err, service.ErrPaymentCurrencyMismatch):
		respondError(c, response.CodeBadRequest, "error.payment_currency_mismatch", nil)
	case errors.Is(err, service.ErrOrderNotFound):
		respondError(c, response.CodeNotFound, "error.order_not_found", nil)
	default:
		respondError(c, response.CodeInternal, "error.payment_callback_failed", err)
	}
}

// HandleEpusdtCallback 处理 BEpusdt 回调
func (h *Handler) HandleEpusdtCallback(c *gin.Context) bool {
	log := requestLog(c)

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	// 恢复请求体供后续使用
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// 尝试解析为 epusdt 回调格式
	data, err := epusdt.ParseCallback(body)
	if err != nil {
		log.Debugw("epusdt_callback_parse_failed", "error", err)
		return false
	}

	// 检查是否有 trade_id 和 order_id（epusdt 回调特征）
	if data.TradeID == "" || data.OrderID == "" {
		log.Debugw("epusdt_callback_missing_fields", "trade_id", data.TradeID, "order_id", data.OrderID)
		return false
	}

	log.Infow("epusdt_callback_received", "trade_id", data.TradeID, "order_id", data.OrderID, "status", data.Status)

	// 通过 trade_id 查找支付记录
	payment, err := h.PaymentRepo.GetLatestByProviderRef(data.TradeID)
	if err != nil || payment == nil {
		log.Warnw("epusdt_callback_payment_not_found", "trade_id", data.TradeID, "error", err)
		c.String(200, "fail")
		return true
	}

	log.Debugw("epusdt_callback_payment_found", "payment_id", payment.ID, "channel_id", payment.ChannelID)

	// 获取支付渠道
	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		log.Warnw("epusdt_callback_channel_not_found", "channel_id", payment.ChannelID, "error", err)
		c.String(200, "fail")
		return true
	}

	// 验证是否为 epusdt 渠道
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderEpusdt {
		log.Warnw("epusdt_callback_invalid_provider", "provider_type", channel.ProviderType)
		c.String(200, "fail")
		return true
	}

	// 解析配置
	cfg, err := epusdt.ParseConfig(channel.ConfigJSON)
	if err != nil {
		log.Warnw("epusdt_callback_config_parse_failed", "error", err)
		c.String(200, "fail")
		return true
	}

	// 验证签名
	if err := epusdt.VerifyCallback(cfg, data); err != nil {
		log.Warnw("epusdt_callback_signature_invalid", "error", err)
		c.String(200, "fail")
		return true
	}

	log.Debugw("epusdt_callback_signature_verified")

	// 转换状态
	status := epusdt.ToPaymentStatus(data.Status)

	// 构建回调输入
	amount := models.Money{}
	amountFloat := data.GetAmount()
	if amountFloat > 0 {
		amount = models.NewMoneyFromDecimal(decimal.NewFromFloat(amountFloat))
	}

	now := time.Now()
	
	// 将原始回调数据保存为 payload（参考易支付的实现）
	payload := make(map[string]interface{})
	payload["trade_id"] = data.TradeID
	payload["order_id"] = data.OrderID
	payload["amount"] = data.GetAmount()
	payload["actual_amount"] = data.GetActualAmount()
	payload["token"] = data.Token
	payload["block_transaction_id"] = data.BlockTransactionID
	payload["status"] = data.Status
	
	input := service.PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     data.OrderID,
		ChannelID:   channel.ID,
		Status:      status,
		ProviderRef: data.TradeID,
		Amount:      amount,
		PaidAt:      &now,
		Payload:     models.JSON(payload),
	}

	// 处理回调
	if _, err := h.PaymentService.HandleCallback(input); err != nil {
		log.Errorw("epusdt_callback_handle_failed", "error", err)
		c.String(200, "fail")
		return true
	}

	log.Infow("epusdt_callback_processed", "payment_id", payment.ID, "status", status)
	c.String(200, "success")
	return true
}
