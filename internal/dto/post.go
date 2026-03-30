package dto

import (
	"time"

	"github.com/dujiao-next/internal/models"
)

// PostResp 文章/公告公共响应
type PostResp struct {
	ID          uint        `json:"id"`
	Slug        string      `json:"slug"`
	Type        string      `json:"type"`
	Title       models.JSON `json:"title"`
	Summary     models.JSON `json:"summary"`
	Content     models.JSON `json:"content"`
	Thumbnail   string      `json:"thumbnail,omitempty"`
	PublishedAt *time.Time  `json:"published_at"`
}

// NewPostResp 从 models.Post 构造响应
func NewPostResp(p *models.Post) PostResp {
	return PostResp{
		ID:          p.ID,
		Slug:        p.Slug,
		Type:        p.Type,
		Title:       p.TitleJSON,
		Summary:     p.SummaryJSON,
		Content:     p.ContentJSON,
		Thumbnail:   p.Thumbnail,
		PublishedAt: p.PublishedAt,
	}
	// 排除：IsPublished(内部状态)、CreatedAt
}

// NewPostRespList 批量转换文章列表
func NewPostRespList(posts []models.Post) []PostResp {
	result := make([]PostResp, 0, len(posts))
	for i := range posts {
		result = append(result, NewPostResp(&posts[i]))
	}
	return result
}
