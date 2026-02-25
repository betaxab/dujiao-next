package service

import (
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

// CategoryService 分类业务服务
type CategoryService struct {
	repo repository.CategoryRepository
}

// NewCategoryService 创建分类服务
func NewCategoryService(repo repository.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

// CreateCategoryInput 创建/更新分类输入
type CreateCategoryInput struct {
	Slug      string
	NameJSON  map[string]interface{}
	Icon      string
	SortOrder int
}

// List 获取分类列表
func (s *CategoryService) List() ([]models.Category, error) {
	return s.repo.List()
}

// Create 创建分类
func (s *CategoryService) Create(input CreateCategoryInput) (*models.Category, error) {
	count, err := s.repo.CountBySlug(input.Slug, nil)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugExists
	}

	category := models.Category{
		Slug:      input.Slug,
		NameJSON:  models.JSON(input.NameJSON),
		Icon:      input.Icon,
		SortOrder: input.SortOrder,
	}
	if err := s.repo.Create(&category); err != nil {
		return nil, err
	}
	return &category, nil
}

// Update 更新分类
func (s *CategoryService) Update(id string, input CreateCategoryInput) (*models.Category, error) {
	category, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if category == nil {
		return nil, ErrNotFound
	}

	count, err := s.repo.CountBySlug(input.Slug, &id)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugExists
	}

	category.Slug = input.Slug
	category.NameJSON = models.JSON(input.NameJSON)
	category.Icon = input.Icon
	category.SortOrder = input.SortOrder

	if err := s.repo.Update(category); err != nil {
		return nil, err
	}
	return category, nil
}

// Delete 删除分类
func (s *CategoryService) Delete(id string) error {
	category, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if category == nil {
		return ErrNotFound
	}

	count, err := s.repo.CountProducts(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrCategoryInUse
	}
	return s.repo.Delete(id)
}
