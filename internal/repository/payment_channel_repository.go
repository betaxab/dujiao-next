package repository

import (
	"errors"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// PaymentChannelRepository 支付渠道数据访问接口
type PaymentChannelRepository interface {
	Create(channel *models.PaymentChannel) error
	Update(channel *models.PaymentChannel) error
	Delete(id uint) error
	GetByID(id uint) (*models.PaymentChannel, error)
	ListByIDs(ids []uint) ([]models.PaymentChannel, error)
	List(filter PaymentChannelListFilter) ([]models.PaymentChannel, int64, error)
}

// GormPaymentChannelRepository GORM 实现
type GormPaymentChannelRepository struct {
	db *gorm.DB
}

// NewPaymentChannelRepository 创建支付渠道仓库
func NewPaymentChannelRepository(db *gorm.DB) *GormPaymentChannelRepository {
	return &GormPaymentChannelRepository{db: db}
}

// Create 创建支付渠道
func (r *GormPaymentChannelRepository) Create(channel *models.PaymentChannel) error {
	return r.db.Create(channel).Error
}

// Update 更新支付渠道
func (r *GormPaymentChannelRepository) Update(channel *models.PaymentChannel) error {
	return r.db.Save(channel).Error
}

// Delete 删除支付渠道
func (r *GormPaymentChannelRepository) Delete(id uint) error {
	return r.db.Delete(&models.PaymentChannel{}, id).Error
}

// GetByID 根据 ID 获取支付渠道
func (r *GormPaymentChannelRepository) GetByID(id uint) (*models.PaymentChannel, error) {
	var channel models.PaymentChannel
	if err := r.db.First(&channel, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &channel, nil
}

// ListByIDs 根据 ID 列表获取支付渠道
func (r *GormPaymentChannelRepository) ListByIDs(ids []uint) ([]models.PaymentChannel, error) {
	if len(ids) == 0 {
		return []models.PaymentChannel{}, nil
	}
	var channels []models.PaymentChannel
	if err := r.db.Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

// List 支付渠道列表
func (r *GormPaymentChannelRepository) List(filter PaymentChannelListFilter) ([]models.PaymentChannel, int64, error) {
	query := r.db.Model(&models.PaymentChannel{})

	if filter.ProviderType != "" {
		query = query.Where("provider_type = ?", filter.ProviderType)
	}
	if filter.ChannelType != "" {
		query = query.Where("channel_type = ?", filter.ChannelType)
	}
	if filter.ActiveOnly {
		query = query.Where("is_active = ?", true)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if filter.PageSize > 0 {
		offset := (filter.Page - 1) * filter.PageSize
		query = query.Limit(filter.PageSize).Offset(offset)
	}

	var channels []models.PaymentChannel
	if err := query.Order("sort_order DESC, id ASC").Find(&channels).Error; err != nil {
		return nil, 0, err
	}
	return channels, total, nil
}
