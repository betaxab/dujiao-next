package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/upstream"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrMappingNotFound         = errors.New("product mapping not found")
	ErrMappingAlreadyExists    = errors.New("product mapping already exists for this upstream product")
	ErrUpstreamProductNotFound = errors.New("upstream product not found")
	ErrMappingInactive         = errors.New("product mapping is inactive")
)

// ProductMappingService 商品映射业务服务
type ProductMappingService struct {
	mappingRepo    repository.ProductMappingRepository
	skuMappingRepo repository.SKUMappingRepository
	productRepo    repository.ProductRepository
	productSKURepo repository.ProductSKURepository
	categoryRepo   repository.CategoryRepository
	connService    *SiteConnectionService
}

// NewProductMappingService 创建商品映射服务
func NewProductMappingService(
	mappingRepo repository.ProductMappingRepository,
	skuMappingRepo repository.SKUMappingRepository,
	productRepo repository.ProductRepository,
	productSKURepo repository.ProductSKURepository,
	categoryRepo repository.CategoryRepository,
	connService *SiteConnectionService,
) *ProductMappingService {
	return &ProductMappingService{
		mappingRepo:    mappingRepo,
		skuMappingRepo: skuMappingRepo,
		productRepo:    productRepo,
		productSKURepo: productSKURepo,
		categoryRepo:   categoryRepo,
		connService:    connService,
	}
}

// ImportUpstreamProduct 从上游导入商品（克隆为本地商品 + 建立映射）
func (s *ProductMappingService) ImportUpstreamProduct(connectionID uint, upstreamProductID uint, categoryID uint, slug string) (*models.ProductMapping, error) {
	if err := validateProductCategoryAssignment(s.categoryRepo, categoryID, 0); err != nil {
		return nil, err
	}

	// 检查是否已存在映射
	existing, err := s.mappingRepo.GetByConnectionAndUpstreamID(connectionID, upstreamProductID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrMappingAlreadyExists
	}

	// 获取连接
	conn, err := s.connService.GetByID(connectionID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, ErrConnectionNotFound
	}

	// 获取适配器
	adapter, err := s.connService.GetAdapter(conn)
	if err != nil {
		return nil, err
	}

	// 拉取上游商品
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	upProduct, err := adapter.GetProduct(ctx, upstreamProductID)
	if err != nil {
		return nil, fmt.Errorf("fetch upstream product: %w", err)
	}
	if upProduct == nil {
		return nil, ErrUpstreamProductNotFound
	}

	// 下载图片到本地
	localImages := s.downloadImages(ctx, adapter, upProduct.Images)

	// 下载 Content 中引用的图片
	localContent := s.downloadContentImages(ctx, adapter, upProduct.Content)

	// 确定交付类型：上游商品映射后统一使用 upstream 类型
	fulfillmentType := constants.FulfillmentTypeUpstream

	// 解析价格（应用连接加价比例）
	markupPercent := conn.PriceMarkupPercent
	roundingMode := conn.PriceRoundingMode

	priceAmount, _ := decimal.NewFromString(upProduct.PriceAmount)
	priceAmount = CalculateMarkedUpPrice(priceAmount, markupPercent, roundingMode)
	if priceAmount.LessThanOrEqual(decimal.Zero) && len(upProduct.SKUs) > 0 {
		// 取加价后 SKU 最低价
		for _, sku := range upProduct.SKUs {
			skuPrice, _ := decimal.NewFromString(sku.PriceAmount)
			localPrice := CalculateMarkedUpPrice(skuPrice, markupPercent, roundingMode)
			if localPrice.GreaterThan(decimal.Zero) && (priceAmount.IsZero() || localPrice.LessThan(priceAmount)) {
				priceAmount = localPrice
			}
		}
	}

	// 自动生成 slug（如果未提供）
	if slug == "" {
		slug = fmt.Sprintf("upstream-%d-%d-%d", connectionID, upstreamProductID, time.Now().UnixMilli())
	}

	// 创建本地商品
	product := models.Product{
		CategoryID:           categoryID,
		Slug:                 slug,
		SeoMetaJSON:          upProduct.SeoMeta,
		TitleJSON:            upProduct.Title,
		DescriptionJSON:      upProduct.Description,
		ContentJSON:          localContent,
		ManualFormSchemaJSON: upProduct.ManualFormSchema,
		PriceAmount:          models.NewMoneyFromDecimal(priceAmount.Round(2)),
		Images:               models.StringArray(localImages),
		Tags:                 models.StringArray(upProduct.Tags),
		PurchaseType:         constants.ProductPurchaseMember,
		FulfillmentType:      fulfillmentType,
		ManualStockTotal:     0,
		IsMapped:             true,
		IsActive:             false, // 默认下架，管理员手动上架
		SortOrder:            0,
	}

	var mapping *models.ProductMapping

	// 使用事务一次性创建本地商品、SKU、映射与 SKU 映射，避免留下半成功数据。
	if err := s.productRepo.Transaction(func(tx *gorm.DB) error {
		productRepo := s.productRepo.WithTx(tx)
		mappingRepo := s.mappingRepo.WithTx(tx)
		skuMappingRepo := s.skuMappingRepo.WithTx(tx)
		if err := productRepo.Create(&product); err != nil {
			return fmt.Errorf("create local product: %w", err)
		}

		// 创建 SKU
		skuRepo := s.productSKURepo.WithTx(tx)
		localSKUs := make([]models.ProductSKU, 0, len(upProduct.SKUs))
		for _, upSKU := range upProduct.SKUs {
			skuPrice, _ := decimal.NewFromString(upSKU.PriceAmount)
			localPrice := CalculateMarkedUpPrice(skuPrice, markupPercent, roundingMode)
			localSKU := models.ProductSKU{
				ProductID:      product.ID,
				SKUCode:        upSKU.SKUCode,
				SpecValuesJSON: upSKU.SpecValues,
				PriceAmount:    models.NewMoneyFromDecimal(localPrice.Round(2)),
				IsActive:       upSKU.IsActive,
				SortOrder:      0,
			}
			if err := skuRepo.Create(&localSKU); err != nil {
				return fmt.Errorf("create local sku: %w", err)
			}
			localSKUs = append(localSKUs, localSKU)
		}

		// 如果没有 SKU，创建默认 SKU
		if len(upProduct.SKUs) == 0 {
			defaultSKU := models.ProductSKU{
				ProductID:      product.ID,
				SKUCode:        models.DefaultSKUCode,
				SpecValuesJSON: models.JSON{},
				PriceAmount:    models.NewMoneyFromDecimal(priceAmount.Round(2)),
				IsActive:       true,
				SortOrder:      0,
			}
			if err := skuRepo.Create(&defaultSKU); err != nil {
				return fmt.Errorf("create default sku: %w", err)
			}
			localSKUs = append(localSKUs, defaultSKU)
		}

		// 确定上游原始交付类型（auto/manual）
		upstreamFulfillmentType := upProduct.FulfillmentType
		if upstreamFulfillmentType != constants.FulfillmentTypeAuto {
			upstreamFulfillmentType = constants.FulfillmentTypeManual
		}

		now := time.Now()
		mapping = &models.ProductMapping{
			ConnectionID:            connectionID,
			LocalProductID:          product.ID,
			UpstreamProductID:       upstreamProductID,
			UpstreamFulfillmentType: upstreamFulfillmentType,
			IsActive:                true,
			LastSyncedAt:            &now,
		}
		if err := mappingRepo.Create(mapping); err != nil {
			return fmt.Errorf("create product mapping: %w", err)
		}
		if err := createSKUMappingsWithRepo(skuMappingRepo, mapping.ID, localSKUs, upProduct.SKUs); err != nil {
			return fmt.Errorf("create sku mappings: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return mapping, nil
}

func createSKUMappingsWithRepo(
	skuMappingRepo repository.SKUMappingRepository,
	mappingID uint,
	localSKUs []models.ProductSKU,
	upstreamSKUs []upstream.UpstreamSKU,
) error {
	if skuMappingRepo == nil {
		return nil
	}
	if len(localSKUs) == 0 || len(upstreamSKUs) == 0 {
		return nil
	}

	// 按 SKUCode 匹配
	upstreamByCode := make(map[string]upstream.UpstreamSKU, len(upstreamSKUs))
	for _, us := range upstreamSKUs {
		upstreamByCode[strings.ToLower(strings.TrimSpace(us.SKUCode))] = us
	}

	for _, localSKU := range localSKUs {
		code := strings.ToLower(strings.TrimSpace(localSKU.SKUCode))
		upSKU, ok := upstreamByCode[code]
		if !ok {
			// 如果只有一个 SKU（DEFAULT），匹配第一个上游 SKU
			if len(localSKUs) == 1 && len(upstreamSKUs) == 1 {
				upSKU = upstreamSKUs[0]
			} else {
				continue
			}
		}

		upPrice, _ := decimal.NewFromString(upSKU.PriceAmount)
		now := time.Now()
		skuMapping := &models.SKUMapping{
			ProductMappingID: mappingID,
			LocalSKUID:       localSKU.ID,
			UpstreamSKUID:    upSKU.ID,
			UpstreamPrice:    models.NewMoneyFromDecimal(upPrice.Round(2)),
			UpstreamIsActive: upSKU.IsActive,
			UpstreamStock:    upSKU.StockQuantity,
			StockSyncedAt:    &now,
		}
		if err := skuMappingRepo.Create(skuMapping); err != nil {
			return err
		}
	}

	return nil
}

// downloadImages 下载上游图片到本地
func (s *ProductMappingService) downloadImages(ctx context.Context, adapter upstream.Adapter, images []string) []string {
	var localImages []string
	for _, img := range images {
		if strings.TrimSpace(img) == "" {
			continue
		}
		localPath, err := adapter.DownloadImage(ctx, img)
		if err != nil {
			// 下载失败保留原始 URL
			localImages = append(localImages, img)
			continue
		}
		localImages = append(localImages, localPath)
	}
	return localImages
}

// downloadContentImages 下载多语言 Content 中的图片并替换 URL
func (s *ProductMappingService) downloadContentImages(ctx context.Context, adapter upstream.Adapter, content models.JSON) models.JSON {
	if len(content) == 0 {
		return content
	}

	// models.JSON 是 map[string]interface{}，值为各语言的 Markdown 文本
	imgRegex := regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)|<img[^>]+src=["']([^"']+)["']`)
	downloaded := make(map[string]string) // originalURL -> localPath

	// 第一遍：收集所有唯一图片 URL
	for _, val := range content {
		text, ok := val.(string)
		if !ok || text == "" {
			continue
		}
		matches := imgRegex.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			url := m[1]
			if url == "" {
				url = m[2]
			}
			if url == "" || strings.HasPrefix(url, "/uploads/") {
				continue
			}
			downloaded[url] = "" // 占位
		}
	}

	if len(downloaded) == 0 {
		return content
	}

	// 下载图片
	for url := range downloaded {
		localPath, err := adapter.DownloadImage(ctx, url)
		if err != nil {
			downloaded[url] = url // 失败保留原始
		} else {
			downloaded[url] = localPath
		}
	}

	// 第二遍：替换所有语言文本中的 URL
	result := make(models.JSON, len(content))
	for lang, val := range content {
		text, ok := val.(string)
		if !ok {
			result[lang] = val
			continue
		}
		for original, local := range downloaded {
			if original != local {
				text = strings.ReplaceAll(text, original, local)
			}
		}
		result[lang] = text
	}

	return result
}

// SyncProduct 同步单个映射商品的上游数据（全量同步）
func (s *ProductMappingService) SyncProduct(mappingID uint) error {
	mapping, err := s.mappingRepo.GetByID(mappingID)
	if err != nil {
		return err
	}
	if mapping == nil {
		return ErrMappingNotFound
	}

	conn, err := s.connService.GetByID(mapping.ConnectionID)
	if err != nil {
		return err
	}
	if conn == nil {
		return ErrConnectionNotFound
	}

	adapter, err := s.connService.GetAdapter(conn)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	upProduct, err := adapter.GetProduct(ctx, mapping.UpstreamProductID)
	if err != nil {
		return fmt.Errorf("fetch upstream product: %w", err)
	}

	now := time.Now()

	// ── 1. 同步本地商品字段（表单配置、上下架状态） ──
	localProduct, err := s.productRepo.GetByID(strconv.FormatUint(uint64(mapping.LocalProductID), 10))
	if err != nil {
		return fmt.Errorf("get local product: %w", err)
	}
	if localProduct != nil {
		changed := false

		// 同步人工交付表单配置
		if upProduct.ManualFormSchema != nil {
			localProduct.ManualFormSchemaJSON = upProduct.ManualFormSchema
			changed = true
		}

		// 如果上游商品已下架，本地也自动下架（但上游上架不自动上架，留给管理员决定）
		if !upProduct.IsActive && localProduct.IsActive {
			localProduct.IsActive = false
			changed = true
		}

		if changed {
			_ = s.productRepo.Update(localProduct)
		}
	}

	// ── 2. 同步 SKU：新增 / 更新 / 停用 ──
	skuMappings, err := s.skuMappingRepo.ListByProductMapping(mappingID)
	if err != nil {
		return err
	}

	// 构建上游 SKU 查找表
	upstreamSKUMap := make(map[uint]upstream.UpstreamSKU, len(upProduct.SKUs))
	for _, us := range upProduct.SKUs {
		upstreamSKUMap[us.ID] = us
	}

	// 构建已有映射查找表（按上游 SKU ID）
	existingByUpstreamID := make(map[uint]*models.SKUMapping, len(skuMappings))
	for i := range skuMappings {
		existingByUpstreamID[skuMappings[i].UpstreamSKUID] = &skuMappings[i]
	}

	// 2a. 更新已有映射 + 同步本地 SKU
	for i := range skuMappings {
		upSKU, ok := upstreamSKUMap[skuMappings[i].UpstreamSKUID]
		if !ok {
			// 上游 SKU 已删除 → 停用本地 SKU 和映射
			skuMappings[i].UpstreamIsActive = false
			skuMappings[i].UpstreamStock = 0
			skuMappings[i].StockSyncedAt = &now
			_ = s.skuMappingRepo.Update(&skuMappings[i])

			// 停用本地 SKU
			localSKU, _ := s.productSKURepo.GetByID(skuMappings[i].LocalSKUID)
			if localSKU != nil && localSKU.IsActive {
				localSKU.IsActive = false
				_ = s.productSKURepo.Update(localSKU)
			}
			continue
		}

		upPrice, _ := decimal.NewFromString(upSKU.PriceAmount)

		// 更新 SKU 映射记录
		skuMappings[i].UpstreamPrice = models.NewMoneyFromDecimal(upPrice.Round(2))
		skuMappings[i].UpstreamIsActive = upSKU.IsActive
		skuMappings[i].StockSyncedAt = &now
		skuMappings[i].UpstreamStock = upSKU.StockQuantity
		_ = s.skuMappingRepo.Update(&skuMappings[i])

		// 同步本地 SKU 字段
		localSKU, _ := s.productSKURepo.GetByID(skuMappings[i].LocalSKUID)
		if localSKU != nil {
			localSKU.SpecValuesJSON = upSKU.SpecValues
			localSKU.IsActive = upSKU.IsActive
			// 如果启用了自动同步价格，按加价比例更新本地售价
			if conn.AutoSyncPrice {
				newLocalPrice := CalculateMarkedUpPrice(upPrice, conn.PriceMarkupPercent, conn.PriceRoundingMode)
				localSKU.PriceAmount = models.NewMoneyFromDecimal(newLocalPrice.Round(2))
			}
			_ = s.productSKURepo.Update(localSKU)
		}
	}

	// 2b. 上游新增的 SKU → 创建本地 SKU + 映射
	for _, upSKU := range upProduct.SKUs {
		if _, exists := existingByUpstreamID[upSKU.ID]; exists {
			continue
		}

		skuPrice, _ := decimal.NewFromString(upSKU.PriceAmount)
		localPrice := CalculateMarkedUpPrice(skuPrice, conn.PriceMarkupPercent, conn.PriceRoundingMode)
		newLocalSKU := models.ProductSKU{
			ProductID:      mapping.LocalProductID,
			SKUCode:        upSKU.SKUCode,
			SpecValuesJSON: upSKU.SpecValues,
			PriceAmount:    models.NewMoneyFromDecimal(localPrice.Round(2)),
			IsActive:       upSKU.IsActive,
			SortOrder:      0,
		}
		if err := s.productSKURepo.Create(&newLocalSKU); err != nil {
			continue
		}

		newMapping := &models.SKUMapping{
			ProductMappingID: mappingID,
			LocalSKUID:       newLocalSKU.ID,
			UpstreamSKUID:    upSKU.ID,
			UpstreamPrice:    models.NewMoneyFromDecimal(skuPrice.Round(2)),
			UpstreamIsActive: upSKU.IsActive,
			UpstreamStock:    upSKU.StockQuantity,
			StockSyncedAt:    &now,
		}
		_ = s.skuMappingRepo.Create(newMapping)
	}

	// ── 2c. 如果启用了自动同步价格，更新 Product.PriceAmount 为最低 SKU 价格 ──
	if conn.AutoSyncPrice && localProduct != nil {
		s.recalcProductPrice(localProduct)
	}

	// ── 3. 更新同步时间 + 上游交付类型 ──
	upFulfillment := upProduct.FulfillmentType
	if upFulfillment != constants.FulfillmentTypeAuto {
		upFulfillment = constants.FulfillmentTypeManual
	}
	mapping.UpstreamFulfillmentType = upFulfillment
	mapping.LastSyncedAt = &now
	return s.mappingRepo.Update(mapping)
}

// SyncAllStock 同步所有活跃映射的库存（供定时任务调用）
func (s *ProductMappingService) SyncAllStock() error {
	mappings, err := s.mappingRepo.ListAllActive()
	if err != nil {
		return err
	}

	var lastErr error
	for _, mapping := range mappings {
		if err := s.SyncProduct(mapping.ID); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// GetByID 获取映射详情
func (s *ProductMappingService) GetByID(id uint) (*models.ProductMapping, error) {
	return s.mappingRepo.GetByID(id)
}

// List 列表查询映射
func (s *ProductMappingService) List(filter repository.ProductMappingListFilter) ([]models.ProductMapping, int64, error) {
	return s.mappingRepo.List(filter)
}

// SetActive 启用/禁用映射
func (s *ProductMappingService) SetActive(id uint, active bool) error {
	mapping, err := s.mappingRepo.GetByID(id)
	if err != nil {
		return err
	}
	if mapping == nil {
		return ErrMappingNotFound
	}
	mapping.IsActive = active
	return s.mappingRepo.Update(mapping)
}

// Delete 删除映射（不删除本地商品）
func (s *ProductMappingService) Delete(id uint) error {
	mapping, err := s.mappingRepo.GetByID(id)
	if err != nil {
		return err
	}
	if mapping == nil {
		return ErrMappingNotFound
	}

	// 删除 SKU 映射
	if err := s.skuMappingRepo.DeleteByProductMapping(id); err != nil {
		return err
	}

	// 将本地商品的 IsMapped 标记还原
	if mapping.LocalProductID > 0 {
		localProduct, err := s.productRepo.GetByID(strconv.FormatUint(uint64(mapping.LocalProductID), 10))
		if err == nil && localProduct != nil {
			localProduct.IsMapped = false
			_ = s.productRepo.Update(localProduct)
		}
	}

	return s.mappingRepo.Delete(id)
}

// GetSKUMappings 获取映射的 SKU 映射列表
func (s *ProductMappingService) GetSKUMappings(mappingID uint) ([]models.SKUMapping, error) {
	return s.skuMappingRepo.ListByProductMapping(mappingID)
}

// ReapplyMarkup 对指定连接的所有映射商品重新应用加价规则
func (s *ProductMappingService) ReapplyMarkup(connectionID uint) (int, error) {
	conn, err := s.connService.GetByID(connectionID)
	if err != nil {
		return 0, err
	}
	if conn == nil {
		return 0, ErrConnectionNotFound
	}

	mappings, err := s.mappingRepo.ListActiveByConnection(connectionID)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, mapping := range mappings {
		skuMappings, err := s.skuMappingRepo.ListByProductMapping(mapping.ID)
		if err != nil {
			continue
		}

		for _, sm := range skuMappings {
			newLocalPrice := CalculateMarkedUpPrice(sm.UpstreamPrice.Decimal, conn.PriceMarkupPercent, conn.PriceRoundingMode)
			localSKU, err := s.productSKURepo.GetByID(sm.LocalSKUID)
			if err != nil || localSKU == nil {
				continue
			}
			localSKU.PriceAmount = models.NewMoneyFromDecimal(newLocalPrice.Round(2))
			_ = s.productSKURepo.Update(localSKU)
		}

		// 更新 Product.PriceAmount
		localProduct, err := s.productRepo.GetByID(strconv.FormatUint(uint64(mapping.LocalProductID), 10))
		if err == nil && localProduct != nil {
			s.recalcProductPrice(localProduct)
			updated++
		}
	}

	return updated, nil
}

// recalcProductPrice 重新计算商品基准价格为最低活跃 SKU 价格
func (s *ProductMappingService) recalcProductPrice(product *models.Product) {
	allSKUs, err := s.productSKURepo.ListByProduct(product.ID, true)
	if err != nil || len(allSKUs) == 0 {
		return
	}
	minPrice := allSKUs[0].PriceAmount.Decimal
	for _, sku := range allSKUs[1:] {
		if sku.PriceAmount.Decimal.LessThan(minPrice) {
			minPrice = sku.PriceAmount.Decimal
		}
	}
	product.PriceAmount = models.NewMoneyFromDecimal(minPrice.Round(2))
	_ = s.productRepo.Update(product)
}

// ListUpstreamProducts 通过连接代理拉取上游商品列表
func (s *ProductMappingService) ListUpstreamProducts(connectionID uint, page, pageSize int) (*upstream.ProductListResult, error) {
	conn, err := s.connService.GetByID(connectionID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, ErrConnectionNotFound
	}

	adapter, err := s.connService.GetAdapter(conn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return adapter.ListProducts(ctx, upstream.ListProductsOpts{
		Page:     page,
		PageSize: pageSize,
	})
}
