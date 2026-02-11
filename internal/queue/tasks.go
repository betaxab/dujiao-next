package queue

import (
	"encoding/json"

	"github.com/dujiao-next/internal/constants"

	"github.com/hibiken/asynq"
)

const (
	// TaskOrderStatusEmail 订单状态邮件通知任务
	TaskOrderStatusEmail = constants.TaskOrderStatusEmail
	// TaskOrderAutoFulfill 自动交付任务
	TaskOrderAutoFulfill = constants.TaskOrderAutoFulfill
	// TaskOrderTimeoutCancel 超时取消任务
	TaskOrderTimeoutCancel = constants.TaskOrderTimeoutCancel
)

// OrderStatusEmailPayload 订单状态邮件任务载荷
type OrderStatusEmailPayload struct {
	OrderID uint   `json:"order_id"`
	Status  string `json:"status"`
}

// OrderAutoFulfillPayload 自动交付任务载荷
type OrderAutoFulfillPayload struct {
	OrderID uint `json:"order_id"`
}

// OrderTimeoutCancelPayload 超时取消任务载荷
type OrderTimeoutCancelPayload struct {
	OrderID uint `json:"order_id"`
}

// NewOrderStatusEmailTask 创建订单状态邮件任务
func NewOrderStatusEmailTask(payload OrderStatusEmailPayload) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskOrderStatusEmail, body), nil
}

// NewOrderAutoFulfillTask 创建自动交付任务
func NewOrderAutoFulfillTask(payload OrderAutoFulfillPayload) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskOrderAutoFulfill, body), nil
}

// NewOrderTimeoutCancelTask 创建超时取消任务
func NewOrderTimeoutCancelTask(payload OrderTimeoutCancelPayload) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskOrderTimeoutCancel, body), nil
}
