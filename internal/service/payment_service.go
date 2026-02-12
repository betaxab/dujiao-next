package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/alipay"
	"github.com/dujiao-next/internal/payment/epay"
	"github.com/dujiao-next/internal/payment/paypal"
	"github.com/dujiao-next/internal/payment/stripe"
	"github.com/dujiao-next/internal/payment/wechatpay"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/repository"

	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PaymentService 支付服务
type PaymentService struct {
	orderRepo   repository.OrderRepository
	productRepo repository.ProductRepository
	paymentRepo repository.PaymentRepository
	channelRepo repository.PaymentChannelRepository
	queueClient *queue.Client
}

// NewPaymentService 创建支付服务
func NewPaymentService(orderRepo repository.OrderRepository, productRepo repository.ProductRepository, paymentRepo repository.PaymentRepository, channelRepo repository.PaymentChannelRepository, queueClient *queue.Client) *PaymentService {
	return &PaymentService{
		orderRepo:   orderRepo,
		productRepo: productRepo,
		paymentRepo: paymentRepo,
		channelRepo: channelRepo,
		queueClient: queueClient,
	}
}

// CreatePaymentInput 创建支付请求
type CreatePaymentInput struct {
	OrderID   uint
	ChannelID uint
	ClientIP  string
	Context   context.Context
}

// CreatePaymentResult 创建支付结果
type CreatePaymentResult struct {
	Payment *models.Payment
	Channel *models.PaymentChannel
}

func hasProviderResult(payment *models.Payment) bool {
	if payment == nil {
		return false
	}
	return strings.TrimSpace(payment.PayURL) != "" || strings.TrimSpace(payment.QRCode) != ""
}

func shouldMarkFulfilling(order *models.Order) bool {
	if order == nil {
		return false
	}
	if len(order.Items) == 0 {
		return false
	}
	for _, item := range order.Items {
		fulfillmentType := strings.TrimSpace(item.FulfillmentType)
		if fulfillmentType == "" || fulfillmentType == constants.FulfillmentTypeManual {
			return true
		}
	}
	return false
}

func paymentLogger(kv ...interface{}) *zap.SugaredLogger {
	if len(kv) == 0 {
		return logger.S()
	}
	return logger.SW(kv...)
}

// PaymentCallbackInput 支付回调输入
type PaymentCallbackInput struct {
	PaymentID   uint
	OrderNo     string
	ChannelID   uint
	Status      string
	ProviderRef string
	Amount      models.Money
	Currency    string
	PaidAt      *time.Time
	Payload     models.JSON
}

// CapturePaymentInput 捕获支付输入。
type CapturePaymentInput struct {
	PaymentID uint
	Context   context.Context
}

// WebhookCallbackInput Webhook 回调输入。
type WebhookCallbackInput struct {
	ChannelID uint
	Headers   map[string]string
	Body      []byte
	Context   context.Context
}

// CreatePayment 创建支付单
func (s *PaymentService) CreatePayment(input CreatePaymentInput) (*CreatePaymentResult, error) {
	if input.OrderID == 0 || input.ChannelID == 0 {
		return nil, ErrPaymentInvalid
	}

	log := paymentLogger(
		"order_id", input.OrderID,
		"channel_id", input.ChannelID,
	)

	order, err := s.orderRepo.GetByID(input.OrderID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if order.ParentID != nil {
		return nil, ErrPaymentInvalid
	}
	if order.Status != constants.OrderStatusPendingPayment {
		return nil, ErrOrderStatusInvalid
	}
	now := time.Now()
	if order.ExpiresAt != nil && !order.ExpiresAt.After(now) {
		return nil, ErrOrderStatusInvalid
	}

	channel, err := s.channelRepo.GetByID(input.ChannelID)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, ErrPaymentChannelNotFound
	}
	if !channel.IsActive {
		return nil, ErrPaymentChannelInactive
	}

	existing, err := s.paymentRepo.GetLatestPendingByOrderChannel(order.ID, channel.ID, now)
	if err != nil {
		return nil, ErrPaymentCreateFailed
	}
	if existing != nil && hasProviderResult(existing) {
		log.Infow("payment_create_reuse_pending",
			"payment_id", existing.ID,
			"provider_type", existing.ProviderType,
			"channel_type", existing.ChannelType,
		)
		return &CreatePaymentResult{Payment: existing, Channel: channel}, nil
	}

	feeRate := channel.FeeRate.Decimal.Round(2)
	if feeRate.LessThan(decimal.Zero) || feeRate.GreaterThan(decimal.NewFromInt(100)) {
		return nil, ErrPaymentChannelConfigInvalid
	}
	feeAmount := decimal.Zero
	if feeRate.GreaterThan(decimal.Zero) {
		feeAmount = order.TotalAmount.Decimal.Mul(feeRate).Div(decimal.NewFromInt(100)).Round(2)
	}
	payableAmount := order.TotalAmount.Decimal.Add(feeAmount).Round(2)

	payment := &models.Payment{
		OrderID:         order.ID,
		ChannelID:       channel.ID,
		ProviderType:    channel.ProviderType,
		ChannelType:     channel.ChannelType,
		InteractionMode: channel.InteractionMode,
		Amount:          models.NewMoneyFromDecimal(payableAmount),
		FeeRate:         models.NewMoneyFromDecimal(feeRate),
		FeeAmount:       models.NewMoneyFromDecimal(feeAmount),
		Currency:        order.Currency,
		Status:          constants.PaymentStatusInitiated,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if shouldUseCNYPaymentCurrency(channel) {
		payment.Currency = "CNY"
	}

	if err := s.paymentRepo.Create(payment); err != nil {
		log.Errorw("payment_create_persist_failed", "error", err)
		return nil, ErrPaymentCreateFailed
	}

	if err := s.applyProviderPayment(input, order, channel, payment); err != nil {
		payment.Status = constants.PaymentStatusFailed
		payment.UpdatedAt = time.Now()
		_ = s.paymentRepo.Update(payment)
		log.Errorw("payment_create_provider_failed",
			"payment_id", payment.ID,
			"provider_type", payment.ProviderType,
			"channel_type", payment.ChannelType,
			"error", err,
		)
		return nil, err
	}

	log.Infow("payment_create_success",
		"payment_id", payment.ID,
		"provider_type", payment.ProviderType,
		"channel_type", payment.ChannelType,
		"interaction_mode", payment.InteractionMode,
		"currency", payment.Currency,
		"amount", payment.Amount.String(),
	)

	return &CreatePaymentResult{Payment: payment, Channel: channel}, nil
}

// HandleCallback 处理支付回调
func (s *PaymentService) HandleCallback(input PaymentCallbackInput) (*models.Payment, error) {
	if input.PaymentID == 0 {
		return nil, ErrPaymentInvalid
	}
	status := normalizePaymentStatus(input.Status)
	if !isPaymentStatusValid(status) {
		return nil, ErrPaymentStatusInvalid
	}

	log := paymentLogger(
		"payment_id", input.PaymentID,
		"target_status", status,
	)

	payment, err := s.paymentRepo.GetByID(input.PaymentID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}

	order, err := s.orderRepo.GetByID(payment.OrderID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}

	if input.ChannelID != 0 && input.ChannelID != payment.ChannelID {
		return nil, ErrPaymentInvalid
	}
	if input.OrderNo != "" && input.OrderNo != order.OrderNo {
		return nil, ErrPaymentInvalid
	}
	if input.Currency != "" && strings.ToUpper(strings.TrimSpace(input.Currency)) != strings.ToUpper(strings.TrimSpace(payment.Currency)) {
		return nil, ErrPaymentCurrencyMismatch
	}
	if !input.Amount.Decimal.IsZero() && input.Amount.Decimal.Cmp(payment.Amount.Decimal) != 0 {
		return nil, ErrPaymentAmountMismatch
	}

	// 幂等处理：已成功的不再回退状态
	if payment.Status == constants.PaymentStatusSuccess {
		return s.updateCallbackMeta(payment, constants.PaymentStatusSuccess, input)
	}
	if payment.Status == status {
		return s.updateCallbackMeta(payment, status, input)
	}

	previousStatus := payment.Status
	now := time.Now()
	updated, orderPaid, err := s.applyPaymentUpdate(payment, order, status, input, now)
	if err != nil {
		log.Errorw("payment_callback_apply_failed",
			"order_id", order.ID,
			"order_no", order.OrderNo,
			"current_status", payment.Status,
			"error", err,
		)
		return nil, err
	}
	if orderPaid && s.queueClient != nil {
		if err := s.queueClient.EnqueueOrderStatusEmail(queue.OrderStatusEmailPayload{
			OrderID: order.ID,
			Status:  constants.OrderStatusPaid,
		}); err != nil {
			log.Warnw("payment_enqueue_status_email_failed",
				"order_id", order.ID,
				"order_no", order.OrderNo,
				"status", constants.OrderStatusPaid,
				"error", err,
			)
		}
		if len(order.Children) > 0 {
			for _, child := range order.Children {
				if shouldAutoFulfill(&child) {
					if err := s.queueClient.EnqueueOrderAutoFulfill(queue.OrderAutoFulfillPayload{
						OrderID: child.ID,
					}, asynq.MaxRetry(3)); err != nil {
						log.Warnw("payment_enqueue_auto_fulfill_failed",
							"order_id", order.ID,
							"child_order_id", child.ID,
							"order_no", order.OrderNo,
							"error", err,
						)
					}
				}
			}
		} else if shouldAutoFulfill(order) {
			if err := s.queueClient.EnqueueOrderAutoFulfill(queue.OrderAutoFulfillPayload{
				OrderID: order.ID,
			}, asynq.MaxRetry(3)); err != nil {
				log.Warnw("payment_enqueue_auto_fulfill_failed",
					"order_id", order.ID,
					"order_no", order.OrderNo,
					"error", err,
				)
			}
		}
	}
	log.Infow("payment_callback_processed",
		"order_id", order.ID,
		"order_no", order.OrderNo,
		"previous_status", previousStatus,
		"new_status", updated.Status,
		"order_paid", orderPaid,
	)
	return updated, nil
}

// ListPayments 管理端支付列表
func (s *PaymentService) ListPayments(filter repository.PaymentListFilter) ([]models.Payment, int64, error) {
	return s.paymentRepo.ListAdmin(filter)
}

// GetPayment 获取支付记录
func (s *PaymentService) GetPayment(id uint) (*models.Payment, error) {
	if id == 0 {
		return nil, ErrPaymentInvalid
	}
	payment, err := s.paymentRepo.GetByID(id)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	return payment, nil
}

// CapturePayment 捕获支付。
func (s *PaymentService) CapturePayment(input CapturePaymentInput) (*models.Payment, error) {
	if input.PaymentID == 0 {
		return nil, ErrPaymentInvalid
	}
	payment, err := s.paymentRepo.GetByID(input.PaymentID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	if payment.Status == constants.PaymentStatusSuccess {
		return payment, nil
	}

	channel, err := s.channelRepo.GetByID(payment.ChannelID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if channel == nil {
		return nil, ErrPaymentChannelNotFound
	}

	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	if providerType != constants.PaymentProviderOfficial {
		return nil, ErrPaymentProviderNotSupported
	}
	if strings.TrimSpace(payment.ProviderRef) == "" {
		return nil, ErrPaymentInvalid
	}

	channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
	switch channelType {
	case constants.PaymentChannelTypePaypal:
		return s.capturePaypalPayment(input, payment, channel)
	case constants.PaymentChannelTypeStripe:
		return s.captureStripePayment(input, payment, channel)
	case constants.PaymentChannelTypeWechat:
		return s.captureWechatPayment(input, payment, channel)
	default:
		return nil, ErrPaymentProviderNotSupported
	}
}

func (s *PaymentService) capturePaypalPayment(input CapturePaymentInput, payment *models.Payment, channel *models.PaymentChannel) (*models.Payment, error) {
	cfg, err := paypal.ParseConfig(channel.ConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}
	if err := paypal.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}

	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	captureResult, err := paypal.CaptureOrder(ctx, cfg, payment.ProviderRef)
	if err != nil {
		switch {
		case errors.Is(err, paypal.ErrConfigInvalid):
			return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		case errors.Is(err, paypal.ErrAuthFailed), errors.Is(err, paypal.ErrRequestFailed):
			return nil, ErrPaymentGatewayRequestFailed
		case errors.Is(err, paypal.ErrResponseInvalid):
			return nil, ErrPaymentGatewayResponseInvalid
		default:
			return nil, ErrPaymentGatewayRequestFailed
		}
	}

	status, ok := mapPaypalStatus(strings.TrimSpace(captureResult.Status))
	if !ok {
		status = constants.PaymentStatusPending
	}
	payload := models.JSON{}
	if captureResult.Raw != nil {
		payload = models.JSON(captureResult.Raw)
	}
	amount := models.Money{}
	if strings.TrimSpace(captureResult.Amount) != "" {
		parsed, parseErr := decimal.NewFromString(strings.TrimSpace(captureResult.Amount))
		if parseErr == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}
	callbackInput := PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     "",
		ChannelID:   channel.ID,
		Status:      status,
		ProviderRef: pickFirstNonEmpty(strings.TrimSpace(captureResult.OrderID), strings.TrimSpace(payment.ProviderRef)),
		Amount:      amount,
		Currency:    strings.ToUpper(strings.TrimSpace(captureResult.Currency)),
		PaidAt:      captureResult.PaidAt,
		Payload:     payload,
	}
	return s.HandleCallback(callbackInput)
}

func (s *PaymentService) captureWechatPayment(input CapturePaymentInput, payment *models.Payment, channel *models.PaymentChannel) (*models.Payment, error) {
	cfg, err := wechatpay.ParseConfig(channel.ConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}
	if err := wechatpay.ValidateConfig(cfg, channel.InteractionMode); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}

	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	queryResult, err := wechatpay.QueryOrderByOutTradeNo(ctx, cfg, payment.ProviderRef)
	if err != nil {
		switch {
		case errors.Is(err, wechatpay.ErrConfigInvalid):
			return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		case errors.Is(err, wechatpay.ErrRequestFailed):
			return nil, ErrPaymentGatewayRequestFailed
		case errors.Is(err, wechatpay.ErrResponseInvalid):
			return nil, ErrPaymentGatewayResponseInvalid
		default:
			return nil, ErrPaymentGatewayRequestFailed
		}
	}

	amount := models.Money{}
	if strings.TrimSpace(queryResult.Amount) != "" {
		parsed, parseErr := decimal.NewFromString(strings.TrimSpace(queryResult.Amount))
		if parseErr == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}
	payload := models.JSON{}
	if queryResult.Raw != nil {
		payload = models.JSON(queryResult.Raw)
	}
	status := strings.TrimSpace(queryResult.Status)
	if status == "" {
		status = constants.PaymentStatusPending
	}
	callbackInput := PaymentCallbackInput{
		PaymentID:   payment.ID,
		ChannelID:   channel.ID,
		Status:      status,
		ProviderRef: pickFirstNonEmpty(strings.TrimSpace(queryResult.TransactionID), strings.TrimSpace(payment.ProviderRef)),
		Amount:      amount,
		Currency:    strings.ToUpper(strings.TrimSpace(queryResult.Currency)),
		PaidAt:      queryResult.PaidAt,
		Payload:     payload,
	}
	return s.HandleCallback(callbackInput)
}

func (s *PaymentService) captureStripePayment(input CapturePaymentInput, payment *models.Payment, channel *models.PaymentChannel) (*models.Payment, error) {
	cfg, err := stripe.ParseConfig(channel.ConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}
	if err := stripe.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}

	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	queryResult, err := stripe.QueryPayment(ctx, cfg, payment.ProviderRef)
	if err != nil {
		return nil, mapStripeGatewayError(err)
	}

	amount := models.Money{}
	if strings.TrimSpace(queryResult.Amount) != "" {
		parsed, parseErr := decimal.NewFromString(strings.TrimSpace(queryResult.Amount))
		if parseErr == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}
	payload := models.JSON{}
	if queryResult.Raw != nil {
		payload = models.JSON(queryResult.Raw)
	}
	status := strings.TrimSpace(queryResult.Status)
	if status == "" {
		status = constants.PaymentStatusPending
	}
	callbackInput := PaymentCallbackInput{
		PaymentID: payment.ID,
		ChannelID: channel.ID,
		Status:    status,
		ProviderRef: pickFirstNonEmpty(
			strings.TrimSpace(queryResult.SessionID),
			strings.TrimSpace(queryResult.PaymentIntentID),
			strings.TrimSpace(payment.ProviderRef),
		),
		Amount:   amount,
		Currency: strings.ToUpper(strings.TrimSpace(queryResult.Currency)),
		PaidAt:   queryResult.PaidAt,
		Payload:  payload,
	}
	return s.HandleCallback(callbackInput)
}

// HandlePaypalWebhook 处理 PayPal webhook。
func (s *PaymentService) HandlePaypalWebhook(input WebhookCallbackInput) (*models.Payment, string, error) {
	if input.ChannelID == 0 {
		return nil, "", ErrPaymentInvalid
	}
	channel, err := s.channelRepo.GetByID(input.ChannelID)
	if err != nil {
		return nil, "", ErrPaymentUpdateFailed
	}
	if channel == nil {
		return nil, "", ErrPaymentChannelNotFound
	}
	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
	if providerType != constants.PaymentProviderOfficial || channelType != constants.PaymentChannelTypePaypal {
		return nil, "", ErrPaymentProviderNotSupported
	}

	cfg, err := paypal.ParseConfig(channel.ConfigJSON)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}
	if err := paypal.ValidateConfig(cfg); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	}

	event, err := paypal.ParseWebhookEvent(input.Body)
	if err != nil {
		return nil, "", ErrPaymentGatewayResponseInvalid
	}

	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	headers := make(http.Header)
	for key, value := range input.Headers {
		headers.Set(key, value)
	}
	if strings.TrimSpace(cfg.WebhookID) != "" {
		if err := paypal.VerifyWebhookSignature(ctx, cfg, headers, event.Raw); err != nil {
			switch {
			case errors.Is(err, paypal.ErrConfigInvalid):
				return nil, event.EventType, fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			case errors.Is(err, paypal.ErrAuthFailed), errors.Is(err, paypal.ErrRequestFailed):
				return nil, event.EventType, ErrPaymentGatewayRequestFailed
			default:
				return nil, event.EventType, ErrPaymentGatewayResponseInvalid
			}
		}
	}

	paypalOrderID := strings.TrimSpace(event.RelatedOrderID())
	if paypalOrderID == "" {
		return nil, event.EventType, ErrPaymentInvalid
	}

	payment, err := s.paymentRepo.GetLatestByProviderRef(paypalOrderID)
	if err != nil {
		return nil, event.EventType, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, event.EventType, nil
	}

	status, ok := paypal.ToPaymentStatus(event.EventType, event.ResourceStatus())
	if !ok {
		return payment, event.EventType, nil
	}

	amount := models.Money{}
	amountValue, amountCurrency := event.CaptureAmount()
	if amountValue != "" {
		if parsed, parseErr := decimal.NewFromString(amountValue); parseErr == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}

	resourceBytes, _ := json.Marshal(event.Raw)
	payloadMap := map[string]interface{}{}
	if len(resourceBytes) > 0 {
		_ = json.Unmarshal(resourceBytes, &payloadMap)
	}

	updated, err := s.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		ChannelID:   channel.ID,
		Status:      status,
		ProviderRef: paypalOrderID,
		Amount:      amount,
		Currency:    strings.ToUpper(strings.TrimSpace(amountCurrency)),
		PaidAt:      event.PaidAt(),
		Payload:     models.JSON(payloadMap),
	})
	if err != nil {
		return nil, event.EventType, err
	}
	return updated, event.EventType, nil
}

// HandleWechatWebhook 处理微信支付回调。
func (s *PaymentService) HandleWechatWebhook(input WebhookCallbackInput) (*models.Payment, string, error) {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	candidates, err := s.resolveWechatWebhookChannels(input.ChannelID)
	if err != nil {
		return nil, "", err
	}

	var lastErr error
	for i := range candidates {
		channel := candidates[i]
		cfg, err := wechatpay.ParseConfig(channel.ConfigJSON)
		if err != nil {
			mappedErr := fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			if input.ChannelID != 0 {
				return nil, "", mappedErr
			}
			lastErr = mappedErr
			continue
		}
		if err := wechatpay.ValidateConfig(cfg, channel.InteractionMode); err != nil {
			mappedErr := fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			if input.ChannelID != 0 {
				return nil, "", mappedErr
			}
			lastErr = mappedErr
			continue
		}

		result, err := wechatpay.VerifyAndDecodeWebhook(ctx, cfg, input.Headers, input.Body)
		if err != nil {
			mappedErr := mapWechatGatewayError(err)
			if input.ChannelID != 0 {
				return nil, "", mappedErr
			}
			lastErr = mappedErr
			continue
		}

		payment, err := s.findWechatWebhookPayment(channel.ID, result)
		if err != nil {
			if errors.Is(err, ErrPaymentNotFound) {
				return nil, result.EventType, nil
			}
			return nil, result.EventType, err
		}
		if payment == nil {
			return nil, result.EventType, nil
		}

		updated, err := s.handleWechatWebhookCallback(channel.ID, payment, result)
		if err != nil {
			return nil, result.EventType, err
		}
		return updated, result.EventType, nil
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", ErrPaymentGatewayResponseInvalid
}

// HandleStripeWebhook 处理 Stripe webhook。
func (s *PaymentService) HandleStripeWebhook(input WebhookCallbackInput) (*models.Payment, string, error) {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	candidates, err := s.resolveStripeWebhookChannels(input.ChannelID)
	if err != nil {
		return nil, "", err
	}

	var lastErr error
	for i := range candidates {
		channel := candidates[i]
		cfg, err := stripe.ParseConfig(channel.ConfigJSON)
		if err != nil {
			mappedErr := fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			if input.ChannelID != 0 {
				return nil, "", mappedErr
			}
			lastErr = mappedErr
			continue
		}
		if err := stripe.ValidateConfig(cfg); err != nil {
			mappedErr := fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			if input.ChannelID != 0 {
				return nil, "", mappedErr
			}
			lastErr = mappedErr
			continue
		}

		result, err := stripe.VerifyAndParseWebhook(cfg, input.Headers, input.Body, time.Now())
		if err != nil {
			mappedErr := mapStripeGatewayError(err)
			if input.ChannelID != 0 {
				return nil, "", mappedErr
			}
			lastErr = mappedErr
			continue
		}

		payment, err := s.findStripeWebhookPayment(channel.ID, result)
		if err != nil {
			if errors.Is(err, ErrPaymentNotFound) {
				return nil, result.EventType, nil
			}
			return nil, result.EventType, err
		}
		if payment == nil {
			return nil, result.EventType, nil
		}

		updated, err := s.handleStripeWebhookCallback(channel.ID, payment, result)
		if err != nil {
			return nil, result.EventType, err
		}
		return updated, result.EventType, nil
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", ErrPaymentGatewayResponseInvalid
}

func (s *PaymentService) resolveStripeWebhookChannels(channelID uint) ([]models.PaymentChannel, error) {
	if channelID != 0 {
		channel, err := s.channelRepo.GetByID(channelID)
		if err != nil {
			return nil, ErrPaymentUpdateFailed
		}
		if channel == nil {
			return nil, ErrPaymentChannelNotFound
		}
		providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
		channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
		if providerType != constants.PaymentProviderOfficial || channelType != constants.PaymentChannelTypeStripe {
			return nil, ErrPaymentProviderNotSupported
		}
		return []models.PaymentChannel{*channel}, nil
	}

	channels, _, err := s.channelRepo.List(repository.PaymentChannelListFilter{
		ProviderType: constants.PaymentProviderOfficial,
		ChannelType:  constants.PaymentChannelTypeStripe,
		ActiveOnly:   true,
	})
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if len(channels) == 0 {
		return nil, ErrPaymentChannelNotFound
	}
	return channels, nil
}

func (s *PaymentService) findStripeWebhookPayment(channelID uint, result *stripe.WebhookResult) (*models.Payment, error) {
	if result == nil {
		return nil, ErrPaymentInvalid
	}
	if result.PaymentID > 0 {
		payment, err := s.paymentRepo.GetByID(result.PaymentID)
		if err != nil {
			return nil, ErrPaymentUpdateFailed
		}
		if payment != nil && payment.ChannelID == channelID {
			return payment, nil
		}
	}

	for _, ref := range []string{
		strings.TrimSpace(result.ProviderRef),
		strings.TrimSpace(result.SessionID),
		strings.TrimSpace(result.PaymentIntentID),
		strings.TrimSpace(result.OrderNo),
	} {
		if ref == "" {
			continue
		}
		payment, err := s.paymentRepo.GetLatestByProviderRef(ref)
		if err != nil {
			return nil, ErrPaymentUpdateFailed
		}
		if payment == nil {
			continue
		}
		if payment.ChannelID != channelID {
			continue
		}
		return payment, nil
	}
	return nil, ErrPaymentNotFound
}

func (s *PaymentService) handleStripeWebhookCallback(channelID uint, payment *models.Payment, result *stripe.WebhookResult) (*models.Payment, error) {
	if payment == nil || result == nil {
		return nil, ErrPaymentInvalid
	}
	amount := models.Money{}
	if strings.TrimSpace(result.Amount) != "" {
		parsed, err := decimal.NewFromString(strings.TrimSpace(result.Amount))
		if err == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}
	payload := models.JSON{}
	if result.Raw != nil {
		payload = models.JSON(result.Raw)
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = constants.PaymentStatusPending
	}
	callbackInput := PaymentCallbackInput{
		PaymentID: payment.ID,
		ChannelID: channelID,
		Status:    status,
		ProviderRef: pickFirstNonEmpty(
			strings.TrimSpace(result.ProviderRef),
			strings.TrimSpace(result.SessionID),
			strings.TrimSpace(result.PaymentIntentID),
			strings.TrimSpace(payment.ProviderRef),
		),
		Amount:   amount,
		Currency: strings.ToUpper(strings.TrimSpace(result.Currency)),
		PaidAt:   result.PaidAt,
		Payload:  payload,
	}
	return s.HandleCallback(callbackInput)
}

func (s *PaymentService) resolveWechatWebhookChannels(channelID uint) ([]models.PaymentChannel, error) {
	if channelID != 0 {
		channel, err := s.channelRepo.GetByID(channelID)
		if err != nil {
			return nil, ErrPaymentUpdateFailed
		}
		if channel == nil {
			return nil, ErrPaymentChannelNotFound
		}
		providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
		channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
		if providerType != constants.PaymentProviderOfficial || channelType != constants.PaymentChannelTypeWechat {
			return nil, ErrPaymentProviderNotSupported
		}
		return []models.PaymentChannel{*channel}, nil
	}

	channels, _, err := s.channelRepo.List(repository.PaymentChannelListFilter{
		ProviderType: constants.PaymentProviderOfficial,
		ChannelType:  constants.PaymentChannelTypeWechat,
		ActiveOnly:   true,
	})
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if len(channels) == 0 {
		return nil, ErrPaymentChannelNotFound
	}
	return channels, nil
}

func (s *PaymentService) findWechatWebhookPayment(channelID uint, result *wechatpay.WebhookResult) (*models.Payment, error) {
	if result == nil {
		return nil, ErrPaymentInvalid
	}
	if paymentID, ok := wechatpay.ParsePaymentIDFromAttach(result.Attach); ok {
		payment, err := s.paymentRepo.GetByID(paymentID)
		if err != nil {
			return nil, ErrPaymentUpdateFailed
		}
		if payment != nil && payment.ChannelID == channelID {
			return payment, nil
		}
	}

	for _, ref := range []string{strings.TrimSpace(result.OrderNo), strings.TrimSpace(result.TransactionID)} {
		if ref == "" {
			continue
		}
		payment, err := s.paymentRepo.GetLatestByProviderRef(ref)
		if err != nil {
			return nil, ErrPaymentUpdateFailed
		}
		if payment == nil {
			continue
		}
		if payment.ChannelID != channelID {
			continue
		}
		return payment, nil
	}
	return nil, ErrPaymentNotFound
}

func (s *PaymentService) handleWechatWebhookCallback(channelID uint, payment *models.Payment, result *wechatpay.WebhookResult) (*models.Payment, error) {
	if payment == nil || result == nil {
		return nil, ErrPaymentInvalid
	}
	amount := models.Money{}
	if strings.TrimSpace(result.Amount) != "" {
		parsed, err := decimal.NewFromString(strings.TrimSpace(result.Amount))
		if err == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}
	payload := models.JSON{}
	if result.Raw != nil {
		payload = models.JSON(result.Raw)
	}
	callbackInput := PaymentCallbackInput{
		PaymentID:   payment.ID,
		ChannelID:   channelID,
		Status:      strings.TrimSpace(result.Status),
		ProviderRef: pickFirstNonEmpty(strings.TrimSpace(result.TransactionID), strings.TrimSpace(payment.ProviderRef)),
		Amount:      amount,
		Currency:    strings.ToUpper(strings.TrimSpace(result.Currency)),
		PaidAt:      result.PaidAt,
		Payload:     payload,
	}
	return s.HandleCallback(callbackInput)
}

func mapWechatGatewayError(err error) error {
	switch {
	case errors.Is(err, wechatpay.ErrConfigInvalid):
		return ErrPaymentChannelConfigInvalid
	case errors.Is(err, wechatpay.ErrRequestFailed):
		return ErrPaymentGatewayRequestFailed
	case errors.Is(err, wechatpay.ErrSignatureInvalid), errors.Is(err, wechatpay.ErrResponseInvalid):
		return ErrPaymentGatewayResponseInvalid
	default:
		return ErrPaymentGatewayRequestFailed
	}
}

func mapStripeGatewayError(err error) error {
	switch {
	case errors.Is(err, stripe.ErrConfigInvalid):
		return ErrPaymentChannelConfigInvalid
	case errors.Is(err, stripe.ErrRequestFailed):
		return ErrPaymentGatewayRequestFailed
	case errors.Is(err, stripe.ErrSignatureInvalid), errors.Is(err, stripe.ErrResponseInvalid):
		return ErrPaymentGatewayResponseInvalid
	default:
		return ErrPaymentGatewayRequestFailed
	}
}

// ListChannels 支付渠道列表
func (s *PaymentService) ListChannels(filter repository.PaymentChannelListFilter) ([]models.PaymentChannel, int64, error) {
	return s.channelRepo.List(filter)
}

// GetChannel 获取支付渠道
func (s *PaymentService) GetChannel(id uint) (*models.PaymentChannel, error) {
	if id == 0 {
		return nil, ErrPaymentInvalid
	}
	channel, err := s.channelRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, ErrPaymentChannelNotFound
	}
	return channel, nil
}

func (s *PaymentService) updateCallbackMeta(payment *models.Payment, status string, input PaymentCallbackInput) (*models.Payment, error) {
	updated := false
	if input.ProviderRef != "" && payment.ProviderRef == "" {
		payment.ProviderRef = input.ProviderRef
		updated = true
	}
	if input.Payload != nil {
		payment.ProviderPayload = input.Payload
		updated = true
	}
	if status != "" && payment.Status != status {
		payment.Status = status
		updated = true
	}
	if payment.Status == constants.PaymentStatusSuccess && payment.PaidAt == nil && input.PaidAt != nil {
		payment.PaidAt = input.PaidAt
		updated = true
	}
	if updated {
		now := time.Now()
		payment.CallbackAt = &now
		payment.UpdatedAt = now
		if err := s.paymentRepo.Update(payment); err != nil {
			return nil, ErrPaymentUpdateFailed
		}
	}
	return payment, nil
}

func (s *PaymentService) applyPaymentUpdate(payment *models.Payment, order *models.Order, status string, input PaymentCallbackInput, now time.Time) (*models.Payment, bool, error) {
	returnVal := payment
	orderPaid := false

	switch status {
	case constants.PaymentStatusSuccess:
		paidAt := now
		if input.PaidAt != nil {
			paidAt = *input.PaidAt
		}
		payment.PaidAt = &paidAt
	case constants.PaymentStatusExpired:
		payment.ExpiredAt = &now
	}

	payment.Status = status
	payment.CallbackAt = &now
	payment.UpdatedAt = now
	if input.ProviderRef != "" {
		payment.ProviderRef = input.ProviderRef
	}
	if input.Payload != nil {
		payment.ProviderPayload = input.Payload
	}

	err := models.DB.Transaction(func(tx *gorm.DB) error {
		paymentRepo := s.paymentRepo.WithTx(tx)
		orderRepo := s.orderRepo.WithTx(tx)
		productRepo := s.productRepo.WithTx(tx)

		if err := paymentRepo.Update(payment); err != nil {
			return ErrPaymentUpdateFailed
		}

		if status == constants.PaymentStatusSuccess && order.Status != constants.OrderStatusPaid {
			if !isTransitionAllowed(order.Status, constants.OrderStatusPaid) {
				return ErrOrderStatusInvalid
			}
			orderUpdates := map[string]interface{}{
				"paid_at":    now,
				"updated_at": now,
			}
			if err := orderRepo.UpdateStatus(order.ID, constants.OrderStatusPaid, orderUpdates); err != nil {
				return ErrOrderUpdateFailed
			}
			order.Status = constants.OrderStatusPaid
			order.PaidAt = &now
			order.UpdatedAt = now
			if len(order.Children) > 0 {
				for idx := range order.Children {
					child := &order.Children[idx]
					childStatus := constants.OrderStatusPaid
					if shouldMarkFulfilling(child) {
						childStatus = constants.OrderStatusFulfilling
					}
					if err := orderRepo.UpdateStatus(child.ID, childStatus, orderUpdates); err != nil {
						return ErrOrderUpdateFailed
					}
					if err := consumeManualStockByItems(productRepo, child.Items); err != nil {
						return err
					}
					child.Status = childStatus
					child.PaidAt = &now
					child.UpdatedAt = now
				}
				parentStatus := calcParentStatus(order.Children, constants.OrderStatusPaid)
				if parentStatus != "" && parentStatus != constants.OrderStatusPaid {
					if err := orderRepo.UpdateStatus(order.ID, parentStatus, map[string]interface{}{
						"updated_at": now,
					}); err != nil {
						return ErrOrderUpdateFailed
					}
					order.Status = parentStatus
				}
			} else {
				if err := consumeManualStockByItems(productRepo, order.Items); err != nil {
					return err
				}
			}
			orderPaid = true
		}

		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return returnVal, orderPaid, nil
}

func (s *PaymentService) applyProviderPayment(input CreatePaymentInput, order *models.Order, channel *models.PaymentChannel, payment *models.Payment) (err error) {
	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
	log := paymentLogger(
		"order_id", order.ID,
		"order_no", order.OrderNo,
		"payment_id", payment.ID,
		"channel_id", channel.ID,
		"provider_type", providerType,
		"channel_type", channelType,
		"interaction_mode", channel.InteractionMode,
	)
	defer func() {
		if err != nil {
			log.Errorw("payment_provider_apply_failed", "error", err)
			return
		}
		log.Infow("payment_provider_apply_success")
	}()
	switch providerType {
	case constants.PaymentProviderEpay:
		if !epay.IsSupportedChannelType(channel.ChannelType) {
			return fmt.Errorf("%w: unsupported channel_type %s", ErrPaymentChannelConfigInvalid, channel.ChannelType)
		}
		cfg, err := epay.ParseConfig(channel.ConfigJSON)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		}
		if err := epay.ValidateConfig(cfg); err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		}
		notifyURL := strings.TrimSpace(cfg.NotifyURL)
		returnURL := appendURLQuery(cfg.ReturnURL, buildOrderReturnQuery(order, "epay_return", ""))
		ctx := input.Context
		if ctx == nil {
			ctx = context.Background()
		}
		subject := buildOrderSubject(order)
		param := strconv.FormatUint(uint64(payment.ID), 10)
		result, err := epay.CreatePayment(ctx, cfg, epay.CreateInput{
			OrderNo:     order.OrderNo,
			PaymentID:   payment.ID,
			Amount:      payment.Amount.String(),
			Subject:     subject,
			ChannelType: channel.ChannelType,
			ClientIP:    strings.TrimSpace(input.ClientIP),
			NotifyURL:   notifyURL,
			ReturnURL:   returnURL,
			Param:       param,
		})
		if notifyURL == "" || returnURL == "" {
			return fmt.Errorf("%w: notify_url/return_url is required", ErrPaymentChannelConfigInvalid)
		}
		if err != nil {
			switch {
			case errors.Is(err, epay.ErrConfigInvalid), errors.Is(err, epay.ErrChannelTypeNotOK), errors.Is(err, epay.ErrSignatureGenerate):
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			case errors.Is(err, epay.ErrRequestFailed):
				return ErrPaymentGatewayRequestFailed
			case errors.Is(err, epay.ErrResponseInvalid):
				return ErrPaymentGatewayResponseInvalid
			default:
				return ErrPaymentGatewayRequestFailed
			}
		}
		payment.PayURL = result.PayURL
		payment.QRCode = result.QRCode
		if result.TradeNo != "" {
			payment.ProviderRef = result.TradeNo
		}
		if result.Raw != nil {
			payment.ProviderPayload = models.JSON(result.Raw)
		}
		payment.UpdatedAt = time.Now()
		if err := s.paymentRepo.Update(payment); err != nil {
			return ErrPaymentUpdateFailed
		}
		return nil
	case constants.PaymentProviderOfficial:
		channelType = strings.ToLower(strings.TrimSpace(channel.ChannelType))
		switch channelType {
		case constants.PaymentChannelTypePaypal:
			cfg, err := paypal.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := paypal.ValidateConfig(cfg); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			ctx := input.Context
			if ctx == nil {
				ctx = context.Background()
			}
			createResult, err := paypal.CreateOrder(ctx, cfg, paypal.CreateInput{
				OrderNo:     order.OrderNo,
				PaymentID:   payment.ID,
				Amount:      payment.Amount.String(),
				Currency:    payment.Currency,
				Description: buildOrderSubject(order),
				ReturnURL:   appendURLQuery(cfg.ReturnURL, buildOrderReturnQuery(order, "pp_return", "")),
				CancelURL:   appendURLQuery(cfg.CancelURL, buildOrderReturnQuery(order, "pp_cancel", "")),
			})
			if err != nil {
				switch {
				case errors.Is(err, paypal.ErrConfigInvalid):
					return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
				case errors.Is(err, paypal.ErrAuthFailed), errors.Is(err, paypal.ErrRequestFailed):
					return ErrPaymentGatewayRequestFailed
				case errors.Is(err, paypal.ErrResponseInvalid):
					return ErrPaymentGatewayResponseInvalid
				default:
					return ErrPaymentGatewayRequestFailed
				}
			}
			payment.PayURL = strings.TrimSpace(createResult.ApprovalURL)
			payment.QRCode = ""
			payment.Status = constants.PaymentStatusPending
			payment.ProviderRef = strings.TrimSpace(createResult.OrderID)
			if createResult.Raw != nil {
				payment.ProviderPayload = models.JSON(createResult.Raw)
			}
			payment.UpdatedAt = time.Now()
			if err := s.paymentRepo.Update(payment); err != nil {
				return ErrPaymentUpdateFailed
			}
			return nil
		case constants.PaymentChannelTypeAlipay:
			payment.Currency = "CNY"
			cfg, err := alipay.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := alipay.ValidateConfig(cfg, channel.InteractionMode); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			ctx := input.Context
			if ctx == nil {
				ctx = context.Background()
			}
			createResult, err := alipay.CreatePayment(ctx, cfg, alipay.CreateInput{
				OrderNo:        order.OrderNo,
				PaymentID:      payment.ID,
				Amount:         payment.Amount.String(),
				Subject:        buildOrderSubject(order),
				NotifyURL:      cfg.NotifyURL,
				ReturnURL:      appendURLQuery(cfg.ReturnURL, buildOrderReturnQuery(order, "alipay_return", "")),
				PassbackParams: strconv.FormatUint(uint64(payment.ID), 10),
			}, channel.InteractionMode)
			if err != nil {
				switch {
				case errors.Is(err, alipay.ErrConfigInvalid), errors.Is(err, alipay.ErrSignGenerate):
					return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
				case errors.Is(err, alipay.ErrRequestFailed):
					return ErrPaymentGatewayRequestFailed
				case errors.Is(err, alipay.ErrResponseInvalid):
					return ErrPaymentGatewayResponseInvalid
				default:
					return ErrPaymentGatewayRequestFailed
				}
			}
			payment.PayURL = strings.TrimSpace(createResult.PayURL)
			payment.QRCode = strings.TrimSpace(createResult.QRCode)
			payment.Status = constants.PaymentStatusPending
			payment.ProviderRef = pickFirstNonEmpty(strings.TrimSpace(createResult.TradeNo), strings.TrimSpace(createResult.OutTradeNo), order.OrderNo)
			if createResult.Raw != nil {
				payment.ProviderPayload = models.JSON(createResult.Raw)
			}
			payment.UpdatedAt = time.Now()
			if err := s.paymentRepo.Update(payment); err != nil {
				return ErrPaymentUpdateFailed
			}
			return nil
		case constants.PaymentChannelTypeWechat:
			payment.Currency = "CNY"
			cfg, err := wechatpay.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := wechatpay.ValidateConfig(cfg, channel.InteractionMode); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			cfgForCreate := *cfg
			cfgForCreate.H5RedirectURL = appendURLQuery(cfg.H5RedirectURL, buildOrderReturnQuery(order, "wechat_return", ""))
			ctx := input.Context
			if ctx == nil {
				ctx = context.Background()
			}
			createResult, err := wechatpay.CreatePayment(ctx, &cfgForCreate, wechatpay.CreateInput{
				OrderNo:     order.OrderNo,
				PaymentID:   payment.ID,
				Amount:      payment.Amount.String(),
				Currency:    payment.Currency,
				Description: buildOrderSubject(order),
				ClientIP:    strings.TrimSpace(input.ClientIP),
				NotifyURL:   cfg.NotifyURL,
			}, channel.InteractionMode)
			if err != nil {
				switch {
				case errors.Is(err, wechatpay.ErrConfigInvalid):
					return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
				case errors.Is(err, wechatpay.ErrRequestFailed):
					return ErrPaymentGatewayRequestFailed
				case errors.Is(err, wechatpay.ErrResponseInvalid):
					return ErrPaymentGatewayResponseInvalid
				default:
					return ErrPaymentGatewayRequestFailed
				}
			}
			payment.PayURL = strings.TrimSpace(createResult.PayURL)
			payment.QRCode = strings.TrimSpace(createResult.QRCode)
			payment.Status = constants.PaymentStatusPending
			payment.ProviderRef = pickFirstNonEmpty(strings.TrimSpace(payment.ProviderRef), order.OrderNo)
			if createResult.Raw != nil {
				payment.ProviderPayload = models.JSON(createResult.Raw)
			}
			payment.UpdatedAt = time.Now()
			if err := s.paymentRepo.Update(payment); err != nil {
				return ErrPaymentUpdateFailed
			}
			return nil
		case constants.PaymentChannelTypeStripe:
			cfg, err := stripe.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := stripe.ValidateConfig(cfg); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			ctx := input.Context
			if ctx == nil {
				ctx = context.Background()
			}
			createResult, err := stripe.CreatePayment(ctx, cfg, stripe.CreateInput{
				OrderNo:     order.OrderNo,
				PaymentID:   payment.ID,
				Amount:      payment.Amount.String(),
				Currency:    payment.Currency,
				Description: buildOrderSubject(order),
				SuccessURL:  appendURLQuery(cfg.SuccessURL, buildOrderReturnQuery(order, "stripe_return", "{CHECKOUT_SESSION_ID}")),
				CancelURL:   appendURLQuery(cfg.CancelURL, buildOrderReturnQuery(order, "stripe_cancel", "")),
			})
			if err != nil {
				return mapStripeGatewayError(err)
			}
			payment.PayURL = strings.TrimSpace(createResult.URL)
			payment.QRCode = ""
			payment.Status = constants.PaymentStatusPending
			payment.ProviderRef = pickFirstNonEmpty(strings.TrimSpace(createResult.SessionID), strings.TrimSpace(createResult.PaymentIntentID), order.OrderNo)
			if createResult.Raw != nil {
				payment.ProviderPayload = models.JSON(createResult.Raw)
			}
			payment.UpdatedAt = time.Now()
			if err := s.paymentRepo.Update(payment); err != nil {
				return ErrPaymentUpdateFailed
			}
			return nil
		default:
			return ErrPaymentProviderNotSupported
		}
	default:
		return ErrPaymentProviderNotSupported
	}
}

// ValidateChannel 校验支付渠道配置
func (s *PaymentService) ValidateChannel(channel *models.PaymentChannel) error {
	if channel == nil {
		return ErrPaymentChannelConfigInvalid
	}
	feeRate := channel.FeeRate.Decimal.Round(2)
	if feeRate.LessThan(decimal.Zero) || feeRate.GreaterThan(decimal.NewFromInt(100)) {
		return ErrPaymentChannelConfigInvalid
	}
	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	switch providerType {
	case constants.PaymentProviderEpay:
		if !epay.IsSupportedChannelType(channel.ChannelType) {
			return fmt.Errorf("%w: unsupported channel_type %s", ErrPaymentChannelConfigInvalid, channel.ChannelType)
		}
		cfg, err := epay.ParseConfig(channel.ConfigJSON)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		}
		if err := epay.ValidateConfig(cfg); err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		}
		return nil
	case constants.PaymentProviderOfficial:
		channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
		switch channelType {
		case constants.PaymentChannelTypePaypal:
			if strings.ToLower(strings.TrimSpace(channel.InteractionMode)) != constants.PaymentInteractionRedirect {
				return ErrPaymentChannelConfigInvalid
			}
			cfg, err := paypal.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := paypal.ValidateConfig(cfg); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			return nil
		case constants.PaymentChannelTypeAlipay:
			cfg, err := alipay.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := alipay.ValidateConfig(cfg, channel.InteractionMode); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			return nil
		case constants.PaymentChannelTypeWechat:
			cfg, err := wechatpay.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := wechatpay.ValidateConfig(cfg, channel.InteractionMode); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			return nil
		case constants.PaymentChannelTypeStripe:
			if strings.ToLower(strings.TrimSpace(channel.InteractionMode)) != constants.PaymentInteractionRedirect {
				return ErrPaymentChannelConfigInvalid
			}
			cfg, err := stripe.ParseConfig(channel.ConfigJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			if err := stripe.ValidateConfig(cfg); err != nil {
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			}
			return nil
		default:
			return ErrPaymentProviderNotSupported
		}
	default:
		return ErrPaymentProviderNotSupported
	}
}

func mapPaypalStatus(status string) (string, bool) {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "COMPLETED":
		return constants.PaymentStatusSuccess, true
	case "PENDING", "APPROVED", "CREATED", "SAVED":
		return constants.PaymentStatusPending, true
	case "DECLINED", "DENIED", "FAILED", "VOIDED":
		return constants.PaymentStatusFailed, true
	default:
		return "", false
	}
}

func pickFirstNonEmpty(values ...string) string {
	for _, val := range values {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendURLQuery(rawURL string, params map[string]string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	for key, value := range params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func buildOrderReturnQuery(order *models.Order, marker string, sessionID string) map[string]string {
	params := map[string]string{}
	if order != nil {
		if orderNo := strings.TrimSpace(order.OrderNo); orderNo != "" {
			params["order_no"] = orderNo
		}
		if order.UserID == 0 {
			params["guest"] = "1"
		}
	}
	if marker = strings.TrimSpace(marker); marker != "" {
		params[marker] = "1"
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		params["session_id"] = sessionID
	}
	return params
}

func shouldUseCNYPaymentCurrency(channel *models.PaymentChannel) bool {
	if channel == nil {
		return false
	}
	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	if providerType != constants.PaymentProviderOfficial {
		return false
	}
	channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
	return channelType == constants.PaymentChannelTypeWechat || channelType == constants.PaymentChannelTypeAlipay
}

func normalizePaymentStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func isPaymentStatusValid(status string) bool {
	switch status {
	case constants.PaymentStatusInitiated, constants.PaymentStatusPending, constants.PaymentStatusSuccess, constants.PaymentStatusFailed, constants.PaymentStatusExpired:
		return true
	default:
		return false
	}
}

func shouldAutoFulfill(order *models.Order) bool {
	if order == nil || len(order.Items) == 0 {
		return false
	}
	for _, item := range order.Items {
		if strings.TrimSpace(item.FulfillmentType) != constants.FulfillmentTypeAuto {
			return false
		}
	}
	return true
}

func buildOrderSubject(order *models.Order) string {
	if order == nil {
		return ""
	}
	if len(order.Items) > 0 {
		title := pickOrderItemTitle(order.Items[0].TitleJSON)
		if title != "" {
			return title
		}
	}
	return order.OrderNo
}

func pickOrderItemTitle(title models.JSON) string {
	if title == nil {
		return ""
	}
	for _, key := range []string{"zh-CN", "zh-TW", "en-US"} {
		if val, ok := title[key]; ok {
			if str, ok := val.(string); ok && strings.TrimSpace(str) != "" {
				return strings.TrimSpace(str)
			}
		}
	}
	for _, val := range title {
		if str, ok := val.(string); ok && strings.TrimSpace(str) != "" {
			return strings.TrimSpace(str)
		}
	}
	return ""
}
