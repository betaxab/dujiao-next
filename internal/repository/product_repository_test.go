package repository

import (
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func setupProductRepositoryTest(t *testing.T) (*GormProductRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.Product{}); err != nil {
		t.Fatalf("migrate product failed: %v", err)
	}
	return NewProductRepository(db), db
}

func createManualProduct(t *testing.T, repo *GormProductRepository, slug string, total int, locked int, sold int) *models.Product {
	t.Helper()
	product := &models.Product{
		CategoryID:        1,
		Slug:              slug,
		TitleJSON:         models.JSON{"zh-CN": "测试商品"},
		PriceAmount:       models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		PriceCurrency:     "CNY",
		PurchaseType:      constants.ProductPurchaseMember,
		FulfillmentType:   constants.FulfillmentTypeManual,
		ManualStockTotal:  total,
		ManualStockLocked: locked,
		ManualStockSold:   sold,
		IsActive:          true,
	}
	if err := repo.Create(product); err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	return product
}

func TestManualStockReserveReleaseConsumeLifecycle(t *testing.T) {
	repo, db := setupProductRepositoryTest(t)
	product := createManualProduct(t, repo, "manual-stock-lifecycle", 10, 0, 0)

	affected, err := repo.ReserveManualStock(product.ID, 3)
	if err != nil {
		t.Fatalf("reserve stock failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("reserve affected want 1 got %d", affected)
	}

	affected, err = repo.ConsumeManualStock(product.ID, 2)
	if err != nil {
		t.Fatalf("consume stock failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("consume affected want 1 got %d", affected)
	}

	affected, err = repo.ReleaseManualStock(product.ID, 1)
	if err != nil {
		t.Fatalf("release stock failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("release affected want 1 got %d", affected)
	}

	var got models.Product
	if err := db.First(&got, product.ID).Error; err != nil {
		t.Fatalf("reload product failed: %v", err)
	}
	if got.ManualStockLocked != 0 {
		t.Fatalf("locked want 0 got %d", got.ManualStockLocked)
	}
	if got.ManualStockSold != 2 {
		t.Fatalf("sold want 2 got %d", got.ManualStockSold)
	}

	affected, err = repo.ReserveManualStock(product.ID, 9)
	if err != nil {
		t.Fatalf("reserve over available failed: %v", err)
	}
	if affected != 0 {
		t.Fatalf("reserve over available affected want 0 got %d", affected)
	}

	affected, err = repo.ReserveManualStock(product.ID, 8)
	if err != nil {
		t.Fatalf("reserve exact available failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("reserve exact available affected want 1 got %d", affected)
	}
}

func TestManualStockConsumeWithLegacyUnreservedOrder(t *testing.T) {
	repo, db := setupProductRepositoryTest(t)
	product := createManualProduct(t, repo, "manual-stock-legacy", 5, 0, 1)

	affected, err := repo.ConsumeManualStock(product.ID, 2)
	if err != nil {
		t.Fatalf("consume stock failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("consume affected want 1 got %d", affected)
	}

	var got models.Product
	if err := db.First(&got, product.ID).Error; err != nil {
		t.Fatalf("reload product failed: %v", err)
	}
	if got.ManualStockLocked != 0 {
		t.Fatalf("locked want 0 got %d", got.ManualStockLocked)
	}
	if got.ManualStockSold != 3 {
		t.Fatalf("sold want 3 got %d", got.ManualStockSold)
	}
}

func TestManualStockUnlimitedDoesNotReserve(t *testing.T) {
	repo, _ := setupProductRepositoryTest(t)
	product := createManualProduct(t, repo, "manual-stock-unlimited", 0, 0, 0)

	affected, err := repo.ReserveManualStock(product.ID, 1)
	if err != nil {
		t.Fatalf("reserve unlimited stock failed: %v", err)
	}
	if affected != 0 {
		t.Fatalf("reserve unlimited affected want 0 got %d", affected)
	}

	affected, err = repo.ConsumeManualStock(product.ID, 1)
	if err != nil {
		t.Fatalf("consume unlimited stock failed: %v", err)
	}
	if affected != 0 {
		t.Fatalf("consume unlimited affected want 0 got %d", affected)
	}
}
