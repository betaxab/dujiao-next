package repository

import (
	"errors"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// AdminRepository 管理员数据访问接口
type AdminRepository interface {
	GetByUsername(username string) (*models.Admin, error)
	GetByID(id uint) (*models.Admin, error)
	List() ([]models.Admin, error)
	Count() (int64, error)
	Create(admin *models.Admin) error
	Update(admin *models.Admin) error
	Delete(id uint) error
}

// GormAdminRepository GORM 实现
type GormAdminRepository struct {
	db *gorm.DB
}

// NewAdminRepository 创建管理员仓库
func NewAdminRepository(db *gorm.DB) *GormAdminRepository {
	return &GormAdminRepository{db: db}
}

// GetByUsername 根据用户名获取管理员
func (r *GormAdminRepository) GetByUsername(username string) (*models.Admin, error) {
	var admin models.Admin
	if err := r.db.Where("username = ?", username).First(&admin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &admin, nil
}

// GetByID 根据 ID 获取管理员
func (r *GormAdminRepository) GetByID(id uint) (*models.Admin, error) {
	var admin models.Admin
	if err := r.db.First(&admin, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &admin, nil
}

// List 获取管理员列表
func (r *GormAdminRepository) List() ([]models.Admin, error) {
	admins := make([]models.Admin, 0)
	err := r.db.
		Select("id", "username", "is_super", "last_login_at", "created_at").
		Order("id ASC").
		Find(&admins).Error
	if err != nil {
		return nil, err
	}
	return admins, nil
}

// Count 统计管理员数量
func (r *GormAdminRepository) Count() (int64, error) {
	var count int64
	if err := r.db.Model(&models.Admin{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Create 创建管理员
func (r *GormAdminRepository) Create(admin *models.Admin) error {
	return r.db.Create(admin).Error
}

// Update 更新管理员
func (r *GormAdminRepository) Update(admin *models.Admin) error {
	return r.db.Save(admin).Error
}

// Delete 删除管理员（软删除）
func (r *GormAdminRepository) Delete(id uint) error {
	if id == 0 {
		return nil
	}
	return r.db.Delete(&models.Admin{}, id).Error
}
