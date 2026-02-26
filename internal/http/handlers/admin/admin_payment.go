package admin

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminPaymentItem 支付记录返回
type AdminPaymentItem struct {
	models.Payment
	ChannelName    string `json:"channel_name"`
	RechargeNo     string `json:"recharge_no,omitempty"`
	RechargeStatus string `json:"recharge_status,omitempty"`
	RechargeUserID uint   `json:"recharge_user_id,omitempty"`
}

// GetAdminPayments 获取支付记录列表
func (h *Handler) GetAdminPayments(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	var orderID uint
	if raw := c.Query("order_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		orderID = uint(parsed)
	}
	var userID uint
	if raw := c.Query("user_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		userID = uint(parsed)
	}

	var channelID uint
	if raw := c.Query("channel_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		channelID = uint(parsed)
	}

	providerType := c.Query("provider_type")
	channelType := c.Query("channel_type")
	status := c.Query("status")
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

	payments, total, err := h.PaymentService.ListPayments(repository.PaymentListFilter{
		Page:         page,
		PageSize:     pageSize,
		UserID:       userID,
		OrderID:      orderID,
		ChannelID:    channelID,
		ProviderType: providerType,
		ChannelType:  channelType,
		Status:       status,
		CreatedFrom:  createdFrom,
		CreatedTo:    createdTo,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	channelNameMap, err := h.resolvePaymentChannelNames(payments)
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}
	rechargeMetaMap, err := h.resolvePaymentRechargeMeta(payments)
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}

	items := make([]AdminPaymentItem, 0, len(payments))
	for _, payment := range payments {
		rechargeMeta := rechargeMetaMap[payment.ID]
		items = append(items, AdminPaymentItem{
			Payment:        payment,
			ChannelName:    channelNameMap[payment.ChannelID],
			RechargeNo:     rechargeMeta.RechargeNo,
			RechargeStatus: rechargeMeta.Status,
			RechargeUserID: rechargeMeta.UserID,
		})
	}

	response.SuccessWithPage(c, items, pagination)
}

// ExportAdminPayments 导出支付记录 CSV
func (h *Handler) ExportAdminPayments(c *gin.Context) {
	var orderID uint
	if raw := c.Query("order_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		orderID = uint(parsed)
	}
	var userID uint
	if raw := c.Query("user_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		userID = uint(parsed)
	}

	var channelID uint
	if raw := c.Query("channel_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		channelID = uint(parsed)
	}

	providerType := c.Query("provider_type")
	channelType := c.Query("channel_type")
	status := c.Query("status")
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

	payments, _, err := h.PaymentService.ListPayments(repository.PaymentListFilter{
		Page:         1,
		PageSize:     0,
		UserID:       userID,
		OrderID:      orderID,
		ChannelID:    channelID,
		ProviderType: providerType,
		ChannelType:  channelType,
		Status:       status,
		CreatedFrom:  createdFrom,
		CreatedTo:    createdTo,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}

	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	rechargeMetaMap, err := h.resolvePaymentRechargeMeta(payments)
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_export_failed", err)
		return
	}
	if err := writer.Write([]string{
		"id",
		"order_id",
		"recharge_no",
		"recharge_status",
		"recharge_user_id",
		"channel_id",
		"provider_type",
		"channel_type",
		"status",
		"amount",
		"currency",
		"created_at",
		"paid_at",
		"expired_at",
		"provider_ref",
	}); err != nil {
		respondError(c, response.CodeInternal, "error.payment_export_failed", err)
		return
	}

	for _, payment := range payments {
		rechargeMeta := rechargeMetaMap[payment.ID]
		record := []string{
			strconv.FormatUint(uint64(payment.ID), 10),
			strconv.FormatUint(uint64(payment.OrderID), 10),
			rechargeMeta.RechargeNo,
			rechargeMeta.Status,
			strconv.FormatUint(uint64(rechargeMeta.UserID), 10),
			strconv.FormatUint(uint64(payment.ChannelID), 10),
			payment.ProviderType,
			payment.ChannelType,
			payment.Status,
			payment.Amount.String(),
			payment.Currency,
			payment.CreatedAt.Format(time.RFC3339),
			formatTimeNullable(payment.PaidAt),
			formatTimeNullable(payment.ExpiredAt),
			payment.ProviderRef,
		}
		if err := writer.Write(record); err != nil {
			respondError(c, response.CodeInternal, "error.payment_export_failed", err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		respondError(c, response.CodeInternal, "error.payment_export_failed", err)
		return
	}

	filename := fmt.Sprintf("payments_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buffer.Bytes())
}

// GetAdminPayment 获取支付记录详情
func (h *Handler) GetAdminPayment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		return
	}

	payment, err := h.PaymentService.GetPayment(uint(id))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentNotFound):
			respondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		}
		return
	}

	channelNameMap, err := h.resolvePaymentChannelNames([]models.Payment{*payment})
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}
	rechargeMetaMap, err := h.resolvePaymentRechargeMeta([]models.Payment{*payment})
	if err != nil {
		respondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}
	rechargeMeta := rechargeMetaMap[payment.ID]
	response.Success(c, AdminPaymentItem{
		Payment:        *payment,
		ChannelName:    channelNameMap[payment.ChannelID],
		RechargeNo:     rechargeMeta.RechargeNo,
		RechargeStatus: rechargeMeta.Status,
		RechargeUserID: rechargeMeta.UserID,
	})
}

func formatTimeNullable(raw *time.Time) string {
	if raw == nil {
		return ""
	}
	return raw.Format(time.RFC3339)
}

func (h *Handler) resolvePaymentChannelNames(payments []models.Payment) (map[uint]string, error) {
	channelIDs := make([]uint, 0, len(payments))
	seen := make(map[uint]struct{})
	for _, payment := range payments {
		if payment.ChannelID == 0 {
			continue
		}
		if _, ok := seen[payment.ChannelID]; ok {
			continue
		}
		seen[payment.ChannelID] = struct{}{}
		channelIDs = append(channelIDs, payment.ChannelID)
	}
	result := make(map[uint]string)
	if len(channelIDs) == 0 {
		return result, nil
	}
	channels, err := h.PaymentChannelRepo.ListByIDs(channelIDs)
	if err != nil {
		return nil, err
	}
	for _, channel := range channels {
		result[channel.ID] = channel.Name
	}
	return result, nil
}

type paymentRechargeMeta struct {
	RechargeNo string
	Status     string
	UserID     uint
}

func (h *Handler) resolvePaymentRechargeMeta(payments []models.Payment) (map[uint]paymentRechargeMeta, error) {
	paymentIDs := make([]uint, 0, len(payments))
	seen := make(map[uint]struct{})
	for _, payment := range payments {
		if payment.ID == 0 {
			continue
		}
		if _, ok := seen[payment.ID]; ok {
			continue
		}
		seen[payment.ID] = struct{}{}
		paymentIDs = append(paymentIDs, payment.ID)
	}
	result := make(map[uint]paymentRechargeMeta)
	if len(paymentIDs) == 0 || h.WalletRepo == nil {
		return result, nil
	}
	orders, err := h.WalletRepo.GetRechargeOrdersByPaymentIDs(paymentIDs)
	if err != nil {
		return nil, err
	}
	for _, order := range orders {
		result[order.PaymentID] = paymentRechargeMeta{
			RechargeNo: strings.TrimSpace(order.RechargeNo),
			Status:     strings.TrimSpace(order.Status),
			UserID:     order.UserID,
		}
	}
	return result, nil
}
