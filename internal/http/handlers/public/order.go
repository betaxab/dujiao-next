package public

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// OrderItemRequest 订单项请求
type OrderItemRequest struct {
	ProductID       uint   `json:"product_id" binding:"required"`
	Quantity        int    `json:"quantity" binding:"required"`
	FulfillmentType string `json:"fulfillment_type"`
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	Items          []OrderItemRequest   `json:"items" binding:"required"`
	CouponCode     string               `json:"coupon_code"`
	ManualFormData map[uint]models.JSON `json:"manual_form_data"`
}

// PreviewOrder 订单金额预览
func (h *Handler) PreviewOrder(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	var items []service.CreateOrderItem
	for _, item := range req.Items {
		items = append(items, service.CreateOrderItem{
			ProductID:       item.ProductID,
			Quantity:        item.Quantity,
			FulfillmentType: item.FulfillmentType,
		})
	}

	preview, err := h.OrderService.PreviewOrder(service.CreateOrderInput{
		UserID:         uid,
		Items:          items,
		CouponCode:     req.CouponCode,
		ClientIP:       c.ClientIP(),
		ManualFormData: req.ManualFormData,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderItem):
			respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		case errors.Is(err, service.ErrInvalidOrderAmount):
			respondError(c, response.CodeBadRequest, "error.order_amount_invalid", nil)
		case errors.Is(err, service.ErrProductPurchaseNotAllowed):
			respondError(c, response.CodeBadRequest, "error.product_purchase_not_allowed", nil)
		case errors.Is(err, service.ErrManualStockInsufficient):
			respondError(c, response.CodeBadRequest, "error.manual_stock_insufficient", nil)
		case errors.Is(err, service.ErrCardSecretInsufficient):
			respondError(c, response.CodeBadRequest, "error.card_secret_insufficient", nil)
		case errors.Is(err, service.ErrOrderCurrencyMismatch):
			respondError(c, response.CodeBadRequest, "error.order_currency_mismatch", nil)
		case errors.Is(err, service.ErrProductPriceInvalid):
			respondError(c, response.CodeBadRequest, "error.product_price_invalid", nil)
		case errors.Is(err, service.ErrProductNotAvailable):
			respondError(c, response.CodeBadRequest, "error.product_not_available", nil)
		case errors.Is(err, service.ErrCouponInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_invalid", nil)
		case errors.Is(err, service.ErrCouponNotFound):
			respondError(c, response.CodeBadRequest, "error.coupon_not_found", nil)
		case errors.Is(err, service.ErrCouponInactive):
			respondError(c, response.CodeBadRequest, "error.coupon_inactive", nil)
		case errors.Is(err, service.ErrCouponNotStarted):
			respondError(c, response.CodeBadRequest, "error.coupon_not_started", nil)
		case errors.Is(err, service.ErrCouponExpired):
			respondError(c, response.CodeBadRequest, "error.coupon_expired", nil)
		case errors.Is(err, service.ErrCouponUsageLimit):
			respondError(c, response.CodeBadRequest, "error.coupon_usage_limit", nil)
		case errors.Is(err, service.ErrCouponPerUserLimit):
			respondError(c, response.CodeBadRequest, "error.coupon_per_user_limit", nil)
		case errors.Is(err, service.ErrCouponMinAmount):
			respondError(c, response.CodeBadRequest, "error.coupon_min_amount", nil)
		case errors.Is(err, service.ErrCouponScopeInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_scope_invalid", nil)
		case errors.Is(err, service.ErrPromotionInvalid):
			respondError(c, response.CodeBadRequest, "error.promotion_invalid", nil)
		case errors.Is(err, service.ErrQueueUnavailable):
			respondError(c, response.CodeInternal, "error.queue_unavailable", nil)
		case errors.Is(err, service.ErrManualFormSchemaInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_schema_invalid", nil)
		case errors.Is(err, service.ErrManualFormRequiredMissing):
			respondError(c, response.CodeBadRequest, "error.manual_form_required_missing", nil)
		case errors.Is(err, service.ErrManualFormFieldInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_field_invalid", nil)
		case errors.Is(err, service.ErrManualFormTypeInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_type_invalid", nil)
		case errors.Is(err, service.ErrManualFormOptionInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_option_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_create_failed", err)
		}
		return
	}

	response.Success(c, preview)
}

// CreateOrder 创建订单
func (h *Handler) CreateOrder(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	var items []service.CreateOrderItem
	for _, item := range req.Items {
		items = append(items, service.CreateOrderItem{
			ProductID:       item.ProductID,
			Quantity:        item.Quantity,
			FulfillmentType: item.FulfillmentType,
		})
	}

	order, err := h.OrderService.CreateOrder(service.CreateOrderInput{
		UserID:         uid,
		Items:          items,
		CouponCode:     req.CouponCode,
		ClientIP:       c.ClientIP(),
		ManualFormData: req.ManualFormData,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderItem):
			respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		case errors.Is(err, service.ErrInvalidOrderAmount):
			respondError(c, response.CodeBadRequest, "error.order_amount_invalid", nil)
		case errors.Is(err, service.ErrProductPurchaseNotAllowed):
			respondError(c, response.CodeBadRequest, "error.product_purchase_not_allowed", nil)
		case errors.Is(err, service.ErrManualStockInsufficient):
			respondError(c, response.CodeBadRequest, "error.manual_stock_insufficient", nil)
		case errors.Is(err, service.ErrCardSecretInsufficient):
			respondError(c, response.CodeBadRequest, "error.card_secret_insufficient", nil)
		case errors.Is(err, service.ErrOrderCurrencyMismatch):
			respondError(c, response.CodeBadRequest, "error.order_currency_mismatch", nil)
		case errors.Is(err, service.ErrProductPriceInvalid):
			respondError(c, response.CodeBadRequest, "error.product_price_invalid", nil)
		case errors.Is(err, service.ErrProductNotAvailable):
			respondError(c, response.CodeBadRequest, "error.product_not_available", nil)
		case errors.Is(err, service.ErrCouponInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_invalid", nil)
		case errors.Is(err, service.ErrCouponNotFound):
			respondError(c, response.CodeBadRequest, "error.coupon_not_found", nil)
		case errors.Is(err, service.ErrCouponInactive):
			respondError(c, response.CodeBadRequest, "error.coupon_inactive", nil)
		case errors.Is(err, service.ErrCouponNotStarted):
			respondError(c, response.CodeBadRequest, "error.coupon_not_started", nil)
		case errors.Is(err, service.ErrCouponExpired):
			respondError(c, response.CodeBadRequest, "error.coupon_expired", nil)
		case errors.Is(err, service.ErrCouponUsageLimit):
			respondError(c, response.CodeBadRequest, "error.coupon_usage_limit", nil)
		case errors.Is(err, service.ErrCouponPerUserLimit):
			respondError(c, response.CodeBadRequest, "error.coupon_per_user_limit", nil)
		case errors.Is(err, service.ErrCouponMinAmount):
			respondError(c, response.CodeBadRequest, "error.coupon_min_amount", nil)
		case errors.Is(err, service.ErrCouponScopeInvalid):
			respondError(c, response.CodeBadRequest, "error.coupon_scope_invalid", nil)
		case errors.Is(err, service.ErrPromotionInvalid):
			respondError(c, response.CodeBadRequest, "error.promotion_invalid", nil)
		case errors.Is(err, service.ErrManualFormSchemaInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_schema_invalid", nil)
		case errors.Is(err, service.ErrManualFormRequiredMissing):
			respondError(c, response.CodeBadRequest, "error.manual_form_required_missing", nil)
		case errors.Is(err, service.ErrManualFormFieldInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_field_invalid", nil)
		case errors.Is(err, service.ErrManualFormTypeInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_type_invalid", nil)
		case errors.Is(err, service.ErrManualFormOptionInvalid):
			respondError(c, response.CodeBadRequest, "error.manual_form_option_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_create_failed", err)
		}
		return
	}

	response.Success(c, order)
}

// ListOrders 获取订单列表
func (h *Handler) ListOrders(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	status := strings.TrimSpace(c.Query("status"))
	orderNo := strings.TrimSpace(c.Query("order_no"))

	orders, total, err := h.OrderService.ListOrdersByUser(repository.OrderListFilter{
		Page:     page,
		PageSize: pageSize,
		UserID:   uid,
		Status:   status,
		OrderNo:  orderNo,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, orders, pagination)
}

// GetOrder 获取订单详情
func (h *Handler) GetOrder(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || orderID == 0 {
		respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.GetOrderByUser(uint(orderID), uid)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	response.Success(c, order)
}

// GetOrderByOrderNo 按订单号获取订单详情
func (h *Handler) GetOrderByOrderNo(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.GetOrderByUserOrderNo(orderNo, uid)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	response.Success(c, order)
}

// CancelOrder 用户取消订单
func (h *Handler) CancelOrder(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || orderID == 0 {
		respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.CancelOrder(uint(orderID), uid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
		case errors.Is(err, service.ErrOrderCancelNotAllowed):
			respondError(c, response.CodeBadRequest, "error.order_cancel_not_allowed", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}

	response.Success(c, order)
}
