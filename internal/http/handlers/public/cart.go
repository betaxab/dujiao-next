package public

import (
	"errors"
	"strconv"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// CartItemRequest 购物车项请求
type CartItemRequest struct {
	ProductID       uint   `json:"product_id" binding:"required"`
	Quantity        int    `json:"quantity" binding:"required"`
	FulfillmentType string `json:"fulfillment_type"`
}

// CartProduct 购物车商品摘要
type CartProduct struct {
	ID              uint               `json:"id"`
	Slug            string             `json:"slug"`
	Title           models.JSON        `json:"title"`
	PriceAmount     models.Money       `json:"price_amount"`
	PriceCurrency   string             `json:"price_currency"`
	Images          models.StringArray `json:"images"`
	Tags            models.StringArray `json:"tags"`
	PurchaseType    string             `json:"purchase_type"`
	FulfillmentType string             `json:"fulfillment_type"`
	IsActive        bool               `json:"is_active"`
}

// CartItemResponse 购物车项响应
type CartItemResponse struct {
	ProductID       uint         `json:"product_id"`
	Quantity        int          `json:"quantity"`
	FulfillmentType string       `json:"fulfillment_type"`
	UnitPrice       models.Money `json:"unit_price"`
	OriginalPrice   models.Money `json:"original_price"`
	Currency        string       `json:"currency"`
	Product         CartProduct  `json:"product"`
}

// GetCart 获取购物车
func (h *Handler) GetCart(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	items, err := h.CartService.ListByUser(uid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderItem):
			respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		case errors.Is(err, service.ErrProductNotAvailable):
			respondError(c, response.CodeBadRequest, "error.product_not_available", nil)
		case errors.Is(err, service.ErrFulfillmentInvalid):
			respondError(c, response.CodeBadRequest, "error.fulfillment_invalid", nil)
		case errors.Is(err, service.ErrPromotionInvalid):
			respondError(c, response.CodeBadRequest, "error.promotion_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		}
		return
	}

	respItems := make([]CartItemResponse, 0, len(items))
	for _, item := range items {
		if item.Product == nil {
			continue
		}
		product := CartProduct{
			ID:              item.Product.ID,
			Slug:            item.Product.Slug,
			Title:           item.Product.TitleJSON,
			PriceAmount:     item.Product.PriceAmount,
			PriceCurrency:   item.Product.PriceCurrency,
			Images:          item.Product.Images,
			Tags:            item.Product.Tags,
			PurchaseType:    item.Product.PurchaseType,
			FulfillmentType: item.Product.FulfillmentType,
			IsActive:        item.Product.IsActive,
		}
		respItems = append(respItems, CartItemResponse{
			ProductID:       item.ProductID,
			Quantity:        item.Quantity,
			FulfillmentType: item.FulfillmentType,
			UnitPrice:       item.UnitPrice,
			OriginalPrice:   item.OriginalPrice,
			Currency:        item.Currency,
			Product:         product,
		})
	}

	response.Success(c, gin.H{"items": respItems})
}

// UpsertCartItem 添加/更新购物车项
func (h *Handler) UpsertCartItem(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	var req CartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if req.Quantity <= 0 {
		if err := h.CartService.RemoveItem(uid, req.ProductID); err != nil {
			respondError(c, response.CodeInternal, "error.order_update_failed", err)
			return
		}
		response.Success(c, gin.H{"updated": true})
		return
	}
	if err := h.CartService.UpsertItem(service.UpsertCartItemInput{
		UserID:          uid,
		ProductID:       req.ProductID,
		Quantity:        req.Quantity,
		FulfillmentType: req.FulfillmentType,
	}); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderItem):
			respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		case errors.Is(err, service.ErrProductNotAvailable):
			respondError(c, response.CodeBadRequest, "error.product_not_available", nil)
		case errors.Is(err, service.ErrFulfillmentInvalid):
			respondError(c, response.CodeBadRequest, "error.fulfillment_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// DeleteCartItem 删除购物车项
func (h *Handler) DeleteCartItem(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	rawID := c.Param("product_id")
	productID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || productID == 0 {
		respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}
	if err := h.CartService.RemoveItem(uid, uint(productID)); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderItem):
			respondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		default:
			respondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
