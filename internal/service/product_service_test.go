package service

import (
	"testing"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func newSyncSingleSKURepo(t *testing.T) repository.ProductSKURepository {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.ProductSKU{}); err != nil {
		t.Fatalf("auto migrate product sku failed: %v", err)
	}
	return repository.NewProductSKURepository(db)
}

func TestSyncSingleProductSKU_MultipleRowsKeepsSingleActive(t *testing.T) {
	repo := newSyncSingleSKURepo(t)
	productID := uint(2001)

	inactiveDefault := models.ProductSKU{
		ProductID:        productID,
		SKUCode:          models.DefaultSKUCode,
		PriceAmount:      models.NewMoneyFromDecimal(decimal.NewFromInt(10)),
		ManualStockTotal: 9,
		IsActive:         false,
		SortOrder:        0,
	}
	firstActive := models.ProductSKU{
		ProductID:        productID,
		SKUCode:          "A",
		PriceAmount:      models.NewMoneyFromDecimal(decimal.NewFromInt(20)),
		ManualStockTotal: 2,
		IsActive:         true,
		SortOrder:        2,
	}
	secondActive := models.ProductSKU{
		ProductID:        productID,
		SKUCode:          "B",
		PriceAmount:      models.NewMoneyFromDecimal(decimal.NewFromInt(30)),
		ManualStockTotal: 4,
		IsActive:         true,
		SortOrder:        1,
	}
	if err := repo.Create(&inactiveDefault); err != nil {
		t.Fatalf("create inactive default sku failed: %v", err)
	}
	inactiveDefault.IsActive = false
	if err := repo.Update(&inactiveDefault); err != nil {
		t.Fatalf("update inactive default sku failed: %v", err)
	}
	if err := repo.Create(&firstActive); err != nil {
		t.Fatalf("create first active sku failed: %v", err)
	}
	if err := repo.Create(&secondActive); err != nil {
		t.Fatalf("create second active sku failed: %v", err)
	}

	targetPrice := decimal.RequireFromString("88.88")
	if err := syncSingleProductSKU(repo, productID, targetPrice, 5, true); err != nil {
		t.Fatalf("sync single sku failed: %v", err)
	}

	skus, err := repo.ListByProduct(productID, false)
	if err != nil {
		t.Fatalf("list sku failed: %v", err)
	}

	activeCount := 0
	for _, sku := range skus {
		if !sku.IsActive {
			continue
		}
		activeCount++
		if sku.ID != firstActive.ID {
			t.Fatalf("expected first active sku id=%d, got id=%d", firstActive.ID, sku.ID)
		}
		if !sku.PriceAmount.Equal(targetPrice) {
			t.Fatalf("expected price %s, got %s", targetPrice.StringFixed(2), sku.PriceAmount.String())
		}
		if sku.ManualStockTotal != 5 {
			t.Fatalf("expected manual stock total 5, got %d", sku.ManualStockTotal)
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly one active sku, got %d", activeCount)
	}
}

func TestSyncSingleProductSKU_NoActivePrefersDefaultCode(t *testing.T) {
	repo := newSyncSingleSKURepo(t)
	productID := uint(2002)

	inactiveA := models.ProductSKU{
		ProductID:        productID,
		SKUCode:          "A",
		PriceAmount:      models.NewMoneyFromDecimal(decimal.NewFromInt(10)),
		ManualStockTotal: 3,
		IsActive:         false,
		SortOrder:        1,
	}
	inactiveDefault := models.ProductSKU{
		ProductID:        productID,
		SKUCode:          models.DefaultSKUCode,
		PriceAmount:      models.NewMoneyFromDecimal(decimal.NewFromInt(20)),
		ManualStockTotal: 8,
		IsActive:         false,
		SortOrder:        0,
	}
	if err := repo.Create(&inactiveA); err != nil {
		t.Fatalf("create inactive sku A failed: %v", err)
	}
	inactiveA.IsActive = false
	if err := repo.Update(&inactiveA); err != nil {
		t.Fatalf("update inactive sku A failed: %v", err)
	}
	if err := repo.Create(&inactiveDefault); err != nil {
		t.Fatalf("create inactive default sku failed: %v", err)
	}
	inactiveDefault.IsActive = false
	if err := repo.Update(&inactiveDefault); err != nil {
		t.Fatalf("update inactive default sku failed: %v", err)
	}

	targetPrice := decimal.RequireFromString("19.90")
	if err := syncSingleProductSKU(repo, productID, targetPrice, 6, true); err != nil {
		t.Fatalf("sync single sku failed: %v", err)
	}

	skus, err := repo.ListByProduct(productID, false)
	if err != nil {
		t.Fatalf("list sku failed: %v", err)
	}

	activeCount := 0
	for _, sku := range skus {
		if !sku.IsActive {
			continue
		}
		activeCount++
		if sku.ID != inactiveDefault.ID {
			t.Fatalf("expected default sku id=%d to be active, got id=%d", inactiveDefault.ID, sku.ID)
		}
		if !sku.PriceAmount.Equal(targetPrice) {
			t.Fatalf("expected price %s, got %s", targetPrice.StringFixed(2), sku.PriceAmount.String())
		}
		if sku.ManualStockTotal != 6 {
			t.Fatalf("expected manual stock total 6, got %d", sku.ManualStockTotal)
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly one active sku, got %d", activeCount)
	}
}
