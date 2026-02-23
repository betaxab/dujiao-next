package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/repository"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// OrderService 订单服务
type OrderService struct {
	orderRepo       repository.OrderRepository
	productRepo     repository.ProductRepository
	productSKURepo  repository.ProductSKURepository
	cardSecretRepo  repository.CardSecretRepository
	couponRepo      repository.CouponRepository
	couponUsageRepo repository.CouponUsageRepository
	promotionRepo   repository.PromotionRepository
	queueClient     *queue.Client
	settingService  *SettingService
	walletService   *WalletService
	expireMinutes   int
}

// NewOrderService 创建订单服务
func NewOrderService(orderRepo repository.OrderRepository, productRepo repository.ProductRepository, productSKURepo repository.ProductSKURepository, cardSecretRepo repository.CardSecretRepository, couponRepo repository.CouponRepository, couponUsageRepo repository.CouponUsageRepository, promotionRepo repository.PromotionRepository, queueClient *queue.Client, settingService *SettingService, walletService *WalletService, expireMinutes int) *OrderService {
	return &OrderService{
		orderRepo:       orderRepo,
		productRepo:     productRepo,
		productSKURepo:  productSKURepo,
		cardSecretRepo:  cardSecretRepo,
		couponRepo:      couponRepo,
		couponUsageRepo: couponUsageRepo,
		promotionRepo:   promotionRepo,
		queueClient:     queueClient,
		settingService:  settingService,
		walletService:   walletService,
		expireMinutes:   expireMinutes,
	}
}

// CreateOrderInput 创建订单输入
type CreateOrderInput struct {
	UserID         uint
	Items          []CreateOrderItem
	CouponCode     string
	ClientIP       string
	ManualFormData map[string]models.JSON
}

// CreateGuestOrderInput 游客创建订单输入
type CreateGuestOrderInput struct {
	Email          string
	OrderPassword  string
	Locale         string
	Items          []CreateOrderItem
	CouponCode     string
	ClientIP       string
	ManualFormData map[string]models.JSON
}

// CreateOrderItem 创建订单项输入
type CreateOrderItem struct {
	ProductID       uint
	SKUID           uint
	Quantity        int
	FulfillmentType string
}

// childOrderPlan 子订单计划数据
type childOrderPlan struct {
	Product           *models.Product
	SKU               *models.ProductSKU
	Item              models.OrderItem
	TotalAmount       decimal.Decimal
	PromotionDiscount decimal.Decimal
	CouponDiscount    decimal.Decimal
	Currency          string
}

var allowedTransitions = map[string]map[string]bool{
	constants.OrderStatusPendingPayment: {
		constants.OrderStatusPaid:     true,
		constants.OrderStatusCanceled: true,
	},
	constants.OrderStatusPaid: {
		constants.OrderStatusFulfilling:         true,
		constants.OrderStatusPartiallyDelivered: true,
		constants.OrderStatusDelivered:          true,
	},
	constants.OrderStatusFulfilling: {
		constants.OrderStatusPartiallyDelivered: true,
		constants.OrderStatusDelivered:          true,
	},
	constants.OrderStatusPartiallyDelivered: {
		constants.OrderStatusDelivered: true,
		constants.OrderStatusCompleted: true,
	},
	constants.OrderStatusDelivered: {
		constants.OrderStatusCompleted: true,
	},
}

// CreateOrder 创建订单
func (s *OrderService) CreateOrder(input CreateOrderInput) (*models.Order, error) {
	if input.UserID == 0 {
		return nil, ErrInvalidOrderItem
	}
	return s.createOrder(orderCreateParams{
		UserID:         input.UserID,
		Items:          input.Items,
		CouponCode:     input.CouponCode,
		ClientIP:       input.ClientIP,
		ManualFormData: input.ManualFormData,
	})
}

// CreateGuestOrder 游客创建订单
func (s *OrderService) CreateGuestOrder(input CreateGuestOrderInput) (*models.Order, error) {
	email, err := normalizeGuestEmail(input.Email)
	if err != nil {
		return nil, err
	}
	password := strings.TrimSpace(input.OrderPassword)
	if password == "" {
		return nil, ErrGuestPasswordRequired
	}
	locale := strings.TrimSpace(input.Locale)
	return s.createOrder(orderCreateParams{
		UserID:         0,
		GuestEmail:     email,
		GuestPassword:  password,
		GuestLocale:    locale,
		Items:          input.Items,
		CouponCode:     input.CouponCode,
		ClientIP:       input.ClientIP,
		IsGuest:        true,
		ManualFormData: input.ManualFormData,
	})
}

type orderCreateParams struct {
	UserID         uint
	GuestEmail     string
	GuestPassword  string
	GuestLocale    string
	Items          []CreateOrderItem
	CouponCode     string
	ClientIP       string
	IsGuest        bool
	ManualFormData map[string]models.JSON
}

// OrderPreview 订单金额预览
type OrderPreview struct {
	Currency                string             `json:"currency"`
	OriginalAmount          models.Money       `json:"original_amount"`
	DiscountAmount          models.Money       `json:"discount_amount"`
	PromotionDiscountAmount models.Money       `json:"promotion_discount_amount"`
	TotalAmount             models.Money       `json:"total_amount"`
	Items                   []OrderPreviewItem `json:"items"`
}

// OrderPreviewItem 订单项金额预览
type OrderPreviewItem struct {
	ProductID         uint               `json:"product_id"`
	SKUID             uint               `json:"sku_id"`
	TitleJSON         models.JSON        `json:"title"`
	SKUSnapshotJSON   models.JSON        `json:"sku_snapshot"`
	Tags              models.StringArray `json:"tags"`
	UnitPrice         models.Money       `json:"unit_price"`
	Quantity          int                `json:"quantity"`
	TotalPrice        models.Money       `json:"total_price"`
	CouponDiscount    models.Money       `json:"coupon_discount_amount"`
	PromotionDiscount models.Money       `json:"promotion_discount_amount"`
	FulfillmentType   string             `json:"fulfillment_type"`
}

type orderBuildResult struct {
	Plans                   []childOrderPlan
	OrderItems              []models.OrderItem
	OriginalAmount          decimal.Decimal
	PromotionDiscountAmount decimal.Decimal
	DiscountAmount          decimal.Decimal
	TotalAmount             decimal.Decimal
	Currency                string
	OrderPromotionID        *uint
	AppliedCoupon           *models.Coupon
}

// PreviewOrder 用户订单金额预览
func (s *OrderService) PreviewOrder(input CreateOrderInput) (*OrderPreview, error) {
	if input.UserID == 0 {
		return nil, ErrInvalidOrderItem
	}
	return s.previewOrder(orderCreateParams{
		UserID:         input.UserID,
		Items:          input.Items,
		CouponCode:     input.CouponCode,
		ClientIP:       input.ClientIP,
		ManualFormData: input.ManualFormData,
	})
}

// PreviewGuestOrder 游客订单金额预览
func (s *OrderService) PreviewGuestOrder(input CreateGuestOrderInput) (*OrderPreview, error) {
	return s.previewOrder(orderCreateParams{
		GuestEmail:     input.Email,
		GuestPassword:  input.OrderPassword,
		GuestLocale:    input.Locale,
		Items:          input.Items,
		CouponCode:     input.CouponCode,
		ClientIP:       input.ClientIP,
		IsGuest:        true,
		ManualFormData: input.ManualFormData,
	})
}

func (s *OrderService) previewOrder(input orderCreateParams) (*OrderPreview, error) {
	result, err := s.buildOrderResult(input)
	if err != nil {
		return nil, err
	}
	items := make([]OrderPreviewItem, 0, len(result.Plans))
	for _, plan := range result.Plans {
		item := plan.Item
		items = append(items, OrderPreviewItem{
			ProductID:         item.ProductID,
			SKUID:             item.SKUID,
			TitleJSON:         item.TitleJSON,
			SKUSnapshotJSON:   item.SKUSnapshotJSON,
			Tags:              item.Tags,
			UnitPrice:         item.UnitPrice,
			Quantity:          item.Quantity,
			TotalPrice:        item.TotalPrice,
			CouponDiscount:    item.CouponDiscount,
			PromotionDiscount: item.PromotionDiscount,
			FulfillmentType:   item.FulfillmentType,
		})
	}
	return &OrderPreview{
		Currency:                result.Currency,
		OriginalAmount:          models.NewMoneyFromDecimal(result.OriginalAmount),
		DiscountAmount:          models.NewMoneyFromDecimal(result.DiscountAmount),
		PromotionDiscountAmount: models.NewMoneyFromDecimal(result.PromotionDiscountAmount),
		TotalAmount:             models.NewMoneyFromDecimal(result.TotalAmount),
		Items:                   items,
	}, nil
}

func (s *OrderService) createOrder(input orderCreateParams) (*models.Order, error) {
	if s.queueClient == nil || !s.queueClient.Enabled() {
		return nil, ErrQueueUnavailable
	}
	result, err := s.buildOrderResult(input)
	if err != nil {
		return nil, err
	}

	if len(input.Items) == 0 {
		return nil, ErrInvalidOrderItem
	}
	if s.productSKURepo == nil {
		return nil, ErrProductSKUInvalid
	}
	if input.IsGuest && input.GuestEmail == "" {
		return nil, ErrGuestEmailRequired
	}
	if input.IsGuest && input.GuestPassword == "" {
		return nil, ErrGuestPasswordRequired
	}

	expireMinutes := s.resolveExpireMinutes()
	now := time.Now()
	expiresAt := now.Add(time.Duration(expireMinutes) * time.Minute)
	order := &models.Order{
		OrderNo:                 generateOrderNo(),
		UserID:                  input.UserID,
		GuestEmail:              input.GuestEmail,
		GuestPassword:           input.GuestPassword,
		GuestLocale:             input.GuestLocale,
		Status:                  constants.OrderStatusPendingPayment,
		Currency:                result.Currency,
		OriginalAmount:          models.NewMoneyFromDecimal(result.OriginalAmount),
		DiscountAmount:          models.NewMoneyFromDecimal(result.DiscountAmount),
		PromotionDiscountAmount: models.NewMoneyFromDecimal(result.PromotionDiscountAmount),
		TotalAmount:             models.NewMoneyFromDecimal(result.TotalAmount),
		WalletPaidAmount:        models.NewMoneyFromDecimal(decimal.Zero),
		OnlinePaidAmount:        models.NewMoneyFromDecimal(result.TotalAmount),
		RefundedAmount:          models.NewMoneyFromDecimal(decimal.Zero),
		CouponID:                nil,
		PromotionID:             result.OrderPromotionID,
		ExpiresAt:               &expiresAt,
		ClientIP:                strings.TrimSpace(input.ClientIP),
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	if result.AppliedCoupon != nil {
		order.CouponID = &result.AppliedCoupon.ID
	}

	err = models.DB.Transaction(func(tx *gorm.DB) error {
		orderRepo := s.orderRepo.WithTx(tx)
		var productSKURepo repository.ProductSKURepository
		if s.productSKURepo != nil {
			productSKURepo = s.productSKURepo.WithTx(tx)
		}
		if err := orderRepo.Create(order, nil); err != nil {
			return err
		}

		for idx := range result.Plans {
			plan := result.Plans[idx]
			childOrder := &models.Order{
				OrderNo:                 buildChildOrderNo(order.OrderNo, idx+1),
				ParentID:                &order.ID,
				UserID:                  order.UserID,
				GuestEmail:              order.GuestEmail,
				GuestPassword:           order.GuestPassword,
				GuestLocale:             order.GuestLocale,
				Status:                  constants.OrderStatusPendingPayment,
				Currency:                plan.Currency,
				OriginalAmount:          models.NewMoneyFromDecimal(plan.TotalAmount),
				DiscountAmount:          models.NewMoneyFromDecimal(plan.CouponDiscount),
				PromotionDiscountAmount: models.NewMoneyFromDecimal(plan.PromotionDiscount),
				TotalAmount:             models.NewMoneyFromDecimal(normalizeOrderAmount(plan.TotalAmount.Sub(plan.CouponDiscount))),
				WalletPaidAmount:        models.NewMoneyFromDecimal(decimal.Zero),
				OnlinePaidAmount:        models.NewMoneyFromDecimal(normalizeOrderAmount(plan.TotalAmount.Sub(plan.CouponDiscount))),
				RefundedAmount:          models.NewMoneyFromDecimal(decimal.Zero),
				CouponID:                nil,
				PromotionID:             plan.Item.PromotionID,
				ExpiresAt:               &expiresAt,
				ClientIP:                order.ClientIP,
				CreatedAt:               now,
				UpdatedAt:               now,
			}
			if result.AppliedCoupon != nil && plan.CouponDiscount.GreaterThan(decimal.Zero) {
				childOrder.CouponID = &result.AppliedCoupon.ID
			}
			if err := orderRepo.Create(childOrder, []models.OrderItem{plan.Item}); err != nil {
				return err
			}

			if strings.TrimSpace(plan.Item.FulfillmentType) == constants.FulfillmentTypeAuto {
				if s.cardSecretRepo == nil {
					return ErrCardSecretInsufficient
				}
				secretRepo := s.cardSecretRepo.WithTx(tx)
				var rows []models.CardSecret
				if err := tx.Where("product_id = ? AND sku_id = ? AND status = ?", plan.Item.ProductID, plan.Item.SKUID, models.CardSecretStatusAvailable).
					Order("id asc").Limit(plan.Item.Quantity).Find(&rows).Error; err != nil {
					return err
				}
				if len(rows) < plan.Item.Quantity {
					return ErrCardSecretInsufficient
				}
				ids := make([]uint, 0, len(rows))
				for _, row := range rows {
					ids = append(ids, row.ID)
				}
				affected, err := secretRepo.Reserve(ids, childOrder.ID, now)
				if err != nil {
					return err
				}
				if int(affected) != len(ids) {
					return ErrCardSecretInsufficient
				}
			}
			if strings.TrimSpace(plan.Item.FulfillmentType) == constants.FulfillmentTypeManual &&
				plan.SKU != nil &&
				shouldEnforceManualSKUStock(plan.Product, plan.SKU) {
				affected, err := productSKURepo.ReserveManualStock(plan.Item.SKUID, plan.Item.Quantity)
				if err != nil {
					return err
				}
				if affected == 0 {
					return ErrManualStockInsufficient
				}
			}
		}

		if result.AppliedCoupon != nil {
			couponRepo := s.couponRepo.WithTx(tx)
			usageRepo := s.couponUsageRepo.WithTx(tx)
			usage := &models.CouponUsage{
				CouponID:       result.AppliedCoupon.ID,
				UserID:         input.UserID,
				OrderID:        order.ID,
				DiscountAmount: models.NewMoneyFromDecimal(result.DiscountAmount),
				CreatedAt:      now,
			}
			if err := usageRepo.Create(usage); err != nil {
				return err
			}
			if err := couponRepo.IncrementUsedCount(result.AppliedCoupon.ID, 1); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrCardSecretInsufficient) {
			return nil, ErrCardSecretInsufficient
		}
		if errors.Is(err, ErrManualStockInsufficient) {
			return nil, ErrManualStockInsufficient
		}
		return nil, ErrOrderCreateFailed
	}

	if s.queueClient != nil {
		if err := s.queueClient.EnqueueOrderTimeoutCancel(queue.OrderTimeoutCancelPayload{
			OrderID: order.ID,
		}, time.Duration(expireMinutes)*time.Minute); err != nil {
			logger.Errorw("order_enqueue_timeout_cancel_failed",
				"order_id", order.ID,
				"order_no", order.OrderNo,
				"error", err,
			)
			full, fetchErr := s.orderRepo.GetByID(order.ID)
			if fetchErr != nil {
				logger.Errorw("order_fetch_for_timeout_rollback_failed",
					"order_id", order.ID,
					"order_no", order.OrderNo,
					"error", fetchErr,
				)
			} else if full != nil {
				if cancelErr := s.cancelOrderWithChildren(full, true); cancelErr != nil {
					logger.Errorw("order_timeout_rollback_cancel_failed",
						"order_id", order.ID,
						"order_no", order.OrderNo,
						"error", cancelErr,
					)
				}
			}
			return nil, ErrQueueUnavailable
		}
	}

	full, err := s.orderRepo.GetByID(order.ID)
	if err == nil && full != nil {
		fillOrderItemsFromChildren(full)
		return full, nil
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

func (s *OrderService) buildOrderResult(input orderCreateParams) (*orderBuildResult, error) {
	if len(input.Items) == 0 {
		return nil, ErrInvalidOrderItem
	}
	if input.IsGuest && input.GuestEmail == "" {
		return nil, ErrGuestEmailRequired
	}
	if input.IsGuest && input.GuestPassword == "" {
		return nil, ErrGuestPasswordRequired
	}

	mergedItems, err := mergeCreateOrderItems(input.Items)
	if err != nil {
		return nil, err
	}
	if len(mergedItems) == 0 {
		return nil, ErrInvalidOrderItem
	}

	var plans []childOrderPlan
	var orderItems []models.OrderItem
	originalAmount := decimal.Zero
	promotionDiscountAmount := decimal.Zero
	currency := s.resolveSiteCurrency()
	now := time.Now()
	var promotionIDValue uint
	var promotionSeen bool
	promotionSame := true
	var noPromotionSeen bool

	promotionService := NewPromotionService(s.promotionRepo)
	manualFormData := input.ManualFormData
	if manualFormData == nil {
		manualFormData = map[string]models.JSON{}
	}
	for _, item := range mergedItems {
		if item.ProductID == 0 || item.Quantity <= 0 {
			return nil, ErrInvalidOrderItem
		}
		product, err := s.productRepo.GetByID(strconv.FormatUint(uint64(item.ProductID), 10))
		if err != nil {
			return nil, err
		}
		if product == nil || !product.IsActive {
			return nil, ErrProductNotAvailable
		}
		purchaseType := strings.TrimSpace(product.PurchaseType)
		if purchaseType == "" {
			purchaseType = constants.ProductPurchaseMember
		}
		if input.IsGuest && purchaseType == constants.ProductPurchaseMember {
			return nil, ErrProductPurchaseNotAllowed
		}
		sku, err := s.resolveOrderSKU(product, item.SKUID)
		if err != nil {
			return nil, err
		}

		productCurrency := currency
		priceCarrier := *product
		priceCarrier.PriceAmount = sku.PriceAmount
		promotion, unitPrice, err := promotionService.ApplyPromotion(&priceCarrier, item.Quantity)
		if err != nil {
			return nil, err
		}
		unitPriceAmount := unitPrice.Decimal.Round(2)
		if unitPriceAmount.LessThanOrEqual(decimal.Zero) || productCurrency == "" {
			return nil, ErrProductPriceInvalid
		}

		basePrice := sku.PriceAmount.Decimal.Round(2)
		promotionDiscount := decimal.Zero
		if promotion != nil && basePrice.GreaterThan(unitPriceAmount) {
			promotionDiscount = basePrice.Sub(unitPriceAmount).
				Mul(decimal.NewFromInt(int64(item.Quantity))).
				Round(2)
			promotionDiscountAmount = promotionDiscountAmount.Add(promotionDiscount).Round(2)
		}

		total := unitPriceAmount.Mul(decimal.NewFromInt(int64(item.Quantity))).Round(2)
		originalAmount = originalAmount.Add(total).Round(2)
		fulfillmentType := strings.TrimSpace(product.FulfillmentType)
		if fulfillmentType == "" {
			fulfillmentType = constants.FulfillmentTypeManual
		}
		if fulfillmentType != constants.FulfillmentTypeManual && fulfillmentType != constants.FulfillmentTypeAuto {
			return nil, ErrFulfillmentInvalid
		}
		if fulfillmentType == constants.FulfillmentTypeManual &&
			shouldEnforceManualSKUStock(product, sku) &&
			manualSKUAvailable(sku) < item.Quantity {
			return nil, ErrManualStockInsufficient
		}

		manualSchemaSnapshot := models.JSON{}
		manualSubmission := models.JSON{}
		if fulfillmentType == constants.FulfillmentTypeManual {
			submission := resolveManualFormSubmission(manualFormData, product.ID, sku.ID)
			normalizedSchema, normalizedSubmission, err := validateAndNormalizeManualForm(product.ManualFormSchemaJSON, submission)
			if err != nil {
				return nil, err
			}
			manualSchemaSnapshot = normalizedSchema
			manualSubmission = normalizedSubmission
		}

		var promotionID *uint
		if promotion != nil {
			pid := promotion.ID
			promotionID = &pid
			if !promotionSeen {
				promotionSeen = true
				promotionIDValue = pid
			} else if promotionIDValue != pid {
				promotionSame = false
			}
		} else {
			noPromotionSeen = true
		}

		orderItem := models.OrderItem{
			ProductID: product.ID,
			SKUID:     sku.ID,
			TitleJSON: product.TitleJSON,
			SKUSnapshotJSON: models.JSON{
				"sku_id":      sku.ID,
				"sku_code":    sku.SKUCode,
				"spec_values": sku.SpecValuesJSON,
			},
			Tags:                         product.Tags,
			UnitPrice:                    models.NewMoneyFromDecimal(unitPriceAmount),
			Quantity:                     item.Quantity,
			TotalPrice:                   models.NewMoneyFromDecimal(total),
			CouponDiscount:               models.NewMoneyFromDecimal(decimal.Zero),
			PromotionDiscount:            models.NewMoneyFromDecimal(promotionDiscount),
			PromotionID:                  promotionID,
			FulfillmentType:              fulfillmentType,
			ManualFormSchemaSnapshotJSON: manualSchemaSnapshot,
			ManualFormSubmissionJSON:     manualSubmission,
			CreatedAt:                    now,
			UpdatedAt:                    now,
		}
		orderItems = append(orderItems, orderItem)
		plans = append(plans, childOrderPlan{
			Product:           product,
			SKU:               sku,
			Item:              orderItem,
			TotalAmount:       total,
			PromotionDiscount: promotionDiscount,
			Currency:          productCurrency,
		})
	}
	if currency == "" {
		return nil, ErrInvalidOrderAmount
	}

	var orderPromotionID *uint
	if promotionSeen && promotionSame && !noPromotionSeen {
		orderPromotionID = &promotionIDValue
	}

	discountAmount := decimal.Zero
	var appliedCoupon *models.Coupon
	couponCode := strings.TrimSpace(input.CouponCode)
	if couponCode != "" {
		couponService := NewCouponService(s.couponRepo, s.couponUsageRepo)
		discount, coupon, err := couponService.ApplyCoupon(models.NewMoneyFromDecimal(originalAmount), couponCode, input.UserID, orderItems)
		if err != nil {
			return nil, err
		}
		discountAmount = discount.Decimal.Round(2)
		appliedCoupon = coupon
	}

	if appliedCoupon != nil && discountAmount.GreaterThan(decimal.Zero) {
		if err := applyCouponDiscountToItems(plans, appliedCoupon, discountAmount); err != nil {
			return nil, err
		}
		discountAmount = decimal.Zero
		for i := range plans {
			discountAmount = discountAmount.Add(plans[i].CouponDiscount).Round(2)
		}
	}

	totalAmount := decimal.Zero
	for i := range plans {
		plan := &plans[i]
		plan.Item.CouponDiscount = models.NewMoneyFromDecimal(plan.CouponDiscount)
		plan.Item.PromotionDiscount = models.NewMoneyFromDecimal(plan.PromotionDiscount)
		plan.Item.TotalPrice = models.NewMoneyFromDecimal(plan.TotalAmount)
		planTotal := plan.TotalAmount.Sub(plan.CouponDiscount).Round(2)
		if planTotal.LessThan(decimal.Zero) {
			planTotal = decimal.Zero
		}
		totalAmount = totalAmount.Add(planTotal).Round(2)
	}

	return &orderBuildResult{
		Plans:                   plans,
		OrderItems:              orderItems,
		OriginalAmount:          originalAmount,
		PromotionDiscountAmount: promotionDiscountAmount,
		DiscountAmount:          discountAmount,
		TotalAmount:             totalAmount,
		Currency:                currency,
		OrderPromotionID:        orderPromotionID,
		AppliedCoupon:           appliedCoupon,
	}, nil
}

func normalizeGuestEmail(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return "", ErrGuestEmailRequired
	}
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", ErrInvalidEmail
	}
	return normalized, nil
}

func (s *OrderService) resolveExpireMinutes() int {
	defaultMinutes := s.expireMinutes
	if defaultMinutes <= 0 {
		defaultMinutes = 15
	}
	if s.settingService == nil {
		return defaultMinutes
	}
	minutes, err := s.settingService.GetOrderPaymentExpireMinutes(defaultMinutes)
	if err != nil {
		return defaultMinutes
	}
	if minutes <= 0 {
		return defaultMinutes
	}
	return minutes
}

func (s *OrderService) resolveSiteCurrency() string {
	if s == nil || s.settingService == nil {
		return constants.SiteCurrencyDefault
	}
	currency, err := s.settingService.GetSiteCurrency(constants.SiteCurrencyDefault)
	if err != nil {
		return constants.SiteCurrencyDefault
	}
	return normalizeSiteCurrency(currency)
}

func (s *OrderService) resolveOrderSKU(product *models.Product, rawSKUID uint) (*models.ProductSKU, error) {
	if product == nil || product.ID == 0 {
		return nil, ErrProductNotAvailable
	}
	if s.productSKURepo == nil {
		return nil, ErrProductSKUInvalid
	}

	if rawSKUID > 0 {
		sku, err := s.productSKURepo.GetByID(rawSKUID)
		if err != nil {
			return nil, err
		}
		if sku == nil || sku.ProductID != product.ID || !sku.IsActive {
			return nil, ErrProductSKUInvalid
		}
		return sku, nil
	}

	// 兼容窗口：无 sku_id 时仅允许“商品存在且仅存在一个启用 SKU”自动回退。
	activeSKUs, err := s.productSKURepo.ListByProduct(product.ID, true)
	if err != nil {
		return nil, err
	}
	if len(activeSKUs) == 1 {
		return &activeSKUs[0], nil
	}
	if len(activeSKUs) == 0 {
		return nil, ErrProductSKUInvalid
	}
	return nil, ErrProductSKURequired
}

func resolveManualFormSubmission(manualFormData map[string]models.JSON, productID, skuID uint) models.JSON {
	if len(manualFormData) == 0 || productID == 0 {
		return models.JSON{}
	}

	itemKey := buildOrderItemKey(productID, skuID)
	if submission, ok := manualFormData[itemKey]; ok {
		if submission == nil {
			return models.JSON{}
		}
		return submission
	}

	legacyKey := strconv.FormatUint(uint64(productID), 10)
	if submission, ok := manualFormData[legacyKey]; ok {
		if submission == nil {
			return models.JSON{}
		}
		return submission
	}

	return models.JSON{}
}

// cancelOrderWithChildren 取消父订单并级联子订单
func (s *OrderService) cancelOrderWithChildren(order *models.Order, rollbackCoupon bool) error {
	if order == nil {
		return ErrOrderNotFound
	}
	now := time.Now()
	err := models.DB.Transaction(func(tx *gorm.DB) error {
		orderRepo := s.orderRepo.WithTx(tx)
		productRepo := s.productRepo.WithTx(tx)
		var productSKURepo repository.ProductSKURepository
		if s.productSKURepo != nil {
			productSKURepo = s.productSKURepo.WithTx(tx)
		}
		updates := map[string]interface{}{
			"canceled_at": now,
			"updated_at":  now,
		}
		if err := orderRepo.UpdateStatus(order.ID, constants.OrderStatusCanceled, updates); err != nil {
			return ErrOrderUpdateFailed
		}
		for _, child := range order.Children {
			if err := orderRepo.UpdateStatus(child.ID, constants.OrderStatusCanceled, updates); err != nil {
				return ErrOrderUpdateFailed
			}
		}
		if s.cardSecretRepo != nil {
			secretRepo := s.cardSecretRepo.WithTx(tx)
			if len(order.Children) > 0 {
				for _, child := range order.Children {
					if _, err := secretRepo.ReleaseByOrder(child.ID); err != nil {
						return err
					}
				}
			} else {
				if _, err := secretRepo.ReleaseByOrder(order.ID); err != nil {
					return err
				}
			}
		}
		if len(order.Children) > 0 {
			for _, child := range order.Children {
				if err := releaseManualStockByItems(productRepo, productSKURepo, child.Items); err != nil {
					return err
				}
			}
		} else {
			if err := releaseManualStockByItems(productRepo, productSKURepo, order.Items); err != nil {
				return err
			}
		}

		if rollbackCoupon {
			couponRepo := s.couponRepo.WithTx(tx)
			usageRepo := s.couponUsageRepo.WithTx(tx)
			usages, err := usageRepo.ListByOrderID(order.ID)
			if err != nil {
				return err
			}
			if len(usages) > 0 {
				if err := usageRepo.DeleteByOrderID(order.ID); err != nil {
					return err
				}
				counts := make(map[uint]int)
				for _, usage := range usages {
					counts[usage.CouponID]++
				}
				for couponID, count := range counts {
					if count <= 0 {
						continue
					}
					if err := couponRepo.DecrementUsedCount(couponID, count); err != nil {
						return err
					}
				}
			}
		}
		if s.walletService != nil {
			if _, err := s.walletService.ReleaseOrderBalance(tx, order, constants.WalletTxnTypeOrderRefund, "订单取消退回余额"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	order.Status = constants.OrderStatusCanceled
	order.CanceledAt = &now
	order.UpdatedAt = now
	for i := range order.Children {
		order.Children[i].Status = constants.OrderStatusCanceled
		order.Children[i].CanceledAt = &now
		order.Children[i].UpdatedAt = now
	}
	return nil
}

// CancelOrder 用户取消订单
func (s *OrderService) CancelOrder(orderID uint, userID uint) (*models.Order, error) {
	order, err := s.orderRepo.GetByIDAndUser(orderID, userID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if order.Status != constants.OrderStatusPendingPayment {
		return nil, ErrOrderCancelNotAllowed
	}
	if err := s.cancelOrderWithChildren(order, false); err != nil {
		return nil, ErrOrderUpdateFailed
	}
	if s.queueClient != nil {
		if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, order.ID, constants.OrderStatusCanceled); err != nil {
			logger.Warnw("order_enqueue_status_email_failed",
				"order_id", order.ID,
				"target_order_id", order.ID,
				"status", constants.OrderStatusCanceled,
				"error", err,
			)
		}
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

// UpdateOrderStatus 管理端更新订单状态
func (s *OrderService) UpdateOrderStatus(orderID uint, targetStatus string) (*models.Order, error) {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}

	target := strings.TrimSpace(targetStatus)
	if target == "" {
		return nil, ErrOrderStatusInvalid
	}
	if order.Status == target {
		return order, nil
	}
	isParent := order.ParentID == nil && len(order.Children) > 0
	if isParent {
		switch target {
		case constants.OrderStatusCanceled:
			if err := s.cancelOrderWithChildren(order, true); err != nil {
				return nil, ErrOrderUpdateFailed
			}
			if s.queueClient != nil {
				if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, order.ID, constants.OrderStatusCanceled); err != nil {
					logger.Warnw("order_enqueue_status_email_failed",
						"order_id", order.ID,
						"target_order_id", order.ID,
						"status", constants.OrderStatusCanceled,
						"error", err,
					)
				}
			}
			return order, nil
		case constants.OrderStatusPaid:
			if order.Status != constants.OrderStatusPendingPayment {
				return nil, ErrOrderStatusInvalid
			}
			now := time.Now()
			err = models.DB.Transaction(func(tx *gorm.DB) error {
				orderRepo := s.orderRepo.WithTx(tx)
				productRepo := s.productRepo.WithTx(tx)
				var productSKURepo repository.ProductSKURepository
				if s.productSKURepo != nil {
					productSKURepo = s.productSKURepo.WithTx(tx)
				}
				updates := map[string]interface{}{
					"paid_at":    now,
					"updated_at": now,
				}
				if err := orderRepo.UpdateStatus(order.ID, constants.OrderStatusPaid, updates); err != nil {
					return ErrOrderUpdateFailed
				}
				for _, child := range order.Children {
					if err := orderRepo.UpdateStatus(child.ID, constants.OrderStatusPaid, updates); err != nil {
						return ErrOrderUpdateFailed
					}
					if err := consumeManualStockByItems(productRepo, productSKURepo, child.Items); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return nil, ErrOrderUpdateFailed
			}
			order.Status = constants.OrderStatusPaid
			order.PaidAt = &now
			order.UpdatedAt = now
			for i := range order.Children {
				order.Children[i].Status = constants.OrderStatusPaid
				order.Children[i].PaidAt = &now
				order.Children[i].UpdatedAt = now
			}
			if s.queueClient != nil {
				if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, order.ID, constants.OrderStatusPaid); err != nil {
					logger.Warnw("order_enqueue_status_email_failed",
						"order_id", order.ID,
						"target_order_id", order.ID,
						"status", constants.OrderStatusPaid,
						"error", err,
					)
				}
			}
			return order, nil
		case constants.OrderStatusCompleted:
			if !canCompleteParentOrder(order) {
				return nil, ErrOrderStatusInvalid
			}
			now := time.Now()
			err = models.DB.Transaction(func(tx *gorm.DB) error {
				orderRepo := s.orderRepo.WithTx(tx)
				updates := map[string]interface{}{"updated_at": now}
				if err := orderRepo.UpdateStatus(order.ID, constants.OrderStatusCompleted, updates); err != nil {
					return ErrOrderUpdateFailed
				}
				for _, child := range order.Children {
					if child.Status == constants.OrderStatusCompleted {
						continue
					}
					if child.Status != constants.OrderStatusDelivered {
						return ErrOrderStatusInvalid
					}
					if err := orderRepo.UpdateStatus(child.ID, constants.OrderStatusCompleted, updates); err != nil {
						return ErrOrderUpdateFailed
					}
				}
				return nil
			})
			if err != nil {
				if errors.Is(err, ErrOrderStatusInvalid) {
					return nil, ErrOrderStatusInvalid
				}
				return nil, ErrOrderUpdateFailed
			}
			order.Status = constants.OrderStatusCompleted
			order.UpdatedAt = now
			for i := range order.Children {
				if order.Children[i].Status == constants.OrderStatusDelivered {
					order.Children[i].Status = constants.OrderStatusCompleted
					order.Children[i].UpdatedAt = now
				}
			}
			if s.queueClient != nil {
				if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, order.ID, constants.OrderStatusCompleted); err != nil {
					logger.Warnw("order_enqueue_status_email_failed",
						"order_id", order.ID,
						"target_order_id", order.ID,
						"status", constants.OrderStatusCompleted,
						"error", err,
					)
				}
			}
			return order, nil
		default:
			return nil, ErrOrderStatusInvalid
		}
	}
	if !isTransitionAllowed(order.Status, target) {
		return nil, ErrOrderStatusInvalid
	}

	now := time.Now()
	updates := map[string]interface{}{
		"updated_at": now,
	}
	switch target {
	case constants.OrderStatusPaid:
		updates["paid_at"] = now
	case constants.OrderStatusCanceled:
		updates["canceled_at"] = now
	}

	if target == constants.OrderStatusCanceled {
		err = models.DB.Transaction(func(tx *gorm.DB) error {
			orderRepo := s.orderRepo.WithTx(tx)
			productRepo := s.productRepo.WithTx(tx)
			var productSKURepo repository.ProductSKURepository
			if s.productSKURepo != nil {
				productSKURepo = s.productSKURepo.WithTx(tx)
			}
			if err := orderRepo.UpdateStatus(order.ID, target, updates); err != nil {
				return ErrOrderUpdateFailed
			}
			if s.cardSecretRepo != nil {
				secretRepo := s.cardSecretRepo.WithTx(tx)
				if _, err := secretRepo.ReleaseByOrder(order.ID); err != nil {
					return err
				}
			}
			if err := releaseManualStockByItems(productRepo, productSKURepo, order.Items); err != nil {
				return err
			}
			if s.walletService != nil {
				if _, err := s.walletService.ReleaseOrderBalance(tx, order, constants.WalletTxnTypeOrderRefund, "订单取消退回余额"); err != nil {
					return err
				}
			}
			return nil
		})
	} else if target == constants.OrderStatusPaid {
		err = models.DB.Transaction(func(tx *gorm.DB) error {
			orderRepo := s.orderRepo.WithTx(tx)
			productRepo := s.productRepo.WithTx(tx)
			var productSKURepo repository.ProductSKURepository
			if s.productSKURepo != nil {
				productSKURepo = s.productSKURepo.WithTx(tx)
			}
			if err := orderRepo.UpdateStatus(order.ID, target, updates); err != nil {
				return ErrOrderUpdateFailed
			}
			if err := consumeManualStockByItems(productRepo, productSKURepo, order.Items); err != nil {
				return err
			}
			return nil
		})
	} else {
		err = s.orderRepo.UpdateStatus(order.ID, target, updates)
	}
	if err != nil {
		return nil, ErrOrderUpdateFailed
	}
	order.Status = target
	order.UpdatedAt = now
	if v, ok := updates["paid_at"]; ok {
		if t, ok := v.(time.Time); ok {
			order.PaidAt = &t
		}
	}
	if v, ok := updates["canceled_at"]; ok {
		if t, ok := v.(time.Time); ok {
			order.CanceledAt = &t
		}
	}
	if order.ParentID != nil {
		parentStatus, syncErr := syncParentStatus(s.orderRepo, *order.ParentID, now)
		if syncErr != nil {
			logger.Warnw("order_sync_parent_status_failed",
				"order_id", order.ID,
				"parent_order_id", *order.ParentID,
				"target_status", target,
				"error", syncErr,
			)
		} else if s.queueClient != nil {
			status := parentStatus
			if status == "" {
				status = target
			}
			if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, *order.ParentID, status); err != nil {
				logger.Warnw("order_enqueue_status_email_failed",
					"order_id", order.ID,
					"target_order_id", *order.ParentID,
					"status", status,
					"error", err,
				)
			}
		}
	} else if s.queueClient != nil {
		if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, order.ID, target); err != nil {
			logger.Warnw("order_enqueue_status_email_failed",
				"order_id", order.ID,
				"target_order_id", order.ID,
				"status", target,
				"error", err,
			)
		}
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

// CancelExpiredOrder 超时取消订单
func (s *OrderService) CancelExpiredOrder(orderID uint) (*models.Order, error) {
	if orderID == 0 {
		return nil, ErrOrderNotFound
	}
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if order.Status != constants.OrderStatusPendingPayment {
		return order, nil
	}
	if order.ExpiresAt == nil {
		return order, nil
	}
	now := time.Now()
	if order.ExpiresAt.After(now) {
		return order, nil
	}
	if err := s.cancelOrderWithChildren(order, true); err != nil {
		return nil, err
	}
	if s.queueClient != nil {
		if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, order.ID, constants.OrderStatusCanceled); err != nil {
			logger.Warnw("order_enqueue_status_email_failed",
				"order_id", order.ID,
				"target_order_id", order.ID,
				"status", constants.OrderStatusCanceled,
				"error", err,
			)
		}
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

// ensureOrderCanceledIfExpired 读取时懒同步过期订单状态
func (s *OrderService) ensureOrderCanceledIfExpired(order *models.Order) error {
	if order == nil {
		return nil
	}
	if order.Status != constants.OrderStatusPendingPayment {
		return nil
	}
	if order.ExpiresAt == nil {
		return nil
	}
	if order.ExpiresAt.After(time.Now()) {
		return nil
	}
	if err := s.cancelOrderWithChildren(order, true); err != nil {
		return err
	}
	if s.queueClient != nil {
		if _, err := enqueueOrderStatusEmailTaskIfEligible(s.orderRepo, s.queueClient, order.ID, constants.OrderStatusCanceled); err != nil {
			logger.Warnw("order_enqueue_status_email_failed",
				"order_id", order.ID,
				"target_order_id", order.ID,
				"status", constants.OrderStatusCanceled,
				"error", err,
			)
		}
	}
	return nil
}

// ensureOrdersCanceledIfExpired 批量懒同步过期订单状态
func (s *OrderService) ensureOrdersCanceledIfExpired(orders []models.Order) error {
	if len(orders) == 0 {
		return nil
	}
	for i := range orders {
		if err := s.ensureOrderCanceledIfExpired(&orders[i]); err != nil {
			return err
		}
	}
	return nil
}

// GetOrderByUser 获取订单详情
func (s *OrderService) GetOrderByUser(orderID uint, userID uint) (*models.Order, error) {
	order, err := s.orderRepo.GetByIDAndUser(orderID, userID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if err := s.ensureOrderCanceledIfExpired(order); err != nil {
		return nil, ErrOrderUpdateFailed
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

// GetOrderByUserOrderNo 按订单号获取用户订单详情
func (s *OrderService) GetOrderByUserOrderNo(orderNo string, userID uint) (*models.Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, ErrOrderNotFound
	}
	order, err := s.orderRepo.GetByOrderNoAndUser(orderNo, userID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if err := s.ensureOrderCanceledIfExpired(order); err != nil {
		return nil, ErrOrderUpdateFailed
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

// GetOrderByGuest 获取游客订单详情
func (s *OrderService) GetOrderByGuest(orderID uint, email, password string) (*models.Order, error) {
	order, err := s.orderRepo.GetByIDAndGuest(orderID, email, password)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrGuestOrderNotFound
	}
	if err := s.ensureOrderCanceledIfExpired(order); err != nil {
		return nil, ErrOrderUpdateFailed
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

// GetOrderByGuestOrderNo 获取游客订单详情（按订单号）
func (s *OrderService) GetOrderByGuestOrderNo(orderNo, email, password string) (*models.Order, error) {
	order, err := s.orderRepo.GetByOrderNoAndGuest(orderNo, email, password)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrGuestOrderNotFound
	}
	if err := s.ensureOrderCanceledIfExpired(order); err != nil {
		return nil, ErrOrderUpdateFailed
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

// ListOrdersByUser 获取订单列表
func (s *OrderService) ListOrdersByUser(filter repository.OrderListFilter) ([]models.Order, int64, error) {
	if filter.UserID == 0 {
		return nil, 0, ErrOrderFetchFailed
	}
	orders, total, err := s.orderRepo.ListByUser(filter)
	if err != nil {
		return nil, 0, ErrOrderFetchFailed
	}
	if err := s.ensureOrdersCanceledIfExpired(orders); err != nil {
		return nil, 0, ErrOrderUpdateFailed
	}
	fillOrdersItemsFromChildren(orders)
	return orders, total, nil
}

// ListOrdersByGuest 获取游客订单列表
func (s *OrderService) ListOrdersByGuest(email, password string, page, pageSize int) ([]models.Order, int64, error) {
	orders, total, err := s.orderRepo.ListByGuest(email, password, page, pageSize)
	if err != nil {
		return nil, 0, ErrOrderFetchFailed
	}
	if err := s.ensureOrdersCanceledIfExpired(orders); err != nil {
		return nil, 0, ErrOrderUpdateFailed
	}
	fillOrdersItemsFromChildren(orders)
	return orders, total, nil
}

// ListOrdersForAdmin 管理端订单列表
func (s *OrderService) ListOrdersForAdmin(filter repository.OrderListFilter) ([]models.Order, int64, error) {
	orders, total, err := s.orderRepo.ListAdmin(filter)
	if err != nil {
		return nil, 0, ErrOrderFetchFailed
	}
	if err := s.ensureOrdersCanceledIfExpired(orders); err != nil {
		return nil, 0, ErrOrderUpdateFailed
	}
	fillOrdersItemsFromChildren(orders)
	return orders, total, nil
}

// GetOrderForAdmin 管理端订单详情
func (s *OrderService) GetOrderForAdmin(orderID uint) (*models.Order, error) {
	if orderID == 0 {
		return nil, ErrOrderNotFound
	}
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if err := s.ensureOrderCanceledIfExpired(order); err != nil {
		return nil, ErrOrderUpdateFailed
	}
	fillOrderItemsFromChildren(order)
	return order, nil
}

func canCompleteParentOrder(order *models.Order) bool {
	if order == nil {
		return false
	}
	if order.Status != constants.OrderStatusDelivered {
		return false
	}
	for _, child := range order.Children {
		if child.Status != constants.OrderStatusDelivered && child.Status != constants.OrderStatusCompleted {
			return false
		}
	}
	return true
}

func isTransitionAllowed(current, target string) bool {
	if current == target {
		return true
	}
	nexts, ok := allowedTransitions[current]
	if !ok {
		return false
	}
	return nexts[target]
}

func generateOrderNo() string {
	now := time.Now().Format("20060102150405")
	randPart := randNumeric(6)
	return fmt.Sprintf("DJ%s%s", now, randPart)
}

func randNumeric(length int) string {
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			b.WriteString("0")
			continue
		}
		b.WriteString(fmt.Sprintf("%d", n.Int64()))
	}
	return b.String()
}

// mergeCreateOrderItems 合并重复商品的下单项
func mergeCreateOrderItems(items []CreateOrderItem) ([]CreateOrderItem, error) {
	if len(items) == 0 {
		return nil, nil
	}
	merged := make([]CreateOrderItem, 0, len(items))
	indexMap := make(map[string]int)
	for _, item := range items {
		if item.ProductID == 0 || item.Quantity <= 0 {
			return nil, ErrInvalidOrderItem
		}
		key := buildOrderItemKey(item.ProductID, item.SKUID)
		if idx, ok := indexMap[key]; ok {
			merged[idx].Quantity += item.Quantity
			continue
		}
		indexMap[key] = len(merged)
		merged = append(merged, CreateOrderItem{
			ProductID: item.ProductID,
			SKUID:     item.SKUID,
			Quantity:  item.Quantity,
		})
	}
	return merged, nil
}

// applyCouponDiscountToItems 分摊优惠券折扣到订单项
func applyCouponDiscountToItems(plans []childOrderPlan, coupon *models.Coupon, discountAmount decimal.Decimal) error {
	if coupon == nil || discountAmount.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	scopeType := strings.ToLower(strings.TrimSpace(coupon.ScopeType))
	if scopeType != constants.ScopeTypeProduct {
		return ErrCouponScopeInvalid
	}
	ids, err := decodeScopeIDs(coupon.ScopeRefIDs)
	if err != nil {
		return ErrCouponScopeInvalid
	}
	eligibleIndexes := make([]int, 0, len(plans))
	eligibleTotal := decimal.Zero
	for i := range plans {
		if _, ok := ids[plans[i].Item.ProductID]; !ok {
			continue
		}
		eligibleIndexes = append(eligibleIndexes, i)
		eligibleTotal = eligibleTotal.Add(plans[i].TotalAmount)
	}
	if len(eligibleIndexes) == 0 || eligibleTotal.LessThanOrEqual(decimal.Zero) {
		return ErrCouponScopeInvalid
	}

	remaining := discountAmount
	for i, idx := range eligibleIndexes {
		if i == len(eligibleIndexes)-1 {
			alloc := remaining.Round(2)
			if alloc.LessThan(decimal.Zero) {
				alloc = decimal.Zero
			}
			if alloc.GreaterThan(plans[idx].TotalAmount) {
				alloc = plans[idx].TotalAmount
			}
			plans[idx].CouponDiscount = alloc
			break
		}
		ratio := plans[idx].TotalAmount.Div(eligibleTotal)
		alloc := discountAmount.Mul(ratio).Round(2)
		if alloc.GreaterThan(remaining) {
			alloc = remaining
		}
		if alloc.LessThan(decimal.Zero) {
			alloc = decimal.Zero
		}
		if alloc.GreaterThan(plans[idx].TotalAmount) {
			alloc = plans[idx].TotalAmount
		}
		plans[idx].CouponDiscount = alloc
		remaining = remaining.Sub(alloc).Round(2)
	}
	return nil
}

// buildChildOrderNo 生成子订单号
func buildChildOrderNo(parentOrderNo string, seq int) string {
	if seq <= 0 {
		return parentOrderNo
	}
	return fmt.Sprintf("%s-%02d", parentOrderNo, seq)
}

// fillOrderItemsFromChildren 从子订单聚合订单项（用于响应兼容）
func fillOrderItemsFromChildren(order *models.Order) {
	if order == nil || len(order.Items) > 0 || len(order.Children) == 0 {
		return
	}
	items := make([]models.OrderItem, 0)
	for _, child := range order.Children {
		for _, item := range child.Items {
			copied := item
			copied.OrderID = order.ID
			items = append(items, copied)
		}
	}
	order.Items = items
}

// fillOrdersItemsFromChildren 批量填充聚合订单项
func fillOrdersItemsFromChildren(orders []models.Order) {
	for i := range orders {
		fillOrderItemsFromChildren(&orders[i])
	}
}

// normalizeOrderAmount 归一化金额精度与下限
func normalizeOrderAmount(amount decimal.Decimal) decimal.Decimal {
	normalized := amount.Round(2)
	if normalized.LessThan(decimal.Zero) {
		return decimal.Zero
	}
	return normalized
}
