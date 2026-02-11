package service

import (
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

func summarizeManualStockItems(items []models.OrderItem) map[uint]int {
	result := make(map[uint]int)
	for _, item := range items {
		if strings.TrimSpace(item.FulfillmentType) != constants.FulfillmentTypeManual {
			continue
		}
		if item.ProductID == 0 || item.Quantity <= 0 {
			continue
		}
		result[item.ProductID] += item.Quantity
	}
	return result
}

func releaseManualStockByItems(productRepo repository.ProductRepository, items []models.OrderItem) error {
	for productID, quantity := range summarizeManualStockItems(items) {
		product, err := productRepo.GetByID(strconv.FormatUint(uint64(productID), 10))
		if err != nil {
			return err
		}
		if product == nil || product.ManualStockTotal <= 0 {
			continue
		}
		if _, err := productRepo.ReleaseManualStock(productID, quantity); err != nil {
			return err
		}
	}
	return nil
}

func consumeManualStockByItems(productRepo repository.ProductRepository, items []models.OrderItem) error {
	for productID, quantity := range summarizeManualStockItems(items) {
		product, err := productRepo.GetByID(strconv.FormatUint(uint64(productID), 10))
		if err != nil {
			return err
		}
		if product == nil || product.ManualStockTotal <= 0 {
			continue
		}
		if _, err := productRepo.ConsumeManualStock(productID, quantity); err != nil {
			return err
		}
	}
	return nil
}
