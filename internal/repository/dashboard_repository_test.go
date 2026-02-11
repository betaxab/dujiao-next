package repository

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func setupDashboardRepositoryTest(t *testing.T) (*GormDashboardRepository, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.Category{}, &models.Product{}, &models.Order{}, &models.OrderItem{}); err != nil {
		t.Fatalf("migrate dashboard models failed: %v", err)
	}
	return NewDashboardRepository(db), db
}

func TestGetTopProductsIncludesChildOrderItems(t *testing.T) {
	repo, db := setupDashboardRepositoryTest(t)
	now := time.Now()

	category := &models.Category{
		Slug:     "test-category",
		NameJSON: models.JSON{"zh-CN": "测试分类"},
	}
	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}

	product := &models.Product{
		CategoryID:      category.ID,
		Slug:            "test-dashboard-product",
		TitleJSON:       models.JSON{"zh-CN": "测试商品"},
		PriceAmount:     models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		PriceCurrency:   "CNY",
		PurchaseType:    constants.ProductPurchaseMember,
		FulfillmentType: constants.FulfillmentTypeManual,
		IsActive:        true,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	parentOrder := &models.Order{
		OrderNo:        "DJ-TEST-PARENT",
		UserID:         1,
		Status:         constants.OrderStatusPaid,
		Currency:       "CNY",
		OriginalAmount: models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		DiscountAmount: models.NewMoneyFromDecimal(decimal.Zero),
		TotalAmount:    models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		CreatedAt:      now,
	}
	if err := db.Create(parentOrder).Error; err != nil {
		t.Fatalf("create parent order failed: %v", err)
	}

	childOrder := &models.Order{
		OrderNo:        "DJ-TEST-PARENT-01",
		ParentID:       &parentOrder.ID,
		UserID:         1,
		Status:         constants.OrderStatusPaid,
		Currency:       "CNY",
		OriginalAmount: models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		DiscountAmount: models.NewMoneyFromDecimal(decimal.Zero),
		TotalAmount:    models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		CreatedAt:      now,
	}
	if err := db.Create(childOrder).Error; err != nil {
		t.Fatalf("create child order failed: %v", err)
	}

	orderItem := &models.OrderItem{
		OrderID:           childOrder.ID,
		ProductID:         product.ID,
		TitleJSON:         models.JSON{"zh-CN": "测试商品"},
		UnitPrice:         models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		Quantity:          2,
		TotalPrice:        models.NewMoneyFromDecimal(decimal.NewFromInt(200)),
		CouponDiscount:    models.NewMoneyFromDecimal(decimal.NewFromInt(10)),
		PromotionDiscount: models.NewMoneyFromDecimal(decimal.NewFromInt(20)),
		FulfillmentType:   constants.FulfillmentTypeManual,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(orderItem).Error; err != nil {
		t.Fatalf("create order item failed: %v", err)
	}

	rows, err := repo.GetTopProducts(now.Add(-time.Hour), now.Add(time.Hour), 5)
	if err != nil {
		t.Fatalf("get top products failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len want 1 got %d", len(rows))
	}
	if rows[0].ProductID != product.ID {
		t.Fatalf("product id want %d got %d", product.ID, rows[0].ProductID)
	}
	if rows[0].PaidOrders != 1 {
		t.Fatalf("paid orders want 1 got %d", rows[0].PaidOrders)
	}
	if rows[0].Quantity != 2 {
		t.Fatalf("quantity want 2 got %d", rows[0].Quantity)
	}
	if rows[0].PaidAmount != 170 {
		t.Fatalf("paid amount want 170 got %.2f", rows[0].PaidAmount)
	}
}
