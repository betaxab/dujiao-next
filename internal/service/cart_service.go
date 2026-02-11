package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

// CartItemDetail 购物车项详情（用于响应）
type CartItemDetail struct {
	ProductID       uint            `json:"product_id"`
	Quantity        int             `json:"quantity"`
	FulfillmentType string          `json:"fulfillment_type"`
	UnitPrice       models.Money    `json:"unit_price"`
	OriginalPrice   models.Money    `json:"original_price"`
	Currency        string          `json:"currency"`
	Product         *models.Product `json:"product"`
}

// UpsertCartItemInput 购物车更新输入
type UpsertCartItemInput struct {
	UserID          uint
	ProductID       uint
	Quantity        int
	FulfillmentType string
}

// CartService 购物车服务
type CartService struct {
	cartRepo      repository.CartRepository
	productRepo   repository.ProductRepository
	promotionRepo repository.PromotionRepository
}

// NewCartService 创建购物车服务
func NewCartService(cartRepo repository.CartRepository, productRepo repository.ProductRepository, promotionRepo repository.PromotionRepository) *CartService {
	return &CartService{
		cartRepo:      cartRepo,
		productRepo:   productRepo,
		promotionRepo: promotionRepo,
	}
}

// ListByUser 获取用户购物车
func (s *CartService) ListByUser(userID uint) ([]CartItemDetail, error) {
	if userID == 0 {
		return nil, ErrInvalidOrderItem
	}
	items, err := s.cartRepo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	details := make([]CartItemDetail, 0, len(items))
	promotionService := NewPromotionService(s.promotionRepo)
	for _, item := range items {
		product := item.Product
		if product == nil || product.ID == 0 {
			p, err := s.productRepo.GetByID(strconv.FormatUint(uint64(item.ProductID), 10))
			if err != nil {
				return nil, err
			}
			product = p
		}
		if product == nil || !product.IsActive {
			_ = s.cartRepo.DeleteByUserAndProduct(userID, item.ProductID)
			continue
		}

		unitPrice := product.PriceAmount
		if promotionService != nil {
			_, discounted, err := promotionService.ApplyPromotion(product, item.Quantity)
			if err != nil {
				return nil, err
			}
			unitPrice = discounted
		}

		fulfillmentType := strings.TrimSpace(product.FulfillmentType)
		if fulfillmentType == "" {
			fulfillmentType = constants.FulfillmentTypeManual
		}

		details = append(details, CartItemDetail{
			ProductID:       item.ProductID,
			Quantity:        item.Quantity,
			FulfillmentType: fulfillmentType,
			UnitPrice:       unitPrice,
			OriginalPrice:   product.PriceAmount,
			Currency:        product.PriceCurrency,
			Product:         product,
		})
	}
	return details, nil
}

// UpsertItem 添加或更新购物车项
func (s *CartService) UpsertItem(input UpsertCartItemInput) error {
	if input.UserID == 0 || input.ProductID == 0 || input.Quantity <= 0 {
		return ErrInvalidOrderItem
	}
	product, err := s.productRepo.GetByID(strconv.FormatUint(uint64(input.ProductID), 10))
	if err != nil {
		return err
	}
	if product == nil || !product.IsActive {
		return ErrProductNotAvailable
	}

	fulfillmentType := strings.TrimSpace(product.FulfillmentType)
	if fulfillmentType == "" {
		fulfillmentType = constants.FulfillmentTypeManual
	}
	if fulfillmentType != constants.FulfillmentTypeManual && fulfillmentType != constants.FulfillmentTypeAuto {
		return ErrFulfillmentInvalid
	}

	now := time.Now()
	item := &models.CartItem{
		UserID:          input.UserID,
		ProductID:       input.ProductID,
		Quantity:        input.Quantity,
		FulfillmentType: fulfillmentType,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return s.cartRepo.Upsert(item)
}

// RemoveItem 删除购物车项
func (s *CartService) RemoveItem(userID, productID uint) error {
	if userID == 0 || productID == 0 {
		return ErrInvalidOrderItem
	}
	return s.cartRepo.DeleteByUserAndProduct(userID, productID)
}
