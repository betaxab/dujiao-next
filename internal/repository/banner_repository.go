package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// BannerRepository Banner 数据访问接口
type BannerRepository interface {
	List(filter BannerListFilter) ([]models.Banner, int64, error)
	ListValidByPosition(position string, limit int, now time.Time) ([]models.Banner, error)
	GetByID(id string) (*models.Banner, error)
	Create(banner *models.Banner) error
	Update(banner *models.Banner) error
	Delete(id string) error
}

// GormBannerRepository GORM 实现
type GormBannerRepository struct {
	db *gorm.DB
}

// NewBannerRepository 创建 Banner 仓库
func NewBannerRepository(db *gorm.DB) *GormBannerRepository {
	return &GormBannerRepository{db: db}
}

// List Banner 列表
func (r *GormBannerRepository) List(filter BannerListFilter) ([]models.Banner, int64, error) {
	var banners []models.Banner
	query := r.db.Model(&models.Banner{})

	if filter.Position != "" {
		query = query.Where("position = ?", filter.Position)
	}
	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}
	if filter.OnlyValid {
		now := time.Now()
		query = query.Where("is_active = ?", true)
		query = query.Where("(start_at IS NULL OR start_at <= ?)", now)
		query = query.Where("(end_at IS NULL OR end_at >= ?)", now)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"name LIKE ? OR json_extract(title_json, '$.zh-CN') LIKE ? OR json_extract(title_json, '$.zh-TW') LIKE ? OR json_extract(title_json, '$.en-US') LIKE ?",
			like, like, like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if filter.PageSize > 0 {
		offset := (filter.Page - 1) * filter.PageSize
		query = query.Limit(filter.PageSize).Offset(offset)
	}

	orderBy := filter.OrderBy
	if orderBy == "" {
		orderBy = "sort_order DESC, created_at DESC"
	}

	if err := query.Order(orderBy).Find(&banners).Error; err != nil {
		return nil, 0, err
	}
	return banners, total, nil
}

// ListValidByPosition 获取指定位置的有效 Banner
func (r *GormBannerRepository) ListValidByPosition(position string, limit int, now time.Time) ([]models.Banner, error) {
	var banners []models.Banner
	query := r.db.Model(&models.Banner{}).
		Where("is_active = ?", true).
		Where("(start_at IS NULL OR start_at <= ?)", now).
		Where("(end_at IS NULL OR end_at >= ?)", now)

	if position != "" {
		query = query.Where("position = ?", position)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("sort_order DESC, created_at DESC").Find(&banners).Error; err != nil {
		return nil, err
	}
	return banners, nil
}

// GetByID 根据 ID 获取 Banner
func (r *GormBannerRepository) GetByID(id string) (*models.Banner, error) {
	var banner models.Banner
	if err := r.db.First(&banner, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &banner, nil
}

// Create 创建 Banner
func (r *GormBannerRepository) Create(banner *models.Banner) error {
	return r.db.Create(banner).Error
}

// Update 更新 Banner
func (r *GormBannerRepository) Update(banner *models.Banner) error {
	return r.db.Save(banner).Error
}

// Delete 删除 Banner
func (r *GormBannerRepository) Delete(id string) error {
	return r.db.Delete(&models.Banner{}, id).Error
}
