package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// CreateCardSecretBatchRequest 批量录入卡密请求
type CreateCardSecretBatchRequest struct {
	ProductID uint     `json:"product_id" binding:"required"`
	Secrets   []string `json:"secrets" binding:"required"`
	BatchNo   string   `json:"batch_no"`
	Note      string   `json:"note"`
}

// UpdateCardSecretRequest 更新卡密请求
type UpdateCardSecretRequest struct {
	Secret *string `json:"secret"`
	Status *string `json:"status"`
}

// CreateCardSecretBatch 批量录入卡密
func (h *Handler) CreateCardSecretBatch(c *gin.Context) {
	adminID, ok := getAdminID(c)
	if !ok {
		return
	}
	var req CreateCardSecretBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	batch, created, err := h.CardSecretService.CreateCardSecretBatch(service.CreateCardSecretBatchInput{
		ProductID: req.ProductID,
		Secrets:   req.Secrets,
		BatchNo:   req.BatchNo,
		Note:      req.Note,
		Source:    constants.CardSecretSourceManual,
		AdminID:   adminID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCardSecretInvalid):
			respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		case errors.Is(err, service.ErrProductNotFound):
			respondError(c, response.CodeNotFound, "error.product_not_found", nil)
		case errors.Is(err, service.ErrProductFetchFailed):
			respondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		case errors.Is(err, service.ErrCardSecretBatchCreateFailed):
			respondError(c, response.CodeInternal, "error.card_secret_batch_create_failed", err)
		default:
			respondError(c, response.CodeInternal, "error.card_secret_create_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"created":  created,
		"batch_id": batch.ID,
		"batch_no": batch.BatchNo,
	})
}

// ImportCardSecretCSV 导入 CSV 卡密
func (h *Handler) ImportCardSecretCSV(c *gin.Context) {
	adminID, ok := getAdminID(c)
	if !ok {
		return
	}
	productID, err := strconv.ParseUint(strings.TrimSpace(c.PostForm("product_id")), 10, 64)
	if err != nil || productID == 0 {
		respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		return
	}
	batchNo := strings.TrimSpace(c.PostForm("batch_no"))
	note := strings.TrimSpace(c.PostForm("note"))

	batch, created, err := h.CardSecretService.ImportCardSecretCSV(service.ImportCardSecretCSVInput{
		ProductID: uint(productID),
		File:      file,
		BatchNo:   batchNo,
		Note:      note,
		AdminID:   adminID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCardSecretInvalid):
			respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		case errors.Is(err, service.ErrProductNotFound):
			respondError(c, response.CodeNotFound, "error.product_not_found", nil)
		case errors.Is(err, service.ErrProductFetchFailed):
			respondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		case errors.Is(err, service.ErrCardSecretBatchCreateFailed):
			respondError(c, response.CodeInternal, "error.card_secret_batch_create_failed", err)
		default:
			respondError(c, response.CodeInternal, "error.card_secret_import_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"created":  created,
		"batch_id": batch.ID,
		"batch_no": batch.BatchNo,
	})
}

// GetCardSecrets 获取卡密列表
func (h *Handler) GetCardSecrets(c *gin.Context) {
	var productID uint64
	rawProductID := strings.TrimSpace(c.Query("product_id"))
	if rawProductID != "" {
		parsed, err := strconv.ParseUint(rawProductID, 10, 64)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
			return
		}
		productID = parsed
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)
	status := strings.TrimSpace(c.Query("status"))

	items, total, err := h.CardSecretService.ListCardSecrets(service.ListCardSecretInput{
		ProductID: uint(productID),
		Status:    status,
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCardSecretInvalid):
			respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.card_secret_fetch_failed", err)
		}
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, items, pagination)
}

// UpdateCardSecret 更新卡密
func (h *Handler) UpdateCardSecret(c *gin.Context) {
	rawID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || rawID == 0 {
		respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		return
	}

	var req UpdateCardSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	secret := ""
	if req.Secret != nil {
		secret = *req.Secret
	}
	status := ""
	if req.Status != nil {
		status = *req.Status
	}
	if strings.TrimSpace(secret) == "" && strings.TrimSpace(status) == "" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	item, err := h.CardSecretService.UpdateCardSecret(uint(rawID), secret, status)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.card_secret_not_found", nil)
		case errors.Is(err, service.ErrCardSecretInvalid):
			respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		case errors.Is(err, service.ErrCardSecretUpdateFailed):
			respondError(c, response.CodeInternal, "error.card_secret_update_failed", err)
		default:
			respondError(c, response.CodeInternal, "error.card_secret_update_failed", err)
		}
		return
	}

	response.Success(c, item)
}

// GetCardSecretStats 获取库存统计
func (h *Handler) GetCardSecretStats(c *gin.Context) {
	productID, err := strconv.ParseUint(strings.TrimSpace(c.Query("product_id")), 10, 64)
	if err != nil || productID == 0 {
		respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		return
	}
	stats, err := h.CardSecretService.GetStats(uint(productID))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCardSecretInvalid):
			respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.card_secret_stats_failed", err)
		}
		return
	}
	response.Success(c, stats)
}

// GetCardSecretBatches 获取卡密批次列表
func (h *Handler) GetCardSecretBatches(c *gin.Context) {
	productID, err := strconv.ParseUint(strings.TrimSpace(c.Query("product_id")), 10, 64)
	if err != nil || productID == 0 {
		respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	items, total, err := h.CardSecretService.ListBatches(uint(productID), page, pageSize)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCardSecretInvalid):
			respondError(c, response.CodeBadRequest, "error.card_secret_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.card_secret_batch_fetch_failed", err)
		}
		return
	}
	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, items, pagination)
}

// GetCardSecretTemplate 下载导入模板
func (h *Handler) GetCardSecretTemplate(c *gin.Context) {
	content := "secret\nCARD-AAA-0001\nCARD-BBB-0002\n"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\"card-secrets-template.csv\"")
	c.String(200, content)
}
