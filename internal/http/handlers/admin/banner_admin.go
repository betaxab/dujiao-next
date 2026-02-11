package admin

import (
	"errors"
	"strconv"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// BannerUpsertRequest Banner 创建/更新请求
type BannerUpsertRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Position     string                 `json:"position"`
	TitleJSON    map[string]interface{} `json:"title"`
	SubtitleJSON map[string]interface{} `json:"subtitle"`
	Image        string                 `json:"image" binding:"required"`
	MobileImage  string                 `json:"mobile_image"`
	LinkType     string                 `json:"link_type"`
	LinkValue    string                 `json:"link_value"`
	OpenInNewTab *bool                  `json:"open_in_new_tab"`
	IsActive     *bool                  `json:"is_active"`
	StartAt      string                 `json:"start_at"`
	EndAt        string                 `json:"end_at"`
	SortOrder    int                    `json:"sort_order"`
}

// GetAdminBanners 获取后台 Banner 列表
func (h *Handler) GetAdminBanners(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	position := c.Query("position")
	search := c.Query("search")

	var isActive *bool
	if raw := c.Query("is_active"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		isActive = &parsed
	}

	banners, total, err := h.BannerService.ListAdmin(position, search, isActive, page, pageSize)
	if err != nil {
		respondError(c, response.CodeInternal, "error.banner_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, banners, pagination)
}

// GetAdminBanner 获取后台 Banner 详情
func (h *Handler) GetAdminBanner(c *gin.Context) {
	id := c.Param("id")
	banner, err := h.BannerService.GetByID(id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.banner_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.banner_fetch_failed", err)
		return
	}
	response.Success(c, banner)
}

// CreateBanner 创建 Banner
func (h *Handler) CreateBanner(c *gin.Context) {
	var req BannerUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	startAt, err := parseTimeNullable(req.StartAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	endAt, err := parseTimeNullable(req.EndAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	banner, err := h.BannerService.Create(service.BannerInput{
		Name:         req.Name,
		Position:     req.Position,
		TitleJSON:    req.TitleJSON,
		SubtitleJSON: req.SubtitleJSON,
		Image:        req.Image,
		MobileImage:  req.MobileImage,
		LinkType:     req.LinkType,
		LinkValue:    req.LinkValue,
		OpenInNewTab: req.OpenInNewTab,
		IsActive:     req.IsActive,
		StartAt:      startAt,
		EndAt:        endAt,
		SortOrder:    req.SortOrder,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidBanner):
			respondError(c, response.CodeBadRequest, "error.banner_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.banner_create_failed", err)
		}
		return
	}

	response.Success(c, banner)
}

// UpdateBanner 更新 Banner
func (h *Handler) UpdateBanner(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	var req BannerUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	startAt, err := parseTimeNullable(req.StartAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	endAt, err := parseTimeNullable(req.EndAt)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	banner, err := h.BannerService.Update(id, service.BannerInput{
		Name:         req.Name,
		Position:     req.Position,
		TitleJSON:    req.TitleJSON,
		SubtitleJSON: req.SubtitleJSON,
		Image:        req.Image,
		MobileImage:  req.MobileImage,
		LinkType:     req.LinkType,
		LinkValue:    req.LinkValue,
		OpenInNewTab: req.OpenInNewTab,
		IsActive:     req.IsActive,
		StartAt:      startAt,
		EndAt:        endAt,
		SortOrder:    req.SortOrder,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidBanner):
			respondError(c, response.CodeBadRequest, "error.banner_invalid", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.banner_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.banner_update_failed", err)
		}
		return
	}

	response.Success(c, banner)
}

// DeleteBanner 删除 Banner
func (h *Handler) DeleteBanner(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	if err := h.BannerService.Delete(id); err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.banner_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.banner_delete_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"deleted": true,
	})
}

// GetPublicBanners 获取前台 Banner 列表
func (h *Handler) GetPublicBanners(c *gin.Context) {
	position := c.DefaultQuery("position", "home_hero")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	banners, err := h.BannerService.ListPublic(position, limit)
	if err != nil {
		respondError(c, response.CodeInternal, "error.banner_fetch_failed", err)
		return
	}

	response.Success(c, banners)
}
