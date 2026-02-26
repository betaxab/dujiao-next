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
	if err := db.AutoMigrate(&models.User{}, &models.Category{}, &models.Product{}, &models.Order{}, &models.OrderItem{}); err != nil {
		t.Fatalf("migrate dashboard models failed: %v", err)
	}
	if err := db.AutoMigrate(&models.PaymentChannel{}, &models.Payment{}); err != nil {
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

func TestPaymentStatsExcludeWalletProvider(t *testing.T) {
	repo, db := setupDashboardRepositoryTest(t)
	now := time.Now().UTC().Truncate(time.Second)

	channel := &models.PaymentChannel{
		Name:            "支付宝",
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionRedirect,
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		IsActive:        true,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	onlineSuccess := &models.Payment{
		OrderID:         1,
		ChannelID:       channel.ID,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(120)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.PaymentStatusSuccess,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(onlineSuccess).Error; err != nil {
		t.Fatalf("create online success payment failed: %v", err)
	}

	onlineFailed := &models.Payment{
		OrderID:         2,
		ChannelID:       channel.ID,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(88)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.PaymentStatusFailed,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(onlineFailed).Error; err != nil {
		t.Fatalf("create online failed payment failed: %v", err)
	}

	walletSuccess := &models.Payment{
		OrderID:         3,
		ChannelID:       0,
		ProviderType:    constants.PaymentProviderWallet,
		ChannelType:     constants.PaymentChannelTypeBalance,
		InteractionMode: constants.PaymentInteractionBalance,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(59)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.PaymentStatusSuccess,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(walletSuccess).Error; err != nil {
		t.Fatalf("create wallet payment failed: %v", err)
	}

	startAt := now.Add(-time.Hour)
	endAt := now.Add(time.Hour)

	overview, err := repo.GetOverview(startAt, endAt)
	if err != nil {
		t.Fatalf("get overview failed: %v", err)
	}
	if overview.PaymentsTotal != 2 {
		t.Fatalf("payments total want 2 got %d", overview.PaymentsTotal)
	}
	if overview.PaymentsSuccess != 1 {
		t.Fatalf("payments success want 1 got %d", overview.PaymentsSuccess)
	}
	if overview.PaymentsFailed != 1 {
		t.Fatalf("payments failed want 1 got %d", overview.PaymentsFailed)
	}

	trends, err := repo.GetPaymentTrends(startAt, endAt)
	if err != nil {
		t.Fatalf("get payment trends failed: %v", err)
	}
	if len(trends) == 0 {
		t.Fatalf("payment trends should not be empty")
	}
	point := trends[0]
	if point.PaymentsSuccess != 1 {
		t.Fatalf("trend payments success want 1 got %d", point.PaymentsSuccess)
	}
	if point.PaymentsFailed != 1 {
		t.Fatalf("trend payments failed want 1 got %d", point.PaymentsFailed)
	}
	if point.GMVPaid != 120 {
		t.Fatalf("trend paid amount want 120 got %.2f", point.GMVPaid)
	}

	topChannels, err := repo.GetTopChannels(startAt, endAt, 5)
	if err != nil {
		t.Fatalf("get top channels failed: %v", err)
	}
	if len(topChannels) != 1 {
		t.Fatalf("top channels len want 1 got %d", len(topChannels))
	}
	if topChannels[0].ProviderType != constants.PaymentProviderOfficial {
		t.Fatalf("top channel provider want %s got %s", constants.PaymentProviderOfficial, topChannels[0].ProviderType)
	}
}
