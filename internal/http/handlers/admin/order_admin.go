package admin

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

// AdminOrderListItem 管理端订单列表返回
type AdminOrderListItem struct {
	models.Order
	UserEmail       string `json:"user_email,omitempty"`
	UserDisplayName string `json:"user_display_name,omitempty"`
}

// AdminOrderDetail 管理端订单详情返回
type AdminOrderDetail struct {
	models.Order
	UserEmail       string             `json:"user_email,omitempty"`
	UserDisplayName string             `json:"user_display_name,omitempty"`
	CouponCode      string             `json:"coupon_code,omitempty"`
	PromotionName   string             `json:"promotion_name,omitempty"`
	Payments        []AdminPaymentItem `json:"payments,omitempty"`
}

// AdminListOrders 管理端订单列表
func (h *Handler) AdminListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	status := strings.TrimSpace(c.Query("status"))
	userIDStr := strings.TrimSpace(c.Query("user_id"))
	orderNo := strings.TrimSpace(c.Query("order_no"))
	guestEmail := strings.TrimSpace(c.Query("guest_email"))
	createdFromRaw := strings.TrimSpace(c.Query("created_from"))
	createdToRaw := strings.TrimSpace(c.Query("created_to"))

	createdFrom, err := parseTimeNullable(createdFromRaw)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	createdTo, err := parseTimeNullable(createdToRaw)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	var userID uint
	if userIDStr != "" {
		if parsed, err := strconv.ParseUint(userIDStr, 10, 64); err == nil {
			userID = uint(parsed)
		}
	}

	orders, total, err := h.OrderService.ListOrdersForAdmin(repository.OrderListFilter{
		Page:        page,
		PageSize:    pageSize,
		UserID:      userID,
		Status:      status,
		OrderNo:     orderNo,
		GuestEmail:  guestEmail,
		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,
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
	userMap := map[uint]models.User{}
	userIDs := make([]uint, 0, len(orders))
	seen := map[uint]struct{}{}
	for _, order := range orders {
		if order.UserID == 0 {
			continue
		}
		if _, ok := seen[order.UserID]; ok {
			continue
		}
		seen[order.UserID] = struct{}{}
		userIDs = append(userIDs, order.UserID)
	}
	if len(userIDs) > 0 {
		users, err := h.UserRepo.ListByIDs(userIDs)
		if err != nil {
			respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		for _, user := range users {
			userMap[user.ID] = user
		}
	}

	items := make([]AdminOrderListItem, 0, len(orders))
	for _, order := range orders {
		var email, displayName string
		if user, ok := userMap[order.UserID]; ok {
			email = user.Email
			displayName = user.DisplayName
		}
		items = append(items, AdminOrderListItem{
			Order:           order,
			UserEmail:       email,
			UserDisplayName: displayName,
		})
	}

	response.SuccessWithPage(c, items, pagination)
}

// AdminGetOrder 管理端订单详情
func (h *Handler) AdminGetOrder(c *gin.Context) {
	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || orderID == 0 {
		respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.GetOrderForAdmin(uint(orderID))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		}
		return
	}
	var email, displayName string
	if order.UserID != 0 {
		user, err := h.UserRepo.GetByID(order.UserID)
		if err != nil {
			respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if user != nil {
			email = user.Email
			displayName = user.DisplayName
		}
	}

	var couponCode string
	if order.CouponID != nil && *order.CouponID > 0 {
		coupon, err := h.CouponRepo.GetByID(*order.CouponID)
		if err != nil {
			respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if coupon != nil {
			couponCode = coupon.Code
		}
	}

	var promotionName string
	if order.PromotionID != nil && *order.PromotionID > 0 {
		promotion, err := h.PromotionRepo.GetByID(*order.PromotionID)
		if err != nil {
			respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if promotion != nil {
			promotionName = promotion.Name
		}
	}

	promotionNameMap := make(map[uint]string)
	for i := range order.Items {
		item := order.Items[i]
		if item.PromotionID == nil || *item.PromotionID == 0 {
			continue
		}
		promotionID := *item.PromotionID
		if _, ok := promotionNameMap[promotionID]; ok {
			continue
		}
		promotion, err := h.PromotionRepo.GetByID(promotionID)
		if err != nil {
			respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if promotion != nil {
			promotionNameMap[promotionID] = promotion.Name
		} else {
			promotionNameMap[promotionID] = ""
		}
	}
	for i := range order.Children {
		for _, item := range order.Children[i].Items {
			if item.PromotionID == nil || *item.PromotionID == 0 {
				continue
			}
			promotionID := *item.PromotionID
			if _, ok := promotionNameMap[promotionID]; ok {
				continue
			}
			promotion, err := h.PromotionRepo.GetByID(promotionID)
			if err != nil {
				respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
				return
			}
			if promotion != nil {
				promotionNameMap[promotionID] = promotion.Name
			} else {
				promotionNameMap[promotionID] = ""
			}
		}
	}
	for i := range order.Items {
		item := &order.Items[i]
		if item.PromotionID == nil || *item.PromotionID == 0 {
			continue
		}
		item.PromotionName = promotionNameMap[*item.PromotionID]
	}
	for i := range order.Children {
		for j := range order.Children[i].Items {
			item := &order.Children[i].Items[j]
			if item.PromotionID == nil || *item.PromotionID == 0 {
				continue
			}
			item.PromotionName = promotionNameMap[*item.PromotionID]
		}
	}

	payments, err := h.PaymentRepo.ListByOrderID(order.ID)
	if err != nil {
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}
	channelNameMap, err := h.resolvePaymentChannelNames(payments)
	if err != nil {
		respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}
	paymentItems := make([]AdminPaymentItem, 0, len(payments))
	for _, payment := range payments {
		paymentItems = append(paymentItems, AdminPaymentItem{
			Payment:     payment,
			ChannelName: channelNameMap[payment.ChannelID],
		})
	}

	response.Success(c, AdminOrderDetail{
		Order:           *order,
		UserEmail:       email,
		UserDisplayName: displayName,
		CouponCode:      couponCode,
		PromotionName:   promotionName,
		Payments:        paymentItems,
	})
}

// AdminUpdateOrderStatusRequest 管理端更新订单状态请求
type AdminUpdateOrderStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// AdminUpdateOrderStatus 管理端更新订单状态
func (h *Handler) AdminUpdateOrderStatus(c *gin.Context) {
	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || orderID == 0 {
		respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	var req AdminUpdateOrderStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	order, err := h.OrderService.UpdateOrderStatus(uint(orderID), req.Status)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
		case errors.Is(err, service.ErrOrderStatusInvalid):
			respondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}

	response.Success(c, order)
}
