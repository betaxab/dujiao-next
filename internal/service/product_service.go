package service

import (
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/shopspring/decimal"
)

// ProductService 商品业务服务
type ProductService struct {
	repo repository.ProductRepository
}

// NewProductService 创建商品服务
func NewProductService(repo repository.ProductRepository) *ProductService {
	return &ProductService{repo: repo}
}

// CreateProductInput 创建/更新商品输入
type CreateProductInput struct {
	CategoryID           uint
	Slug                 string
	SeoMetaJSON          map[string]interface{}
	TitleJSON            map[string]interface{}
	DescriptionJSON      map[string]interface{}
	ContentJSON          map[string]interface{}
	ManualFormSchemaJSON map[string]interface{}
	PriceAmount          decimal.Decimal
	PriceCurrency        string
	Images               []string
	Tags                 []string
	PurchaseType         string
	FulfillmentType      string
	ManualStockTotal     *int
	IsActive             *bool
	SortOrder            int
}

// ListPublic 获取公开商品列表
func (s *ProductService) ListPublic(categoryID, search string, page, pageSize int) ([]models.Product, int64, error) {
	filter := repository.ProductListFilter{
		Page:         page,
		PageSize:     pageSize,
		CategoryID:   categoryID,
		Search:       search,
		OnlyActive:   true,
		WithCategory: true,
	}
	return s.repo.List(filter)
}

// GetPublicBySlug 获取公开商品详情
func (s *ProductService) GetPublicBySlug(slug string) (*models.Product, error) {
	product, err := s.repo.GetBySlug(slug, true)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return nil, ErrNotFound
	}
	return product, nil
}

// ListAdmin 获取后台商品列表
func (s *ProductService) ListAdmin(categoryID, search, manualStockStatus string, page, pageSize int) ([]models.Product, int64, error) {
	filter := repository.ProductListFilter{
		Page:              page,
		PageSize:          pageSize,
		CategoryID:        categoryID,
		Search:            search,
		ManualStockStatus: normalizeManualStockStatus(manualStockStatus),
		OnlyActive:        false,
		WithCategory:      true,
	}
	return s.repo.List(filter)
}

// GetAdminByID 获取后台商品详情
func (s *ProductService) GetAdminByID(id string) (*models.Product, error) {
	product, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return nil, ErrNotFound
	}
	return product, nil
}

// Create 创建商品
func (s *ProductService) Create(input CreateProductInput) (*models.Product, error) {
	currency := strings.TrimSpace(input.PriceCurrency)
	priceAmount := input.PriceAmount.Round(2)
	if priceAmount.LessThanOrEqual(decimal.Zero) || currency == "" {
		return nil, ErrProductPriceInvalid
	}
	count, err := s.repo.CountBySlug(input.Slug, nil)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugExists
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	purchaseType := normalizePurchaseType(input.PurchaseType)
	if purchaseType == "" {
		return nil, ErrProductPurchaseInvalid
	}
	fulfillmentType := normalizeFulfillmentType(input.FulfillmentType)
	if fulfillmentType == "" {
		return nil, ErrFulfillmentInvalid
	}

	manualStockTotal := 0
	if input.ManualStockTotal != nil {
		manualStockTotal = *input.ManualStockTotal
	}
	if manualStockTotal < 0 {
		return nil, ErrManualStockInvalid
	}

	product := models.Product{
		CategoryID:           input.CategoryID,
		Slug:                 input.Slug,
		SeoMetaJSON:          models.JSON(input.SeoMetaJSON),
		TitleJSON:            models.JSON(input.TitleJSON),
		DescriptionJSON:      models.JSON(input.DescriptionJSON),
		ContentJSON:          models.JSON(input.ContentJSON),
		ManualFormSchemaJSON: models.JSON{},
		PriceAmount:          models.NewMoneyFromDecimal(priceAmount),
		PriceCurrency:        currency,
		Images:               models.StringArray(input.Images),
		Tags:                 models.StringArray(input.Tags),
		PurchaseType:         purchaseType,
		FulfillmentType:      fulfillmentType,
		ManualStockTotal:     manualStockTotal,
		ManualStockLocked:    0,
		ManualStockSold:      0,
		IsActive:             isActive,
		SortOrder:            input.SortOrder,
	}
	if fulfillmentType == constants.FulfillmentTypeManual {
		_, normalizedSchemaJSON, err := parseManualFormSchema(models.JSON(input.ManualFormSchemaJSON))
		if err != nil {
			return nil, err
		}
		product.ManualFormSchemaJSON = normalizedSchemaJSON
	}

	if err := s.repo.Create(&product); err != nil {
		return nil, err
	}
	return &product, nil
}

// Update 更新商品
func (s *ProductService) Update(id string, input CreateProductInput) (*models.Product, error) {
	currency := strings.TrimSpace(input.PriceCurrency)
	priceAmount := input.PriceAmount.Round(2)
	if priceAmount.LessThanOrEqual(decimal.Zero) || currency == "" {
		return nil, ErrProductPriceInvalid
	}
	product, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return nil, ErrNotFound
	}

	count, err := s.repo.CountBySlug(input.Slug, &id)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugExists
	}

	product.CategoryID = input.CategoryID
	product.Slug = input.Slug
	product.SeoMetaJSON = models.JSON(input.SeoMetaJSON)
	product.TitleJSON = models.JSON(input.TitleJSON)
	product.DescriptionJSON = models.JSON(input.DescriptionJSON)
	product.ContentJSON = models.JSON(input.ContentJSON)
	product.ManualFormSchemaJSON = models.JSON{}
	product.PriceAmount = models.NewMoneyFromDecimal(priceAmount)
	product.PriceCurrency = currency
	product.SortOrder = input.SortOrder
	product.Images = models.StringArray(input.Images)
	product.Tags = models.StringArray(input.Tags)
	if input.IsActive != nil {
		product.IsActive = *input.IsActive
	}
	rawPurchaseType := strings.TrimSpace(input.PurchaseType)
	if rawPurchaseType == "" {
		rawPurchaseType = product.PurchaseType
	}
	purchaseType := normalizePurchaseType(rawPurchaseType)
	if purchaseType == "" {
		return nil, ErrProductPurchaseInvalid
	}
	product.PurchaseType = purchaseType
	rawFulfillmentType := strings.TrimSpace(input.FulfillmentType)
	if rawFulfillmentType == "" {
		rawFulfillmentType = product.FulfillmentType
	}
	fulfillmentType := normalizeFulfillmentType(rawFulfillmentType)
	if fulfillmentType == "" {
		return nil, ErrFulfillmentInvalid
	}
	product.FulfillmentType = fulfillmentType
	if fulfillmentType == constants.FulfillmentTypeManual {
		_, normalizedSchemaJSON, err := parseManualFormSchema(models.JSON(input.ManualFormSchemaJSON))
		if err != nil {
			return nil, err
		}
		product.ManualFormSchemaJSON = normalizedSchemaJSON
	}

	if input.ManualStockTotal != nil {
		manualStockTotal := *input.ManualStockTotal
		if manualStockTotal < 0 {
			return nil, ErrManualStockInvalid
		}
		if manualStockTotal < product.ManualStockLocked+product.ManualStockSold {
			return nil, ErrManualStockInvalid
		}
		product.ManualStockTotal = manualStockTotal
	}

	if err := s.repo.Update(product); err != nil {
		return nil, err
	}
	return product, nil
}

func normalizePurchaseType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", constants.ProductPurchaseMember:
		return constants.ProductPurchaseMember
	case constants.ProductPurchaseGuest:
		return constants.ProductPurchaseGuest
	default:
		return ""
	}
}

func normalizeFulfillmentType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", constants.FulfillmentTypeManual:
		return constants.FulfillmentTypeManual
	case constants.FulfillmentTypeAuto:
		return constants.FulfillmentTypeAuto
	default:
		return ""
	}
}

func normalizeManualStockStatus(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "all":
		return ""
	case "low", "normal", "unlimited":
		return value
	default:
		return ""
	}
}

// Delete 删除商品
func (s *ProductService) Delete(id string) error {
	product, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if product == nil {
		return ErrNotFound
	}
	return s.repo.Delete(id)
}
