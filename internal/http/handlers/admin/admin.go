package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// GetAdminProducts 获取商品列表 (Admin)
func (h *Handler) GetAdminProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)
	categoryID := c.Query("category_id")
	search := c.Query("search")
	manualStockStatus := c.Query("manual_stock_status")

	products, total, err := h.ProductService.ListAdmin(categoryID, search, manualStockStatus, page, pageSize)
	if err != nil {
		respondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, products, pagination)
}

// GetAdminProduct 获取商品详情 (Admin)
func (h *Handler) GetAdminProduct(c *gin.Context) {
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	product, err := h.ProductService.GetAdminByID(id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		return
	}

	response.Success(c, product)
}

// GetAdminPosts 获取文章列表 (Admin)
func (h *Handler) GetAdminPosts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)
	postType := c.Query("type")
	search := c.Query("search")

	posts, total, err := h.PostService.ListAdmin(postType, search, page, pageSize)
	if err != nil {
		respondError(c, response.CodeInternal, "error.post_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, posts, pagination)
}

// GetAdminCategories 获取分类列表 (Admin)
func (h *Handler) GetAdminCategories(c *gin.Context) {
	categories, err := h.CategoryService.List()
	if err != nil {
		respondError(c, response.CodeInternal, "error.category_fetch_failed", err)
		return
	}

	response.Success(c, categories)
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username       string                `json:"username" binding:"required"`
	Password       string                `json:"password" binding:"required"`
	CaptchaPayload CaptchaPayloadRequest `json:"captcha_payload"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token     string                 `json:"token"`
	User      map[string]interface{} `json:"user"`
	ExpiresAt string                 `json:"expires_at"`
}

// AdminLogin 管理员登录
func (h *Handler) AdminLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if h.CaptchaService != nil {
		if captchaErr := h.CaptchaService.Verify(constants.CaptchaSceneLogin, req.CaptchaPayload.toServicePayload(), c.ClientIP()); captchaErr != nil {
			switch {
			case errors.Is(captchaErr, service.ErrCaptchaRequired):
				respondError(c, response.CodeBadRequest, "error.captcha_required", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaInvalid):
				respondError(c, response.CodeBadRequest, "error.captcha_invalid", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaConfigInvalid):
				respondError(c, response.CodeInternal, "error.captcha_config_invalid", captchaErr)
				return
			default:
				respondError(c, response.CodeInternal, "error.captcha_verify_failed", captchaErr)
				return
			}
		}
	}

	admin, token, expiresAt, err := h.AuthService.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			respondError(c, response.CodeUnauthorized, "error.admin_login_invalid", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.login_failed", err)
		return
	}
	response.Success(c, LoginResponse{
		Token: token,
		User: map[string]interface{}{
			"id":       admin.ID,
			"username": admin.Username,
		},
		ExpiresAt: expiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// UpdatePasswordRequest 修改密码请求
type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UpdateAdminPassword 修改管理员密码
func (h *Handler) UpdateAdminPassword(c *gin.Context) {
	// 获取当前登录用户 ID
	id, ok := getAdminID(c)
	if !ok {
		return
	}

	var req UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if err := h.AuthService.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidPassword) {
			respondError(c, response.CodeBadRequest, "error.password_old_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrWeakPassword) {
			locale := i18n.ResolveLocale(c)
			if perr, ok := err.(interface {
				Key() string
				Args() []interface{}
			}); ok {
				msg := i18n.Sprintf(locale, perr.Key(), perr.Args()...)
				respondErrorWithMsg(c, response.CodeBadRequest, msg, nil)
				return
			}
			respondError(c, response.CodeBadRequest, "error.password_weak", nil)
			return
		}
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.save_failed", err)
		return
	}

	response.Success(c, nil)
}

// ====================  商品管理  ====================

// CreateProductRequest 创建商品请求
type CreateProductRequest struct {
	CategoryID       uint                   `json:"category_id" binding:"required"`
	Slug             string                 `json:"slug" binding:"required"`
	SeoMetaJSON      map[string]interface{} `json:"seo_meta"`
	TitleJSON        map[string]interface{} `json:"title" binding:"required"`
	DescriptionJSON  map[string]interface{} `json:"description"`
	ContentJSON      map[string]interface{} `json:"content"`
	ManualFormSchema map[string]interface{} `json:"manual_form_schema"`
	PriceAmount      float64                `json:"price_amount" binding:"required"`
	PriceCurrency    string                 `json:"price_currency" binding:"required"`
	Images           []string               `json:"images"`
	Tags             []string               `json:"tags"`
	PurchaseType     string                 `json:"purchase_type"`
	FulfillmentType  string                 `json:"fulfillment_type"`
	ManualStockTotal *int                   `json:"manual_stock_total"`
	IsActive         *bool                  `json:"is_active"`
	SortOrder        int                    `json:"sort_order"`
}

// CreateProduct 创建商品
func (h *Handler) CreateProduct(c *gin.Context) {
	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	product, err := h.ProductService.Create(service.CreateProductInput{
		CategoryID:           req.CategoryID,
		Slug:                 req.Slug,
		SeoMetaJSON:          req.SeoMetaJSON,
		TitleJSON:            req.TitleJSON,
		DescriptionJSON:      req.DescriptionJSON,
		ContentJSON:          req.ContentJSON,
		ManualFormSchemaJSON: req.ManualFormSchema,
		PriceAmount:          decimal.NewFromFloat(req.PriceAmount),
		PriceCurrency:        req.PriceCurrency,
		Images:               req.Images,
		Tags:                 req.Tags,
		PurchaseType:         req.PurchaseType,
		FulfillmentType:      req.FulfillmentType,
		ManualStockTotal:     req.ManualStockTotal,
		IsActive:             req.IsActive,
		SortOrder:            req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, service.ErrSlugExists) {
			respondError(c, response.CodeBadRequest, "error.slug_exists", nil)
			return
		}
		if errors.Is(err, service.ErrProductPriceInvalid) {
			respondError(c, response.CodeBadRequest, "error.product_price_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductPurchaseInvalid) {
			respondError(c, response.CodeBadRequest, "error.product_purchase_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrFulfillmentInvalid) {
			respondError(c, response.CodeBadRequest, "error.fulfillment_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualFormSchemaInvalid) {
			respondError(c, response.CodeBadRequest, "error.manual_form_schema_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualStockInvalid) {
			respondError(c, response.CodeBadRequest, "error.manual_stock_invalid", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.product_create_failed", err)
		return
	}

	response.Success(c, product)
}

// UpdateProduct 更新商品
func (h *Handler) UpdateProduct(c *gin.Context) {
	id := c.Param("id")

	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	product, err := h.ProductService.Update(id, service.CreateProductInput{
		CategoryID:           req.CategoryID,
		Slug:                 req.Slug,
		SeoMetaJSON:          req.SeoMetaJSON,
		TitleJSON:            req.TitleJSON,
		DescriptionJSON:      req.DescriptionJSON,
		ContentJSON:          req.ContentJSON,
		ManualFormSchemaJSON: req.ManualFormSchema,
		PriceAmount:          decimal.NewFromFloat(req.PriceAmount),
		PriceCurrency:        req.PriceCurrency,
		Images:               req.Images,
		Tags:                 req.Tags,
		PurchaseType:         req.PurchaseType,
		FulfillmentType:      req.FulfillmentType,
		ManualStockTotal:     req.ManualStockTotal,
		IsActive:             req.IsActive,
		SortOrder:            req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		if errors.Is(err, service.ErrSlugExists) {
			respondError(c, response.CodeBadRequest, "error.slug_used", nil)
			return
		}
		if errors.Is(err, service.ErrProductPriceInvalid) {
			respondError(c, response.CodeBadRequest, "error.product_price_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductPurchaseInvalid) {
			respondError(c, response.CodeBadRequest, "error.product_purchase_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrFulfillmentInvalid) {
			respondError(c, response.CodeBadRequest, "error.fulfillment_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualFormSchemaInvalid) {
			respondError(c, response.CodeBadRequest, "error.manual_form_schema_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualStockInvalid) {
			respondError(c, response.CodeBadRequest, "error.manual_stock_invalid", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.product_update_failed", err)
		return
	}

	response.Success(c, product)
}

// DeleteProduct 删除商品（软删除）
func (h *Handler) DeleteProduct(c *gin.Context) {
	id := c.Param("id")

	if err := h.ProductService.Delete(id); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.product_delete_failed", err)
		return
	}

	response.Success(c, nil)
}

// ====================  文章管理  ====================

// CreatePostRequest 创建文章请求
type CreatePostRequest struct {
	Slug        string                 `json:"slug" binding:"required"`
	Type        string                 `json:"type" binding:"required"` // blog 或 notice
	TitleJSON   map[string]interface{} `json:"title" binding:"required"`
	SummaryJSON map[string]interface{} `json:"summary"`
	ContentJSON map[string]interface{} `json:"content"`
	Thumbnail   string                 `json:"thumbnail"`
	IsPublished *bool                  `json:"is_published"`
}

// CreatePost 创建文章
func (h *Handler) CreatePost(c *gin.Context) {
	var req CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	post, err := h.PostService.Create(service.CreatePostInput{
		Slug:        req.Slug,
		Type:        req.Type,
		TitleJSON:   req.TitleJSON,
		SummaryJSON: req.SummaryJSON,
		ContentJSON: req.ContentJSON,
		Thumbnail:   req.Thumbnail,
		IsPublished: req.IsPublished,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidPostType) {
			respondError(c, response.CodeBadRequest, "error.post_type_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrSlugExists) {
			respondError(c, response.CodeBadRequest, "error.slug_exists", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.post_create_failed", err)
		return
	}

	response.Success(c, post)
}

// UpdatePost 更新文章
func (h *Handler) UpdatePost(c *gin.Context) {
	id := c.Param("id")

	var req CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	post, err := h.PostService.Update(id, service.CreatePostInput{
		Slug:        req.Slug,
		Type:        req.Type,
		TitleJSON:   req.TitleJSON,
		SummaryJSON: req.SummaryJSON,
		ContentJSON: req.ContentJSON,
		Thumbnail:   req.Thumbnail,
		IsPublished: req.IsPublished,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidPostType) {
			respondError(c, response.CodeBadRequest, "error.post_type_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.post_not_found", nil)
			return
		}
		if errors.Is(err, service.ErrSlugExists) {
			respondError(c, response.CodeBadRequest, "error.slug_used", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.post_update_failed", err)
		return
	}

	response.Success(c, post)
}

// DeletePost 删除文章（软删除）
func (h *Handler) DeletePost(c *gin.Context) {
	id := c.Param("id")

	if err := h.PostService.Delete(id); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.post_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.post_delete_failed", err)
		return
	}

	response.Success(c, nil)
}

// ====================  分类管理  ====================

// CreateCategoryRequest 创建分类请求
type CreateCategoryRequest struct {
	Slug      string                 `json:"slug" binding:"required"`
	NameJSON  map[string]interface{} `json:"name" binding:"required"`
	SortOrder int                    `json:"sort_order"`
}

// CreateCategory 创建分类
func (h *Handler) CreateCategory(c *gin.Context) {
	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	category, err := h.CategoryService.Create(service.CreateCategoryInput{
		Slug:      req.Slug,
		NameJSON:  req.NameJSON,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, service.ErrSlugExists) {
			respondError(c, response.CodeBadRequest, "error.slug_exists", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.category_create_failed", err)
		return
	}

	response.Success(c, category)
}

// UpdateCategory 更新分类
func (h *Handler) UpdateCategory(c *gin.Context) {
	id := c.Param("id")

	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	category, err := h.CategoryService.Update(id, service.CreateCategoryInput{
		Slug:      req.Slug,
		NameJSON:  req.NameJSON,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.category_not_found", nil)
			return
		}
		if errors.Is(err, service.ErrSlugExists) {
			respondError(c, response.CodeBadRequest, "error.slug_used", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.category_update_failed", err)
		return
	}

	response.Success(c, category)
}

// DeleteCategory 删除分类（软删除）
func (h *Handler) DeleteCategory(c *gin.Context) {
	id := c.Param("id")

	if err := h.CategoryService.Delete(id); err != nil {
		if errors.Is(err, service.ErrCategoryInUse) {
			respondError(c, response.CodeBadRequest, "error.category_in_use", nil)
			return
		}
		if errors.Is(err, service.ErrNotFound) {
			respondError(c, response.CodeNotFound, "error.category_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.category_delete_failed", err)
		return
	}

	response.Success(c, nil)
}

// ====================  设置管理  ====================

// GetSettings 获取设置
func (h *Handler) GetSettings(c *gin.Context) {
	key := c.DefaultQuery("key", constants.SettingKeySiteConfig)

	value, err := h.SettingService.GetByKey(key)
	if err != nil {
		respondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	if value == nil {
		response.Success(c, gin.H{})
		return
	}

	response.Success(c, value)
}

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	Key   string                 `json:"key" binding:"required"`
	Value map[string]interface{} `json:"value" binding:"required"`
}

// UpdateSettings 更新设置
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	value, err := h.SettingService.Update(req.Key, req.Value)
	if err != nil {
		respondError(c, response.CodeInternal, "error.settings_save_failed", err)
		return
	}

	if req.Key == constants.SettingKeySiteConfig {
		_ = cache.Del(c.Request.Context(), publicConfigCacheKey)
	}
	response.Success(c, value)
}

// ====================  文件上传  ====================

// UploadFile 文件上传
func (h *Handler) UploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.file_missing", nil)
		return
	}
	scene := c.DefaultPostForm("scene", "common")

	// 保存文件
	url, err := h.UploadService.SaveFile(file, scene)
	if err != nil {
		respondError(c, response.CodeInternal, "error.upload_failed", err)
		return
	}

	response.Success(c, gin.H{
		"url":      url,
		"filename": file.Filename,
		"size":     file.Size,
	})
}
