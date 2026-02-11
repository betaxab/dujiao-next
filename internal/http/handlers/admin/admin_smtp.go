package admin

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// GetSMTPSettings 获取 SMTP 配置（脱敏）
func (h *Handler) GetSMTPSettings(c *gin.Context) {
	setting, err := h.SettingService.GetSMTPSetting(h.Config.Email)
	if err != nil {
		respondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	response.Success(c, service.MaskSMTPSettingForAdmin(setting))
}

// UpdateSMTPSettings 更新 SMTP 配置
func (h *Handler) UpdateSMTPSettings(c *gin.Context) {
	var req service.SMTPSettingPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	setting, err := h.SettingService.PatchSMTPSetting(h.Config.Email, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSMTPConfigInvalid):
			respondErrorWithMsg(c, response.CodeBadRequest, err.Error(), nil)
		default:
			respondError(c, response.CodeInternal, "error.settings_save_failed", err)
		}
		return
	}

	h.Config.Email = service.SMTPSettingToConfig(setting)
	if h.EmailService != nil {
		h.EmailService.SetConfig(&h.Config.Email)
	}

	response.Success(c, service.MaskSMTPSettingForAdmin(setting))
}

// SMTPTestSendRequest SMTP 测试发送请求
type SMTPTestSendRequest struct {
	ToEmail string `json:"to_email" binding:"required"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// TestSMTPSettings 测试 SMTP 配置发送
func (h *Handler) TestSMTPSettings(c *gin.Context) {
	var req SMTPTestSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	toEmail := strings.TrimSpace(req.ToEmail)
	if toEmail == "" {
		respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		return
	}

	setting, err := h.SettingService.GetSMTPSetting(h.Config.Email)
	if err != nil {
		respondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}

	configForSend := service.SMTPSettingToConfig(setting)
	configForSend.Enabled = true
	tempEmailService := service.NewEmailService(&configForSend)

	if err := tempEmailService.SendCustomEmail(toEmail, req.Subject, req.Body); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrEmailRecipientRejected):
			respondError(c, response.CodeBadRequest, "error.email_recipient_not_found", nil)
		case errors.Is(err, service.ErrEmailServiceDisabled),
			errors.Is(err, service.ErrEmailServiceNotConfigured):
			respondError(c, response.CodeBadRequest, "error.email_service_not_configured", err)
		default:
			respondError(c, response.CodeInternal, "error.send_verify_code_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"sent": true})
}
