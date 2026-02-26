package public

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/alipay"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func (h *Handler) HandleAlipayCallback(c *gin.Context) bool {
	log := requestLog(c)
	form, err := parseCallbackForm(c)
	if err != nil {
		log.Warnw("alipay_callback_form_parse_failed", "error", err)
		return false
	}
	if !isAlipayCallbackForm(form) {
		log.Debugw("alipay_callback_not_matched")
		return false
	}
	log.Infow("alipay_callback_received",
		"client_ip", c.ClientIP(),
		"out_trade_no", strings.TrimSpace(getFirstValue(form, "out_trade_no")),
		"trade_no", strings.TrimSpace(getFirstValue(form, "trade_no")),
		"trade_status", strings.TrimSpace(getFirstValue(form, "trade_status")),
		"raw_form", callbackRawFormForLog(form),
	)

	payment, channel, err := h.findAlipayCallbackPayment(form)
	if err != nil || payment == nil || channel == nil {
		log.Warnw("alipay_callback_payment_not_found",
			"out_trade_no", strings.TrimSpace(getFirstValue(form, "out_trade_no")),
			"trade_no", strings.TrimSpace(getFirstValue(form, "trade_no")),
			"error", err,
		)
		c.String(200, constants.AlipayCallbackFail)
		return true
	}

	cfg, err := alipay.ParseConfig(channel.ConfigJSON)
	if err != nil {
		log.Warnw("alipay_callback_config_parse_failed",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"error", err,
		)
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	if err := alipay.VerifyCallback(cfg, form); err != nil {
		log.Warnw("alipay_callback_signature_invalid",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"error", err,
		)
		h.enqueuePaymentExceptionAlert(c, models.JSON{
			"alert_type":  "alipay_signature_invalid",
			"alert_level": "error",
			"payment_id":  fmt.Sprintf("%d", payment.ID),
			"message":     strings.TrimSpace(err.Error()),
			"provider":    constants.PaymentChannelTypeAlipay,
		})
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	if err := alipay.VerifyCallbackOwnership(cfg, form); err != nil {
		log.Warnw("alipay_callback_ownership_invalid",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"error", err,
		)
		h.enqueuePaymentExceptionAlert(c, models.JSON{
			"alert_type":  "alipay_ownership_invalid",
			"alert_level": "error",
			"payment_id":  fmt.Sprintf("%d", payment.ID),
			"message":     strings.TrimSpace(err.Error()),
			"provider":    constants.PaymentChannelTypeAlipay,
		})
		c.String(200, constants.AlipayCallbackFail)
		return true
	}

	input, err := parseAlipayCallback(form, payment.ID)
	if err != nil {
		log.Warnw("alipay_callback_parse_failed",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"error", err,
		)
		h.enqueuePaymentExceptionAlert(c, models.JSON{
			"alert_type":  "alipay_callback_parse_failed",
			"alert_level": "error",
			"payment_id":  fmt.Sprintf("%d", payment.ID),
			"message":     strings.TrimSpace(err.Error()),
			"provider":    constants.PaymentChannelTypeAlipay,
		})
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	input.ChannelID = channel.ID
	updated, err := h.PaymentService.HandleCallback(*input)
	if err != nil {
		log.Warnw("alipay_callback_handle_failed",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"order_no", input.OrderNo,
			"provider_ref", input.ProviderRef,
			"status", input.Status,
			"error", err,
		)
		h.enqueuePaymentExceptionAlert(c, models.JSON{
			"alert_type":  "alipay_callback_handle_failed",
			"alert_level": "error",
			"payment_id":  fmt.Sprintf("%d", payment.ID),
			"order_no":    strings.TrimSpace(input.OrderNo),
			"message":     strings.TrimSpace(err.Error()),
			"provider":    constants.PaymentChannelTypeAlipay,
		})
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	log.Infow("alipay_callback_processed",
		"payment_id", payment.ID,
		"channel_id", channel.ID,
		"order_no", input.OrderNo,
		"provider_ref", input.ProviderRef,
		"status", updated.Status,
	)
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
