package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/provider"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/service"

	"github.com/hibiken/asynq"
)

// Consumer 异步任务消费者
type Consumer struct {
	*provider.Container
}

// NewConsumer 创建消费者
func NewConsumer(c *provider.Container) *Consumer {
	return &Consumer{
		Container: c,
	}
}

// Register 注册消费者
func (c *Consumer) Register(mux *asynq.ServeMux) {
	if c == nil || mux == nil {
		logger.Debugw("worker_register_skip_nil", "consumer_nil", c == nil, "mux_nil", mux == nil)
		return
	}
	mux.HandleFunc(queue.TaskOrderStatusEmail, c.handleOrderStatusEmail)
	mux.HandleFunc(queue.TaskOrderAutoFulfill, c.handleOrderAutoFulfill)
	mux.HandleFunc(queue.TaskOrderTimeoutCancel, c.handleOrderTimeoutCancel)
}

func (c *Consumer) handleOrderStatusEmail(_ context.Context, task *asynq.Task) error {
	if c == nil || task == nil {
		logger.Debugw("worker_order_status_email_skip_nil", "consumer_nil", c == nil, "task_nil", task == nil)
		return nil
	}
	var payload queue.OrderStatusEmailPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.Warnw("worker_order_status_email_unmarshal_failed", "error", err)
		return err
	}
	if payload.OrderID == 0 {
		logger.Debugw("worker_order_status_email_skip_invalid_payload", "order_id", payload.OrderID)
		return nil
	}
	order, err := c.OrderRepo.GetByID(payload.OrderID)
	if err != nil {
		logger.Warnw("worker_order_status_email_fetch_order_failed", "order_id", payload.OrderID, "error", err)
		return err
	}
	if order == nil {
		logger.Debugw("worker_order_status_email_skip_order_not_found", "order_id", payload.OrderID)
		return nil
	}
	var receiverEmail string
	var locale string
	if order.UserID != 0 {
		user, err := c.UserRepo.GetByID(order.UserID)
		if err != nil {
			logger.Warnw("worker_order_status_email_fetch_user_failed", "order_id", order.ID, "user_id", order.UserID, "error", err)
			return err
		}
		if user != nil {
			receiverEmail = strings.TrimSpace(user.Email)
			locale = strings.TrimSpace(user.Locale)
		}
	} else {
		receiverEmail = strings.TrimSpace(order.GuestEmail)
		locale = strings.TrimSpace(order.GuestLocale)
	}
	if receiverEmail == "" {
		logger.Debugw("worker_order_status_email_skip_empty_receiver", "order_id", order.ID, "order_no", order.OrderNo)
		return nil
	}
	if isTelegramPlaceholderReceiver(receiverEmail) {
		logger.Debugw("worker_order_status_email_skip_placeholder_receiver", "order_id", order.ID, "order_no", order.OrderNo)
		return nil
	}
	if c.EmailService == nil {
		logger.Warnw("worker_order_status_email_skip_email_service_nil", "order_id", order.ID, "order_no", order.OrderNo)
		return nil
	}
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = order.Status
	}
	var payloadText string
	if order.Fulfillment != nil {
		payloadText = strings.TrimSpace(order.Fulfillment.Payload)
	}
	if payloadText == "" && len(order.Children) > 0 {
		parts := make([]string, 0)
		for _, child := range order.Children {
			if child.Fulfillment == nil {
				continue
			}
			content := strings.TrimSpace(child.Fulfillment.Payload)
			if content == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("[%s]\\n%s", child.OrderNo, content))
		}
		if len(parts) > 0 {
			payloadText = strings.Join(parts, "\n\n")
		}
	}
	input := service.OrderStatusEmailInput{
		OrderNo:         order.OrderNo,
		Status:          status,
		Amount:          order.TotalAmount,
		Currency:        order.Currency,
		FulfillmentInfo: payloadText,
		IsGuest:         order.UserID == 0,
	}
	if err := c.EmailService.SendOrderStatusEmail(receiverEmail, input, locale); err != nil {
		logger.Warnw("worker_order_status_email_send_failed",
			"order_id", order.ID,
			"order_no", order.OrderNo,
			"receiver_email", receiverEmail,
			"status", status,
			"error", err,
		)
		return err
	}
	return nil
}

func (c *Consumer) handleOrderAutoFulfill(_ context.Context, task *asynq.Task) error {
	if c == nil || task == nil {
		logger.Debugw("worker_order_auto_fulfill_skip_nil", "consumer_nil", c == nil, "task_nil", task == nil)
		return nil
	}
	var payload queue.OrderAutoFulfillPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.Warnw("worker_order_auto_fulfill_unmarshal_failed", "error", err)
		return err
	}
	if payload.OrderID == 0 {
		logger.Debugw("worker_order_auto_fulfill_skip_invalid_payload", "order_id", payload.OrderID)
		return nil
	}
	_, err := c.FulfillmentService.CreateAuto(payload.OrderID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFulfillmentExists):
			logger.Debugw("worker_order_auto_fulfill_skip_exists", "order_id", payload.OrderID)
			return nil
		case errors.Is(err, service.ErrFulfillmentNotAuto):
			logger.Debugw("worker_order_auto_fulfill_skip_not_auto", "order_id", payload.OrderID)
			return nil
		case errors.Is(err, service.ErrOrderStatusInvalid):
			logger.Debugw("worker_order_auto_fulfill_skip_invalid_status", "order_id", payload.OrderID)
			return nil
		case errors.Is(err, service.ErrOrderNotFound):
			logger.Debugw("worker_order_auto_fulfill_skip_order_not_found", "order_id", payload.OrderID)
			return nil
		default:
			logger.Warnw("worker_order_auto_fulfill_failed", "order_id", payload.OrderID, "error", err)
			return err
		}
	}
	return nil
}

func (c *Consumer) handleOrderTimeoutCancel(_ context.Context, task *asynq.Task) error {
	if c == nil || task == nil {
		logger.Debugw("worker_order_timeout_cancel_skip_nil", "consumer_nil", c == nil, "task_nil", task == nil)
		return nil
	}
	var payload queue.OrderTimeoutCancelPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.Warnw("worker_order_timeout_cancel_unmarshal_failed", "error", err)
		return err
	}
	if payload.OrderID == 0 {
		logger.Debugw("worker_order_timeout_cancel_skip_invalid_payload", "order_id", payload.OrderID)
		return nil
	}
	if c.OrderService == nil {
		logger.Warnw("worker_order_timeout_cancel_skip_order_service_nil", "order_id", payload.OrderID)
		return nil
	}
	_, err := c.OrderService.CancelExpiredOrder(payload.OrderID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			logger.Debugw("worker_order_timeout_cancel_skip_order_not_found", "order_id", payload.OrderID)
			return nil
		case errors.Is(err, service.ErrOrderFetchFailed):
			logger.Warnw("worker_order_timeout_cancel_fetch_failed", "order_id", payload.OrderID, "error", err)
			return nil
		case errors.Is(err, service.ErrOrderUpdateFailed):
			logger.Warnw("worker_order_timeout_cancel_update_failed", "order_id", payload.OrderID, "error", err)
			return err
		default:
			logger.Warnw("worker_order_timeout_cancel_failed", "order_id", payload.OrderID, "error", err)
			return err
		}
	}
	return nil
}

func isTelegramPlaceholderReceiver(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "telegram_") && strings.HasSuffix(normalized, "@login.local")
}
