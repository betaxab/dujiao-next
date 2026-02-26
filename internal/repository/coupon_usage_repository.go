package repository

import (
	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// CouponUsageRepository 优惠券使用记录数据访问接口
type CouponUsageRepository interface {
	Create(usage *models.CouponUsage) error
	CountByUser(couponID, userID uint) (int64, error)
	ListByOrderID(orderID uint) ([]models.CouponUsage, error)
	ListByUser(filter CouponUsageListFilter) ([]models.CouponUsage, int64, error)
	DeleteByOrderID(orderID uint) error
	WithTx(tx *gorm.DB) *GormCouponUsageRepository
}

// GormCouponUsageRepository GORM 实现
type GormCouponUsageRepository struct {
	db *gorm.DB
}

// NewCouponUsageRepository 创建优惠券使用记录仓库
func NewCouponUsageRepository(db *gorm.DB) *GormCouponUsageRepository {
	return &GormCouponUsageRepository{db: db}
}

// WithTx 绑定事务
func (r *GormCouponUsageRepository) WithTx(tx *gorm.DB) *GormCouponUsageRepository {
	if tx == nil {
		return r
	}
	return &GormCouponUsageRepository{db: tx}
}

// Create 创建使用记录
func (r *GormCouponUsageRepository) Create(usage *models.CouponUsage) error {
	return r.db.Create(usage).Error
}

// CountByUser 获取用户使用次数
func (r *GormCouponUsageRepository) CountByUser(couponID, userID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&models.CouponUsage{}).
		Where("coupon_id = ? AND user_id = ?", couponID, userID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListByOrderID 获取订单使用记录
func (r *GormCouponUsageRepository) ListByOrderID(orderID uint) ([]models.CouponUsage, error) {
	var usages []models.CouponUsage
	if err := r.db.Where("order_id = ?", orderID).Find(&usages).Error; err != nil {
		return nil, err
	}
	return usages, nil
}

// ListByUser 获取用户使用记录
func (r *GormCouponUsageRepository) ListByUser(filter CouponUsageListFilter) ([]models.CouponUsage, int64, error) {
	query := r.db.Model(&models.CouponUsage{}).Where("user_id = ?", filter.UserID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = applyPagination(query, filter.Page, filter.PageSize)

	var usages []models.CouponUsage
	if err := query.Order("id desc").Find(&usages).Error; err != nil {
		return nil, 0, err
	}
	return usages, total, nil
}

// DeleteByOrderID 删除订单使用记录
func (r *GormCouponUsageRepository) DeleteByOrderID(orderID uint) error {
	return r.db.Where("order_id = ?", orderID).Delete(&models.CouponUsage{}).Error
}
