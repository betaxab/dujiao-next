package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
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
	"github.com/dujiao-next/internal/payment/epusdt"
	"github.com/dujiao-next/internal/payment/paypal"
	"github.com/dujiao-next/internal/payment/stripe"
	"github.com/dujiao-next/internal/payment/wechatpay"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/repository"

	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PaymentService 支付服务
type PaymentService struct {
	orderRepo   repository.OrderRepository
	productRepo repository.ProductRepository
	paymentRepo repository.PaymentRepository
	channelRepo repository.PaymentChannelRepository
	walletRepo  repository.WalletRepository
	queueClient *queue.Client
	walletSvc   *WalletService
}

// NewPaymentService 创建支付服务
func NewPaymentService(orderRepo repository.OrderRepository, productRepo repository.ProductRepository, paymentRepo repository.PaymentRepository, channelRepo repository.PaymentChannelRepository, walletRepo repository.WalletRepository, queueClient *queue.Client, walletSvc *WalletService) *PaymentService {
	return &PaymentService{
		orderRepo:   orderRepo,
		productRepo: productRepo,
		paymentRepo: paymentRepo,
		channelRepo: channelRepo,
		walletRepo:  walletRepo,
		queueClient: queueClient,
		walletSvc:   walletSvc,
	}
}

// CreatePaymentInput 创建支付请求
type CreatePaymentInput struct {
	OrderID    uint
	ChannelID  uint
	UseBalance bool
	ClientIP   string
	Context    context.Context
}

// CreatePaymentResult 创建支付结果
type CreatePaymentResult struct {
	Payment          *models.Payment
	Channel          *models.PaymentChannel
	OrderPaid        bool
	WalletPaidAmount models.Money
	OnlinePayAmount  models.Money
}

// CreateWalletRechargePaymentInput 创建钱包充值支付请求
type CreateWalletRechargePaymentInput struct {
	UserID    uint
	ChannelID uint
	Amount    models.Money
	Currency  string
	Remark    string
	ClientIP  string
	Context   context.Context
}

// CreateWalletRechargePaymentResult 创建钱包充值支付结果
type CreateWalletRechargePaymentResult struct {
	Recharge *models.WalletRechargeOrder
	Payment  *models.Payment
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
	if input.OrderID == 0 {
		return nil, ErrPaymentInvalid
	}

	log := paymentLogger(
		"order_id", input.OrderID,
		"channel_id", input.ChannelID,
	)

	var payment *models.Payment
	var order *models.Order
	var channel *models.PaymentChannel
	feeRate := decimal.Zero
	reusedPending := false
	orderPaidByWallet := false
	now := time.Now()

	err := models.DB.Transaction(func(tx *gorm.DB) error {
		var lockedOrder models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Items").
			Preload("Children").
			Preload("Children.Items").
			First(&lockedOrder, input.OrderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return ErrOrderFetchFailed
		}
		if lockedOrder.ParentID != nil {
			return ErrPaymentInvalid
		}
		if lockedOrder.Status != constants.OrderStatusPendingPayment {
			return ErrOrderStatusInvalid
		}
		if lockedOrder.ExpiresAt != nil && !lockedOrder.ExpiresAt.After(time.Now()) {
			return ErrOrderStatusInvalid
		}

		paymentRepo := s.paymentRepo.WithTx(tx)
		channelRepo := s.channelRepo.WithTx(tx)
		if input.ChannelID != 0 {
			if channel == nil {
				// 事务内必须使用 tx 绑定仓储，避免在单连接池下发生自锁等待。
				resolvedChannel, err := channelRepo.GetByID(input.ChannelID)
				if err != nil {
					return err
				}
				if resolvedChannel == nil {
					return ErrPaymentChannelNotFound
				}
				if !resolvedChannel.IsActive {
					return ErrPaymentChannelInactive
				}
				resolvedFeeRate := resolvedChannel.FeeRate.Decimal.Round(2)
				if resolvedFeeRate.LessThan(decimal.Zero) || resolvedFeeRate.GreaterThan(decimal.NewFromInt(100)) {
					return ErrPaymentChannelConfigInvalid
				}
				channel = resolvedChannel
				feeRate = resolvedFeeRate
			}

			existing, err := paymentRepo.GetLatestPendingByOrderChannel(lockedOrder.ID, channel.ID, time.Now())
			if err != nil {
				return ErrPaymentCreateFailed
			}
			if existing != nil && hasProviderResult(existing) {
				reusedPending = true
				payment = existing
				order = &lockedOrder
				return nil
			}
		}

		if s.walletSvc != nil {
			if input.UseBalance {
				if _, err := s.walletSvc.ApplyOrderBalance(tx, &lockedOrder, true); err != nil {
					return err
				}
			} else if lockedOrder.WalletPaidAmount.Decimal.GreaterThan(decimal.Zero) {
				if _, err := s.walletSvc.ReleaseOrderBalance(tx, &lockedOrder, constants.WalletTxnTypeOrderRefund, "用户改为在线支付，退回余额"); err != nil {
					return err
				}
			}
		}

		onlineAmount := normalizeOrderAmount(lockedOrder.TotalAmount.Decimal.Sub(lockedOrder.WalletPaidAmount.Decimal))
		if onlineAmount.LessThanOrEqual(decimal.Zero) {
			if err := s.markOrderPaid(tx, &lockedOrder, time.Now()); err != nil {
				return err
			}
			orderPaidByWallet = true
			order = &lockedOrder
			return nil
		}
		if channel == nil {
			return ErrPaymentInvalid
		}

		feeAmount := decimal.Zero
		if feeRate.GreaterThan(decimal.Zero) {
			feeAmount = onlineAmount.Mul(feeRate).Div(decimal.NewFromInt(100)).Round(2)
		}
		payableAmount := onlineAmount.Add(feeAmount).Round(2)
		payment = &models.Payment{
			OrderID:         lockedOrder.ID,
			ChannelID:       channel.ID,
			ProviderType:    channel.ProviderType,
			ChannelType:     channel.ChannelType,
			InteractionMode: channel.InteractionMode,
			Amount:          models.NewMoneyFromDecimal(payableAmount),
			FeeRate:         models.NewMoneyFromDecimal(feeRate),
			FeeAmount:       models.NewMoneyFromDecimal(feeAmount),
			Currency:        lockedOrder.Currency,
			Status:          constants.PaymentStatusInitiated,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if shouldUseCNYPaymentCurrency(channel) {
			payment.Currency = "CNY"
		}

		if err := paymentRepo.Create(payment); err != nil {
			return ErrPaymentCreateFailed
		}
		if err := tx.Model(&models.Order{}).Where("id = ?", lockedOrder.ID).Updates(map[string]interface{}{
			"online_paid_amount": models.NewMoneyFromDecimal(onlineAmount),
			"updated_at":         time.Now(),
		}).Error; err != nil {
			return ErrOrderUpdateFailed
		}
		lockedOrder.OnlinePaidAmount = models.NewMoneyFromDecimal(onlineAmount)
		lockedOrder.UpdatedAt = time.Now()
		order = &lockedOrder
		return nil
	})
	if err != nil {
		return nil, err
	}

	if order == nil {
		return nil, ErrOrderFetchFailed
	}

	if reusedPending {
		log.Infow("payment_create_reuse_pending",
			"payment_id", payment.ID,
			"provider_type", payment.ProviderType,
			"channel_type", payment.ChannelType,
		)
		return &CreatePaymentResult{
			Payment:          payment,
			Channel:          channel,
			WalletPaidAmount: order.WalletPaidAmount,
			OnlinePayAmount:  order.OnlinePaidAmount,
		}, nil
	}

	if orderPaidByWallet {
		s.enqueueOrderPaidAsync(order, log)
		return &CreatePaymentResult{
			Payment:          nil,
			Channel:          nil,
			OrderPaid:        true,
			WalletPaidAmount: order.WalletPaidAmount,
			OnlinePayAmount:  models.NewMoneyFromDecimal(decimal.Zero),
		}, nil
	}

	if payment == nil {
		return nil, ErrPaymentCreateFailed
	}

	if err := s.applyProviderPayment(input, order, channel, payment); err != nil {
		rollbackErr := models.DB.Transaction(func(tx *gorm.DB) error {
			paymentRepo := s.paymentRepo.WithTx(tx)
			payment.Status = constants.PaymentStatusFailed
			payment.UpdatedAt = time.Now()
			if updateErr := paymentRepo.Update(payment); updateErr != nil {
				return updateErr
			}
			if s.walletSvc == nil {
				return nil
			}
			var lockedOrder models.Order
			if findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedOrder, order.ID).Error; findErr != nil {
				return findErr
			}
			_, refundErr := s.walletSvc.ReleaseOrderBalance(tx, &lockedOrder, constants.WalletTxnTypeOrderRefund, "在线支付创建失败，退回余额")
			return refundErr
		})
		if rollbackErr != nil {
			log.Errorw("payment_create_provider_failed_with_rollback_error",
				"payment_id", payment.ID,
				"order_id", order.ID,
				"provider_type", payment.ProviderType,
				"channel_type", payment.ChannelType,
				"provider_error", err,
				"rollback_error", rollbackErr,
			)
		} else {
			log.Errorw("payment_create_provider_failed",
				"payment_id", payment.ID,
				"provider_type", payment.ProviderType,
				"channel_type", payment.ChannelType,
				"error", err,
			)
		}
		return nil, err
	}

	log.Infow("payment_create_success",
		"payment_id", payment.ID,
		"provider_type", payment.ProviderType,
		"channel_type", payment.ChannelType,
		"interaction_mode", payment.InteractionMode,
		"currency", payment.Currency,
		"amount", payment.Amount.String(),
		"wallet_paid_amount", order.WalletPaidAmount.String(),
		"online_pay_amount", order.OnlinePaidAmount.String(),
	)

	return &CreatePaymentResult{
		Payment:          payment,
		Channel:          channel,
		WalletPaidAmount: order.WalletPaidAmount,
		OnlinePayAmount:  order.OnlinePaidAmount,
	}, nil
}

// CreateWalletRechargePayment 创建钱包充值支付单
func (s *PaymentService) CreateWalletRechargePayment(input CreateWalletRechargePaymentInput) (*CreateWalletRechargePaymentResult, error) {
	if input.UserID == 0 || input.ChannelID == 0 {
		return nil, ErrPaymentInvalid
	}
	amount := input.Amount.Decimal.Round(2)
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, ErrWalletInvalidAmount
	}
	if s.walletRepo == nil {
		return nil, ErrPaymentCreateFailed
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

	feeRate := channel.FeeRate.Decimal.Round(2)
	if feeRate.LessThan(decimal.Zero) || feeRate.GreaterThan(decimal.NewFromInt(100)) {
		return nil, ErrPaymentChannelConfigInvalid
	}
	feeAmount := decimal.Zero
	if feeRate.GreaterThan(decimal.Zero) {
		feeAmount = amount.Mul(feeRate).Div(decimal.NewFromInt(100)).Round(2)
	}
	payableAmount := amount.Add(feeAmount).Round(2)
	currency := normalizeWalletCurrency(input.Currency)
	if shouldUseCNYPaymentCurrency(channel) {
		currency = "CNY"
	}
	now := time.Now()

	var payment *models.Payment
	var recharge *models.WalletRechargeOrder
	err = models.DB.Transaction(func(tx *gorm.DB) error {
		rechargeNo := generateWalletRechargeNo()
		paymentRepo := s.paymentRepo.WithTx(tx)
		payment = &models.Payment{
			OrderID:         0,
			ChannelID:       channel.ID,
			ProviderType:    channel.ProviderType,
			ChannelType:     channel.ChannelType,
			InteractionMode: channel.InteractionMode,
			Amount:          models.NewMoneyFromDecimal(payableAmount),
			FeeRate:         models.NewMoneyFromDecimal(feeRate),
			FeeAmount:       models.NewMoneyFromDecimal(feeAmount),
			Currency:        currency,
			Status:          constants.PaymentStatusInitiated,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := paymentRepo.Create(payment); err != nil {
			return ErrPaymentCreateFailed
		}

		rechargeRepo := s.walletRepo.WithTx(tx)
		recharge = &models.WalletRechargeOrder{
			RechargeNo:      rechargeNo,
			UserID:          input.UserID,
			PaymentID:       payment.ID,
			ChannelID:       channel.ID,
			ProviderType:    channel.ProviderType,
			ChannelType:     channel.ChannelType,
			InteractionMode: channel.InteractionMode,
			Amount:          models.NewMoneyFromDecimal(amount),
			PayableAmount:   models.NewMoneyFromDecimal(payableAmount),
			FeeRate:         models.NewMoneyFromDecimal(feeRate),
			FeeAmount:       models.NewMoneyFromDecimal(feeAmount),
			Currency:        currency,
			Status:          constants.WalletRechargeStatusPending,
			Remark:          cleanWalletRemark(input.Remark, "余额充值"),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := rechargeRepo.CreateRechargeOrder(recharge); err != nil {
			return ErrPaymentCreateFailed
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if payment == nil || recharge == nil {
		return nil, ErrPaymentCreateFailed
	}

	// 复用支付网关下单逻辑，使用充值单号作为业务单号。
	virtualOrder := &models.Order{
		OrderNo: recharge.RechargeNo,
		UserID:  recharge.UserID,
	}
	if err := s.applyProviderPayment(CreatePaymentInput{
		ChannelID: input.ChannelID,
		ClientIP:  input.ClientIP,
		Context:   input.Context,
	}, virtualOrder, channel, payment); err != nil {
		_ = models.DB.Transaction(func(tx *gorm.DB) error {
			rechargeRepo := s.walletRepo.WithTx(tx)
			paymentRepo := s.paymentRepo.WithTx(tx)
			failedAt := time.Now()
			payment.Status = constants.PaymentStatusFailed
			payment.UpdatedAt = failedAt
			if updateErr := paymentRepo.Update(payment); updateErr != nil {
				return updateErr
			}
			lockedRecharge, getErr := rechargeRepo.GetRechargeOrderByPaymentIDForUpdate(payment.ID)
			if getErr != nil || lockedRecharge == nil {
				return getErr
			}
			lockedRecharge.Status = constants.WalletRechargeStatusFailed
			lockedRecharge.UpdatedAt = failedAt
			return rechargeRepo.UpdateRechargeOrder(lockedRecharge)
		})
		return nil, err
	}

	reloadedRecharge, err := s.walletRepo.GetRechargeOrderByPaymentID(payment.ID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if reloadedRecharge != nil {
		recharge = reloadedRecharge
	}
	return &CreateWalletRechargePaymentResult{
		Recharge: recharge,
		Payment:  payment,
	}, nil
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
	if payment.OrderID == 0 {
		return s.handleWalletRechargeCallback(payment, status, input)
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
	if orderPaid {
		s.enqueueOrderPaidAsync(order, log)
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

func (s *PaymentService) handleWalletRechargeCallback(payment *models.Payment, status string, input PaymentCallbackInput) (*models.Payment, error) {
	if s.walletRepo == nil {
		return nil, ErrPaymentUpdateFailed
	}
	recharge, err := s.walletRepo.GetRechargeOrderByPaymentID(payment.ID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if recharge == nil {
		return nil, ErrWalletRechargeNotFound
	}

	if input.ChannelID != 0 && input.ChannelID != payment.ChannelID {
		return nil, ErrPaymentInvalid
	}
	if input.OrderNo != "" && input.OrderNo != recharge.RechargeNo {
		return nil, ErrPaymentInvalid
	}
	if input.Currency != "" && strings.ToUpper(strings.TrimSpace(input.Currency)) != strings.ToUpper(strings.TrimSpace(payment.Currency)) {
		return nil, ErrPaymentCurrencyMismatch
	}
	if !input.Amount.Decimal.IsZero() && input.Amount.Decimal.Cmp(payment.Amount.Decimal) != 0 {
		return nil, ErrPaymentAmountMismatch
	}

	// 幂等处理：已成功状态仅更新回调元信息。
	if payment.Status == constants.PaymentStatusSuccess {
		return s.updateCallbackMeta(payment, constants.PaymentStatusSuccess, input)
	}
	if payment.Status == status {
		return s.updateCallbackMeta(payment, status, input)
	}

	now := time.Now()
	updated, err := s.applyWalletRechargePaymentUpdate(payment, status, input, now)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *PaymentService) applyWalletRechargePaymentUpdate(payment *models.Payment, status string, input PaymentCallbackInput, now time.Time) (*models.Payment, error) {
	paymentVal := payment

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
		rechargeRepo := s.walletRepo.WithTx(tx)

		if err := paymentRepo.Update(payment); err != nil {
			return ErrPaymentUpdateFailed
		}
		recharge, err := rechargeRepo.GetRechargeOrderByPaymentIDForUpdate(payment.ID)
		if err != nil {
			return ErrPaymentUpdateFailed
		}
		if recharge == nil {
			return ErrWalletRechargeNotFound
		}
		if recharge.Status == constants.WalletRechargeStatusSuccess {
			return nil
		}

		switch status {
		case constants.PaymentStatusSuccess:
			if s.walletSvc == nil {
				return ErrWalletAccountNotFound
			}
			if _, err := s.walletSvc.ApplyRechargePayment(tx, recharge); err != nil {
				return err
			}
			recharge.Status = constants.WalletRechargeStatusSuccess
			paidAt := now
			if payment.PaidAt != nil {
				paidAt = *payment.PaidAt
			}
			recharge.PaidAt = &paidAt
		case constants.PaymentStatusFailed:
			recharge.Status = constants.WalletRechargeStatusFailed
		case constants.PaymentStatusExpired:
			recharge.Status = constants.WalletRechargeStatusExpired
		default:
			recharge.Status = constants.WalletRechargeStatusPending
		}
		recharge.UpdatedAt = now
		if err := rechargeRepo.UpdateRechargeOrder(recharge); err != nil {
			return ErrPaymentUpdateFailed
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paymentVal, nil
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

		if err := paymentRepo.Update(payment); err != nil {
			return ErrPaymentUpdateFailed
		}

		if status == constants.PaymentStatusSuccess && order.Status != constants.OrderStatusPaid {
			if err := s.markOrderPaid(tx, order, now); err != nil {
				return err
			}
			orderPaid = true
		}
		if (status == constants.PaymentStatusFailed || status == constants.PaymentStatusExpired) && order.Status == constants.OrderStatusPendingPayment && s.walletSvc != nil {
			if _, err := s.walletSvc.ReleaseOrderBalance(tx, order, constants.WalletTxnTypeOrderRefund, "在线支付失败，退回余额"); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return returnVal, orderPaid, nil
}

// markOrderPaid 在事务内将订单更新为已支付并处理库存
func (s *PaymentService) markOrderPaid(tx *gorm.DB, order *models.Order, now time.Time) error {
	if order == nil {
		return ErrOrderNotFound
	}
	if !isTransitionAllowed(order.Status, constants.OrderStatusPaid) {
		return ErrOrderStatusInvalid
	}
	orderRepo := s.orderRepo.WithTx(tx)
	productRepo := s.productRepo.WithTx(tx)

	onlineAmount := normalizeOrderAmount(order.TotalAmount.Decimal.Sub(order.WalletPaidAmount.Decimal))
	orderUpdates := map[string]interface{}{
		"paid_at":            now,
		"online_paid_amount": models.NewMoneyFromDecimal(onlineAmount),
		"updated_at":         now,
	}
	if err := orderRepo.UpdateStatus(order.ID, constants.OrderStatusPaid, orderUpdates); err != nil {
		return ErrOrderUpdateFailed
	}
	order.Status = constants.OrderStatusPaid
	order.PaidAt = &now
	order.OnlinePaidAmount = models.NewMoneyFromDecimal(onlineAmount)
	order.UpdatedAt = now

	if len(order.Children) > 0 {
		for idx := range order.Children {
			child := &order.Children[idx]
			childStatus := constants.OrderStatusPaid
			if shouldMarkFulfilling(child) {
				childStatus = constants.OrderStatusFulfilling
			}
			if err := orderRepo.UpdateStatus(child.ID, childStatus, map[string]interface{}{
				"paid_at":    now,
				"updated_at": now,
			}); err != nil {
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
				"online_paid_amount": models.NewMoneyFromDecimal(onlineAmount),
				"updated_at":         now,
			}); err != nil {
				return ErrOrderUpdateFailed
			}
			order.Status = parentStatus
		}
		return nil
	}

	if err := consumeManualStockByItems(productRepo, order.Items); err != nil {
		return err
	}
	return nil
}

func (s *PaymentService) enqueueOrderPaidAsync(order *models.Order, log *zap.SugaredLogger) {
	if s.queueClient == nil || order == nil {
		return
	}
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
		return
	}
	if shouldAutoFulfill(order) {
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
	case constants.PaymentProviderEpusdt:
		cfg, err := epusdt.ParseConfig(channel.ConfigJSON)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		}
		// 如果配置中没有指定 trade_type，根据 channel_type 自动设置
		if strings.TrimSpace(cfg.TradeType) == "" {
			cfg.TradeType = epusdt.ResolveTradeType(channel.ChannelType)
		}
		if err := epusdt.ValidateConfig(cfg); err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		}
		notifyURL := strings.TrimSpace(cfg.NotifyURL)
		returnURL := strings.TrimSpace(cfg.ReturnURL)
		if notifyURL == "" || returnURL == "" {
			return fmt.Errorf("%w: notify_url/return_url is required", ErrPaymentChannelConfigInvalid)
		}
		ctx := input.Context
		if ctx == nil {
			ctx = context.Background()
		}
		subject := buildOrderSubject(order)
		result, err := epusdt.CreatePayment(ctx, cfg, epusdt.CreateInput{
			OrderNo:   order.OrderNo,
			PaymentID: payment.ID,
			Amount:    payment.Amount.String(),
			Name:      subject,
			NotifyURL: notifyURL,
			ReturnURL: returnURL,
		})
		if err != nil {
			switch {
			case errors.Is(err, epusdt.ErrConfigInvalid):
				return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
			case errors.Is(err, epusdt.ErrRequestFailed):
				return ErrPaymentGatewayRequestFailed
			case errors.Is(err, epusdt.ErrResponseInvalid):
				return ErrPaymentGatewayResponseInvalid
			default:
				return ErrPaymentGatewayRequestFailed
			}
		}
		payment.PayURL = result.PaymentURL
		payment.QRCode = result.PaymentURL
		if result.TradeID != "" {
			payment.ProviderRef = result.TradeID
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
	case constants.PaymentProviderEpusdt:
		if !epusdt.IsSupportedChannelType(channel.ChannelType) {
			return fmt.Errorf("%w: unsupported channel_type %s", ErrPaymentChannelConfigInvalid, channel.ChannelType)
		}
		if strings.ToLower(strings.TrimSpace(channel.InteractionMode)) != constants.PaymentInteractionRedirect &&
			strings.ToLower(strings.TrimSpace(channel.InteractionMode)) != constants.PaymentInteractionQR {
			return ErrPaymentChannelConfigInvalid
		}
		cfg, err := epusdt.ParseConfig(channel.ConfigJSON)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
		}
		if err := epusdt.ValidateConfig(cfg); err != nil {
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

func generateWalletRechargeNo() string {
	now := time.Now().Format("20060102150405")
	return fmt.Sprintf("WR%s%s", now, randNumericCode(6))
}

func randNumericCode(length int) string {
	if length <= 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			b.WriteString("0")
			continue
		}
		b.WriteString(strconv.FormatInt(n.Int64(), 10))
	}
	return b.String()
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
