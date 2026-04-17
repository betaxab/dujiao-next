package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestBuildFulfillmentBatchRemarkMap(t *testing.T) {
	dsn := fmt.Sprintf("file:order_service_batch_remark_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.CardSecretBatch{}, &models.CardSecret{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	batchA := models.CardSecretBatch{
		ProductID:  1,
		SKUID:      1,
		BatchNo:    "BATCH-A",
		Source:     "manual",
		TotalCount: 2,
		Remark:     "  商品说明A  ",
	}
	batchB := models.CardSecretBatch{
		ProductID:  1,
		SKUID:      1,
		BatchNo:    "BATCH-B",
		Source:     "manual",
		TotalCount: 1,
		Remark:     "商品说明B",
	}
	batchC := models.CardSecretBatch{
		ProductID:  1,
		SKUID:      1,
		BatchNo:    "BATCH-C",
		Source:     "manual",
		TotalCount: 1,
		Remark:     "商品说明A",
	}
	for _, batch := range []*models.CardSecretBatch{&batchA, &batchB, &batchC} {
		if err := db.Create(batch).Error; err != nil {
			t.Fatalf("create batch failed: %v", err)
		}
	}

	parentOrderID := uint(101)
	childOrderID := uint(102)
	missingBatchID := uint(99999)

	secrets := []models.CardSecret{
		{
			ProductID: 1, SKUID: 1, BatchID: &batchA.ID, Secret: "A-1", Status: models.CardSecretStatusUsed, OrderID: &parentOrderID,
		},
		{
			ProductID: 1, SKUID: 1, BatchID: &batchA.ID, Secret: "A-2", Status: models.CardSecretStatusUsed, OrderID: &parentOrderID,
		},
		{
			ProductID: 1, SKUID: 1, BatchID: &batchB.ID, Secret: "B-1", Status: models.CardSecretStatusUsed, OrderID: &parentOrderID,
		},
		{
			ProductID: 1, SKUID: 1, BatchID: &batchB.ID, Secret: "B-RESERVED", Status: models.CardSecretStatusReserved, OrderID: &parentOrderID,
		},
		{
			ProductID: 1, SKUID: 1, BatchID: &batchC.ID, Secret: "C-1", Status: models.CardSecretStatusUsed, OrderID: &childOrderID,
		},
		{
			ProductID: 1, SKUID: 1, BatchID: &missingBatchID, Secret: "MISSING-1", Status: models.CardSecretStatusUsed, OrderID: &childOrderID,
		},
	}
	if err := db.Create(&secrets).Error; err != nil {
		t.Fatalf("create card secrets failed: %v", err)
	}

	svc := NewOrderService(OrderServiceOptions{
		CardSecretRepo:      repository.NewCardSecretRepository(db),
		CardSecretBatchRepo: repository.NewCardSecretBatchRepository(db),
	})

	order := &models.Order{
		ID:      parentOrderID,
		OrderNo: "DJ-PARENT-001",
		Children: []models.Order{
			{
				ID:      childOrderID,
				OrderNo: "DJ-PARENT-001-1",
			},
		},
	}

	remarks := svc.BuildFulfillmentBatchRemarkMap(order)
	if got := remarks[parentOrderID]; got != "商品说明A\n商品说明B" {
		t.Fatalf("unexpected parent batch remark: %q", got)
	}
	if got := remarks[childOrderID]; got != "商品说明A" {
		t.Fatalf("unexpected child batch remark: %q", got)
	}
}
