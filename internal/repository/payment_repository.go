package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// PaymentRepository 支付数据访问接口
type PaymentRepository interface {
	Create(payment *models.Payment) error
	Update(payment *models.Payment) error
	GetByID(id uint) (*models.Payment, error)
	GetByIDs(ids []uint) ([]models.Payment, error)
	GetLatestByProviderRef(providerRef string) (*models.Payment, error)
	ListByOrderID(orderID uint) ([]models.Payment, error)
	GetLatestPendingByOrder(orderID uint, now time.Time) (*models.Payment, error)
	GetLatestPendingByOrderChannel(orderID uint, channelID uint, now time.Time) (*models.Payment, error)
	ListAdmin(filter PaymentListFilter) ([]models.Payment, int64, error)
	WithTx(tx *gorm.DB) *GormPaymentRepository
}

// GormPaymentRepository GORM 实现
type GormPaymentRepository struct {
	db *gorm.DB
}

// NewPaymentRepository 创建支付仓库
func NewPaymentRepository(db *gorm.DB) *GormPaymentRepository {
	return &GormPaymentRepository{db: db}
}

// WithTx 绑定事务
func (r *GormPaymentRepository) WithTx(tx *gorm.DB) *GormPaymentRepository {
	if tx == nil {
		return r
	}
	return &GormPaymentRepository{db: tx}
}

// Create 创建支付记录
func (r *GormPaymentRepository) Create(payment *models.Payment) error {
	return r.db.Create(payment).Error
}

// Update 更新支付记录
func (r *GormPaymentRepository) Update(payment *models.Payment) error {
	return r.db.Save(payment).Error
}

// GetByID 根据 ID 获取支付记录
func (r *GormPaymentRepository) GetByID(id uint) (*models.Payment, error) {
	var payment models.Payment
	if err := r.db.First(&payment, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &payment, nil
}

// GetByIDs 根据 ID 列表获取支付记录
func (r *GormPaymentRepository) GetByIDs(ids []uint) ([]models.Payment, error) {
	if len(ids) == 0 {
		return []models.Payment{}, nil
	}
	var payments []models.Payment
	if err := r.db.Where("id IN ?", ids).Find(&payments).Error; err != nil {
		return nil, err
	}
	return payments, nil
}

// GetLatestByProviderRef 根据第三方流水号获取最新支付记录
func (r *GormPaymentRepository) GetLatestByProviderRef(providerRef string) (*models.Payment, error) {
	providerRef = strings.TrimSpace(providerRef)
	if providerRef == "" {
		return nil, nil
	}
	var payment models.Payment
	result := r.db.Where("provider_ref = ?", providerRef).Order("id desc").Limit(1).Find(&payment)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &payment, nil
}

// ListByOrderID 获取订单支付记录
func (r *GormPaymentRepository) ListByOrderID(orderID uint) ([]models.Payment, error) {
	var payments []models.Payment
	if err := r.db.Where("order_id = ?", orderID).Order("id desc").Find(&payments).Error; err != nil {
		return nil, err
	}
	return payments, nil
}

// GetLatestPendingByOrder 获取订单最新待支付记录
func (r *GormPaymentRepository) GetLatestPendingByOrder(orderID uint, now time.Time) (*models.Payment, error) {
	var payment models.Payment
	result := r.db.Where("order_id = ? AND status IN ? AND (expired_at IS NULL OR expired_at > ?) AND ((pay_url IS NOT NULL AND pay_url <> '') OR (qr_code IS NOT NULL AND qr_code <> ''))",
		orderID,
		[]string{constants.PaymentStatusInitiated, constants.PaymentStatusPending},
		now,
	).Order("id desc").Limit(1).Find(&payment)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &payment, nil
}

// GetLatestPendingByOrderChannel 获取订单+渠道最新待支付记录
func (r *GormPaymentRepository) GetLatestPendingByOrderChannel(orderID uint, channelID uint, now time.Time) (*models.Payment, error) {
	var payment models.Payment
	result := r.db.Where("order_id = ? AND channel_id = ? AND status IN ? AND (expired_at IS NULL OR expired_at > ?) AND ((pay_url IS NOT NULL AND pay_url <> '') OR (qr_code IS NOT NULL AND qr_code <> ''))",
		orderID,
		channelID,
		[]string{constants.PaymentStatusInitiated, constants.PaymentStatusPending},
		now,
	).Order("id desc").Limit(1).Find(&payment)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &payment, nil
}

// ListAdmin 管理端支付列表
func (r *GormPaymentRepository) ListAdmin(filter PaymentListFilter) ([]models.Payment, int64, error) {
	query := r.db.Model(&models.Payment{})

	if filter.UserID != 0 {
		query = query.
			Joins("LEFT JOIN orders ON orders.id = payments.order_id").
			Joins("LEFT JOIN wallet_recharge_orders ON wallet_recharge_orders.payment_id = payments.id").
			Where("(orders.user_id = ? OR wallet_recharge_orders.user_id = ?)", filter.UserID, filter.UserID)
	}
	if filter.OrderID != 0 {
		query = query.Where("payments.order_id = ?", filter.OrderID)
	}
	if filter.ChannelID != 0 {
		query = query.Where("payments.channel_id = ?", filter.ChannelID)
	}
	if filter.ProviderType != "" {
		query = query.Where("payments.provider_type = ?", filter.ProviderType)
	}
	if filter.ChannelType != "" {
		query = query.Where("payments.channel_type = ?", filter.ChannelType)
	}
	if filter.Status != "" {
		query = query.Where("payments.status = ?", filter.Status)
	}
	if filter.CreatedFrom != nil {
		query = query.Where("payments.created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("payments.created_at <= ?", *filter.CreatedTo)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = applyPagination(query, filter.Page, filter.PageSize)

	var payments []models.Payment
	if err := query.Order("payments.id desc").Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}
