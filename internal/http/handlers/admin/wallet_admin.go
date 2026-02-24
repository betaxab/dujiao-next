package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// AdminAdjustUserWalletRequest 管理端用户余额调整请求
type AdminAdjustUserWalletRequest struct {
	Amount    string `json:"amount" binding:"required"`
	Operation string `json:"operation"` // add/subtract
	Currency  string `json:"currency"`
	Remark    string `json:"remark"`
}

// AdminRefundOrderToWalletRequest 管理端订单退款到余额请求
type AdminRefundOrderToWalletRequest struct {
	Amount string `json:"amount" binding:"required"`
	Remark string `json:"remark"`
}

// GetAdminUserWallet 管理端获取用户钱包信息
func (h *Handler) GetAdminUserWallet(c *gin.Context) {
	userID, ok := parsePathUint(c, "id")
	if !ok {
		respondError(c, response.CodeBadRequest, "error.user_id_invalid", nil)
		return
	}
	user, err := h.UserRepo.GetByID(userID)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	if user == nil {
		respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		return
	}
	account, err := h.WalletService.GetAccount(userID)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, gin.H{
		"user":    user,
		"account": account,
	})
}

// GetAdminUserWalletTransactions 管理端获取用户钱包流水
func (h *Handler) GetAdminUserWalletTransactions(c *gin.Context) {
	userID, ok := parsePathUint(c, "id")
	if !ok {
		respondError(c, response.CodeBadRequest, "error.user_id_invalid", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	filter := repository.WalletTransactionListFilter{
		Page:      page,
		PageSize:  pageSize,
		UserID:    userID,
		Type:      strings.TrimSpace(c.Query("type")),
		Direction: strings.TrimSpace(c.Query("direction")),
	}
	transactions, total, err := h.WalletService.ListTransactions(filter)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, transactions, pagination)
}

// AdjustAdminUserWallet 管理端增减用户余额
func (h *Handler) AdjustAdminUserWallet(c *gin.Context) {
	userID, ok := parsePathUint(c, "id")
	if !ok {
		respondError(c, response.CodeBadRequest, "error.user_id_invalid", nil)
		return
	}
	var req AdminAdjustUserWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	op := strings.ToLower(strings.TrimSpace(req.Operation))
	delta := amount
	if op == "" {
		op = "add"
	}
	if op == "subtract" {
		delta = amount.Neg()
	}
	if op != "add" && op != "subtract" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" && h.SettingService != nil {
		siteCurrency, currencyErr := h.SettingService.GetSiteCurrency(constants.SiteCurrencyDefault)
		if currencyErr == nil {
			currency = siteCurrency
		}
	}

	account, txn, err := h.WalletService.AdminAdjustBalance(service.WalletAdjustInput{
		UserID:   userID,
		Delta:    models.NewMoneyFromDecimal(delta),
		Currency: currency,
		Remark:   strings.TrimSpace(req.Remark),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWalletInvalidAmount):
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		case errors.Is(err, service.ErrWalletInsufficientBalance):
			respondError(c, response.CodeBadRequest, "error.payment_amount_mismatch", nil)
		default:
			respondError(c, response.CodeInternal, "error.user_update_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"account":     account,
		"transaction": txn,
	})
}

// AdminRefundOrderToWallet 管理端订单退款到余额
func (h *Handler) AdminRefundOrderToWallet(c *gin.Context) {
	orderID, ok := parsePathUint(c, "id")
	if !ok {
		respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}
	var req AdminRefundOrderToWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	order, txn, err := h.WalletService.AdminRefundToWallet(service.AdminRefundToWalletInput{
		OrderID: orderID,
		Amount:  models.NewMoneyFromDecimal(amount),
		Remark:  strings.TrimSpace(req.Remark),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
		case errors.Is(err, service.ErrOrderStatusInvalid):
			respondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		case errors.Is(err, service.ErrWalletInvalidAmount), errors.Is(err, service.ErrWalletRefundExceeded), errors.Is(err, service.ErrWalletNotSupportedForGuest):
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"order":       order,
		"transaction": txn,
	})
}

func parsePathUint(c *gin.Context, key string) (uint, bool) {
	raw := strings.TrimSpace(c.Param(key))
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed == 0 {
		return 0, false
	}
	return uint(parsed), true
}
