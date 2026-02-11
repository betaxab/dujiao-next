package repository

import (
	"errors"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// FulfillmentRepository 交付数据访问接口
type FulfillmentRepository interface {
	Create(fulfillment *models.Fulfillment) error
	GetByOrderID(orderID uint) (*models.Fulfillment, error)
}

// GormFulfillmentRepository GORM 实现
type GormFulfillmentRepository struct {
	db *gorm.DB
}

// NewFulfillmentRepository 创建交付仓库
func NewFulfillmentRepository(db *gorm.DB) *GormFulfillmentRepository {
	return &GormFulfillmentRepository{db: db}
}

// Create 创建交付记录
func (r *GormFulfillmentRepository) Create(fulfillment *models.Fulfillment) error {
	return r.db.Create(fulfillment).Error
}

// GetByOrderID 根据订单 ID 获取交付记录
func (r *GormFulfillmentRepository) GetByOrderID(orderID uint) (*models.Fulfillment, error) {
	var fulfillment models.Fulfillment
	if err := r.db.Where("order_id = ?", orderID).First(&fulfillment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &fulfillment, nil
}
