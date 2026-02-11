package admin

import (
	"errors"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminCreateFulfillmentRequest 管理端录入交付请求
type AdminCreateFulfillmentRequest struct {
	OrderID      uint        `json:"order_id" binding:"required"`
	Payload      string      `json:"payload"`
	DeliveryData models.JSON `json:"delivery_data"`
}

// AdminCreateFulfillment 管理端录入交付内容
func (h *Handler) AdminCreateFulfillment(c *gin.Context) {
	adminID, ok := getAdminID(c)
	if !ok {
		return
	}

	var req AdminCreateFulfillmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	fulfillment, err := h.FulfillmentService.CreateManual(service.CreateManualInput{
		OrderID:      req.OrderID,
		AdminID:      adminID,
		Payload:      req.Payload,
		DeliveryData: req.DeliveryData,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFulfillmentExists):
			respondError(c, response.CodeBadRequest, "error.fulfillment_exists", nil)
		case errors.Is(err, service.ErrFulfillmentInvalid):
			respondError(c, response.CodeBadRequest, "error.fulfillment_invalid", nil)
		case errors.Is(err, service.ErrOrderStatusInvalid):
			respondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		case errors.Is(err, service.ErrOrderNotFound):
			respondError(c, response.CodeNotFound, "error.order_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.fulfillment_create_failed", err)
		}
		return
	}

	response.Success(c, fulfillment)
}
