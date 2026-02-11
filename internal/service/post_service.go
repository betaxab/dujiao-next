package service

import (
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

// PostService 文章业务服务
type PostService struct {
	repo repository.PostRepository
}

// NewPostService 创建文章服务
func NewPostService(repo repository.PostRepository) *PostService {
	return &PostService{repo: repo}
}

// CreatePostInput 创建/更新文章输入
type CreatePostInput struct {
	Slug        string
	Type        string
	TitleJSON   map[string]interface{}
	SummaryJSON map[string]interface{}
	ContentJSON map[string]interface{}
	Thumbnail   string
	IsPublished *bool
}

var allowedPostTypes = map[string]struct{}{
	constants.PostTypeBlog:   {},
	constants.PostTypeNotice: {},
}

// ListPublic 获取公开文章列表
func (s *PostService) ListPublic(postType string, page, pageSize int) ([]models.Post, int64, error) {
	filter := repository.PostListFilter{
		Page:          page,
		PageSize:      pageSize,
		Type:          postType,
		OnlyPublished: true,
		OrderBy:       "published_at DESC, created_at DESC",
	}
	return s.repo.List(filter)
}

// GetPublicBySlug 获取公开文章详情
func (s *PostService) GetPublicBySlug(slug string) (*models.Post, error) {
	post, err := s.repo.GetBySlug(slug, true)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, ErrNotFound
	}
	return post, nil
}

// ListAdmin 获取后台文章列表
func (s *PostService) ListAdmin(postType, search string, page, pageSize int) ([]models.Post, int64, error) {
	filter := repository.PostListFilter{
		Page:     page,
		PageSize: pageSize,
		Type:     postType,
		Search:   search,
		OrderBy:  "created_at DESC",
	}
	return s.repo.List(filter)
}

// Create 创建文章
func (s *PostService) Create(input CreatePostInput) (*models.Post, error) {
	if !isAllowedPostType(input.Type) {
		return nil, ErrInvalidPostType
	}

	count, err := s.repo.CountBySlug(input.Slug, nil)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugExists
	}

	isPublished := false
	if input.IsPublished != nil {
		isPublished = *input.IsPublished
	}

	post := models.Post{
		Slug:        input.Slug,
		Type:        input.Type,
		TitleJSON:   models.JSON(input.TitleJSON),
		SummaryJSON: models.JSON(input.SummaryJSON),
		ContentJSON: models.JSON(input.ContentJSON),
		Thumbnail:   input.Thumbnail,
		IsPublished: isPublished,
	}

	if err := s.repo.Create(&post); err != nil {
		return nil, err
	}
	return &post, nil
}

// Update 更新文章
func (s *PostService) Update(id string, input CreatePostInput) (*models.Post, error) {
	if !isAllowedPostType(input.Type) {
		return nil, ErrInvalidPostType
	}

	post, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, ErrNotFound
	}

	count, err := s.repo.CountBySlug(input.Slug, &id)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugExists
	}

	post.Slug = input.Slug
	post.Type = input.Type
	post.TitleJSON = models.JSON(input.TitleJSON)
	post.SummaryJSON = models.JSON(input.SummaryJSON)
	post.ContentJSON = models.JSON(input.ContentJSON)
	post.Thumbnail = input.Thumbnail
	if input.IsPublished != nil {
		post.IsPublished = *input.IsPublished
	}

	if err := s.repo.Update(post); err != nil {
		return nil, err
	}
	return post, nil
}

// Delete 删除文章
func (s *PostService) Delete(id string) error {
	post, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if post == nil {
		return ErrNotFound
	}
	return s.repo.Delete(id)
}

func isAllowedPostType(postType string) bool {
	_, ok := allowedPostTypes[postType]
	return ok
}
