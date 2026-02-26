package repository

import (
	"errors"
	"fmt"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// CouponRepository 优惠券数据访问接口
type CouponRepository interface {
	GetByID(id uint) (*models.Coupon, error)
	GetByCode(code string) (*models.Coupon, error)
	ListByIDs(ids []uint) ([]models.Coupon, error)
	Create(coupon *models.Coupon) error
	Update(coupon *models.Coupon) error
	Delete(id uint) error
	List(filter CouponListFilter) ([]models.Coupon, int64, error)
	IncrementUsedCount(id uint, delta int) error
	DecrementUsedCount(id uint, delta int) error
	WithTx(tx *gorm.DB) *GormCouponRepository
}

// CouponListFilter 优惠券列表筛选
type CouponListFilter struct {
	ID         uint
	Code       string
	ScopeRefID uint
	IsActive   *bool
	Page       int
	PageSize   int
}

// GormCouponRepository GORM 实现
type GormCouponRepository struct {
	db *gorm.DB
}

// NewCouponRepository 创建优惠券仓库
func NewCouponRepository(db *gorm.DB) *GormCouponRepository {
	return &GormCouponRepository{db: db}
}

// WithTx 绑定事务
func (r *GormCouponRepository) WithTx(tx *gorm.DB) *GormCouponRepository {
	if tx == nil {
		return r
	}
	return &GormCouponRepository{db: tx}
}

// GetByID 根据ID获取优惠券
func (r *GormCouponRepository) GetByID(id uint) (*models.Coupon, error) {
	var coupon models.Coupon
	if err := r.db.First(&coupon, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &coupon, nil
}

// GetByCode 根据优惠码获取优惠券
func (r *GormCouponRepository) GetByCode(code string) (*models.Coupon, error) {
	var coupon models.Coupon
	if err := r.db.Where("code = ?", code).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &coupon, nil
}

// ListByIDs 批量获取优惠券
func (r *GormCouponRepository) ListByIDs(ids []uint) ([]models.Coupon, error) {
	if len(ids) == 0 {
		return []models.Coupon{}, nil
	}
	var coupons []models.Coupon
	if err := r.db.Where("id IN ?", ids).Find(&coupons).Error; err != nil {
		return nil, err
	}
	return coupons, nil
}

// Create 创建优惠券
func (r *GormCouponRepository) Create(coupon *models.Coupon) error {
	return r.db.Create(coupon).Error
}

// Update 更新优惠券
func (r *GormCouponRepository) Update(coupon *models.Coupon) error {
	return r.db.Save(coupon).Error
}

// Delete 删除优惠券
func (r *GormCouponRepository) Delete(id uint) error {
	return r.db.Delete(&models.Coupon{}, id).Error
}

// List 获取优惠券列表
func (r *GormCouponRepository) List(filter CouponListFilter) ([]models.Coupon, int64, error) {
	var coupons []models.Coupon
	query := r.db.Model(&models.Coupon{})

	if filter.ID > 0 {
		query = query.Where("id = ?", filter.ID)
	}
	if filter.Code != "" {
		query = query.Where("code = ?", filter.Code)
	}
	if filter.ScopeRefID > 0 {
		// scope_ref_ids 存储格式为 JSON 数组（例如 [1,2,3]），按边界匹配避免误命中（如 1 命中 11）。
		exact := fmt.Sprintf("[%d]", filter.ScopeRefID)
		prefix := fmt.Sprintf("[%d,%%", filter.ScopeRefID)
		middle := fmt.Sprintf("%%,%d,%%", filter.ScopeRefID)
		suffix := fmt.Sprintf("%%,%d]", filter.ScopeRefID)
		query = query.Where(
			"(scope_ref_ids = ? OR scope_ref_ids LIKE ? OR scope_ref_ids LIKE ? OR scope_ref_ids LIKE ?)",
			exact,
			prefix,
			middle,
			suffix,
		)
	}
	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = applyPagination(query, filter.Page, filter.PageSize)

	if err := query.Order("id desc").Find(&coupons).Error; err != nil {
		return nil, 0, err
	}
	return coupons, total, nil
}

// IncrementUsedCount 增加优惠券使用次数
func (r *GormCouponRepository) IncrementUsedCount(id uint, delta int) error {
	if delta == 0 {
		delta = 1
	}
	return r.db.Model(&models.Coupon{}).
		Where("id = ?", id).
		UpdateColumn("used_count", gorm.Expr("used_count + ?", delta)).Error
}

// DecrementUsedCount 减少优惠券使用次数
func (r *GormCouponRepository) DecrementUsedCount(id uint, delta int) error {
	if delta == 0 {
		delta = 1
	}
	if delta < 0 {
		delta = -delta
	}
	return r.db.Model(&models.Coupon{}).
		Where("id = ?", id).
		Where("used_count >= ?", delta).
		UpdateColumn("used_count", gorm.Expr("used_count - ?", delta)).Error
}
