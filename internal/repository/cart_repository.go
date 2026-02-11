package repository

import (
	"errors"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// CartRepository 购物车数据访问接口
type CartRepository interface {
	ListByUser(userID uint) ([]models.CartItem, error)
	Upsert(item *models.CartItem) error
	DeleteByUserAndProduct(userID, productID uint) error
	ClearByUser(userID uint) error
	WithTx(tx *gorm.DB) *GormCartRepository
}

// GormCartRepository GORM 实现
type GormCartRepository struct {
	db *gorm.DB
}

// NewCartRepository 创建购物车仓库
func NewCartRepository(db *gorm.DB) *GormCartRepository {
	return &GormCartRepository{db: db}
}

// WithTx 绑定事务
func (r *GormCartRepository) WithTx(tx *gorm.DB) *GormCartRepository {
	if tx == nil {
		return r
	}
	return &GormCartRepository{db: tx}
}

// ListByUser 获取用户购物车项
func (r *GormCartRepository) ListByUser(userID uint) ([]models.CartItem, error) {
	var items []models.CartItem
	if err := r.db.Preload("Product").Where("user_id = ?", userID).Order("updated_at desc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// Upsert 添加或更新购物车项
func (r *GormCartRepository) Upsert(item *models.CartItem) error {
	if item == nil {
		return nil
	}
	var existing models.CartItem
	err := r.db.Where("user_id = ? AND product_id = ?", item.UserID, item.ProductID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(item).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"quantity":         item.Quantity,
		"fulfillment_type": item.FulfillmentType,
		"updated_at":       item.UpdatedAt,
	}
	return r.db.Model(&existing).Updates(updates).Error
}

// DeleteByUserAndProduct 删除购物车项
func (r *GormCartRepository) DeleteByUserAndProduct(userID, productID uint) error {
	return r.db.Where("user_id = ? AND product_id = ?", userID, productID).Delete(&models.CartItem{}).Error
}

// ClearByUser 清空购物车
func (r *GormCartRepository) ClearByUser(userID uint) error {
	return r.db.Where("user_id = ?", userID).Delete(&models.CartItem{}).Error
}
