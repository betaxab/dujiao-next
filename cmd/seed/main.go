package main

import (
	"fmt"
	"github.com/dujiao-next/internal/logger"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"github.com/shopspring/decimal"
)

func main() {
	// 连接数据库
	cfg := config.Load()
	logger.Init(cfg.Server.Mode, cfg.Log.ToLoggerOptions())
	stdLog := logger.StdLogger()
	if err := models.InitDB(cfg.Database.Driver, cfg.Database.DSN, models.DBPoolConfig{
		MaxOpenConns:           cfg.Database.Pool.MaxOpenConns,
		MaxIdleConns:           cfg.Database.Pool.MaxIdleConns,
		ConnMaxLifetimeSeconds: cfg.Database.Pool.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: cfg.Database.Pool.ConnMaxIdleTimeSeconds,
	}); err != nil {
		stdLog.Fatalf("Failed to connect database: %v", err)
	}

	// 自动迁移
	if err := models.AutoMigrate(); err != nil {
		stdLog.Fatalf("Failed to migrate database: %v", err)
	}

	// 添加分类
	categories := []models.Category{
		{
			NameJSON: models.JSON(map[string]interface{}{
				"zh-CN": "电子产品",
				"zh-TW": "電子產品",
				"en-US": "Electronics",
			}),
			Slug: "electronics",
		},
		{
			NameJSON: models.JSON(map[string]interface{}{
				"zh-CN": "生活用品",
				"zh-TW": "生活用品",
				"en-US": "Lifestyle",
			}),
			Slug: "lifestyle",
		},
		{
			NameJSON: models.JSON(map[string]interface{}{
				"zh-CN": "数码配件",
				"zh-TW": "數碼配件",
				"en-US": "Accessories",
			}),
			Slug: "accessories",
		},
	}

	for _, cat := range categories {
		var existing models.Category
		if err := models.DB.Where("slug = ?", cat.Slug).First(&existing).Error; err != nil {
			// 不存在则创建
			if err := models.DB.Create(&cat).Error; err != nil {
				stdLog.Printf("Failed to create category %s: %v", cat.Slug, err)
			} else {
				stdLog.Printf("Created category: %s", cat.Slug)
			}
		} else {
			stdLog.Printf("Category already exists: %s", cat.Slug)
		}
	}

	// 获取分类ID
	categoryIDs := map[string]uint{}
	var categoryList []models.Category
	if err := models.DB.Where("slug IN ?", []string{"electronics", "lifestyle", "accessories"}).Find(&categoryList).Error; err != nil {
		stdLog.Printf("Failed to load categories: %v", err)
	}
	for _, cat := range categoryList {
		categoryIDs[cat.Slug] = cat.ID
	}
	electronicsID := categoryIDs["electronics"]
	lifestyleID := categoryIDs["lifestyle"]
	accessoriesID := categoryIDs["accessories"]

	// 添加商品
	products := []models.Product{
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "无线蓝牙耳机",
				"zh-TW": "無線藍牙耳機",
				"en-US": "Wireless Bluetooth Earphones",
			}),
			Slug: "wireless-earphones",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "高品质音质，长续航，舒适佩戴",
				"zh-TW": "高品質音質，長續航，舒適佩戴",
				"en-US": "High quality sound, long battery life, comfortable to wear",
			}),
			ContentJSON: models.JSON(map[string]interface{}{
				"zh-CN": "采用最新蓝牙5.0技术，支持主动降噪，续航时间长达24小时。人体工学设计，佩戴舒适，适合长时间使用。",
				"zh-TW": "採用最新藍牙5.0技術，支持主動降噪，續航時間長達24小時。人體工學設計，佩戴舒適，適合長時間使用。",
				"en-US": "Features the latest Bluetooth 5.0 technology, active noise cancellation, and up to 24 hours of battery life. Ergonomic design for comfortable extended wear.",
			}),
			PriceAmount:   models.NewMoneyFromDecimal(decimal.NewFromFloat(99.99)),
			PriceCurrency: "USD",
			CategoryID:    electronicsID,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1590658268037-6bf12165a8df?w=800",
			}),
			Tags:     models.StringArray([]string{"Audio", "Wireless", "Headphones"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "智能手表",
				"zh-TW": "智能手錶",
				"en-US": "Smart Watch",
			}),
			Slug: "smart-watch",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "健康监测，运动追踪，消息提醒",
				"zh-TW": "健康監測，運動追蹤，消息提醒",
				"en-US": "Health monitoring, fitness tracking, message notifications",
			}),
			ContentJSON: models.JSON(map[string]interface{}{
				"zh-CN": "全天候心率监测，多种运动模式，防水设计，支持消息推送和通话功能。",
				"zh-TW": "全天候心率監測，多種運動模式，防水設計，支持消息推送和通話功能。",
				"en-US": "24/7 heart rate monitoring, multiple sport modes, waterproof design, supports message push and calling.",
			}),
			PriceAmount:   models.NewMoneyFromDecimal(decimal.NewFromFloat(199.99)),
			PriceCurrency: "USD",
			CategoryID:    electronicsID,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1579586337278-3befd40fd17a?w=800",
			}),
			Tags:     models.StringArray([]string{"Wearable", "Health", "Smart"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "便携充电宝",
				"zh-TW": "便攜充電寶",
				"en-US": "Portable Power Bank",
			}),
			Slug: "power-bank",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "大容量，快速充电，多设备兼容",
				"zh-TW": "大容量，快速充電，多設備兼容",
				"en-US": "High capacity, fast charging, multi-device compatible",
			}),
			PriceAmount:   models.NewMoneyFromDecimal(decimal.NewFromFloat(49.99)),
			PriceCurrency: "USD",
			CategoryID:    accessoriesID,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1609091839311-d5365f9ff1c5?w=800",
			}),
			Tags:     models.StringArray([]string{"Charger", "Portable", "Accessory"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "多功能背包",
				"zh-TW": "多功能背包",
				"en-US": "Multi-function Backpack",
			}),
			Slug: "backpack",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "大容量，防水防盗，USB充电接口",
				"zh-TW": "大容量，防水防盜，USB充電接口",
				"en-US": "Large capacity, waterproof and anti-theft, USB charging port",
			}),
			PriceAmount:   models.NewMoneyFromDecimal(decimal.NewFromFloat(79.99)),
			PriceCurrency: "USD",
			CategoryID:    lifestyleID,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1553062407-98eeb64c6a62?w=800",
			}),
			Tags:     models.StringArray([]string{"Bag", "Travel", "Lifestyle"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "演示商品-人工交付（库存不限）",
				"zh-TW": "演示商品-人工交付（庫存不限）",
				"en-US": "Demo Product - Manual (Unlimited)",
			}),
			Slug: "demo-manual-unlimited",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "用于前台库存徽章展示：人工交付，库存不限。",
				"zh-TW": "用於前台庫存徽章展示：人工交付，庫存不限。",
				"en-US": "For stock badge demo: manual fulfillment with unlimited stock.",
			}),
			PriceAmount:       models.NewMoneyFromDecimal(decimal.NewFromFloat(29.90)),
			PriceCurrency:     "USD",
			CategoryID:        accessoriesID,
			PurchaseType:      constants.ProductPurchaseGuest,
			FulfillmentType:   constants.FulfillmentTypeManual,
			ManualStockTotal:  0,
			ManualStockLocked: 0,
			ManualStockSold:   0,
			SortOrder:         910,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1512499617640-c74ae3a79d37?w=800",
			}),
			Tags:     models.StringArray([]string{"库存演示", "Manual", "Unlimited"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "演示商品-人工交付（库存紧张）",
				"zh-TW": "演示商品-人工交付（庫存緊張）",
				"en-US": "Demo Product - Manual (Low Stock)",
			}),
			Slug: "demo-manual-low-stock",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "用于前台库存徽章展示：人工交付，剩余库存较低。",
				"zh-TW": "用於前台庫存徽章展示：人工交付，剩餘庫存較低。",
				"en-US": "For stock badge demo: manual fulfillment with low remaining stock.",
			}),
			PriceAmount:       models.NewMoneyFromDecimal(decimal.NewFromFloat(39.90)),
			PriceCurrency:     "USD",
			CategoryID:        accessoriesID,
			PurchaseType:      constants.ProductPurchaseMember,
			FulfillmentType:   constants.FulfillmentTypeManual,
			ManualStockTotal:  5,
			ManualStockLocked: 2,
			ManualStockSold:   1,
			SortOrder:         900,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1499951360447-b19be8fe80f5?w=800",
			}),
			Tags:     models.StringArray([]string{"库存演示", "Manual", "Low"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "演示商品-人工交付（已售罄）",
				"zh-TW": "演示商品-人工交付（已售罄）",
				"en-US": "Demo Product - Manual (Sold Out)",
			}),
			Slug: "demo-manual-sold-out",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "用于前台库存徽章与禁购按钮展示：人工交付，已售罄。",
				"zh-TW": "用於前台庫存徽章與禁購按鈕展示：人工交付，已售罄。",
				"en-US": "For stock badge and disabled purchase demo: manual fulfillment sold out.",
			}),
			PriceAmount:       models.NewMoneyFromDecimal(decimal.NewFromFloat(49.90)),
			PriceCurrency:     "USD",
			CategoryID:        lifestyleID,
			PurchaseType:      constants.ProductPurchaseGuest,
			FulfillmentType:   constants.FulfillmentTypeManual,
			ManualStockTotal:  6,
			ManualStockLocked: 1,
			ManualStockSold:   5,
			SortOrder:         890,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1516321165247-4aa89a48be28?w=800",
			}),
			Tags:     models.StringArray([]string{"库存演示", "Manual", "SoldOut"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "演示商品-自动交付（库存充足）",
				"zh-TW": "演示商品-自動交付（庫存充足）",
				"en-US": "Demo Product - Auto (In Stock)",
			}),
			Slug: "demo-auto-in-stock",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "用于前台库存徽章展示：自动交付，卡密库存充足。",
				"zh-TW": "用於前台庫存徽章展示：自動交付，卡密庫存充足。",
				"en-US": "For stock badge demo: auto fulfillment with sufficient card secrets.",
			}),
			PriceAmount:     models.NewMoneyFromDecimal(decimal.NewFromFloat(59.90)),
			PriceCurrency:   "USD",
			CategoryID:      electronicsID,
			PurchaseType:    constants.ProductPurchaseMember,
			FulfillmentType: constants.FulfillmentTypeAuto,
			SortOrder:       880,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?w=800",
			}),
			Tags:     models.StringArray([]string{"库存演示", "Auto", "InStock"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "演示商品-自动交付（库存紧张）",
				"zh-TW": "演示商品-自動交付（庫存緊張）",
				"en-US": "Demo Product - Auto (Low Stock)",
			}),
			Slug: "demo-auto-low-stock",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "用于前台库存徽章展示：自动交付，卡密库存较低。",
				"zh-TW": "用於前台庫存徽章展示：自動交付，卡密庫存較低。",
				"en-US": "For stock badge demo: auto fulfillment with low card secret stock.",
			}),
			PriceAmount:     models.NewMoneyFromDecimal(decimal.NewFromFloat(69.90)),
			PriceCurrency:   "USD",
			CategoryID:      electronicsID,
			PurchaseType:    constants.ProductPurchaseGuest,
			FulfillmentType: constants.FulfillmentTypeAuto,
			SortOrder:       870,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1511381939415-e44015466834?w=800",
			}),
			Tags:     models.StringArray([]string{"库存演示", "Auto", "Low"}),
			IsActive: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "演示商品-自动交付（已售罄）",
				"zh-TW": "演示商品-自動交付（已售罄）",
				"en-US": "Demo Product - Auto (Sold Out)",
			}),
			Slug: "demo-auto-sold-out",
			DescriptionJSON: models.JSON(map[string]interface{}{
				"zh-CN": "用于前台库存徽章与禁购按钮展示：自动交付，卡密已售罄。",
				"zh-TW": "用於前台庫存徽章與禁購按鈕展示：自動交付，卡密已售罄。",
				"en-US": "For stock badge and disabled purchase demo: auto fulfillment sold out.",
			}),
			PriceAmount:     models.NewMoneyFromDecimal(decimal.NewFromFloat(79.90)),
			PriceCurrency:   "USD",
			CategoryID:      electronicsID,
			PurchaseType:    constants.ProductPurchaseMember,
			FulfillmentType: constants.FulfillmentTypeAuto,
			SortOrder:       860,
			Images: models.StringArray([]string{
				"https://images.unsplash.com/photo-1515879218367-8466d910aaa4?w=800",
			}),
			Tags:     models.StringArray([]string{"库存演示", "Auto", "SoldOut"}),
			IsActive: true,
		},
	}

	for _, prod := range products {
		if prod.CategoryID == 0 {
			stdLog.Printf("Skip product %s: category_id missing", prod.Slug)
			continue
		}
		var existing models.Product
		if err := models.DB.Where("slug = ?", prod.Slug).First(&existing).Error; err != nil {
			if err := models.DB.Create(&prod).Error; err != nil {
				stdLog.Printf("Failed to create product %s: %v", prod.Slug, err)
			} else {
				stdLog.Printf("Created product: %s", prod.Slug)
			}
		} else {
			existing.TitleJSON = prod.TitleJSON
			existing.DescriptionJSON = prod.DescriptionJSON
			existing.ContentJSON = prod.ContentJSON
			existing.PriceAmount = prod.PriceAmount
			existing.PriceCurrency = prod.PriceCurrency
			existing.CategoryID = prod.CategoryID
			existing.Images = prod.Images
			existing.Tags = prod.Tags
			existing.PurchaseType = prod.PurchaseType
			existing.FulfillmentType = prod.FulfillmentType
			existing.ManualStockTotal = prod.ManualStockTotal
			existing.ManualStockLocked = prod.ManualStockLocked
			existing.ManualStockSold = prod.ManualStockSold
			existing.IsActive = prod.IsActive
			existing.SortOrder = prod.SortOrder
			if err := models.DB.Save(&existing).Error; err != nil {
				stdLog.Printf("Failed to update product %s: %v", prod.Slug, err)
			} else {
				stdLog.Printf("Updated product: %s", prod.Slug)
			}
		}
	}

	// 为自动交付演示商品准备卡密库存
	autoStockSeedPlans := []struct {
		Slug      string
		BatchNo   string
		Total     int
		Available int
	}{
		{Slug: "demo-auto-in-stock", BatchNo: "seed-demo-auto-in-stock", Total: 8, Available: 8},
		{Slug: "demo-auto-low-stock", BatchNo: "seed-demo-auto-low-stock", Total: 3, Available: 2},
		{Slug: "demo-auto-sold-out", BatchNo: "seed-demo-auto-sold-out", Total: 4, Available: 0},
	}

	for _, plan := range autoStockSeedPlans {
		var product models.Product
		if err := models.DB.Where("slug = ?", plan.Slug).First(&product).Error; err != nil {
			stdLog.Printf("Skip card secret seed for %s: product not found", plan.Slug)
			continue
		}

		var batch models.CardSecretBatch
		if err := models.DB.Where("batch_no = ?", plan.BatchNo).First(&batch).Error; err != nil {
			batch = models.CardSecretBatch{
				ProductID:  product.ID,
				BatchNo:    plan.BatchNo,
				Source:     constants.CardSecretSourceManual,
				TotalCount: plan.Total,
				Note:       "库存演示数据",
			}
			if err := models.DB.Create(&batch).Error; err != nil {
				stdLog.Printf("Failed to create card secret batch for %s: %v", plan.Slug, err)
				continue
			}
		} else {
			batch.ProductID = product.ID
			batch.Source = constants.CardSecretSourceManual
			batch.TotalCount = plan.Total
			batch.Note = "库存演示数据"
			if err := models.DB.Save(&batch).Error; err != nil {
				stdLog.Printf("Failed to update card secret batch for %s: %v", plan.Slug, err)
				continue
			}
		}

		for i := 1; i <= plan.Total; i++ {
			secretCode := fmt.Sprintf("%s-%03d", plan.BatchNo, i)
			status := models.CardSecretStatusUsed
			if i <= plan.Available {
				status = models.CardSecretStatusAvailable
			}

			batchID := batch.ID
			now := time.Now()
			var usedAt *time.Time
			if status == models.CardSecretStatusUsed {
				usedAt = &now
			}

			var existingSecret models.CardSecret
			if err := models.DB.Where("secret = ?", secretCode).First(&existingSecret).Error; err != nil {
				item := models.CardSecret{
					ProductID: product.ID,
					BatchID:   &batchID,
					Secret:    secretCode,
					Status:    status,
					UsedAt:    usedAt,
				}
				if err := models.DB.Create(&item).Error; err != nil {
					stdLog.Printf("Failed to create card secret %s: %v", secretCode, err)
				}
				continue
			}

			existingSecret.ProductID = product.ID
			existingSecret.BatchID = &batchID
			existingSecret.Status = status
			existingSecret.OrderID = nil
			existingSecret.ReservedAt = nil
			existingSecret.UsedAt = usedAt
			if err := models.DB.Save(&existingSecret).Error; err != nil {
				stdLog.Printf("Failed to update card secret %s: %v", secretCode, err)
			}
		}

		stdLog.Printf("Seeded auto stock for %s: total=%d available=%d", plan.Slug, plan.Total, plan.Available)
	}

	// 添加博客文章
	posts := []models.Post{
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "欢迎访问独角工作室",
				"zh-TW": "歡迎訪問獨角工作室",
				"en-US": "Welcome to D&J STUDIO",
			}),
			Slug: "welcome",
			SummaryJSON: models.JSON(map[string]interface{}{
				"zh-CN": "感谢您访问我们的网站，我们致力于提供优质的产品和服务。",
				"zh-TW": "感謝您訪問我們的網站，我們致力於提供優質的產品和服務。",
				"en-US": "Thank you for visiting our website. We are committed to providing quality products and services.",
			}),
			ContentJSON: models.JSON(map[string]interface{}{
				"zh-CN": "独角工作室成立于2024年，我们专注于为客户提供优质的电子产品和生活用品。\n\n我们的使命是通过创新和卓越的服务，为客户创造价值。无论您需要什么，我们都会全力以赴为您提供最好的产品和体验。\n\n欢迎随时通过 Telegram 或 WhatsApp 联系我们！",
				"zh-TW": "獨角工作室成立於2024年，我們專注於為客戶提供優質的電子產品和生活用品。\n\n我們的使命是通過創新和卓越的服務，為客戶創造價值。無論您需要什麼，我們都會全力以赴為您提供最好的產品和體驗。\n\n歡迎隨時通過 Telegram 或 WhatsApp 聯絡我們！",
				"en-US": "D&J STUDIO was founded in 2024. We focus on providing quality electronics and lifestyle products to our customers.\n\nOur mission is to create value for customers through innovation and excellent service. Whatever you need, we will do our best to provide you with the best products and experience.\n\nFeel free to contact us via Telegram or WhatsApp anytime!",
			}),
			Type:        constants.PostTypeBlog,
			IsPublished: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "春季新品上架通知",
				"zh-TW": "春季新品上架通知",
				"en-US": "Spring New Arrivals Announcement",
			}),
			Slug: "spring-arrivals",
			SummaryJSON: models.JSON(map[string]interface{}{
				"zh-CN": "我们的春季新品已经上架，包括多款热门电子产品和配件。",
				"zh-TW": "我們的春季新品已經上架，包括多款熱門電子產品和配件。",
				"en-US": "Our spring collection is now available, featuring popular electronics and accessories.",
			}),
			ContentJSON: models.JSON(map[string]interface{}{
				"zh-CN": "亲爱的客户，\n\n我们很高兴地宣布，春季新品现已全面上架！\n\n本次上新包括：\n- 最新款无线蓝牙耳机\n- 智能手表系列\n- 多功能背包\n- 便携充电宝\n\n所有新品均享受限时优惠，欢迎选购！",
				"zh-TW": "親愛的客戶，\n\n我們很高興地宣布，春季新品現已全面上架！\n\n本次上新包括：\n- 最新款無線藍牙耳機\n- 智能手錶系列\n- 多功能背包\n- 便攜充電寶\n\n所有新品均享受限時優惠，歡迎選購！",
				"en-US": "Dear Customers,\n\nWe are pleased to announce that our spring collection is now fully available!\n\nThis release includes:\n- Latest wireless Bluetooth earphones\n- Smart watch series\n- Multi-function backpacks\n- Portable power banks\n\nAll new products come with limited-time offers. Welcome to shop!",
			}),
			Type:        constants.PostTypeNotice,
			IsPublished: true,
		},
		{
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "如何选择合适的蓝牙耳机",
				"zh-TW": "如何選擇合適的藍牙耳機",
				"en-US": "How to Choose the Right Bluetooth Earphones",
			}),
			Slug: "choose-earphones",
			SummaryJSON: models.JSON(map[string]interface{}{
				"zh-CN": "选购蓝牙耳机时需要考虑的几个关键因素。",
				"zh-TW": "選購藍牙耳機時需要考慮的幾個關鍵因素。",
				"en-US": "Key factors to consider when buying Bluetooth earphones.",
			}),
			ContentJSON: models.JSON(map[string]interface{}{
				"zh-CN": "选择蓝牙耳机时，以下几点很重要：\n\n1. 音质表现\n2. 续航时间\n3. 佩戴舒适度\n4. 降噪功能\n5. 价格预算\n\n根据您的需求和预算，我们可以为您推荐最适合的产品。",
				"zh-TW": "選擇藍牙耳機時，以下幾點很重要：\n\n1. 音質表現\n2. 續航時間\n3. 佩戴舒適度\n4. 降噪功能\n5. 價格預算\n\n根據您的需求和預算，我們可以為您推薦最適合的產品。",
				"en-US": "When choosing Bluetooth earphones, these factors are important:\n\n1. Sound quality\n2. Battery life\n3. Comfort\n4. Noise cancellation\n5. Budget\n\nBased on your needs and budget, we can recommend the most suitable products for you.",
			}),
			Type:        constants.PostTypeBlog,
			IsPublished: true,
		},
	}

	for _, post := range posts {
		var existing models.Post
		if err := models.DB.Where("slug = ?", post.Slug).First(&existing).Error; err != nil {
			if err := models.DB.Create(&post).Error; err != nil {
				stdLog.Printf("Failed to create post %s: %v", post.Slug, err)
			} else {
				stdLog.Printf("Created post: %s", post.Slug)
			}
		} else {
			stdLog.Printf("Post already exists: %s", post.Slug)
		}
	}

	// 添加 Banner（首页主视觉）
	now := time.Now()
	primaryStart := now.Add(-24 * time.Hour)
	primaryEnd := now.AddDate(0, 2, 0)
	secondaryStart := now.Add(-12 * time.Hour)
	secondaryEnd := now.AddDate(0, 1, 0)
	draftStart := now.Add(-6 * time.Hour)
	draftEnd := now.AddDate(0, 0, 15)
	flashSaleStart := now.Add(-2 * time.Hour)
	flashSaleEnd := now.AddDate(0, 0, 7)
	newArrivalStart := now.Add(-8 * time.Hour)
	newArrivalEnd := now.AddDate(0, 1, 15)

	banners := []models.Banner{
		{
			Name:     "首页主视觉-新品精选",
			Position: constants.BannerPositionHomeHero,
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "2026 春季上新",
				"zh-TW": "2026 春季上新",
				"en-US": "Spring 2026 Collection",
			}),
			SubtitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "耳机、手表与数码配件限时优惠",
				"zh-TW": "耳機、手錶與數碼配件限時優惠",
				"en-US": "Limited offers on audio, watches and accessories",
			}),
			Image:        "https://images.unsplash.com/photo-1498049794561-7780e7231661?auto=format&fit=crop&w=1920&q=80",
			MobileImage:  "https://images.unsplash.com/photo-1498049794561-7780e7231661?auto=format&fit=crop&w=900&q=80",
			LinkType:     constants.BannerLinkTypeInternal,
			LinkValue:    "/products",
			OpenInNewTab: false,
			IsActive:     true,
			StartAt:      &primaryStart,
			EndAt:        &primaryEnd,
			SortOrder:    300,
		},
		{
			Name:     "首页主视觉-品牌故事",
			Position: constants.BannerPositionHomeHero,
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "关于独角工作室",
				"zh-TW": "關於獨角工作室",
				"en-US": "About D&J Studio",
			}),
			SubtitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "了解我们的选品标准与服务承诺",
				"zh-TW": "了解我們的選品標準與服務承諾",
				"en-US": "See our product standards and service commitment",
			}),
			Image:        "https://images.unsplash.com/photo-1556740749-887f6717d7e4?auto=format&fit=crop&w=1920&q=80",
			MobileImage:  "https://images.unsplash.com/photo-1556740749-887f6717d7e4?auto=format&fit=crop&w=900&q=80",
			LinkType:     constants.BannerLinkTypeExternal,
			LinkValue:    "https://dujiao.studio/about",
			OpenInNewTab: true,
			IsActive:     true,
			StartAt:      &secondaryStart,
			EndAt:        &secondaryEnd,
			SortOrder:    200,
		},
		{
			Name:     "首页主视觉-预备素材",
			Position: constants.BannerPositionHomeHero,
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "后台预备 Banner",
				"zh-TW": "後台預備 Banner",
				"en-US": "Prepared Banner",
			}),
			SubtitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "用于演示启停与排序调整",
				"zh-TW": "用於演示啟停與排序調整",
				"en-US": "For demo of enable and sorting controls",
			}),
			Image:        "https://images.unsplash.com/photo-1545239351-1141bd82e8a6?auto=format&fit=crop&w=1920&q=80",
			MobileImage:  "https://images.unsplash.com/photo-1545239351-1141bd82e8a6?auto=format&fit=crop&w=900&q=80",
			LinkType:     constants.BannerLinkTypeNone,
			LinkValue:    "",
			OpenInNewTab: false,
			IsActive:     false,
			StartAt:      &draftStart,
			EndAt:        &draftEnd,
			SortOrder:    100,
		},
		{
			Name:     "首页主视觉-爆款闪购",
			Position: constants.BannerPositionHomeHero,
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "72 小时闪购专场",
				"zh-TW": "72 小時閃購專場",
				"en-US": "72-Hour Flash Sale",
			}),
			SubtitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "热门耳机与智能手表低至 6 折",
				"zh-TW": "熱門耳機與智能手錶低至 6 折",
				"en-US": "Top audio and smart watches up to 40% off",
			}),
			Image:        "https://images.unsplash.com/photo-1607082349566-187342175e2f?auto=format&fit=crop&w=1920&q=80",
			MobileImage:  "https://images.unsplash.com/photo-1607082349566-187342175e2f?auto=format&fit=crop&w=900&q=80",
			LinkType:     constants.BannerLinkTypeInternal,
			LinkValue:    "/products?tag=flash-sale",
			OpenInNewTab: false,
			IsActive:     true,
			StartAt:      &flashSaleStart,
			EndAt:        &flashSaleEnd,
			SortOrder:    260,
		},
		{
			Name:     "首页主视觉-新品到货",
			Position: constants.BannerPositionHomeHero,
			TitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "本周新品到货",
				"zh-TW": "本週新品到貨",
				"en-US": "New Arrivals This Week",
			}),
			SubtitleJSON: models.JSON(map[string]interface{}{
				"zh-CN": "上新 20+ 款数码配件，立即抢先体验",
				"zh-TW": "上新 20+ 款數碼配件，立即搶先體驗",
				"en-US": "20+ new accessories just landed, explore now",
			}),
			Image:        "https://images.unsplash.com/photo-1517336714739-489689fd1ca8?auto=format&fit=crop&w=1920&q=80",
			MobileImage:  "https://images.unsplash.com/photo-1517336714739-489689fd1ca8?auto=format&fit=crop&w=900&q=80",
			LinkType:     constants.BannerLinkTypeInternal,
			LinkValue:    "/products?sort=new",
			OpenInNewTab: false,
			IsActive:     true,
			StartAt:      &newArrivalStart,
			EndAt:        &newArrivalEnd,
			SortOrder:    240,
		},
	}

	for _, banner := range banners {
		var existing models.Banner
		if err := models.DB.Where("name = ? AND position = ?", banner.Name, banner.Position).First(&existing).Error; err != nil {
			if err := models.DB.Select("*").Create(&banner).Error; err != nil {
				stdLog.Printf("Failed to create banner %s: %v", banner.Name, err)
			} else {
				stdLog.Printf("Created banner: %s", banner.Name)
			}
		} else {
			existing.TitleJSON = banner.TitleJSON
			existing.SubtitleJSON = banner.SubtitleJSON
			existing.Image = banner.Image
			existing.MobileImage = banner.MobileImage
			existing.LinkType = banner.LinkType
			existing.LinkValue = banner.LinkValue
			existing.OpenInNewTab = banner.OpenInNewTab
			existing.IsActive = banner.IsActive
			existing.StartAt = banner.StartAt
			existing.EndAt = banner.EndAt
			existing.SortOrder = banner.SortOrder
			if err := models.DB.Save(&existing).Error; err != nil {
				stdLog.Printf("Failed to update banner %s: %v", banner.Name, err)
			} else {
				stdLog.Printf("Updated banner: %s", banner.Name)
			}
		}
	}

	// 更新网站配置
	configData := map[string]interface{}{
		"contact": map[string]string{
			"telegram": "https://t.me/dujiaostudio",
			"whatsapp": "https://wa.me/1234567890",
		},
	}

	var setting models.Setting
	if err := models.DB.Where("key = ?", "site_config").First(&setting).Error; err != nil {
		// 不存在则创建
		setting = models.Setting{
			Key:       "site_config",
			ValueJSON: models.JSON(configData),
		}
		if err := models.DB.Create(&setting).Error; err != nil {
			stdLog.Printf("Failed to create setting: %v", err)
		} else {
			stdLog.Println("Created site config")
		}
	} else {
		// 更新
		setting.ValueJSON = models.JSON(configData)
		if err := models.DB.Save(&setting).Error; err != nil {
			stdLog.Printf("Failed to update setting: %v", err)
		} else {
			stdLog.Println("Updated site config")
		}
	}

	fmt.Println("\n✅ Test data created successfully!")
	fmt.Println("Summary:")
	fmt.Println("- 3 Categories")
	fmt.Println("- 10 Products (含库存演示商品)")
	fmt.Println("- 3 Posts (2 blog + 1 notice)")
	fmt.Println("- 5 Banners (home_hero)")
	fmt.Println("- 3 Auto stock plans with card secrets")
	fmt.Println("- Site configuration")
}
