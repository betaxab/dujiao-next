package repository

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// PostRepository 文章数据访问接口
type PostRepository interface {
	List(filter PostListFilter) ([]models.Post, int64, error)
	GetBySlug(slug string, onlyPublished bool) (*models.Post, error)
	GetByID(id string) (*models.Post, error)
	Create(post *models.Post) error
	Update(post *models.Post) error
	Delete(id string) error
	CountBySlug(slug string, excludeID *string) (int64, error)
}

// GormPostRepository GORM 实现
type GormPostRepository struct {
	db *gorm.DB
}

// NewPostRepository 创建文章仓库
func NewPostRepository(db *gorm.DB) *GormPostRepository {
	return &GormPostRepository{db: db}
}

// List 文章列表
func (r *GormPostRepository) List(filter PostListFilter) ([]models.Post, int64, error) {
	var posts []models.Post
	query := r.db.Model(&models.Post{})

	if filter.OnlyPublished {
		query = query.Where("is_published = ?", true)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		condition, argCount := buildLocalizedLikeCondition(r.db, []string{"slug"}, []string{"title_json"})
		query = query.Where(condition, repeatLikeArgs(like, argCount)...)
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
		orderBy = "created_at DESC"
	}

	if err := query.Order(orderBy).Find(&posts).Error; err != nil {
		return nil, 0, err
	}
	return posts, total, nil
}

// GetBySlug 根据 slug 获取文章
func (r *GormPostRepository) GetBySlug(slug string, onlyPublished bool) (*models.Post, error) {
	query := r.db.Where("slug = ?", slug)
	if onlyPublished {
		query = query.Where("is_published = ?", true)
	}

	var post models.Post
	if err := query.First(&post).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &post, nil
}

// GetByID 根据 ID 获取文章
func (r *GormPostRepository) GetByID(id string) (*models.Post, error) {
	var post models.Post
	if err := r.db.First(&post, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &post, nil
}

// Create 创建文章
func (r *GormPostRepository) Create(post *models.Post) error {
	return r.db.Create(post).Error
}

// Update 更新文章
func (r *GormPostRepository) Update(post *models.Post) error {
	return r.db.Save(post).Error
}

// Delete 删除文章
func (r *GormPostRepository) Delete(id string) error {
	return r.db.Delete(&models.Post{}, id).Error
}

// CountBySlug 统计 slug 数量
func (r *GormPostRepository) CountBySlug(slug string, excludeID *string) (int64, error) {
	var count int64
	query := r.db.Model(&models.Post{}).Where("slug = ?", slug)
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
