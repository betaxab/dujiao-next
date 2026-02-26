package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
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

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PaymentService 支付服务
type PaymentService struct {
	orderRepo       repository.OrderRepository
	productRepo     repository.ProductRepository
	productSKURepo  repository.ProductSKURepository
	paymentRepo     repository.PaymentRepository
	channelRepo     repository.PaymentChannelRepository
	walletRepo      repository.WalletRepository
	queueClient     *queue.Client
	walletSvc       *WalletService
	notificationSvc *NotificationService
}

// NewPaymentService 创建支付服务
func NewPaymentService(orderRepo repository.OrderRepository, productRepo repository.ProductRepository, productSKURepo repository.ProductSKURepository, paymentRepo repository.PaymentRepository, channelRepo repository.PaymentChannelRepository, walletRepo repository.WalletRepository, queueClient *queue.Client, walletSvc *WalletService, notificationSvc *NotificationService) *PaymentService {
	return &PaymentService{
		orderRepo:       orderRepo,
		productRepo:     productRepo,
		productSKURepo:  productSKURepo,
		paymentRepo:     paymentRepo,
		channelRepo:     channelRepo,
		walletRepo:      walletRepo,
		queueClient:     queueClient,
		walletSvc:       walletSvc,
		notificationSvc: notificationSvc,
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
			walletPaidAmount := normalizeOrderAmount(lockedOrder.WalletPaidAmount.Decimal)
			paidAt := time.Now()
			payment = &models.Payment{
				OrderID:         lockedOrder.ID,
				ChannelID:       0,
				ProviderType:    constants.PaymentProviderWallet,
				ChannelType:     constants.PaymentChannelTypeBalance,
				InteractionMode: constants.PaymentInteractionBalance,
				Amount:          models.NewMoneyFromDecimal(walletPaidAmount),
				FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
				FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
				Currency:        lockedOrder.Currency,
				Status:          constants.PaymentStatusSuccess,
				CreatedAt:       paidAt,
				UpdatedAt:       paidAt,
				PaidAt:          &paidAt,
			}
			if err := paymentRepo.Create(payment); err != nil {
				return ErrPaymentCreateFailed
			}
			if err := s.markOrderPaid(tx, &lockedOrder, paidAt); err != nil {
				return err
			}
			orderPaidByWallet = true
			order = &lockedOrder
			return nil
		}
		if channel == nil {
			return ErrPaymentInvalid
		}
		if err := validatePaymentCurrencyForChannel(lockedOrder.Currency, channel); err != nil {
			return err
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
		log.Infow("payment_create_wallet_success",
			"payment_id", payment.ID,
			"provider_type", payment.ProviderType,
			"channel_type", payment.ChannelType,
			"interaction_mode", payment.InteractionMode,
			"currency", payment.Currency,
			"amount", payment.Amount.String(),
			"wallet_paid_amount", order.WalletPaidAmount.String(),
			"online_pay_amount", order.OnlinePaidAmount.String(),
		)
		s.enqueueOrderPaidAsync(order, payment, log)
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
	if err := validatePaymentCurrencyForChannel(currency, channel); err != nil {
		return nil, err
	}
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

func validatePaymentCurrencyForChannel(currency string, channel *models.PaymentChannel) error {
	normalized := strings.ToUpper(strings.TrimSpace(currency))
	if !settingCurrencyCodePattern.MatchString(normalized) {
		return ErrPaymentCurrencyMismatch
	}
	if shouldUseCNYPaymentCurrency(channel) && normalized != constants.SiteCurrencyDefault {
		return ErrPaymentCurrencyMismatch
	}
	return nil
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
	for _, key := range constants.SupportedLocales {
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
