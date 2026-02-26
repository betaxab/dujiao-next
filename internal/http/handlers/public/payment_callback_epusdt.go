package public

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/epusdt"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

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

	log.Infow("epusdt_callback_received",
		"trade_id", data.TradeID,
		"order_id", data.OrderID,
		"status", data.Status,
		"raw_body", callbackRawBodyForLog(body),
	)

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

	// 将回调数据结构体序列化为 JSON 保存
	payloadBytes, _ := json.Marshal(data)
	var payload models.JSON
	_ = json.Unmarshal(payloadBytes, &payload)

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
