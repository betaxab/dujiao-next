package dto

import (
	"time"

	"github.com/dujiao-next/internal/models"
)

// AffiliateProfileResp 推广用户资料响应
type AffiliateProfileResp struct {
	ID            uint      `json:"id"`
	AffiliateCode string    `json:"code"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// NewAffiliateProfileResp 从 models.AffiliateProfile 构造响应
func NewAffiliateProfileResp(p *models.AffiliateProfile) AffiliateProfileResp {
	return AffiliateProfileResp{
		ID:            p.ID,
		AffiliateCode: p.AffiliateCode,
		Status:        p.Status,
		CreatedAt:     p.CreatedAt,
	}
	// 排除：UserID、User、UpdatedAt
}

// AffiliateCommissionResp 佣金记录响应
type AffiliateCommissionResp struct {
	ID               uint         `json:"id"`
	OrderID          uint         `json:"order_id"`
	CommissionType   string       `json:"commission_type"`
	CommissionAmount models.Money `json:"commission_amount"`
	Status           string       `json:"status"`
	ConfirmAt        *time.Time   `json:"confirm_at,omitempty"`
	AvailableAt      *time.Time   `json:"available_at,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
}

// NewAffiliateCommissionResp 从 models.AffiliateCommission 构造响应
func NewAffiliateCommissionResp(c *models.AffiliateCommission) AffiliateCommissionResp {
	return AffiliateCommissionResp{
		ID:               c.ID,
		OrderID:          c.OrderID,
		CommissionType:   c.CommissionType,
		CommissionAmount: c.CommissionAmount,
		Status:           c.Status,
		ConfirmAt:        c.ConfirmAt,
		AvailableAt:      c.AvailableAt,
		CreatedAt:        c.CreatedAt,
	}
	// 排除：AffiliateProfileID、OrderItemID、BaseAmount、RatePercent、
	// WithdrawRequestID、InvalidReason、UpdatedAt、关联 Order/AffiliateProfile/WithdrawRequest
}

// NewAffiliateCommissionRespList 批量转换佣金列表
func NewAffiliateCommissionRespList(commissions []models.AffiliateCommission) []AffiliateCommissionResp {
	result := make([]AffiliateCommissionResp, 0, len(commissions))
	for i := range commissions {
		result = append(result, NewAffiliateCommissionResp(&commissions[i]))
	}
	return result
}

// AffiliateWithdrawResp 提现记录响应
type AffiliateWithdrawResp struct {
	ID           uint         `json:"id"`
	Amount       models.Money `json:"amount"`
	Channel      string       `json:"channel"`
	Account      string       `json:"account"`
	Status       string       `json:"status"`
	RejectReason string       `json:"reject_reason,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

// NewAffiliateWithdrawResp 从 models.AffiliateWithdrawRequest 构造响应
func NewAffiliateWithdrawResp(w *models.AffiliateWithdrawRequest) AffiliateWithdrawResp {
	return AffiliateWithdrawResp{
		ID:           w.ID,
		Amount:       w.Amount,
		Channel:      w.Channel,
		Account:      w.Account,
		Status:       w.Status,
		RejectReason: w.RejectReason,
		CreatedAt:    w.CreatedAt,
	}
	// 排除：AffiliateProfileID、ProcessedBy、ProcessedAt、UpdatedAt、关联
}

// NewAffiliateWithdrawRespList 批量转换提现列表
func NewAffiliateWithdrawRespList(withdraws []models.AffiliateWithdrawRequest) []AffiliateWithdrawResp {
	result := make([]AffiliateWithdrawResp, 0, len(withdraws))
	for i := range withdraws {
		result = append(result, NewAffiliateWithdrawResp(&withdraws[i]))
	}
	return result
}
