package public

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/dto"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// OrderItemRequest 订单项请求
type OrderItemRequest struct {
	ProductID       uint   `json:"product_id" binding:"required"`
	SKUID           uint   `json:"sku_id"`
	Quantity        int    `json:"quantity" binding:"required"`
	FulfillmentType string `json:"fulfillment_type"`
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	Items               []OrderItemRequest     `json:"items" binding:"required"`
	CouponCode          string                 `json:"coupon_code"`
	AffiliateCode       string                 `json:"affiliate_code"`
	AffiliateVisitorKey string                 `json:"affiliate_visitor_key"`
	ManualFormData      map[string]models.JSON `json:"manual_form_data"`
}

// PreviewOrder 订单金额预览
func (h *Handler) PreviewOrder(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	var items []service.CreateOrderItem
	for _, item := range req.Items {
		items = append(items, service.CreateOrderItem{
			ProductID:       item.ProductID,
			SKUID:           item.SKUID,
			Quantity:        item.Quantity,
			FulfillmentType: item.FulfillmentType,
		})
	}

	preview, err := h.OrderService.PreviewOrder(service.CreateOrderInput{
		UserID:              uid,
		Items:               items,
		CouponCode:          req.CouponCode,
		AffiliateCode:       req.AffiliateCode,
		AffiliateVisitorKey: req.AffiliateVisitorKey,
		ClientIP:            c.ClientIP(),
		ManualFormData:      req.ManualFormData,
	})
	if err != nil {
		respondUserOrderPreviewError(c, err)
		return
	}

	response.Success(c, preview)
}

// CreateOrder 创建订单
func (h *Handler) CreateOrder(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	var items []service.CreateOrderItem
	for _, item := range req.Items {
		items = append(items, service.CreateOrderItem{
			ProductID:       item.ProductID,
			SKUID:           item.SKUID,
			Quantity:        item.Quantity,
			FulfillmentType: item.FulfillmentType,
		})
	}

	order, err := h.OrderService.CreateOrder(service.CreateOrderInput{
		UserID:              uid,
		Items:               items,
		CouponCode:          req.CouponCode,
		AffiliateCode:       req.AffiliateCode,
		AffiliateVisitorKey: req.AffiliateVisitorKey,
		ClientIP:            c.ClientIP(),
		ManualFormData:      req.ManualFormData,
	})
	if err != nil {
		respondUserOrderCreateError(c, err)
		return
	}

	response.Success(c, dto.NewOrderDetail(order))
}

// CreateOrderAndPayRequest 创建订单并发起支付请求
type CreateOrderAndPayRequest struct {
	Items               []OrderItemRequest     `json:"items" binding:"required"`
	CouponCode          string                 `json:"coupon_code"`
	AffiliateCode       string                 `json:"affiliate_code"`
	AffiliateVisitorKey string                 `json:"affiliate_visitor_key"`
	ManualFormData      map[string]models.JSON `json:"manual_form_data"`
	ChannelID           uint                   `json:"channel_id"`
	UseBalance          bool                   `json:"use_balance"`
}

// CreateOrderAndPay 创建订单并发起支付（合并接口）
func (h *Handler) CreateOrderAndPay(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	var req CreateOrderAndPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	var items []service.CreateOrderItem
	for _, item := range req.Items {
		items = append(items, service.CreateOrderItem{
			ProductID:       item.ProductID,
			SKUID:           item.SKUID,
			Quantity:        item.Quantity,
			FulfillmentType: item.FulfillmentType,
		})
	}

	order, err := h.OrderService.CreateOrder(service.CreateOrderInput{
		UserID:              uid,
		Items:               items,
		CouponCode:          req.CouponCode,
		AffiliateCode:       req.AffiliateCode,
		AffiliateVisitorKey: req.AffiliateVisitorKey,
		ClientIP:            c.ClientIP(),
		ManualFormData:      req.ManualFormData,
	})
	if err != nil {
		respondUserOrderCreateError(c, err)
		return
	}

	orderResp := dto.NewOrderDetail(order)

	// 如果未指定支付渠道且未使用余额，仅返回订单
	if req.ChannelID == 0 && !req.UseBalance {
		response.Success(c, gin.H{
			"order":    orderResp,
			"order_no": order.OrderNo,
		})
		return
	}

	result, err := h.PaymentService.CreatePayment(service.CreatePaymentInput{
		OrderID:    order.ID,
		ChannelID:  req.ChannelID,
		UseBalance: req.UseBalance,
		ClientIP:   c.ClientIP(),
		Context:    c.Request.Context(),
	})
	if err != nil {
		resp := gin.H{
			"order":         orderResp,
			"order_no":      order.OrderNo,
			"payment_error": err.Error(),
		}
		response.Success(c, resp)
		return
	}

	resp := gin.H{
		"order":              orderResp,
		"order_no":           order.OrderNo,
		"order_paid":         result.OrderPaid,
		"wallet_paid_amount": result.WalletPaidAmount,
		"online_pay_amount":  result.OnlinePayAmount,
	}
	if result.Payment != nil {
		resp["payment_id"] = result.Payment.ID
		resp["provider_type"] = result.Payment.ProviderType
		resp["channel_type"] = result.Payment.ChannelType
		resp["interaction_mode"] = result.Payment.InteractionMode
		resp["pay_url"] = result.Payment.PayURL
		resp["qr_code"] = result.Payment.QRCode
		resp["expires_at"] = result.Payment.ExpiredAt
	}
	response.Success(c, resp)
}

// ListOrders 获取订单列表
func (h *Handler) ListOrders(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = shared.NormalizePagination(page, pageSize)

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
		shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, dto.NewOrderSummaryList(orders), pagination)
}

// GetOrder 获取订单详情
func (h *Handler) GetOrder(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.GetOrderByUser(orderID, uid)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	response.Success(c, dto.NewOrderDetailTruncated(order))
}

// GetOrderByOrderNo 按订单号获取订单详情
func (h *Handler) GetOrderByOrderNo(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.GetOrderByUserOrderNo(orderNo, uid)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	response.Success(c, dto.NewOrderDetailTruncated(order))
}

// CancelOrder 用户取消订单
func (h *Handler) CancelOrder(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.CancelOrder(orderID, uid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
		case errors.Is(err, service.ErrOrderCancelNotAllowed):
			shared.RespondError(c, response.CodeBadRequest, "error.order_cancel_not_allowed", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}

	response.Success(c, dto.NewOrderDetail(order))
}

// DownloadFulfillment 下载订单交付内容（登录用户）
// 支持传入父订单 ID 或子订单 ID
func (h *Handler) DownloadFulfillment(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}
	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}
	// 先按父订单查找
	order, err := h.OrderService.GetOrderByUser(orderID, uid)
	if err != nil || order == nil {
		// 可能是子订单 ID，通过 repo 直接查找并验证 user_id
		raw, rawErr := h.OrderRepo.GetByID(orderID)
		if rawErr != nil || raw == nil || raw.UserID != uid {
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		order = raw
	}
	respondFulfillmentDownload(c, order)
}

func respondFulfillmentDownload(c *gin.Context, order *models.Order) {
	payload := collectFulfillmentPayload(order)
	if payload == "" {
		shared.RespondError(c, response.CodeNotFound, "error.fulfillment_not_found", nil)
		return
	}
	filename := "fulfillment-" + order.OrderNo + ".txt"
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(200, "text/plain; charset=utf-8", []byte(payload))
}

func collectFulfillmentPayload(order *models.Order) string {
	if order.Fulfillment != nil && order.Fulfillment.Payload != "" {
		return order.Fulfillment.Payload
	}
	var parts []string
	for _, child := range order.Children {
		if child.Fulfillment != nil && child.Fulfillment.Payload != "" {
			parts = append(parts, child.Fulfillment.Payload)
		}
	}
	return strings.Join(parts, "\n")
}
