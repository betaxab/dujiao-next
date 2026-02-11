package service

import (
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/shopspring/decimal"
)

// PromotionService 活动价服务
type PromotionService struct {
	promotionRepo repository.PromotionRepository
}

// NewPromotionService 创建活动价服务
func NewPromotionService(promotionRepo repository.PromotionRepository) *PromotionService {
	return &PromotionService{
		promotionRepo: promotionRepo,
	}
}

// ApplyPromotion 应用活动价规则
func (s *PromotionService) ApplyPromotion(product *models.Product, quantity int) (*models.Promotion, models.Money, error) {
	if product == nil || quantity <= 0 {
		return nil, models.Money{}, ErrPromotionInvalid
	}

	now := time.Now()
	promotion, err := s.promotionRepo.GetActiveByProduct(product.ID, now)
	if err != nil {
		return nil, models.Money{}, err
	}
	if promotion == nil {
		return nil, product.PriceAmount, nil
	}
	if strings.ToLower(strings.TrimSpace(promotion.ScopeType)) != constants.ScopeTypeProduct {
		return nil, product.PriceAmount, ErrPromotionInvalid
	}

	subtotal := product.PriceAmount.Decimal.Mul(decimal.NewFromInt(int64(quantity)))
	if promotion.MinAmount.Decimal.GreaterThan(decimal.Zero) && subtotal.Cmp(promotion.MinAmount.Decimal) < 0 {
		return nil, product.PriceAmount, nil
	}

	unitPrice, err := s.calculateUnitPrice(product.PriceAmount, promotion)
	if err != nil {
		return nil, models.Money{}, err
	}

	return promotion, unitPrice, nil
}

func (s *PromotionService) calculateUnitPrice(base models.Money, promotion *models.Promotion) (models.Money, error) {
	value := promotion.Value.Decimal
	if value.LessThanOrEqual(decimal.Zero) {
		return models.Money{}, ErrPromotionInvalid
	}

	switch strings.ToLower(strings.TrimSpace(promotion.Type)) {
	case constants.PromotionTypeFixed:
		discounted := base.Decimal.Sub(value)
		if discounted.LessThan(decimal.Zero) {
			discounted = decimal.Zero
		}
		return models.NewMoneyFromDecimal(discounted), nil
	case constants.PromotionTypePercent:
		percent := decimal.NewFromInt(100).Sub(value)
		if percent.LessThan(decimal.Zero) {
			percent = decimal.Zero
		}
		discounted := base.Decimal.Mul(percent).Div(decimal.NewFromInt(100))
		return models.NewMoneyFromDecimal(discounted), nil
	case constants.PromotionTypeSpecialPrice:
		return models.NewMoneyFromDecimal(value), nil
	default:
		return models.Money{}, ErrPromotionInvalid
	}
}
